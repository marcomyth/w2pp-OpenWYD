package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Item base stat persistence (0023_item_stats), the item-side sibling of
// mobtemplate.go. Postgres owns the override for a Release/Common/ItemList.csv
// entry's static numbers; the tmServer only ever reads it (via dbServer), once
// at boot. There is no hot-reload: these numbers feed the equip score model,
// which is recomputed per character, so swapping them under a running server
// would leave two players wearing the same item with different stats.
//
// Mutations reuse npc_audit/npc_config_meta through auditAndBump, exactly as the
// mob template stats do. Bumping the NPC config version for an item stat write
// is a harmless no-op for the unrelated npc_definition poll, and keeping one
// moderation trail beats a parallel one nobody would think to read.

// itemStatField binds one override column to its place in domain.ItemStat.
//
// A table rather than three hand-written 43-column SQL statements. Every column
// here appears in a SELECT list, an INSERT placeholder list and an ON CONFLICT
// SET clause; writing those out separately means three places to forget the same
// column, and a forgotten column is a number that silently stops being saved.
// Building all three from one ordered list makes that impossible, and the
// pointer accessor is compiler-checked, so a renamed field fails the build.
type itemStatField struct {
	col string
	ptr func(*domain.ItemStat) *int16
}

var itemStatFields = []itemStatField{
	{"req_level", func(s *domain.ItemStat) *int16 { return &s.ReqLevel }},
	{"req_str", func(s *domain.ItemStat) *int16 { return &s.ReqStr }},
	{"req_int", func(s *domain.ItemStat) *int16 { return &s.ReqInt }},
	{"req_dex", func(s *domain.ItemStat) *int16 { return &s.ReqDex }},
	{"req_con", func(s *domain.ItemStat) *int16 { return &s.ReqCon }},

	{"damage", func(s *domain.ItemStat) *int16 { return &s.Damage }},
	{"damageadd", func(s *domain.ItemStat) *int16 { return &s.DamageAdd }},
	{"ac", func(s *domain.ItemStat) *int16 { return &s.AC }},
	{"acadd", func(s *domain.ItemStat) *int16 { return &s.ACAdd }},
	{"magic", func(s *domain.ItemStat) *int16 { return &s.Magic }},
	{"magicadd", func(s *domain.ItemStat) *int16 { return &s.MagicAdd }},
	{"critical", func(s *domain.ItemStat) *int16 { return &s.Critical }},
	{"critical2", func(s *domain.ItemStat) *int16 { return &s.Critical2 }},
	{"runspeed", func(s *domain.ItemStat) *int16 { return &s.RunSpeed }},

	{"str", func(s *domain.ItemStat) *int16 { return &s.Str }},
	{"intel", func(s *domain.ItemStat) *int16 { return &s.Int }},
	{"dex", func(s *domain.ItemStat) *int16 { return &s.Dex }},
	{"con", func(s *domain.ItemStat) *int16 { return &s.Con }},

	{"hp", func(s *domain.ItemStat) *int16 { return &s.Hp }},
	{"hpadd", func(s *domain.ItemStat) *int16 { return &s.HpAdd }},
	{"hpadd2", func(s *domain.ItemStat) *int16 { return &s.HpAdd2 }},
	{"mp", func(s *domain.ItemStat) *int16 { return &s.Mp }},
	{"mpadd", func(s *domain.ItemStat) *int16 { return &s.MpAdd }},
	{"mpadd2", func(s *domain.ItemStat) *int16 { return &s.MpAdd2 }},

	{"resist1", func(s *domain.ItemStat) *int16 { return &s.Resist1 }},
	{"resist2", func(s *domain.ItemStat) *int16 { return &s.Resist2 }},
	{"resist3", func(s *domain.ItemStat) *int16 { return &s.Resist3 }},
	{"resist4", func(s *domain.ItemStat) *int16 { return &s.Resist4 }},
	{"resistall", func(s *domain.ItemStat) *int16 { return &s.ResistAll }},

	{"special1", func(s *domain.ItemStat) *int16 { return &s.Special1 }},
	{"special2", func(s *domain.ItemStat) *int16 { return &s.Special2 }},
	{"special3", func(s *domain.ItemStat) *int16 { return &s.Special3 }},
	{"special4", func(s *domain.ItemStat) *int16 { return &s.Special4 }},
	{"specialall", func(s *domain.ItemStat) *int16 { return &s.SpecialAll }},

	{"itemlevel", func(s *domain.ItemStat) *int16 { return &s.ItemLevel }},
	{"itemtype", func(s *domain.ItemStat) *int16 { return &s.ItemType }},
	{"mobtype", func(s *domain.ItemStat) *int16 { return &s.MobType }},
	{"wtype", func(s *domain.ItemStat) *int16 { return &s.WType }},
	{"pos", func(s *domain.ItemStat) *int16 { return &s.Pos }},
	{"sanc", func(s *domain.ItemStat) *int16 { return &s.Sanc }},
	{"nosanc", func(s *domain.ItemStat) *int16 { return &s.NoSanc }},
	{"incubate", func(s *domain.ItemStat) *int16 { return &s.Incubate }},
	{"incudelay", func(s *domain.ItemStat) *int16 { return &s.IncuDelay }},
}

// The three SQL fragments, built once from the table above.
var (
	itemStatSelect = func() string {
		cols := make([]string, 0, len(itemStatFields)+1)
		cols = append(cols, "item_index")
		for _, f := range itemStatFields {
			cols = append(cols, f.col)
		}
		return strings.Join(cols, ", ")
	}()

	// $1 is item_index; the override columns follow, and updated_by takes the
	// last placeholder so updated_at can be now() rather than a client clock.
	itemStatInsert = func() string {
		n := len(itemStatFields) + 2 // item_index + columns + updated_by
		ph := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			ph = append(ph, "$"+strconv.Itoa(i))
		}
		set := make([]string, 0, len(itemStatFields)+2)
		for _, f := range itemStatFields {
			set = append(set, f.col+" = EXCLUDED."+f.col)
		}
		set = append(set, "updated_by = EXCLUDED.updated_by", "updated_at = now()")
		return `INSERT INTO item_stat (` + itemStatSelect + `, updated_by, updated_at)
			VALUES (` + strings.Join(ph, ",") + `, now())
			ON CONFLICT (item_index) DO UPDATE SET ` + strings.Join(set, ", ")
	}()
)

// scanTargets returns pointers in itemStatFields order, for Scan.
func scanTargets(st *domain.ItemStat) []any {
	out := make([]any, 0, len(itemStatFields)+1)
	out = append(out, &st.ItemIndex)
	for _, f := range itemStatFields {
		out = append(out, f.ptr(st))
	}
	return out
}

// insertArgs returns values in the same order, for Exec.
func insertArgs(st domain.ItemStat, moderatorID int64) []any {
	out := make([]any, 0, len(itemStatFields)+2)
	out = append(out, st.ItemIndex)
	for _, f := range itemStatFields {
		out = append(out, *f.ptr(&st))
	}
	return append(out, moderatorID)
}

// ListItemStats returns every item stat override, ordered by item index. This is
// the full snapshot the tmServer applies once at boot.
func (s *Store) ListItemStats(ctx context.Context) ([]domain.ItemStat, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+itemStatSelect+` FROM item_stat ORDER BY item_index`)
	if err != nil {
		return nil, fmt.Errorf("store: list item stats: %w", err)
	}
	defer rows.Close()

	var out []domain.ItemStat
	for rows.Next() {
		var st domain.ItemStat
		if err := rows.Scan(scanTargets(&st)...); err != nil {
			return nil, fmt.Errorf("store: scan item stat: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate item stats: %w", err)
	}
	return out, nil
}

// GetItemStat returns one override, or ErrNotFound when the item has none — in
// which case the caller should fall back to the catalog's own numbers.
func (s *Store) GetItemStat(ctx context.Context, itemIndex int32) (domain.ItemStat, error) {
	var st domain.ItemStat
	err := s.pool.QueryRow(ctx,
		`SELECT `+itemStatSelect+` FROM item_stat WHERE item_index = $1`, itemIndex).
		Scan(scanTargets(&st)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ItemStat{}, ErrNotFound
	}
	if err != nil {
		return domain.ItemStat{}, fmt.Errorf("store: get item stat %d: %w", itemIndex, err)
	}
	return st, nil
}

// UpsertItemStat writes an item's override whole and records who did it.
func (s *Store) UpsertItemStat(ctx context.Context, st domain.ItemStat, moderatorID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		before, err := fetchItemStatJSON(ctx, tx, st.ItemIndex)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, itemStatInsert, insertArgs(st, moderatorID)...); err != nil {
			return fmt.Errorf("store: upsert item stat %d: %w", st.ItemIndex, err)
		}
		after, err := fetchItemStatJSON(ctx, tx, st.ItemIndex)
		if err != nil {
			return err
		}
		return auditAndBump(ctx, tx, nil, moderatorID, "upsert_item_stat", before, after)
	})
}

// DeleteItemStat drops an override so the catalog's own numbers apply again.
func (s *Store) DeleteItemStat(ctx context.Context, itemIndex int32, moderatorID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		before, err := fetchItemStatJSON(ctx, tx, itemIndex)
		if err != nil {
			return err
		}
		if before == nil {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM item_stat WHERE item_index = $1`, itemIndex); err != nil {
			return fmt.Errorf("store: delete item stat %d: %w", itemIndex, err)
		}
		return auditAndBump(ctx, tx, nil, moderatorID, "delete_item_stat", before, nil)
	})
}

func fetchItemStatJSON(ctx context.Context, tx pgx.Tx, itemIndex int32) ([]byte, error) {
	var js []byte
	err := tx.QueryRow(ctx, `SELECT to_jsonb(s) FROM item_stat s WHERE item_index = $1`, itemIndex).Scan(&js)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return js, err
}
