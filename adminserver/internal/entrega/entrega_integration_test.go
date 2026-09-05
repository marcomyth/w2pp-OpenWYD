//go:build integration

// Integration tests for the panel's item mailbox writes, against a real
// PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./adminserver/internal/entrega/
//
// The payload shape is the point. delivery_queue.payload is read by the tmServer
// drain, which was written against the donate shop's struct — so these tests
// assert the exact JSON keys rather than a Go round trip, which would agree with
// itself no matter what it spelled.
package entrega

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

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
		t.Fatalf("seed %q: %v", nome, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM account WHERE id = $1`, id) })
	return id
}

func TestPayloadKeysAreTheOnesTheGameReads(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	alvo := conta(t, pool, "entrega_payload_test")
	ator := conta(t, pool, "entrega_ator_test")

	id, err := s.Enfileirar(ctx, ator, alvo, Item{
		Index: 1415,
		Eff:   [3][2]uint8{{7, 42}, {8, 9}, {0, 0}},
		Dias:  30,
	})
	if err != nil {
		t.Fatalf("Enfileirar: %v", err)
	}

	var raw []byte
	var kind, status, source string
	if err := pool.QueryRow(ctx,
		`SELECT payload, kind, status, source FROM delivery_queue WHERE id = $1`, id).
		Scan(&raw, &kind, &status, &source); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if kind != "item" || status != "pending" {
		t.Errorf("kind/status = %q/%q, want item/pending", kind, status)
	}
	if source == "" {
		t.Error("source ficou vazio — não dá para rastrear quem entregou")
	}

	// The keys, spelled out. The drain reads these names; a Go round trip would
	// pass even if every one of them were wrong.
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload não é JSON: %v", err)
	}
	for chave, quer := range map[string]float64{
		"item_index": 1415, "eff1": 7, "effv1": 42, "eff2": 8, "effv2": 9, "eff3": 0, "effv3": 0,
	} {
		v, ok := got[chave]
		if !ok {
			t.Errorf("payload não tem a chave %q", chave)
			continue
		}
		if v.(float64) != quer {
			t.Errorf("payload[%q] = %v, want %v", chave, v, quer)
		}
	}
	exp, ok := got["expires_at"].(float64)
	if !ok {
		t.Fatal("payload não tem expires_at")
	}
	// 30 days out, give or take the time this test takes to run.
	quer := time.Now().Add(30 * 24 * time.Hour).Unix()
	if d := int64(exp) - quer; d < -60 || d > 60 {
		t.Errorf("expires_at = %v, want perto de %v", int64(exp), quer)
	}
}

func TestPermanentGrantHasNoExpiry(t *testing.T) {
	// 0 has to mean permanent, not "expired in 1970": the drain compares against
	// now(), so a past timestamp would make the item vanish on arrival.
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	alvo := conta(t, pool, "entrega_perm_test")

	id, err := s.Enfileirar(ctx, alvo, alvo, Item{Index: 1415})
	if err != nil {
		t.Fatalf("Enfileirar: %v", err)
	}
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM delivery_queue WHERE id = $1`, id).Scan(&raw); err != nil {
		t.Fatalf("read row: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["expires_at"].(float64) != 0 {
		t.Errorf("expires_at = %v, want 0 (permanente)", got["expires_at"])
	}
}

func TestPendentesEDepoisCancelar(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	alvo := conta(t, pool, "entrega_fila_test")
	outra := conta(t, pool, "entrega_outra_test")

	id, err := s.Enfileirar(ctx, alvo, alvo, Item{Index: 2000, Eff: [3][2]uint8{{7, 42}}})
	if err != nil {
		t.Fatalf("Enfileirar: %v", err)
	}

	fila, err := s.Pendentes(ctx, alvo)
	if err != nil {
		t.Fatalf("Pendentes: %v", err)
	}
	if len(fila) != 1 || fila[0].ID != id || fila[0].ItemIndex != 2000 {
		t.Fatalf("fila = %+v, want a entrega %d do item 2000", fila, id)
	}
	if fila[0].Eff[0] != [2]uint8{7, 42} {
		t.Errorf("efeito = %v, want {7 42}", fila[0].Eff[0])
	}
	if fila[0].Expira() {
		t.Error("uma entrega permanente apareceu como temporária")
	}

	// Another account cannot cancel it, even knowing the id.
	if err := s.Cancelar(ctx, outra, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelar de outra conta = %v, want ErrNotFound", err)
	}
	if fila, _ := s.Pendentes(ctx, alvo); len(fila) != 1 {
		t.Fatal("a entrega sumiu apesar da recusa")
	}

	if err := s.Cancelar(ctx, alvo, id); err != nil {
		t.Fatalf("Cancelar: %v", err)
	}
	if fila, _ := s.Pendentes(ctx, alvo); len(fila) != 0 {
		t.Error("a entrega continuou na fila depois do cancelamento")
	}
	if err := s.Cancelar(ctx, alvo, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancelar duas vezes = %v, want ErrNotFound", err)
	}
}

func TestCancelarRecusaOQueJaFoiEntregue(t *testing.T) {
	// Once the player has it, taking it back is a different act. Saying so beats
	// a button that silently does nothing.
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	alvo := conta(t, pool, "entrega_entregue_test")

	id, err := s.Enfileirar(ctx, alvo, alvo, Item{Index: 1415})
	if err != nil {
		t.Fatalf("Enfileirar: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE delivery_queue SET status = 'delivered' WHERE id = $1`, id); err != nil {
		t.Fatalf("marcar como entregue: %v", err)
	}

	if err := s.Cancelar(ctx, alvo, id); !errors.Is(err, ErrJaEntregue) {
		t.Errorf("cancelar entregue = %v, want ErrJaEntregue", err)
	}
	if fila, _ := s.Pendentes(ctx, alvo); len(fila) != 0 {
		t.Error("uma entrega já feita apareceu como pendente")
	}
}

func TestPrazoForaDaFaixaERecusado(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)
	alvo := conta(t, pool, "entrega_prazo_test")

	for _, dias := range []int{-1, MaxDias + 1} {
		if _, err := s.Enfileirar(ctx, alvo, alvo, Item{Index: 1415, Dias: dias}); !errors.Is(err, ErrDias) {
			t.Errorf("prazo %d = %v, want ErrDias", dias, err)
		}
	}
	if fila, _ := s.Pendentes(ctx, alvo); len(fila) != 0 {
		t.Error("um prazo recusado ainda assim enfileirou")
	}
}
