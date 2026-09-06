//go:build integration

// Integration tests for the ground log, against a real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// What earns a database here is the pairing and the sweep. The pairing is the
// whole reason the table exists — a "largou" and the "pegou" that followed on
// the same floor slot — and it is only visible in the order rows come back. The
// sweep is silent when wrong: an expiry that never fires looks like nothing at
// all until the table is enormous.
package store

import (
	"context"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func limparChao(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `DELETE FROM ground_log`); err != nil {
		t.Fatalf("limpar ground_log: %v", err)
	}
}

func storeChao(t *testing.T) *Store {
	t.Helper()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparChao(t, s)
	t.Cleanup(func() { limparChao(t, s) })
	return s
}

func TestChaoEmparelhaLargouComPegou(t *testing.T) {
	ctx := context.Background()
	s := storeChao(t)
	id := conta(t, s, "conta_chao")

	espada := domain.TradeItem{Index: 1100}
	espada.Eff[0] = [2]uint8{1, 9} // refino: é o que separa esta espada da comum
	espada.Eff[1] = [2]uint8{4, 2}
	espada.Eff[2] = [2]uint8{7, 3}

	if err := s.RecordGround(ctx, domain.GroundEvent{
		Acao: domain.GroundLargou, AccountID: id, Character: "Doador",
		Item: espada, X: 2100, Y: 2100, GroundID: 77,
	}); err != nil {
		t.Fatalf("RecordGround largou: %v", err)
	}
	if err := s.RecordGround(ctx, domain.GroundEvent{
		Acao: domain.GroundPegou, AccountID: id, Character: "Pegador",
		Item: espada, X: 2100, Y: 2100, GroundID: 77,
	}); err != nil {
		t.Fatalf("RecordGround pegou: %v", err)
	}

	gs, err := s.ListGround(ctx, GroundQuery{})
	if err != nil {
		t.Fatalf("ListGround: %v", err)
	}
	if len(gs) != 2 {
		t.Fatalf("linhas = %d, want 2", len(gs))
	}
	// Newest first: the pickup is what somebody opening a ticket sees on top.
	if gs[0].Acao != domain.GroundPegou || gs[1].Acao != domain.GroundLargou {
		t.Fatalf("ordem = %q,%q, want pegou,largou", gs[0].Acao, gs[1].Acao)
	}
	if gs[0].Character != "Pegador" || gs[1].Character != "Doador" {
		t.Errorf("nomes = %q,%q", gs[0].Character, gs[1].Character)
	}
	if gs[0].GroundID != 77 || gs[1].GroundID != 77 {
		t.Errorf("chao_id = %d,%d, want 77 nos dois", gs[0].GroundID, gs[1].GroundID)
	}
	// The effects are why an index alone is not enough: two items share 1100 and
	// only one of them is refined.
	if gs[0].Item != espada {
		t.Errorf("item = %+v, want %+v", gs[0].Item, espada)
	}
	if gs[0].AccountID != id {
		t.Errorf("conta = %d, want %d", gs[0].AccountID, id)
	}
	if gs[0].At.IsZero() {
		t.Error("ocorrido_em vazio")
	}
}

func TestChaoFiltraPorPersonagem(t *testing.T) {
	ctx := context.Background()
	s := storeChao(t)

	for _, nome := range []string{"Alpha", "Beta", "Alpha"} {
		if err := s.RecordGround(ctx, domain.GroundEvent{
			Acao: domain.GroundLargou, Character: nome,
			Item: domain.TradeItem{Index: 1100}, GroundID: 3,
		}); err != nil {
			t.Fatalf("RecordGround %q: %v", nome, err)
		}
	}

	gs, err := s.ListGround(ctx, GroundQuery{Char: "Alpha"})
	if err != nil {
		t.Fatalf("ListGround: %v", err)
	}
	if len(gs) != 2 {
		t.Fatalf("linhas de Alpha = %d, want 2", len(gs))
	}
	for _, g := range gs {
		if g.Character != "Alpha" {
			t.Errorf("filtro devolveu %q", g.Character)
		}
	}
}

// A drop with no account behind it (a mob's loot, a session that already went)
// has to store NULL, not a zero that would read as account number zero.
func TestChaoAceitaSemConta(t *testing.T) {
	ctx := context.Background()
	s := storeChao(t)

	if err := s.RecordGround(ctx, domain.GroundEvent{
		Acao: domain.GroundLargou, Character: "Anonimo",
		Item: domain.TradeItem{Index: 1100},
	}); err != nil {
		t.Fatalf("RecordGround: %v", err)
	}
	gs, err := s.ListGround(ctx, GroundQuery{})
	if err != nil {
		t.Fatalf("ListGround: %v", err)
	}
	if len(gs) != 1 || gs[0].AccountID != 0 {
		t.Fatalf("linhas = %+v, want uma com conta 0", gs)
	}
}

func TestChaoVarreVencidos(t *testing.T) {
	ctx := context.Background()
	s := storeChao(t)

	if err := s.RecordGround(ctx, domain.GroundEvent{
		Acao: domain.GroundPegou, Character: "Velho",
		Item: domain.TradeItem{Index: 1100},
	}); err != nil {
		t.Fatalf("RecordGround: %v", err)
	}
	// Age the row past its own deadline instead of waiting thirty days.
	if _, err := s.pool.Exec(ctx,
		`UPDATE ground_log SET expira_em = now() - interval '1 minute'`); err != nil {
		t.Fatalf("envelhecer linha: %v", err)
	}

	gs, err := s.ListGround(ctx, GroundQuery{})
	if err != nil {
		t.Fatalf("ListGround: %v", err)
	}
	if len(gs) != 0 {
		t.Fatalf("linhas = %d, want 0 (vencida)", len(gs))
	}
	// Read, not hidden: the sweep has to actually delete, or the table grows
	// forever with rows nobody can see.
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM ground_log`).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Errorf("linhas na tabela = %d, want 0", n)
	}
}

// The prazo is thirty days from the write, not a fixed date compiled in.
func TestChaoGuardaTrintaDias(t *testing.T) {
	ctx := context.Background()
	s := storeChao(t)

	if err := s.RecordGround(ctx, domain.GroundEvent{
		Acao: domain.GroundLargou, Character: "Novo",
		Item: domain.TradeItem{Index: 1100},
	}); err != nil {
		t.Fatalf("RecordGround: %v", err)
	}
	var falta time.Duration
	var expira, agora time.Time
	if err := s.pool.QueryRow(ctx,
		`SELECT expira_em, now() FROM ground_log`).Scan(&expira, &agora); err != nil {
		t.Fatalf("ler expira_em: %v", err)
	}
	falta = expira.Sub(agora)
	if falta < 29*24*time.Hour || falta > 31*24*time.Hour {
		t.Errorf("prazo = %v, want perto de %d dias", falta, domain.GroundRetentionDays)
	}
}

// An unknown action is refused before it reaches SQL, so a caller typo cannot
// quietly write a row nothing will ever match.
func TestChaoRecusaAcaoDesconhecida(t *testing.T) {
	s := storeChao(t)
	err := s.RecordGround(context.Background(), domain.GroundEvent{
		Acao: "jogou", Character: "Fulano", Item: domain.TradeItem{Index: 1100},
	})
	if err == nil {
		t.Fatal("RecordGround aceitou ação desconhecida")
	}
}
