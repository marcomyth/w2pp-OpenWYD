//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestTetoDaRodadaNoBanco: a linha que já existe ganha os tetos decididos da 0073
// ao migrar, o valor gravado volta como foi (zero incluído) e o CHECK recusa
// negativo.
func TestTetoDaRodadaNoBanco(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	resetTestSchema(ctx, pool)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	st := New(pool)
	inicial, err := st.WorldEventConfig(ctx)
	if err != nil || inicial.RoundXPCap != domain.DefaultRoundXPCap || inicial.RoundXPCapDouble != domain.DefaultRoundXPCapDouble {
		t.Fatalf("config inicial = %v / %v (%v), want os tetos decididos", inicial.RoundXPCap, inicial.RoundXPCapDouble, err)
	}

	cfg := domain.DefaultWorldEventConfig()
	cfg.RoundXPCap = [5]int64{1, 0, 3, 4, 5}
	cfg.RoundXPCapDouble = [5]int64{6, 7, 8, 9, 0}
	if err := st.UpsertWorldEventConfig(ctx, cfg, 0); err != nil {
		t.Fatalf("UpsertWorldEventConfig: %v", err)
	}
	got, err := st.WorldEventConfig(ctx)
	if err != nil || got.RoundXPCap != cfg.RoundXPCap || got.RoundXPCapDouble != cfg.RoundXPCapDouble {
		t.Fatalf("voltou %v / %v (%v), want %v / %v", got.RoundXPCap, got.RoundXPCapDouble, err, cfg.RoundXPCap, cfg.RoundXPCapDouble)
	}

	ruim := domain.DefaultWorldEventConfig()
	ruim.RoundXPCapDouble[4] = -1
	if err := st.UpsertWorldEventConfig(ctx, ruim, 0); err == nil {
		t.Error("o banco aceitou um teto negativo")
	}
}
