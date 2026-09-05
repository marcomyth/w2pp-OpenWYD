//go:build integration

// Integration tests for the item base stat store (0023_item_stats). They require
// a real database and are excluded from the default build. Run with:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestItemStatCRUD exercises the whole moderator write path. It matters more
// than a usual CRUD test because the three SQL statements are built at init from
// domain.ItemStatFields rather than written out: a column that fell out of one of them
// would compile, and would only show up as a number that silently stops being
// saved.
func TestItemStatCRUD(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM item_stat; UPDATE npc_config_meta SET version = 0 WHERE id = TRUE`)

	s := New(pool)

	var modID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, role) VALUES ('mod_itemstat_test','x','moderator') RETURNING id`).
		Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, modID) })

	if _, err := s.GetItemStat(ctx, 1415); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on an un-overridden item = %v, want ErrNotFound", err)
	}

	// Every field gets a distinct value, so a column dropped from the generated
	// INSERT or SELECT shows up as a mismatch instead of coinciding with a zero.
	want := domain.ItemStat{ItemIndex: 1415}
	v := reflect.ValueOf(&want).Elem()
	for i, n := 0, v.NumField(); i < n; i++ {
		if f := v.Field(i); f.Kind() == reflect.Int16 {
			f.SetInt(int64(i + 1))
		}
	}

	if err := s.UpsertItemStat(ctx, want, modID); err != nil {
		t.Fatalf("UpsertItemStat: %v", err)
	}

	got, err := s.GetItemStat(ctx, 1415)
	if err != nil {
		t.Fatalf("GetItemStat: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip lost a column:\n got %+v\nwant %+v", got, want)
	}

	// A second upsert has to update in place, not fail on the primary key and
	// not leave the first row behind.
	danoAntes := want.Damage
	want.Damage = 999
	if err := s.UpsertItemStat(ctx, want, modID); err != nil {
		t.Fatalf("second UpsertItemStat: %v", err)
	}
	all, err := s.ListItemStats(ctx)
	if err != nil {
		t.Fatalf("ListItemStats: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("list = %d rows, want 1 — the upsert inserted instead of updating", len(all))
	}
	if all[0].Damage != 999 {
		t.Errorf("damage = %d, want 999", all[0].Damage)
	}

	// Both writes are audited and both bump the config version.
	var audits int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM npc_audit WHERE account_id = $1 AND action = 'upsert_item_stat'`, modID).
		Scan(&audits); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits != 2 {
		t.Errorf("audit rows = %d, want 2", audits)
	}

	// The "before" of the second write has to be the first write's row, not
	// null: that is what makes the trail readable after the fact.
	var beforeDamage *int16
	if err := pool.QueryRow(ctx, `
		SELECT (before->>'damage')::smallint FROM npc_audit
		 WHERE account_id = $1 AND action = 'upsert_item_stat'
		 ORDER BY id DESC LIMIT 1`, modID).Scan(&beforeDamage); err != nil {
		t.Fatalf("read audit before: %v", err)
	}
	if beforeDamage == nil {
		t.Error("audit before is null — the trail cannot say what the value was")
	} else if *beforeDamage != danoAntes {
		t.Errorf("audit before.damage = %d, want %d (what the first write stored)", *beforeDamage, danoAntes)
	}

	if err := s.DeleteItemStat(ctx, 1415, modID); err != nil {
		t.Fatalf("DeleteItemStat: %v", err)
	}
	if _, err := s.GetItemStat(ctx, 1415); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := s.DeleteItemStat(ctx, 1415, modID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

// TestItemStatColumnsMatchTheTable catches the failure the generated SQL is
// meant to prevent, from the other side: a field added to domain.ItemStat but
// forgotten in domain.ItemStatFields would round-trip as zero and every other test
// would still pass.
func TestItemStatColumnsMatchTheTable(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var colunas int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		 WHERE table_name = 'item_stat'
		   AND column_name NOT IN ('item_index', 'updated_by', 'updated_at')`).Scan(&colunas); err != nil {
		t.Fatalf("count columns: %v", err)
	}
	if colunas != len(domain.ItemStatFields) {
		t.Errorf("item_stat has %d editable columns but domain.ItemStatFields lists %d", colunas, len(domain.ItemStatFields))
	}

	// And the struct: every int16 field must be bound to a column.
	campos := 0
	v := reflect.ValueOf(domain.ItemStat{})
	for i, n := 0, v.NumField(); i < n; i++ {
		if v.Field(i).Kind() == reflect.Int16 {
			campos++
		}
	}
	if campos != len(domain.ItemStatFields) {
		t.Errorf("domain.ItemStat has %d int16 fields but domain.ItemStatFields lists %d", campos, len(domain.ItemStatFields))
	}
}
