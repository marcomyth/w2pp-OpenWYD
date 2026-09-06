//go:build integration

// Integration tests for the /reportar queue, against a real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// The ordering and the expiry are what earn a database here. Both are SQL, both
// are silent when wrong — a queue in the wrong order just looks like a queue,
// and an expiry that never fires looks like nothing at all.
package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// conta seeds an account and returns its id. internal/store has no shared test
// helper for this, and the trade tests inline the same INSERT.
func conta(t *testing.T, s *Store, nome string) int64 {
	t.Helper()
	var id int64
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO account (name, pass_hash, role) VALUES ($1, 'x', 'moderator')
		 ON CONFLICT (name) DO UPDATE SET pass_hash = 'x' RETURNING id`, nome).Scan(&id)
	if err != nil {
		t.Fatalf("seed conta %q: %v", nome, err)
	}
	t.Cleanup(func() { _, _ = s.pool.Exec(context.Background(), `DELETE FROM account WHERE id = $1`, id) })
	return id
}

func limparReports(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `DELETE FROM player_report`); err != nil {
		t.Fatalf("limpar player_report: %v", err)
	}
}

func denuncia(t *testing.T, s *Store, personagem, texto string) {
	t.Helper()
	err := s.RecordReport(context.Background(), domain.PlayerReport{
		Account: "conta_" + personagem, Character: personagem, Level: 200,
		Text: texto, X: 2800, Y: 2600, Nearby: []string{"Xdfgh", "Xdfgi"},
	})
	if err != nil {
		t.Fatalf("RecordReport(%q): %v", personagem, err)
	}
}

func TestDenunciaGuardaOMomento(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })

	denuncia(t, s, "Vandalyzz", "o cara ali esta botando")

	rs, err := s.ListReports(ctx, ReportQuery{})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("denúncias = %d, want 1", len(rs))
	}
	r := rs[0]
	if r.Character != "Vandalyzz" || r.Text != "o cara ali esta botando" {
		t.Errorf("denúncia = %+v", r)
	}
	// The snapshot is the point: without the position the report is "somewhere
	// on the map", and without the bystanders "somebody here is botting" is not
	// checkable.
	if r.X != 2800 || r.Y != 2600 {
		t.Errorf("posição = %d,%d, want 2800,2600", r.X, r.Y)
	}
	if len(r.Nearby) != 2 || r.Nearby[0] != "Xdfgh" {
		t.Errorf("por perto = %v", r.Nearby)
	}
	if !r.Aberto() {
		t.Error("uma denúncia nova já nasceu tratada")
	}
	// The row carries its own deadline, so nothing has to remember the policy.
	if r.ExpiresAt.Before(time.Now().Add(time.Duration(domain.ReportRetentionDays-1) * 24 * time.Hour)) {
		t.Errorf("prazo = %v, want ~%d dias à frente", r.ExpiresAt, domain.ReportRetentionDays)
	}
}

func TestAFilaComecaPelaEsperaMaisLonga(t *testing.T) {
	// Whoever waited longest is who should be seen next. Newest-first would hide
	// exactly the person who has been waiting since yesterday.
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })

	denuncia(t, s, "Primeira", "cheguei antes")
	denuncia(t, s, "Segunda", "cheguei depois")
	if _, err := s.pool.Exec(ctx,
		`UPDATE player_report SET criado_em = now() - interval '2 hours' WHERE char_nome = 'Primeira'`); err != nil {
		t.Fatalf("envelhecer: %v", err)
	}

	rs, err := s.ListReports(ctx, ReportQuery{})
	if err != nil || len(rs) != 2 {
		t.Fatalf("ListReports = %d, %v", len(rs), err)
	}
	if rs[0].Character != "Primeira" {
		t.Errorf("primeira da fila = %q, want Primeira", rs[0].Character)
	}
}

func TestTratadaSaiDaFilaMasNaoSomeDaTela(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })
	ator := conta(t, s, "mod_report_test")

	denuncia(t, s, "Tratada", "ja resolveram")
	denuncia(t, s, "Aberta", "ninguem viu ainda")
	rs, _ := s.ListReports(ctx, ReportQuery{})
	var id int64
	for _, r := range rs {
		if r.Character == "Tratada" {
			id = r.ID
		}
	}
	if err := s.MarkReportHandled(ctx, id, ator); err != nil {
		t.Fatalf("MarkReportHandled: %v", err)
	}

	abertas, err := s.ListReports(ctx, ReportQuery{SoAbertos: true})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(abertas) != 1 || abertas[0].Character != "Aberta" {
		t.Fatalf("abertas = %+v, want só a Aberta", abertas)
	}
	// Still readable without the filter: what was handled is how somebody
	// answers "was this ever looked at".
	todas, _ := s.ListReports(ctx, ReportQuery{})
	if len(todas) != 2 {
		t.Fatalf("todas = %d, want 2", len(todas))
	}
	for _, r := range todas {
		if r.Character == "Tratada" && (r.Aberto() || r.HandledBy != ator) {
			t.Errorf("a tratada não registrou quem tratou: %+v", r)
		}
	}
}

func TestTratarDuasVezesNaoRoubaOCredito(t *testing.T) {
	// Two moderators clicking the same row is a race, not a mistake.
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })
	primeiro := conta(t, s, "mod_report_um")
	segundo := conta(t, s, "mod_report_dois")

	denuncia(t, s, "Disputada", "quem pega")
	rs, _ := s.ListReports(ctx, ReportQuery{})
	id := rs[0].ID

	if err := s.MarkReportHandled(ctx, id, primeiro); err != nil {
		t.Fatalf("primeiro: %v", err)
	}
	if err := s.MarkReportHandled(ctx, id, segundo); err != nil {
		t.Fatalf("segundo virou erro: %v", err)
	}
	rs, _ = s.ListReports(ctx, ReportQuery{})
	if rs[0].HandledBy != primeiro {
		t.Errorf("tratada por %d, want %d — o primeiro fica com o crédito", rs[0].HandledBy, primeiro)
	}
}

func TestDenunciaVencidaSaiDoBanco(t *testing.T) {
	// The schema promises the row goes away, and this is the only thing that
	// keeps that promise — there is no periodic job to hang it on.
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })

	denuncia(t, s, "Velha", "isso foi ano passado")
	if _, err := s.pool.Exec(ctx,
		`UPDATE player_report SET expira_em = now() - interval '1 day'`); err != nil {
		t.Fatalf("vencer: %v", err)
	}

	rs, err := s.ListReports(ctx, ReportQuery{})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(rs) != 0 {
		t.Errorf("denúncias = %d, want 0 — vencida ainda aparece", len(rs))
	}
	var restam int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM player_report`).Scan(&restam); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if restam != 0 {
		t.Errorf("linhas no banco = %d, want 0 — o prazo não apagou nada", restam)
	}
}

func TestTextoLongoDemaisECortadoEmVezDeRecusado(t *testing.T) {
	// A crafted packet must not write a megabyte per report, and a player who
	// pasted too much should still be heard.
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })

	longo := strings.Repeat("a", domain.MaxReportText*3)
	if err := s.RecordReport(ctx, domain.PlayerReport{
		Account: "c", Character: "Prolixo", Text: longo,
	}); err != nil {
		t.Fatalf("RecordReport: %v", err)
	}
	rs, _ := s.ListReports(ctx, ReportQuery{})
	if len(rs) != 1 {
		t.Fatalf("denúncias = %d, want 1", len(rs))
	}
	if len(rs[0].Text) != domain.MaxReportText {
		t.Errorf("texto guardado = %d bytes, want %d", len(rs[0].Text), domain.MaxReportText)
	}
}

func TestDenunciaVaziaERecusada(t *testing.T) {
	// An empty report is a mis-typed command, not a complaint. Storing it would
	// put a blank row at the top of the queue.
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	if err := s.RecordReport(context.Background(), domain.PlayerReport{
		Account: "c", Character: "Mudo", Text: "   ",
	}); err == nil {
		t.Error("uma denúncia sem texto foi aceita")
	}
}

func TestDenunciasDeUmaContaSo(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })
	conta := conta(t, s, "dono_report_test")

	if err := s.RecordReport(ctx, domain.PlayerReport{
		AccountID: conta, Account: "dono_report_test", Character: "Dono", Text: "minha",
	}); err != nil {
		t.Fatalf("RecordReport: %v", err)
	}
	denuncia(t, s, "Outro", "de outra conta")

	rs, err := s.ListReports(ctx, ReportQuery{AccountID: conta})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(rs) != 1 || rs[0].Character != "Dono" {
		t.Errorf("denúncias da conta = %+v, want só a dela", rs)
	}
}

func TestContagemDaFila(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	limparReports(t, s)
	t.Cleanup(func() { limparReports(t, s) })
	ator := conta(t, s, "mod_report_conta")

	denuncia(t, s, "Uma", "a")
	denuncia(t, s, "Duas", "b")
	rs, _ := s.ListReports(ctx, ReportQuery{})
	if err := s.MarkReportHandled(ctx, rs[0].ID, ator); err != nil {
		t.Fatalf("MarkReportHandled: %v", err)
	}

	c, err := s.CountReports(ctx)
	if err != nil {
		t.Fatalf("CountReports: %v", err)
	}
	if c.Abertos != 1 || c.Total != 2 {
		t.Errorf("contagem = %+v, want 1 aberto de 2", c)
	}
	if c.MaisAntigo.IsZero() {
		t.Error("com fila aberta, a espera mais longa não pode vir zerada")
	}
}
