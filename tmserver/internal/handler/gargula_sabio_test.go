package handler

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const gargulaMigracao = "0151_gargula_sabio_guardas.up.sql"

// A Gárgula chefe volta 1 h depois da morte; os guardas seguem a fila comum.
func TestGargulaSabioRenasceEm1Hora(t *testing.T) {
	chefe := geradorSozinho(10_000, 10)
	chefe.LeaderName = gargulaSabioChefeTemplate
	guarda := geradorSozinho(10_000, 12)
	guarda.LeaderName = golemGuardaTemplate
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: chefe, 31: guarda})
	d := dispatcherQuieto()
	if got := d.esperaDoRenascimento(w, 30); got != msPorHora {
		t.Errorf("Gárgula chefe volta em %d ms, want %d", got, msPorHora)
	}
	if got := d.esperaDoRenascimento(w, 31); got != world.DefaultRespawnDelay {
		t.Errorf("guarda volta em %d ms, want os %d da fila", got, world.DefaultRespawnDelay)
	}
}

// Depois do boot a Gárgula chefe só aparece 1 h depois; os guardas ficam de pé.
func TestGargulaSabioNasceUmaHoraDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	bloco := func(nome string, x int16) *world.Generator {
		g := geradorSozinho(10_000, x)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 10_000, 0)
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: bloco(gargulaSabioChefeTemplate, 10), 31: bloco(golemGuardaTemplate, 20)})
	for _, idx := range []int{30, 31} {
		if len(w.GenerateMob(idx)) != 1 {
			t.Fatalf("o boot não levantou o bloco %d", idx)
		}
	}
	dispatcherQuieto().ApplyGargulaSabioBoot(w)
	if doBloco(w, 30) != nil || doBloco(w, 31) == nil {
		t.Fatal("depois do boot: want só o guarda de pé")
	}
	if got := w.SpawnDueRespawns(agora + msPorHora - 1); len(got) != 0 {
		t.Fatalf("a Gárgula voltou antes de 1 h: %v", got)
	}
	if got := w.SpawnDueRespawns(agora + msPorHora); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("1 h depois do boot voltaram %d, want a Gárgula de pé", len(got))
	}
}

// Cada morte da Gárgula chefe solta UMA Arma C com o add alto do Boss Conjurador;
// a Gárgula comum não solta nada disso.
func TestGargulaSabioSoltaArmaC(t *testing.T) {
	armas := slices.Concat(armasCFisicas, armasCMagicas)
	for i := range 20 {
		d, w, killer := mobKilledWorld(t)
		for range i { // cada mundo novo começa na mesma semente
			w.Rand().Intn(2)
		}
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(260, 0, 0), gargulaSabioChefeTemplate))
		var achou []world.Item
		for _, it := range killer.Carry {
			if slices.Contains(armas, it.Index) {
				achou = append(achou, it)
			}
		}
		if len(achou) != 1 {
			t.Fatalf("%d Armas C na bolsa, want 1", len(achou))
		}
		add := achou[0].Effects[1]
		fisica := slices.Contains(armasCFisicas, achou[0].Index)
		if (fisica && (add.Effect != efDamage || (add.Value != 72 && add.Value != 81))) ||
			(!fisica && (add.Effect != efMagic || (add.Value != 32 && add.Value != 36))) {
			t.Errorf("arma %d com add %+v, want o add alto do Boss Conjurador", achou[0].Index, add)
		}
	}
	d, w, killer := mobKilledWorld(t)
	d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(232, 0, 0), "Gargula_Sabio"))
	for _, it := range killer.Carry {
		if slices.Contains(armas, it.Index) {
			t.Fatalf("a Gárgula comum soltou a Arma C %d", it.Index)
		}
	}
}

// Os templates: cópias com o nome de dentro do original; a Gárgula entre a comum e
// os mini chefes da lava; os guardas acima do Golem de Fogo; Carry vazio nos dois.
func TestGargulaSabioTemplates(t *testing.T) {
	root := releaseDir(t)
	ler := func(nome string) savefmt.Mob {
		b, _, err := npctemplate.Load(root, nome)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		m, _, err := savefmt.DecodeMobAny(b)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		return m
	}
	vidaReal := func(m savefmt.Mob) int64 {
		if m.Equip[13].Index == itemDivisorDobro {
			v := int64(m.Equip[13].Effects[0].Value)
			return int64(m.CurrentScore.MaxHp) * max(v, 2)
		}
		return int64(m.CurrentScore.MaxHp)
	}
	chefe, comum, mini := ler(gargulaSabioChefeTemplate), ler("Gargula_Sabio"), ler(bossGolemTemplate)
	guarda, golem := ler(golemGuardaTemplate), ler("Golem_de_Fogo")
	for _, par := range []struct {
		copia, original savefmt.Mob
		nome            string
	}{{chefe, comum, gargulaSabioChefeTemplate}, {guarda, golem, golemGuardaTemplate}} {
		if !bytes.Equal(par.copia.Name[:], par.original.Name[:]) {
			t.Errorf("%s: nome de dentro %q, want o do original %q", par.nome, par.copia.Name, par.original.Name)
		}
		if par.copia.Equip[0].Index != par.original.Equip[0].Index {
			t.Errorf("%s: corpo %d, want %d", par.nome, par.copia.Equip[0].Index, par.original.Equip[0].Index)
		}
		for i, it := range par.copia.Carry {
			if it.Index != 0 {
				t.Errorf("%s: Carry[%d] = %d, want vazio", par.nome, i, it.Index)
			}
		}
		if par.copia.Exp > 10_000 {
			t.Errorf("%s: XP %d, acima do teto de 10.000 da Dungeon (0091)", par.nome, par.copia.Exp)
		}
		if par.copia.BaseScore.MaxHp != par.copia.CurrentScore.MaxHp || par.copia.BaseScore.Damage != par.copia.CurrentScore.Damage {
			t.Errorf("%s: BaseScore e CurrentScore diferentes", par.nome)
		}
	}
	if vidaReal(comum) >= vidaReal(chefe) || vidaReal(chefe) >= vidaReal(mini) {
		t.Errorf("vida real: comum %d, chefe %d, mini chefe %d; want a chefe no meio", vidaReal(comum), vidaReal(chefe), vidaReal(mini))
	}
	if comum.CurrentScore.Damage >= chefe.CurrentScore.Damage || chefe.CurrentScore.Damage >= mini.CurrentScore.Damage {
		t.Errorf("dano: comum %d, chefe %d, mini chefe %d; want a chefe no meio", comum.CurrentScore.Damage, chefe.CurrentScore.Damage, mini.CurrentScore.Damage)
	}
	if vidaReal(guarda) <= vidaReal(golem) || guarda.CurrentScore.Damage <= golem.CurrentScore.Damage || guarda.CurrentScore.AC <= golem.CurrentScore.AC {
		t.Errorf("guarda com vida %d, dano %d, defesa %d; want acima do Golem de Fogo (%d, %d, %d)",
			vidaReal(guarda), guarda.CurrentScore.Damage, guarda.CurrentScore.AC,
			vidaReal(golem), golem.CurrentScore.Damage, golem.CurrentScore.AC)
	}
}

// A 0151 dá aos guardas as Poeiras, as dez Jóias e a Moeda de 1Mi, e só isso; os
// itens existem no ItemList.
func TestGargulaSabioMigracaoGuardas(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, gargulaMigracao)
	if len(linhas) != 1 {
		t.Fatalf("a 0151 cita %d monstros, want só o Golem_Guarda", len(linhas))
	}
	want := []int16{412, 413, 4026}
	for j := int16(3200); j <= 3209; j++ {
		want = append(want, j)
	}
	porItem := linhas[golemGuardaTemplate]
	if len(porItem) != len(want) {
		t.Errorf("%d itens no guarda, want %d", len(porItem), len(want))
	}
	for _, item := range want {
		if c := porItem[item]; c <= 0 {
			t.Errorf("item %d a %d, want > 0", item, c)
		}
		if _, ok := items.Get(int(item)); !ok {
			t.Errorf("item %d não existe no ItemList", item)
		}
	}
}
