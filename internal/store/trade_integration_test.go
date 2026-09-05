//go:build integration

// Integration tests for the trade log (0025_trade_log). Run with:
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func TestTradeLogRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM trade_log`)
	s := New(pool)

	quero := domain.TradeRecord{
		CharA: "Vendedor", CharB: "Comprador",
		AccountA: 0, AccountB: 0,
		GoldA: 0, GoldB: 5000,
		ItemsA: []domain.TradeItem{
			{Index: 1415, Eff: [3][2]uint8{{7, 42}, {8, 9}, {0, 0}}},
			{Index: 2000},
		},
		ItemsB: nil,
	}
	if err := s.RecordTrade(ctx, quero); err != nil {
		t.Fatalf("RecordTrade: %v", err)
	}

	got, err := s.ListTrades(ctx, TradeQuery{Char: "Vendedor"})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("trocas = %d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0].ItemsA, quero.ItemsA) {
		t.Errorf("itens de A voltaram %+v, want %+v", got[0].ItemsA, quero.ItemsA)
	}
	if got[0].GoldB != 5000 {
		t.Errorf("ouro de B = %d, want 5000", got[0].GoldB)
	}
	if got[0].At.IsZero() {
		t.Error("ocorrido_em ficou zerado")
	}

	// The other side finds it too: a complaint names whichever player is
	// complaining, and they might have been on either side of the window.
	if outro, err := s.ListTrades(ctx, TradeQuery{Char: "Comprador"}); err != nil || len(outro) != 1 {
		t.Fatalf("busca pelo outro lado = %d trocas, %v; want 1", len(outro), err)
	}
	if nada, err := s.ListTrades(ctx, TradeQuery{Char: "Ninguem"}); err != nil || len(nada) != 0 {
		t.Fatalf("busca por quem não trocou = %d, %v; want 0", len(nada), err)
	}
	// An empty name browses everything, which is the recent-activity view.
	if todas, err := s.ListTrades(ctx, TradeQuery{}); err != nil || len(todas) != 1 {
		t.Fatalf("busca vazia = %d trocas, %v; want 1", len(todas), err)
	}
}

func TestTradeLogSurvivesADeletedCharacter(t *testing.T) {
	// No foreign key, on purpose: DeleteCharacter is player-invocable and
	// physical, so a suspect must not be able to delete the evidence by deleting
	// the character it names.
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM trade_log`)
	s := New(pool)

	var conta int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash) VALUES ('trade_evidencia','x') RETURNING id`).
		Scan(&conta); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	if err := s.RecordTrade(ctx, domain.TradeRecord{
		CharA: "Sumido", CharB: "Vitima", AccountA: conta, GoldB: 100,
	}); err != nil {
		t.Fatalf("RecordTrade: %v", err)
	}

	// Deleting the account must not take the row with it.
	if _, err := pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, conta); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	got, err := s.ListTrades(ctx, TradeQuery{Char: "Sumido"})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("trocas = %d, want 1 — apagar a conta levou a prova junto", len(got))
	}
	if got[0].CharA != "Sumido" {
		t.Errorf("nome = %q, want Sumido", got[0].CharA)
	}
}

func TestTradeLogLimitIsBounded(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM trade_log`)
	s := New(pool)

	for i := 0; i < 5; i++ {
		if err := s.RecordTrade(ctx, domain.TradeRecord{CharA: "A", CharB: "B", GoldA: int32(i + 1)}); err != nil {
			t.Fatalf("RecordTrade %d: %v", i, err)
		}
	}
	got, err := s.ListTrades(ctx, TradeQuery{Limit: 2})
	if err != nil {
		t.Fatalf("ListTrades: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("trocas = %d, want 2", len(got))
	}
	// Newest first: the last written must lead, or a scam report opens on the
	// oldest trade of the day.
	if got[0].GoldA != 5 {
		t.Errorf("primeira troca = ouro %d, want 5 (a mais recente)", got[0].GoldA)
	}

	// A silly limit is clamped rather than honoured.
	if muitas, err := s.ListTrades(ctx, TradeQuery{Limit: 100000}); err != nil || len(muitas) != 5 {
		t.Fatalf("limite absurdo = %d trocas, %v; want as 5 existentes", len(muitas), err)
	}
}
