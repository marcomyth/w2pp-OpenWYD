//go:build integration

// Integration tests for the presence read the account page leans on, against a
// real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./adminserver/internal/personagem/
//
// This one earns a database because its failure mode is silent: a wrong column
// or a wrong table gives an empty map, and an empty map renders as a list with
// no marks on it — which looks exactly like nobody being online.
package personagem

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("W2PP_TEST_DSN")
	if dsn == "" {
		t.Skip("W2PP_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := store.Pool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func conta(t *testing.T, pool *pgxpool.Pool, nome string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO account (name, pass_hash, role) VALUES ($1, 'x', 'player')
		 ON CONFLICT (name) DO UPDATE SET pass_hash = 'x' RETURNING id`, nome).Scan(&id)
	if err != nil {
		t.Fatalf("seed conta %q: %v", nome, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM account WHERE id = $1`, id) })
	return id
}

func personagem(t *testing.T, pool *pgxpool.Pool, contaID int64, slot int, nome string, emJogo bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO character (account_id, slot, name, online_since)
		VALUES ($1, $2, $3, CASE WHEN $4 THEN now() ELSE NULL END)`,
		contaID, slot, nome, emJogo)
	if err != nil {
		t.Fatalf("seed personagem %q: %v", nome, err)
	}
}

func TestEmJogoPorSlotSeparaQuemEstaDentro(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	id := conta(t, pool, "emjogo_test")
	personagem(t, pool, id, 0, "EmJogoTestFora", false)
	personagem(t, pool, id, 2, "EmJogoTestDentro", true)

	got, err := s.EmJogoPorSlot(ctx, id)
	if err != nil {
		t.Fatalf("EmJogoPorSlot: %v", err)
	}
	// Keyed by slot, not by position: the account page addresses characters by
	// slot, and slots are sparse — this account has 0 and 2.
	if len(got) != 2 {
		t.Fatalf("got = %v, want dois slots", got)
	}
	if got[0] {
		t.Error("um personagem fora do jogo veio marcado como em jogo")
	}
	if !got[2] {
		t.Error("o personagem em jogo não veio marcado — a lista mostraria que dá para editar")
	}
}

func TestEmJogoPorSlotSoVeAContaPedida(t *testing.T) {
	// The map is keyed by slot, so a character from another account leaking in
	// would silently overwrite the mark of the slot with the same number.
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	minha := conta(t, pool, "emjogo_minha_test")
	outra := conta(t, pool, "emjogo_outra_test")
	personagem(t, pool, minha, 0, "EmJogoMinhaFora", false)
	personagem(t, pool, outra, 0, "EmJogoOutraDentro", true)

	got, err := s.EmJogoPorSlot(ctx, minha)
	if err != nil {
		t.Fatalf("EmJogoPorSlot: %v", err)
	}
	if len(got) != 1 || got[0] {
		t.Errorf("got = %v, want só o slot 0 desta conta, fora do jogo", got)
	}
}

func TestEmJogoPorSlotSemPersonagemNaoEErro(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	id := conta(t, pool, "emjogo_vazia_test")

	got, err := s.EmJogoPorSlot(ctx, id)
	if err != nil {
		t.Fatalf("EmJogoPorSlot: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want vazio", got)
	}
}
