package panel

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
)

type fakeReceitas struct {
	mu       sync.Mutex
	rows     map[int32]domain.GeneratorRecipe
	renovou  []int32
	apagados []int32
}

func newFakeReceitas(rows ...domain.GeneratorRecipe) *fakeReceitas {
	f := &fakeReceitas{rows: map[int32]domain.GeneratorRecipe{}}
	for _, r := range rows {
		f.rows[r.Index] = r
	}
	return f
}

func (f *fakeReceitas) GeneratorRecipes(context.Context) (domain.GeneratorRecipeConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var cfg domain.GeneratorRecipeConfig
	for _, r := range f.rows {
		cfg.Recipes = append(cfg.Recipes, r)
	}
	return cfg, nil
}

func (f *fakeReceitas) SetGeneratorRecipe(_ context.Context, rec domain.GeneratorRecipe, renovar bool, _ int64) (domain.GeneratorRecipe, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	antes, tinha := f.rows[rec.Index]
	f.rows[rec.Index] = rec
	if renovar {
		f.renovou = append(f.renovou, rec.Index)
	}
	return antes, tinha, nil
}

func (f *fakeReceitas) DeleteGeneratorRecipe(_ context.Context, idx int32, _ int64) (domain.GeneratorRecipe, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	antes, tinha := f.rows[idx]
	delete(f.rows, idx)
	f.apagados = append(f.apagados, idx)
	return antes, tinha, nil
}

// arquivoDeTeste is a three-block NPCGener: 0 and 1 are wolves, 2 a spider.
func arquivoDeTeste() []npcgener.Generator {
	lobo := npcgener.Generator{Leader: "Lobo", Follower: "Filhote", MinGroup: 1, MaxGroup: 2, MaxNumMob: 6,
		MinuteGenerate: 2, SegX: [5]int16{2100, 0, 0, 0, 2110}, SegY: [5]int16{2100, 0, 0, 0, 2090},
		SegRange: [5]int{3}}
	aranha := npcgener.Generator{Leader: "Aranha", MaxNumMob: 4, SegX: [5]int16{900}, SegY: [5]int16{900}}
	return []npcgener.Generator{lobo, lobo, aranha}
}

// moldesConhecidos is the template check with the names the tests use.
func moldesConhecidos(name string) (string, bool) {
	switch name {
	case "Lobo", "Filhote", "Aranha", "Urso", "Esquisito":
		return name, true
	}
	return "", false
}

// corposConhecidos is the body check: Esquisito exists in npc/ but wears a body
// no monster of the file wears.
func corposConhecidos(name string) (int16, bool) {
	if name == "Esquisito" {
		return 777, false
	}
	return 12, true
}

func newTestPanelReceitas(t *testing.T, cargo string, rc Receitas, log AuditLog) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(cargo), Writer: newFakeWriter(), Audit: log,
		Receitas: rc, NPCGener: arquivoDeTeste(), MoldeExiste: moldesConhecidos,
		CorpoConhecido: corposConhecidos,
		Sessions:       session.New(time.Hour),
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// receitaValida is a whole form for block idx: a bear at (3000,3000).
func receitaValida(token string, idx string) url.Values {
	v := url.Values{
		"csrf": {token}, "bloco": {idx}, "leader": {"Urso"}, "follower": {""},
		"vivos": {"3"}, "min_grupo": {"0"}, "max_grupo": {"0"}, "minuto": {"0"},
		"rota": {"0"}, "formacao": {"0"}, "nota": {"zona nova do norte"},
	}
	for _, i := range []string{"0", "1", "2", "3", "4"} {
		v.Set("x"+i, "0")
		v.Set("y"+i, "0")
		v.Set("raio"+i, "0")
		v.Set("espera"+i, "0")
	}
	v.Set("x0", "3000")
	v.Set("y0", "3000")
	v.Set("raio0", "4")
	return v
}

// The form of a block of the file starts from the file, and shows the file's
// value beside each field.
func TestReceitaDoArquivoPreencheOFormulario(t *testing.T) {
	get := signedIn(t, newTestPanelReceitas(t, roleAdmin, newFakeReceitas(), newFakeAudit()))
	rec := get("/blocos/receita?bloco=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, primeiraLinha(rec.Body.String()))
	}
	corpo := rec.Body.String()
	for _, quero := range []string{`name="leader" value="Aranha"`, `name="vivos" inputmode="numeric" value="4"`,
		`name="x0" inputmode="numeric" value="900"`, "como no arquivo", "arquivo: Aranha"} {
		if !strings.Contains(corpo, quero) {
			t.Errorf("a página não traz %q", quero)
		}
	}
}

// Between the file's end and 20000 is where the file grows; neither the page nor
// the save accepts an index there.
func TestReceitaRecusaIndiceNoVao(t *testing.T) {
	rc := newFakeReceitas()
	h := newTestPanelReceitas(t, roleAdmin, rc, newFakeAudit())
	if rec := signedIn(t, h)("/blocos/receita?bloco=500"); rec.Code != http.StatusNotFound {
		t.Errorf("GET no vão = %d, quero 404", rec.Code)
	}
	post, token := signedInPost(t, h)
	if rec := post("/blocos/receita", receitaValida(token, "500")); rec.Code != http.StatusBadRequest {
		t.Errorf("POST no vão = %d, quero 400", rec.Code)
	}
	if len(rc.rows) != 0 {
		t.Error("gravou uma receita no vão")
	}
}

// A new block starts at 20000, then takes the next free one; &de= starts it as
// a copy of another block.
func TestBlocoNovoPegaOProximoLivre(t *testing.T) {
	rc := newFakeReceitas(domain.GeneratorRecipe{Index: domain.NewGeneratorIndexBase, Leader: "Urso", SegX: [5]int32{1}, SegY: [5]int32{1}})
	get := signedIn(t, newTestPanelReceitas(t, roleAdmin, rc, newFakeAudit()))
	corpo := get("/blocos/receita?novo=1&de=2").Body.String()
	if !strings.Contains(corpo, "Bloco #20001") {
		t.Errorf("o bloco novo não é o 20001:\n%s", primeiraLinha(corpo))
	}
	if !strings.Contains(corpo, `name="leader" value="Aranha"`) {
		t.Error("&de=2 não copiou a Aranha do bloco 2")
	}
}

func TestGravarReceitaEAuditar(t *testing.T) {
	rc := newFakeReceitas()
	log := newFakeAudit()
	h := newTestPanelReceitas(t, roleAdmin, rc, log)
	post, token := signedInPost(t, h)

	form := receitaValida(token, "1")
	form.Set("renovar", "1")
	rec := post("/blocos/receita", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/blocos/receita?") || !strings.Contains(loc, "bloco=1") {
		t.Errorf("voltou para %q", loc)
	}
	got, ok := rc.rows[1]
	if !ok || got.Leader != "Urso" || got.MaxNumMob != 3 || got.SegX[0] != 3000 || got.SegRange[0] != 4 || got.Nota != "zona nova do norte" {
		t.Fatalf("gravou %+v", got)
	}
	if len(rc.renovou) != 1 {
		t.Error("o \"trocar os vivos agora\" não chegou ao banco")
	}
	recs := log.recorded()
	if len(recs) != 1 || recs[0].Action != audit.ActionSetBlockRecipe {
		t.Fatalf("auditoria = %+v", recs)
	}
	// With no row before, the log says what the FILE had, not an empty recipe.
	antes, _ := recs[0].Old.(map[string]any)
	if antes["lider"] != "Lobo" || antes["origem"] != "NPCGener.txt" {
		t.Errorf("o antes da auditoria ficou %+v", antes)
	}
}

// Each refusal names the field, and nothing reaches the database.
func TestReceitaInvalidaERecusada(t *testing.T) {
	rc := newFakeReceitas()
	h := newTestPanelReceitas(t, roleAdmin, rc, newFakeAudit())
	post, token := signedInPost(t, h)

	casos := []struct {
		nome, campo, valor, quero string
	}{
		{"sem líder", "leader", "", "líder"},
		{"molde que não existe", "leader", "Ursso", "Não existe molde"},
		{"seguidor que não existe", "follower", "Filhotte", "Não existe molde"},
		{"líder de corpo desconhecido", "leader", "Esquisito", "corpo 777"},
		{"seguidor de corpo desconhecido", "follower", "Esquisito", "fechar o cliente"},
		{"nome com barra", "leader", "../Urso", "sem espaços nem barras"},
		{"sem início", "x0", "0", "Início"},
		{"coordenada fora do mapa", "y0", "5000", "Y de Início"},
		{"teto absurdo", "vivos", "5000", "teto de vivos"},
		{"formação que não existe", "formacao", "7", "formação"},
		{"número que não é número", "minuto", "dois", "ritmo"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			form := receitaValida(token, "1")
			form.Set(c.campo, c.valor)
			rec := post("/blocos/receita", form)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, quero 400", rec.Code)
			}
			if !strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(c.quero)) {
				t.Errorf("a recusa não fala de %q: %s", c.quero, rec.Body.String())
			}
		})
	}
	if len(rc.rows) != 0 {
		t.Fatalf("gravou %d receitas apesar dos erros", len(rc.rows))
	}
}

// "0" is how the file writes "no follower"; the form accepts it the same way.
func TestSeguidorZeroESemSeguidor(t *testing.T) {
	rc := newFakeReceitas()
	post, token := signedInPost(t, newTestPanelReceitas(t, roleAdmin, rc, newFakeAudit()))
	form := receitaValida(token, "1")
	form.Set("follower", "0")
	if rec := post("/blocos/receita", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if rc.rows[1].Follower != "" {
		t.Errorf("seguidor gravado %q, quero vazio", rc.rows[1].Follower)
	}
}

func TestApagarReceitaDeBlocoNovoVoltaParaALista(t *testing.T) {
	novo := domain.GeneratorRecipe{Index: domain.NewGeneratorIndexBase, Leader: "Urso", SegX: [5]int32{1}, SegY: [5]int32{1}}
	rc := newFakeReceitas(novo)
	log := newFakeAudit()
	post, token := signedInPost(t, newTestPanelReceitas(t, roleAdmin, rc, log))
	rec := post("/blocos/receita/limpar", url.Values{"csrf": {token}, "bloco": {"20000"}})
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/blocos/receitas?") {
		t.Fatalf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	if _, ainda := rc.rows[domain.NewGeneratorIndexBase]; ainda {
		t.Error("a receita continua gravada")
	}
	if recs := log.recorded(); len(recs) != 1 || recs[0].Action != audit.ActionClearBlockRecipe {
		t.Fatalf("auditoria = %+v", recs)
	}
}

func TestModeradorVeMasNaoGravaReceita(t *testing.T) {
	rc := newFakeReceitas()
	h := newTestPanelReceitas(t, roleModerator, rc, newFakeAudit())
	if rec := signedIn(t, h)("/blocos/receita?bloco=1"); rec.Code != http.StatusOK {
		t.Errorf("moderador abrindo a receita = %d, quero 200", rec.Code)
	}
	post, token := signedInPost(t, h)
	if rec := post("/blocos/receita", receitaValida(token, "1")); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, quero 403", rec.Code)
	}
	if len(rc.rows) != 0 {
		t.Fatal("um moderador gravou uma receita")
	}
}

func TestListaDeReceitasMostraOQueMudou(t *testing.T) {
	rc := newFakeReceitas(domain.GeneratorRecipe{Index: 2, Leader: "Urso", MaxNumMob: 3,
		SegX: [5]int32{950}, SegY: [5]int32{950}, Nota: "trocada em 25/09"})
	corpo := signedIn(t, newTestPanelReceitas(t, roleAdmin, rc, newFakeAudit()))("/blocos/receitas").Body.String()
	for _, quero := range []string{"#2", "Urso", "no arquivo: Aranha", "trocada em 25/09"} {
		if !strings.Contains(corpo, quero) {
			t.Errorf("a lista não traz %q", quero)
		}
	}
}
