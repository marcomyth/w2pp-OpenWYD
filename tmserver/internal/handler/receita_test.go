package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mobstat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// fakeRecipes is the database side of the block recipes.
type fakeRecipes struct {
	mu  sync.Mutex
	cfg domain.GeneratorRecipeConfig
}

func (f *fakeRecipes) Version(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg.Version, nil
}

func (f *fakeRecipes) Snapshot(context.Context) (domain.GeneratorRecipeConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return domain.GeneratorRecipeConfig{Version: f.cfg.Version,
		Recipes: append([]domain.GeneratorRecipe(nil), f.cfg.Recipes...)}, nil
}

// moldesDeTeste resolves the names a test knows, each to its own file name.
func moldesDeTeste(nomes ...string) RecipeTemplateLoader {
	conhecidos := map[string]bool{}
	for _, n := range nomes {
		conhecidos[n] = true
	}
	return func(name string) ([]byte, string, error) {
		if !conhecidos[name] {
			return nil, "", errors.New("npctemplate: no such file")
		}
		return plainMobTemplate(name), name, nil
	}
}

// Block 8 is the Lobo at (8,8): 0-7 are Coliseu blocks, event-owned, which a
// renew leaves to the event (the same layout the /gm npc tests use).
const blocoDoLobo = 8

// mundoComLobo builds the file (eight Coliseu placeholders and the Lobo), the
// world raised from it, and a dispatcher that reads recipes from src.
func mundoComLobo(t *testing.T, src GeneratorRecipeSource) (*Dispatcher, *world.World) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Recipes: src, RecipeTemplate: moldesDeTeste("Lobo", "Urso", "Filhote")})
	w := world.New(world.Config{GridDim: 64}, log, world.NopPersistence{}, d.Handle)

	base := make([]npcgener.Generator, blocoDoLobo+1)
	for i := range base {
		base[i] = npcgener.Generator{Leader: "Coliseu", MaxNumMob: 1, SegX: [5]int16{2}, SegY: [5]int16{2}}
	}
	base[blocoDoLobo] = npcgener.Generator{Leader: "Lobo", MaxNumMob: 1, SegX: [5]int16{8}, SegY: [5]int16{8}}
	gens := make([]*world.Generator, blocoDoLobo+1)
	gens[blocoDoLobo] = &world.Generator{Name: "Lobo", MaxNumMob: 1, SegX: [5]int16{8}, SegY: [5]int16{8},
		LeaderTmpl: plainMobTemplate("Lobo"), LeaderName: "Lobo"}
	w.RegisterGenerators(gens)
	w.GenerateMob(blocoDoLobo)
	d.SetRecipeBase(base)
	return d, w
}

// ursoEm is a whole recipe: one bear at (x,y).
func ursoEm(idx int32, x, y int32) domain.GeneratorRecipe {
	return domain.GeneratorRecipe{Index: idx, Leader: "Urso", MaxNumMob: 1,
		SegX: [5]int32{x}, SegY: [5]int32{y}}
}

// vivosDo lists the live mobs of one block.
func vivosDo(w *world.World, idx int) []*world.Entity {
	var out []*world.Entity
	w.ForEachMob(func(_ int, m *world.Entity) {
		if int(m.GenIndex) == idx {
			out = append(out, m)
		}
	})
	return out
}

// aplicarAgora runs what the poll does once the snapshot is in, without the
// running loop: read the templates, then apply.
func aplicarAgora(d *Dispatcher, w *world.World, cfg domain.GeneratorRecipeConfig) int {
	return d.applyGeneratorRecipes(w, cfg, lerMoldes(d.recipeTemplate, nomesAResolver(cfg, d.recipeBase, d.recipeApplied)), false)
}

// At boot the recipe replaces what the populate raised from the file: a zone
// the panel changed must not open the day with the old monsters in it.
func TestReceitaNoBootTrocaOsVivos(t *testing.T) {
	src := &fakeRecipes{cfg: domain.GeneratorRecipeConfig{Version: 3,
		Recipes: []domain.GeneratorRecipe{ursoEm(blocoDoLobo, 30, 30)}}}
	d, w := mundoComLobo(t, src)

	d.ApplyGeneratorRecipesBoot(w)

	g := w.GeneratorAt(blocoDoLobo)
	if g.Name != "Urso" || g.LeaderName != "Urso" || g.SegX[0] != 30 || g.Rev != 1 {
		t.Fatalf("bloco = %q/%q em %d, Rev %d; quero o Urso em 30, Rev 1", g.Name, g.LeaderName, g.SegX[0], g.Rev)
	}
	vivos := vivosDo(w, blocoDoLobo)
	if len(vivos) != 1 || vivos[0].TemplateName != "Urso" {
		t.Fatalf("vivos do bloco = %d (%v), quero só o Urso", len(vivos), nomes(vivos))
	}
	if d.recipeVersion != 3 {
		t.Errorf("versão aplicada = %d, quero 3", d.recipeVersion)
	}
}

// While the server runs, a change reaches the next ones to be born: the wolf
// already on the map stays until it dies. Asking to renew replaces it now.
func TestReceitaAoVivoEsperaAMorteOuORenovar(t *testing.T) {
	src := &fakeRecipes{}
	d, w := mundoComLobo(t, src)
	d.ApplyGeneratorRecipesBoot(w)

	cfg := domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{ursoEm(blocoDoLobo, 30, 30)}}
	if n := aplicarAgora(d, w, cfg); n != 1 {
		t.Fatalf("trocou %d blocos, quero 1", n)
	}
	if g := w.GeneratorAt(blocoDoLobo); g.Name != "Urso" {
		t.Fatalf("a receita não foi para o bloco: %q", g.Name)
	}
	if vivos := vivosDo(w, blocoDoLobo); len(vivos) != 1 || vivos[0].TemplateName != "Lobo" {
		t.Fatalf("vivos = %v; quero o Lobo de antes ainda de pé", nomes(vivos))
	}

	// Same recipe, renovar bumped: the panel's "trocar os vivos agora".
	cfg.Version, cfg.Recipes[0].Renovar = 2, 1
	aplicarAgora(d, w, cfg)
	if vivos := vivosDo(w, blocoDoLobo); len(vivos) != 1 || vivos[0].TemplateName != "Urso" {
		t.Fatalf("depois de renovar, vivos = %v; quero só o Urso", nomes(vivos))
	}
	if g := w.GeneratorAt(blocoDoLobo); g.Rev != 1 {
		t.Errorf("renovar sem mudar a receita moveu o Rev para %d", g.Rev)
	}
}

// A block the panel created lives at 20000 and up: it is raised as soon as it
// arrives, and deleting its row takes it out of the world for good.
func TestBlocoNovoNasceEVaiEmbora(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	const novo = domain.NewGeneratorIndexBase

	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{ursoEm(novo, 40, 40)}})
	if vivos := vivosDo(w, novo); len(vivos) != 1 || vivos[0].TemplateName != "Urso" {
		t.Fatalf("bloco novo: vivos = %v, quero um Urso", nomes(vivos))
	}

	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 2})
	if vivos := vivosDo(w, novo); len(vivos) != 0 {
		t.Fatalf("linha apagada, e ainda há %d vivos do bloco", len(vivos))
	}
	if ids := w.GenerateMob(novo); len(ids) != 0 {
		t.Error("um bloco que saiu do banco ainda gera")
	}
	if _, ficou := d.recipeApplied[novo]; ficou {
		t.Error("o bloco retirado continua anotado como aplicado")
	}
}

// Deleting the row of a block of the file gives it back to the file.
func TestApagarALinhaVoltaAoArquivo(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{ursoEm(blocoDoLobo, 30, 30)}})
	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 2})

	g := w.GeneratorAt(blocoDoLobo)
	if g.Name != "Lobo" || g.LeaderName != "Lobo" || g.SegX[0] != 8 {
		t.Errorf("bloco = %q em %d; quero o Lobo do arquivo em 8", g.Name, g.SegX[0])
	}
	if g.Rev != 2 {
		t.Errorf("Rev = %d; quero 2 (a ida e a volta)", g.Rev)
	}
}

// A name that does not resolve keeps the block as it was: a zone emptied by a
// typo is worse than one that did not change.
func TestMoldeQueNaoCarregaMantemOBloco(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	rec := ursoEm(blocoDoLobo, 30, 30)
	rec.Leader = "Urso_Que_Nao_Existe"
	if n := aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{rec}}); n != 0 {
		t.Fatalf("trocou %d blocos com um molde que não existe", n)
	}
	if g := w.GeneratorAt(blocoDoLobo); g.Name != "Lobo" || g.Rev != 0 || g.LeaderTmpl == nil {
		t.Errorf("bloco = %q, Rev %d; quero o Lobo intocado", g.Name, g.Rev)
	}
}

// A missing follower degrades the block to leader-only groups, as at boot.
func TestSeguidorQueNaoCarregaGeraSoOLider(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	rec := ursoEm(blocoDoLobo, 30, 30)
	rec.Follower = "Sumido"
	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{rec}})
	if g := w.GeneratorAt(blocoDoLobo); g.Name != "Urso" || g.FollowerTmpl != nil {
		t.Errorf("bloco = %q, seguidor %v; quero o Urso sozinho", g.Name, g.FollowerTmpl != nil)
	}
}

// Between the end of the file and 20000 is where the file will grow: a row there
// would be taken over by the next block somebody appends to the .txt.
func TestIndiceEntreOArquivoEOsNovosEIgnorado(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{ursoEm(500, 30, 30)}})
	if w.GeneratorAt(500) != nil {
		t.Error("uma linha no índice 500 criou um bloco")
	}
}

// A row equal to the file changes nothing — and must not move Rev, or every
// queued death of the block would go through a regeneration for no reason.
func TestLinhaIgualAoArquivoNaoMexe(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	igual := domain.GeneratorRecipe{Index: blocoDoLobo, Leader: "Lobo", MaxNumMob: 1,
		SegX: [5]int32{8}, SegY: [5]int32{8}}
	if n := aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{igual}}); n != 0 {
		t.Errorf("trocou %d blocos com uma linha igual ao arquivo", n)
	}
	if g := w.GeneratorAt(blocoDoLobo); g.Rev != 0 {
		t.Errorf("Rev = %d, quero 0", g.Rev)
	}
}

// The /monstros sheet goes on the recipe's template, as it does on the file's.
func TestReceitaLevaAFichaDoPainel(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	d.mobStatOverrides = map[string]mobstat.Override{"Urso": {Level: 77, MaxHp: 5000, Hp: 5000}}
	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{ursoEm(blocoDoLobo, 30, 30)}})
	if lvl := protocol.ParseMobBasics(w.GeneratorAt(blocoDoLobo).LeaderTmpl).Level; lvl != 77 {
		t.Errorf("nível no molde do bloco = %d, quero 77 (a ficha do painel)", lvl)
	}
}

// /gm renovar is the same renew, typed by a GM or sent by the panel.
func TestGMRenovar(t *testing.T) {
	d, w := mundoComLobo(t, &fakeRecipes{})
	aplicarAgora(d, w, domain.GeneratorRecipeConfig{Version: 1, Recipes: []domain.GeneratorRecipe{ursoEm(blocoDoLobo, 30, 30)}})

	out := d.RunBlockCommand(w, "gm", 0, 0, "renovar 8")
	if len(out) != 1 || !strings.Contains(out[0], "1 tirados, 1 gerados") {
		t.Fatalf("resposta = %v", out)
	}
	if vivos := vivosDo(w, blocoDoLobo); len(vivos) != 1 || vivos[0].TemplateName != "Urso" {
		t.Errorf("vivos = %v, quero o Urso", nomes(vivos))
	}

	w.GeneratorAt(blocoDoLobo).Off = true
	if out := d.RunBlockCommand(w, "gm", 0, 0, "renovar 8"); !strings.Contains(out[0], "desligado") {
		t.Errorf("bloco desligado: %v", out)
	}
	if out := d.RunBlockCommand(w, "gm", 0, 0, "renovar 9999"); !strings.Contains(out[0], "não existe") {
		t.Errorf("bloco inexistente: %v", out)
	}
}

func nomes(es []*world.Entity) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.TemplateName)
	}
	return out
}
