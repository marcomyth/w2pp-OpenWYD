package panel

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/level"
)

type fakeMesa struct {
	mu      sync.Mutex
	regras  map[[2]int32]domain.XPRule
	versao  int64
	lerErr  error
	gravErr error
	ator    []int64
}

func newFakeMesa() *fakeMesa {
	return &fakeMesa{regras: map[[2]int32]domain.XPRule{}}
}

func (f *fakeMesa) XPConfig(context.Context) (domain.XPConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lerErr != nil {
		return domain.XPConfig{}, f.lerErr
	}
	cfg := domain.XPConfig{Version: f.versao}
	for _, r := range f.regras {
		cfg.Rules = append(cfg.Rules, r)
	}
	return cfg, nil
}

func (f *fakeMesa) UpsertXPRule(_ context.Context, r domain.XPRule, ator int64) (domain.XPRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gravErr != nil {
		return domain.XPRule{}, f.gravErr
	}
	antes := f.regras[[2]int32{r.Zone, r.Tier}]
	f.regras[[2]int32{r.Zone, r.Tier}] = r
	f.versao++
	f.ator = append(f.ator, ator)
	return antes, nil
}

func (f *fakeMesa) DeleteXPRule(_ context.Context, zona, evo int32) (domain.XPRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	antes := f.regras[[2]int32{zona, evo}]
	delete(f.regras, [2]int32{zona, evo})
	f.versao++
	return antes, nil
}

func newTestPanelMesa(t *testing.T, cargo string, mesa MesaXP, log AuditLog) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(cargo), Writer: newFakeWriter(), Audit: log, MesaXP: mesa,
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

func abrirMesa(t *testing.T, h http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	c := sessionCookie(postLogin(h, "chefe", testPassword))
	if c == nil {
		t.Fatal("o login não devolveu cookie")
	}
	req := httptest.NewRequest(http.MethodGet, "/auditoria/xp"+query, nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestMesaMostraATabelaDoLegado is the page's first job: before anything is
// edited, it has to show what the server actually pays today.
func TestMesaMostraATabelaDoLegado(t *testing.T) {
	h := newTestPanelMesa(t, roleAdmin, newFakeMesa(), newFakeAudit())
	rec := abrirMesa(t, h, "?zona=0&evolucao=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	corpo := rec.Body.String()
	for _, quero := range []string{"Mesa de XP", "Campo", "Mortal", "1.07", "Pesadelo Arcano"} {
		if !strings.Contains(corpo, quero) {
			t.Errorf("a página não mostra %q", quero)
		}
	}
}

// TestMesaNaoTemJavaScript: the panel is served under CSP default-src 'none',
// where a script tag dies silently in production while every build, test and
// lint stays green. The only defence is refusing to write one.
func TestMesaNaoTemJavaScript(t *testing.T) {
	h := newTestPanelMesa(t, roleAdmin, newFakeMesa(), newFakeAudit())
	corpo := abrirMesa(t, h, "").Body.String()
	for _, proibido := range []string{"<script", "onclick=", "onchange=", "javascript:"} {
		if strings.Contains(strings.ToLower(corpo), proibido) {
			t.Errorf("a página usa %q, e a CSP do painel mata isso calado", proibido)
		}
	}
}

func TestMesaSimulaUmaMorte(t *testing.T) {
	h := newTestPanelMesa(t, roleAdmin, newFakeMesa(), newFakeAudit())
	// Level 371 Mortal, mob 371 worth 20.000, no bonuses, no events, Kefra dead:
	// the same kill internal/level pins at 2726 for the general field.
	rec := abrirMesa(t, h, "?simular=1&zona=0&evolucao=2&mob_exp=20000&mob_nivel=371&nivel=371&segundos=6")
	corpo := rec.Body.String()
	if !strings.Contains(corpo, "2726") {
		t.Errorf("a simulação não mostrou 2726 de XP por morte")
	}
	if !strings.Contains(corpo, "Mortes para subir") {
		t.Error("a simulação não mostrou quantas mortes faltam para o nível")
	}
}

// TestMesaAvisaQuandoOMonstroNaoPagaNada is the case the whole level-cut system
// exists for, and the page has to name it instead of showing a silent zero.
func TestMesaAvisaQuandoOMonstroNaoPagaNada(t *testing.T) {
	h := newTestPanelMesa(t, roleAdmin, newFakeMesa(), newFakeAudit())
	corpo := abrirMesa(t, h, "?simular=1&zona=0&evolucao=2&mob_exp=1&mob_nivel=1&nivel=390").Body.String()
	if !strings.Contains(corpo, "não paga nada") {
		t.Error("a página não avisou que o monstro não paga nada para este nível")
	}
}

func TestAdminGravaUmaTabela(t *testing.T) {
	mesa := newFakeMesa()
	log := newFakeAudit()
	h := newTestPanelMesa(t, roleAdmin, mesa, log)
	post, token := signedInPost(t, h)

	rec := post("/auditoria/xp", url.Values{
		"csrf": {token}, "zona": {"1"}, "evolucao": {"2"}, "taxa": {"150"},
		"corte_nivel":   {"200", "", "acima"},
		"corte_divisor": {"1,5", "", "4"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}

	regra := mesa.regras[[2]int32{1, 2}]
	if regra.RatePercent != 150 {
		t.Errorf("taxa = %d, quero 150", regra.RatePercent)
	}
	if len(regra.Cuts) != 2 {
		t.Fatalf("gravou %d cortes, quero 2 (a linha vazia é descartada)", len(regra.Cuts))
	}
	if regra.Cuts[0] != (domain.XPCut{UpTo: 200, Divisor: 1.5}) {
		t.Errorf("primeiro corte = %+v", regra.Cuts[0])
	}
	if regra.Cuts[1].UpTo != level.CutOpenEnded || regra.Cuts[1].Divisor != 4 {
		t.Errorf("o corte aberto ficou %+v", regra.Cuts[1])
	}

	recs := log.recorded()
	if len(recs) != 1 || recs[0].Action != audit.ActionSetXPRule {
		t.Fatalf("auditoria = %+v", recs)
	}
	novo, _ := recs[0].New.(map[string]any)
	if novo["zona"] != "Pesadelo Arcano" || novo["evolucao"] != "Mortal" {
		t.Errorf("a auditoria guardou %+v — deveria ler como palavras", novo)
	}
}

// TestTabelaVaziaNaoViraTabelaDoLegado is the distinction the whole storage
// model turns on: saving a form with no cuts means "esta tabela não divide
// nada", not "volte ao legado".
func TestTabelaVaziaNaoViraTabelaDoLegado(t *testing.T) {
	mesa := newFakeMesa()
	h := newTestPanelMesa(t, roleAdmin, mesa, newFakeAudit())
	post, token := signedInPost(t, h)

	if rec := post("/auditoria/xp", url.Values{
		"csrf": {token}, "zona": {"0"}, "evolucao": {"2"}, "taxa": {"100"},
		"corte_nivel": {"", "", ""}, "corte_divisor": {"", "", ""},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	regra := mesa.regras[[2]int32{0, 2}]
	if regra.Cuts == nil {
		t.Fatal("gravou nil, que quer dizer «use a tabela do legado»")
	}
	if len(regra.Cuts) != 0 {
		t.Fatalf("gravou %d cortes, quero nenhum", len(regra.Cuts))
	}
}

func TestDivisorInvalidoERecusado(t *testing.T) {
	mesa := newFakeMesa()
	h := newTestPanelMesa(t, roleAdmin, mesa, newFakeAudit())
	post, token := signedInPost(t, h)

	for _, caso := range []struct{ nome, nivel, divisor string }{
		{"divisor zero", "200", "0"},
		{"divisor negativo", "200", "-2"},
		{"divisor em branco", "200", ""},
		{"nível não numérico", "abacaxi", "2"},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			rec := post("/auditoria/xp", url.Values{
				"csrf": {token}, "zona": {"0"}, "evolucao": {"2"}, "taxa": {"100"},
				"corte_nivel": {caso.nivel}, "corte_divisor": {caso.divisor},
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, quero 400", rec.Code)
			}
		})
	}
	if len(mesa.regras) != 0 {
		t.Fatalf("gravou %d regras apesar dos erros", len(mesa.regras))
	}
}

func TestLimparVoltaAoLegadoEAudita(t *testing.T) {
	mesa := newFakeMesa()
	mesa.regras[[2]int32{0, 2}] = domain.XPRule{Zone: 0, Tier: 2, RatePercent: 300, Cuts: []domain.XPCut{}}
	log := newFakeAudit()
	h := newTestPanelMesa(t, roleAdmin, mesa, log)
	post, token := signedInPost(t, h)

	if rec := post("/auditoria/xp/limpar", url.Values{
		"csrf": {token}, "zona": {"0"}, "evolucao": {"2"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if _, ainda := mesa.regras[[2]int32{0, 2}]; ainda {
		t.Error("a regra continua gravada")
	}
	recs := log.recorded()
	if len(recs) != 1 || recs[0].Action != audit.ActionClearXPRule {
		t.Fatalf("auditoria = %+v", recs)
	}
}

// TestModeradorNaoMexeNaMesa: these tables govern every player's progress, so
// they sit behind the same door as the audit log they live under.
func TestModeradorNaoMexeNaMesa(t *testing.T) {
	mesa := newFakeMesa()
	h := newTestPanelMesa(t, roleModerator, mesa, newFakeAudit())
	post, token := signedInPost(t, h)

	if rec := post("/auditoria/xp", url.Values{
		"csrf": {token}, "zona": {"0"}, "evolucao": {"2"}, "taxa": {"999"},
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, quero 403", rec.Code)
	}
	if len(mesa.regras) != 0 {
		t.Fatal("um moderador conseguiu gravar")
	}
}

// TestSemMesaNaoHaRota: the panel has to run against a deployment without the
// migration, and the route simply not existing is safer than a page that errors.
func TestSemMesaNaoHaRota(t *testing.T) {
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Sessions: session.New(time.Hour),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := abrirMesa(t, h.Routes(), "")
	if rec.Code == http.StatusOK {
		t.Fatal("a rota respondeu sem a Mesa configurada")
	}
}

// TestZonaInvalidaERecusada guards the one place a number from the browser
// indexes a table.
func TestZonaInvalidaERecusada(t *testing.T) {
	mesa := newFakeMesa()
	h := newTestPanelMesa(t, roleAdmin, mesa, newFakeAudit())
	post, token := signedInPost(t, h)

	for _, caso := range []url.Values{
		{"csrf": {token}, "zona": {"99"}, "evolucao": {"2"}, "taxa": {"100"}},
		{"csrf": {token}, "zona": {"-1"}, "evolucao": {"2"}, "taxa": {"100"}},
		{"csrf": {token}, "zona": {"0"}, "evolucao": {"9"}, "taxa": {"100"}},
	} {
		if rec := post("/auditoria/xp", caso); rec.Code != http.StatusBadRequest {
			t.Errorf("zona=%s evolucao=%s: status = %d, quero 400",
				caso.Get("zona"), caso.Get("evolucao"), rec.Code)
		}
	}
	// The page itself must survive a bad query string rather than panic on it.
	if rec := abrirMesa(t, h, "?zona=99&evolucao=abacaxi"); rec.Code != http.StatusOK {
		t.Errorf("a página caiu com uma query inválida: %d", rec.Code)
	}
}
