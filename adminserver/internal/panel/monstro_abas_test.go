package panel

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
)

// Every field lands on exactly one tab. A field with no tab would simply stop
// being editable — it would render on no page at all, and nobody would notice
// until they went looking for it.
func TestTodoCampoTemAba(t *testing.T) {
	validas := map[string]bool{}
	for _, a := range gamedata.AbasMob() {
		validas[a.ID] = true
	}
	campos := gamedata.NewMobStat("x", false).Fields()
	if len(campos) == 0 {
		t.Fatal("nenhum campo — o teste não estaria verificando nada")
	}
	vistas := map[string]int{}
	for _, c := range campos {
		aba := c.Aba()
		if !validas[aba] {
			t.Errorf("campo %q caiu na aba %q, que não existe", c.Nome, aba)
		}
		vistas[aba]++
	}
	// Equipment has its own tab with no numeric fields; every other tab must
	// actually have something on it.
	for _, a := range gamedata.AbasMob() {
		if a.ID == gamedata.AbaEquipamento {
			continue
		}
		if vistas[a.ID] == 0 {
			t.Errorf("a aba %q ficou vazia", a.ID)
		}
	}
}

// Nothing is marked as changed when there is no file to compare against —
// otherwise a template with no override would render every field in gold.
func TestSemArquivoNadaEhAlterado(t *testing.T) {
	for _, c := range gamedata.NewMobStat("x", false).Fields() {
		if c.Alterado() {
			t.Errorf("campo %q apareceu como alterado sem ter com o que comparar", c.Nome)
		}
	}
}

func TestMontaAbas(t *testing.T) {
	campos := []gamedata.MobField{
		{Nome: "damage", Grupo: "Combate", Valor: 645, Arquivo: 430, Comparavel: true},
		{Nome: "ac", Grupo: "Combate", Valor: 1820, Arquivo: 1820, Comparavel: true},
		{Nome: "max_hp", Grupo: "Vida", Valor: 14000, Arquivo: 9800, Comparavel: true},
		{Nome: "clan", Grupo: "Identidade", Valor: 4, Arquivo: 4, Comparavel: true},
	}
	abas := montaAbas(campos, gamedata.AbaVida)

	if abas.Atual != gamedata.AbaVida {
		t.Errorf("aba atual = %q, want vida", abas.Atual)
	}
	if abas.Alterados != 2 {
		t.Errorf("total de alterados = %d, want 2", abas.Alterados)
	}
	porID := map[string]abaMob{}
	for _, a := range abas.Itens {
		porID[a.ID] = a
	}
	if porID[gamedata.AbaCombate].Alterados != 1 {
		t.Errorf("combate = %d alterados, want 1", porID[gamedata.AbaCombate].Alterados)
	}
	if porID[gamedata.AbaVida].Alterados != 1 {
		t.Errorf("vida = %d alterados, want 1", porID[gamedata.AbaVida].Alterados)
	}
	if porID[gamedata.AbaAvancado].Alterados != 0 {
		t.Errorf("avançado = %d alterados, want 0 (o clã está igual ao arquivo)", porID[gamedata.AbaAvancado].Alterados)
	}
	if !porID[gamedata.AbaVida].Atual {
		t.Error("a aba vida deveria estar marcada como atual")
	}
}

// An unknown or missing tab falls back to the first one. Rendering an empty page
// because of a typo in the URL is worse than showing something.
func TestAbaDesconhecidaCaiNaPrimeira(t *testing.T) {
	for _, entrada := range []string{"", "inventada", "COMBATE"} {
		if got := montaAbas(nil, entrada).Atual; got != gamedata.AbaCombate {
			t.Errorf("aba %q resolveu para %q, want combate", entrada, got)
		}
	}
}

// The summary is what stays put while the tabs change, so it has to carry the
// figures somebody balances against — and flag the ones already edited.
func TestResumoTrazOsNumerosQueImportam(t *testing.T) {
	campos := []gamedata.MobField{
		{Nome: "level", Valor: 399, Arquivo: 399, Comparavel: true},
		{Nome: "max_hp", Valor: 14000, Arquivo: 9800, Comparavel: true},
		{Nome: "damage", Valor: 645, Arquivo: 430, Comparavel: true},
		{Nome: "ac", Valor: 1820, Arquivo: 1820, Comparavel: true},
		{Nome: "exp", Valor: 48000, Arquivo: 32000, Comparavel: true},
		{Nome: "coin", Valor: 12400, Arquivo: 12400, Comparavel: true},
		{Nome: "resist1", Valor: 40, Arquivo: 40, Comparavel: true},
	}
	r := resumoDe(campos)
	if len(r) != 6 {
		t.Fatalf("resumo com %d itens, want 6 (resistência não entra)", len(r))
	}
	porRotulo := map[string]resumoCampo{}
	for _, c := range r {
		porRotulo[c.Rotulo] = c
	}
	if hp := porRotulo["HP"]; !hp.Alterado || hp.Valor != 14000 || hp.Arquivo != 9800 {
		t.Errorf("HP = %+v, want 14000 alterado de 9800", hp)
	}
	if nivel := porRotulo["Nível"]; nivel.Alterado {
		t.Error("nível igual ao arquivo não deveria aparecer como alterado")
	}
}

// Saving from one tab must not touch another. The form only carries the fields
// of the visible tab, and setMonstro treats an absent field as "leave it" — this
// is what makes per-tab editing safe, and it is the failure that would silently
// zero half a monster.
func TestGravarUmaAbaNaoMexeNasOutras(t *testing.T) {
	game := newFakeGameData()
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	// Only the Vida tab's fields go up, exactly as the rendered form would send.
	rec := post("/monstros/Kentania", url.Values{
		"csrf": {token}, "aba": {gamedata.AbaVida}, "max_hp": {"14000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(game.mobSaved) != 1 {
		t.Fatalf("saves = %d, want 1", len(game.mobSaved))
	}
	got := game.mobSaved[0]
	if v := campoDe(t, got, "max_hp"); v != 14000 {
		t.Errorf("max_hp = %d, want 14000", v)
	}
	// A Combate field the form never carried keeps whatever it had.
	if v := campoDe(t, got, "level"); v == 0 {
		t.Error("level foi zerado por um save que nem carregava esse campo")
	}
}

// The chosen tab rides through the save, so the operator lands back where the
// work was instead of being bounced to Combate on every write.
func TestSalvarVoltaParaAMesmaAba(t *testing.T) {
	game := newFakeGameData()
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	rec := post("/monstros/Kentania", url.Values{
		"csrf": {token}, "aba": {gamedata.AbaRecompensa}, "coin": {"500"},
	})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "aba=recompensa") {
		t.Errorf("redirect = %q, esperava voltar para a aba recompensa", loc)
	}
}

func TestAbaInvalidaNaoEntraNoRedirect(t *testing.T) {
	game := newFakeGameData()
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	rec := post("/monstros/Kentania", url.Values{
		"csrf": {token}, "aba": {"../../etc"}, "coin": {"500"},
	})
	if loc := rec.Header().Get("Location"); strings.Contains(loc, "etc") {
		t.Errorf("redirect = %q, uma aba inventada não deveria entrar na URL", loc)
	}
}
