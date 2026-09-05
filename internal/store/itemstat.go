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

// The three SQL fragments, built once from the table above.
var (
	itemStatSelect = func() string {
		cols := make([]string, 0, len(domain.ItemStatFields)+1)
		cols = append(cols, "item_index")
		for _, f := range domain.ItemStatFields {
			cols = append(cols, f.Col)
		}
		return strings.Join(cols, ", ")
	}()

	// $1 is item_index; the override columns follow, and updated_by takes the
	// last placeholder so updated_at can be now() rather than a client clock.
	itemStatInsert = func() string {
		n := len(domain.ItemStatFields) + 2 // item_index + columns + updated_by
		ph := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			ph = append(ph, "$"+strconv.Itoa(i))
		}
		set := make([]string, 0, len(domain.ItemStatFields)+2)
		for _, f := range domain.ItemStatFields {
			set = append(set, f.Col+" = EXCLUDED."+f.Col)
		}
		set = append(set, "updated_by = EXCLUDED.updated_by", "updated_at = now()")
		return `INSERT INTO item_stat (` + itemStatSelect + `, updated_by, updated_at)
			VALUES (` + strings.Join(ph, ",") + `, now())
			ON CONFLICT (item_index) DO UPDATE SET ` + strings.Join(set, ", ")
	}()
)

// scanTargets returns pointers in domain.ItemStatFields order, for Scan.
func scanTargets(st *domain.ItemStat) []any {
	out := make([]any, 0, len(domain.ItemStatFields)+1)
	out = append(out, &st.ItemIndex)
	for _, f := range domain.ItemStatFields {
		out = append(out, f.Ptr(st))
	}
	return out
}

// insertArgs returns values in the same order, for Exec.
func insertArgs(st domain.ItemStat, moderatorID int64) []any {
	out := make([]any, 0, len(domain.ItemStatFields)+2)
	out = append(out, st.ItemIndex)
	for _, f := range domain.ItemStatFields {
		out = append(out, *f.Ptr(&st))
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
