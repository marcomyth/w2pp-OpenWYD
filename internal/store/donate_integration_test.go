//go:build integration

// Integration tests for the donate web-shop store (issue #34). They require a
// real database and are excluded from the default build. Run with:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestDonateShopCRUD exercises the moderator catalog write path: create, update,
// toggle enabled, list, get, delete.
func TestDonateShopCRUD(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM donate_shop_audit; DELETE FROM donate_shop_item`)
	s := New(pool)

	var modID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, role) VALUES ('mod_donate_test','x','moderator') RETURNING id`).
		Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, modID) })

	id, err := s.UpsertDonateShopItem(ctx, domain.DonateShopItem{
		ItemIndex: 3540, Eff1: 1, EffV1: 9, Price: 100, Title: "Set Celestial",
		Description: "shiny", Enabled: true, ExpiresDays: 30,
	}, modID)
	if err != nil {
		t.Fatalf("upsert create: %v", err)
	}

	got, err := s.GetDonateShopItem(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ItemIndex != 3540 || got.Price != 100 || !got.Enabled || got.ExpiresDays != 30 || got.Eff1 != 1 || got.EffV1 != 9 {
		t.Errorf("offer mismatch: %+v", got)
	}

	// Update in place (same id).
	got.Price = 250
	got.Title = "Set Celestial +11"
	if _, err := s.UpsertDonateShopItem(ctx, got, modID); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if updated, _ := s.GetDonateShopItem(ctx, id); updated.Price != 250 || updated.Title != "Set Celestial +11" {
		t.Errorf("update not applied: %+v", updated)
	}

	// Updating a missing id is NotFound.
	if _, err := s.UpsertDonateShopItem(ctx, domain.DonateShopItem{ID: 999999, ItemIndex: 1, Price: 1}, modID); !errors.Is(err, ErrNotFound) {
		t.Errorf("upsert missing id err = %v, want ErrNotFound", err)
	}

	if err := s.SetDonateShopItemEnabled(ctx, id, false, modID); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	all, err := s.ListDonateShopItems(ctx)
	if err != nil || len(all) != 1 || all[0].Enabled {
		t.Fatalf("list = %+v (err %v), want 1 disabled offer", all, err)
	}
	if enabled, _ := s.ListEnabledDonateShopItems(ctx); len(enabled) != 0 {
		t.Errorf("enabled list = %d, want 0 (offer disabled)", len(enabled))
	}

	if err := s.DeleteDonateShopItem(ctx, id, modID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetDonateShopItem(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete err = %v, want ErrNotFound", err)
	}
}

// TestDonateCreditAndBuy exercises the wallet credit + purchase + drain path.
func TestDonateCreditAndBuy(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM donate_shop_audit; DELETE FROM donate_shop_item; DELETE FROM delivery_queue`)
	s := New(pool)

	var accID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, donate_balance) VALUES ('buyer_donate_test','x',0) RETURNING id`).
		Scan(&accID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, accID) })

	// Credit the wallet.
	if bal, err := s.CreditDonateBalance(ctx, accID, 300, 0, "manual grant"); err != nil || bal != 300 {
		t.Fatalf("credit = (%d, %v), want (300, nil)", bal, err)
	}

	// One enabled offer at price 100 (with an effect + 7-day expiry).
	itemID, err := s.UpsertDonateShopItem(ctx, domain.DonateShopItem{
		ItemIndex: 1234, Eff1: 2, EffV1: 5, Price: 100, Enabled: true, ExpiresDays: 7,
	}, 0)
	if err != nil {
		t.Fatalf("upsert offer: %v", err)
	}

	// Buy it: balance drops to 200 and a pending item delivery appears.
	bal, err := s.BuyDonateItem(ctx, accID, itemID)
	if err != nil || bal != 200 {
		t.Fatalf("buy = (%d, %v), want (200, nil)", bal, err)
	}
	pending, err := s.PendingItemDeliveries(ctx, accID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v (err %v), want 1", pending, err)
	}
	d := pending[0]
	// The 7 days ride un-started, as EF_WDAY in the first free slot: the clock
	// starts when the player equips it, not at the purchase.
	if d.Item.Index != 1234 || d.Item.Eff1 != 2 || d.Item.EffV1 != 5 ||
		d.Item.Eff2 != efWDay || d.Item.EffV2 != 7 || d.Item.ExpiresAt != 0 {
		t.Errorf("delivery payload mismatch: %+v", d.Item)
	}

	// Buying again with only 200 left works twice more, then goes insufficient.
	if _, err := s.BuyDonateItem(ctx, accID, itemID); err != nil {
		t.Fatalf("buy 2: %v", err)
	}
	if _, err := s.BuyDonateItem(ctx, accID, itemID); err != nil {
		t.Fatalf("buy 3: %v", err)
	}
	if _, err := s.BuyDonateItem(ctx, accID, itemID); !errors.Is(err, ErrInsufficientDonate) {
		t.Errorf("buy 4 err = %v, want ErrInsufficientDonate", err)
	}

	// A disabled offer rejects the purchase.
	disabledID, _ := s.UpsertDonateShopItem(ctx, domain.DonateShopItem{ItemIndex: 5, Price: 1, Enabled: false}, 0)
	if _, err := s.CreditDonateBalance(ctx, accID, 10, 0, "topup"); err != nil {
		t.Fatalf("topup: %v", err)
	}
	if _, err := s.BuyDonateItem(ctx, accID, disabledID); !errors.Is(err, ErrShopItemDisabled) {
		t.Errorf("buy disabled err = %v, want ErrShopItemDisabled", err)
	}

	// An unknown offer is NotFound.
	if _, err := s.BuyDonateItem(ctx, accID, 987654); !errors.Is(err, ErrNotFound) {
		t.Errorf("buy unknown err = %v, want ErrNotFound", err)
	}

	// Drain: mark the first pending delivery delivered, a fake one lost, and write
	// the cargo — all in one transaction. The delivered row leaves the pending set.
	all, _ := s.PendingItemDeliveries(ctx, accID)
	if len(all) != 3 {
		t.Fatalf("pending before drain = %d, want 3", len(all))
	}
	deliveredIDs := []int64{all[0].ID, all[1].ID}
	lostIDs := []int64{all[2].ID}
	cargoItems := []domain.Item{{Slot: 0, Index: 1234, Eff1: 2, EffV1: 5, ExpiresAt: all[0].Item.ExpiresAt}}
	if err := s.SaveCargoWithDeliveries(ctx, accID, 0, cargoItems, deliveredIDs, lostIDs); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if remaining, _ := s.PendingItemDeliveries(ctx, accID); len(remaining) != 0 {
		t.Errorf("pending after drain = %d, want 0", len(remaining))
	}
	// The cargo now holds the delivered item.
	_, items, err := s.LoadCargo(ctx, accID)
	if err != nil || len(items) != 1 || items[0].Index != 1234 {
		t.Errorf("cargo after drain = %+v (err %v), want 1 item index 1234", items, err)
	}
}

// TestCreditDonateInGameNaoContaComoReceita é a decisão de projeto do RCoin
// escrita como teste: a moeda enche a MESMA carteira que o ajuste manual, mas
// não pode aparecer no relatório de receita.
//
// O painel soma 'credit_balance' como "staff distribuiu donate". Um jogador
// gastando uma moeda que já era dele não é uma distribuição nem dinheiro novo:
// contá-la ali inflaria o relatório a cada moeda aberta. Por isso ela grava
// 'credit_item', e por isso este teste existe — trocar a ação de volta passaria
// em todo o resto.
func TestCreditDonateInGameNaoContaComoReceita(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM donate_shop_audit; DELETE FROM donate_shop_item; DELETE FROM delivery_queue`)
	s := New(pool)

	var accID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, donate_balance) VALUES ('rcoin_test','x',0) RETURNING id`).
		Scan(&accID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, accID) })

	// Duas moedas: a carteira acumula.
	if bal, err := s.CreditDonateInGame(ctx, accID, 100, "Hero", "rcoin 3393"); err != nil || bal != 100 {
		t.Fatalf("primeira moeda = (%d, %v), quer (100, nil)", bal, err)
	}
	if bal, err := s.CreditDonateInGame(ctx, accID, 1000, "Hero", "rcoin 3394"); err != nil || bal != 1100 {
		t.Fatalf("segunda moeda = (%d, %v), quer (1100, nil)", bal, err)
	}
	if bal, err := s.DonateBalance(ctx, accID); err != nil || bal != 1100 {
		t.Fatalf("saldo = (%d, %v), quer (1100, nil)", bal, err)
	}

	// Um valor não positivo é recusado e não encosta na carteira.
	for _, amount := range []int32{0, -50} {
		if _, err := s.CreditDonateInGame(ctx, accID, amount, "Hero", "erro"); err == nil {
			t.Errorf("CreditDonateInGame aceitou %d, queria recusa", amount)
		}
	}
	if bal, _ := s.DonateBalance(ctx, accID); bal != 1100 {
		t.Errorf("a recusa mexeu na carteira: %d, quer 1100", bal)
	}

	// A auditoria grava credit_item, e NÃO o credit_balance do ajuste manual.
	var item, manual int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE action = $2), count(*) FILTER (WHERE action = $3)
		  FROM donate_shop_audit WHERE (after->>'account_id')::bigint = $1`,
		accID, LedgerActionItem, LedgerActionCredit).Scan(&item, &manual); err != nil {
		t.Fatalf("contar auditoria: %v", err)
	}
	if item != 2 || manual != 0 {
		t.Fatalf("auditoria: %d de '%s' e %d de '%s', quer 2 e 0",
			item, LedgerActionItem, manual, LedgerActionCredit)
	}
}
