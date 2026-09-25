package handler

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const caveirasMigracao = "0144_dungeon_caveiras_restos_e_armas_c.up.sql"

// O bloco do Boss Conjurador volta 3 h depois da morte; os da lava seguem em 2 h.
func TestBossConjuradorRenasceEm3Horas(t *testing.T) {
	boss := geradorSozinho(10_000, 10)
	boss.LeaderName = bossConjuradorTemplate
	lava := geradorSozinho(10_000, 12)
	lava.LeaderName = bossGolemTemplate
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{30: boss, 31: lava})
	d := dispatcherQuieto()
	if got := d.esperaDoRenascimento(w, 30); got != 3*msPorHora {
		t.Errorf("Boss Conjurador volta em %d ms, want 3 h", got)
	}
	if got := d.esperaDoRenascimento(w, 31); got != 2*msPorHora {
		t.Errorf("Boss Golem volta em %d ms, want as 2 h dele", got)
	}
}

// Depois do boot o chefe só aparece 3 h depois, e o monstro comum fica de pé.
func TestBossConjuradorNasceHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	w := mundoDaLava(t, &agora, bossConjuradorTemplate)
	dispatcherQuieto().ApplyBossConjuradorBoot(w)
	if doBloco(w, 30) != nil || doBloco(w, 31) == nil {
		t.Fatal("depois do boot: want só o monstro comum de pé")
	}
	tres := uint32(conjuradorHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + tres - 1); len(got) != 0 {
		t.Fatalf("o chefe voltou antes das 3 h: %v", got)
	}
	if got := w.SpawnDueRespawns(agora + tres); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("3 h depois do boot voltaram %d, want o chefe de pé", len(got))
	}
}

// Sem luta, o chefe some depois de 30 min e volta 3 h depois de ter nascido — o
// ciclo dele, não o de 2 h da lava.
func TestBossConjuradorSomeSemLuta(t *testing.T) {
	var agora uint32 = 1_000
	w := mundoDaLava(t, &agora, bossConjuradorTemplate)
	d := dispatcherQuieto()
	passadaDaLava(d, w)
	nasceu := agora

	agora = nasceu + lavaChefeSemLuta - 1
	passadaDaLava(d, w)
	if doBloco(w, 30) == nil {
		t.Fatal("o chefe sumiu antes dos 30 min")
	}
	agora = nasceu + lavaChefeSemLuta
	passadaDaLava(d, w)
	if doBloco(w, 30) != nil {
		t.Fatal("30 min sem luta e o chefe continua de pé")
	}
	volta := nasceu + conjuradorHoras*msPorHora
	if got := w.SpawnDueRespawns(volta - 1); len(got) != 0 {
		t.Fatalf("voltou antes das 3 h desde que nasceu: %v", got)
	}
	if got := w.SpawnDueRespawns(volta); len(got) != 1 || doBloco(w, 30) == nil {
		t.Fatalf("3 h depois de nascer voltaram %d, want o chefe de pé", len(got))
	}
}

// Uma Arma C dos monstros do spot sai com um add pedido — dano nas físicas, magia
// nas lanças e cajados —, o refino do bônus fica e o segundo add sai; e todos os
// degraus aparecem.
func TestCaveirasCarimbamArmasC(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	for _, mob := range []string{"Caveira_Lanc", "Conj_Caveira"} {
		m := spawnNamed(t, w, expMobTemplate(120, 0, 0), mob)
		for _, c := range []struct {
			armas  []int16
			efeito uint8
			want   []int
		}{
			{armasCFisicas, efDamage, []int{45, 54, 63}},
			{armasCMagicas, efMagic, []int{20, 24, 28}},
		} {
			visto := map[int]int{}
			for i := range 600 {
				arma := world.Item{Index: c.armas[i%len(c.armas)], Effects: [3]world.Effect{{Effect: efSanc, Value: 2}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
				d.caveirasFinish(w, m, &arma)
				if arma.Effects[0] != (world.Effect{Effect: efSanc, Value: 2}) {
					t.Fatalf("%s %d: refino %+v, want o do bônus", mob, arma.Index, arma.Effects[0])
				}
				if arma.Effects[1].Effect != c.efeito || !slices.Contains(c.want, int(arma.Effects[1].Value)) {
					t.Fatalf("%s %d: add %+v, want efeito %d em %v", mob, arma.Index, arma.Effects[1], c.efeito, c.want)
				}
				if arma.Effects[2] != (world.Effect{}) {
					t.Fatalf("%s %d: segundo add %+v, want vazio", mob, arma.Index, arma.Effects[2])
				}
				visto[int(arma.Effects[1].Value)]++
			}
			for _, v := range c.want {
				if visto[v] == 0 {
					t.Errorf("%s: o add %d nunca saiu em 600 armas", mob, v)
				}
			}
			if visto[c.want[0]] <= visto[c.want[len(c.want)-1]] {
				t.Errorf("%s: o maior add (%d) saiu tanto quanto o menor (%d)", mob, visto[c.want[len(c.want)-1]], visto[c.want[0]])
			}
		}
	}
}

// Fora do spot nada muda: a mesma arma de outro monstro, e o que não é Arma C
// dos monstros do spot, passam como vieram.
func TestCaveirasNaoMexemEmOutroDrop(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	arma := world.Item{Index: 868, Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
	antes := arma
	d.caveirasFinish(w, spawnNamed(t, w, expMobTemplate(120, 0, 0), "Caveira_Lanc_"), &arma)
	if arma != antes {
		t.Errorf("Lâmina Espiritual de outra caveira virou %+v", arma)
	}
	for _, idx := range []int16{869, 820, 419} { // Arma D, a arma B do template, Resto
		it := world.Item{Index: idx, Effects: antes.Effects}
		d.caveirasFinish(w, spawnNamed(t, w, expMobTemplate(120, 0, 0), "Caveira_Lanc"), &it)
		if it.Effects != antes.Effects {
			t.Errorf("item %d da Caveira Lanc ganhou efeitos %+v", idx, it.Effects)
		}
	}
}

// Cada morte do chefe entrega exatamente uma Arma C, com o add alto do tipo dela,
// e as mortes seguidas variam a arma.
func TestBossConjuradorSoltaUmaArma(t *testing.T) {
	armas := slices.Concat(armasCFisicas, armasCMagicas)
	vistas := map[int16]bool{}
	// Um mundo só: todo mundo novo começa na mesma semente e soltaria a mesma arma.
	d, w, killer := mobKilledWorld(t)
	for range 200 {
		killer.Carry = [len(killer.Carry)]world.Item{}
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(350, 0, 0), bossConjuradorTemplate))
		var achou []world.Item
		for _, it := range killer.Carry {
			if slices.Contains(armas, it.Index) {
				achou = append(achou, it)
			}
		}
		if len(achou) != 1 {
			t.Fatalf("%d armas na bolsa, want 1: %+v", len(achou), achou)
		}
		it := achou[0]
		vistas[it.Index] = true
		efeito, want := uint8(efDamage), []int{72, 81}
		if slices.Contains(armasCMagicas, it.Index) {
			efeito, want = efMagic, []int{32, 36}
		}
		if it.Effects[0] != (world.Effect{Effect: efSanc, Value: 0}) || it.Effects[1].Effect != efeito ||
			!slices.Contains(want, int(it.Effects[1].Value)) || it.Effects[2] != (world.Effect{}) {
			t.Fatalf("arma %d com %+v, want +0 e efeito %d em %v", it.Index, it.Effects, efeito, want)
		}
	}
	if len(vistas) < 10 {
		t.Errorf("só %d armas diferentes em 200 mortes", len(vistas))
	}
}

// O chefe tem os números do Troll Enigma, o corpo do Conj Caveira, a Foice
// Esqueleto +11, o Dragão Menor de nível 100, XP no teto da Dungeon e Carry vazio.
func TestBossConjuradorTemplate(t *testing.T) {
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
	b, conj, enigma := ler(bossConjuradorTemplate), ler("Conj_Caveira"), ler("ATroll_Enigma")
	if nome := string(bytes.TrimRight(b.Name[:], string([]byte{0}))); nome != bossConjuradorTemplate {
		t.Errorf("nome %q, want o do arquivo", nome)
	}
	for i := range 6 {
		if b.Equip[i].Index != conj.Equip[i].Index {
			t.Errorf("Equip[%d] = %d, o Conj Caveira usa %d", i, b.Equip[i].Index, conj.Equip[i].Index)
		}
	}
	for _, s := range []savefmt.Score{b.BaseScore, b.CurrentScore} {
		e := enigma.CurrentScore
		if s.Level != e.Level || s.MaxHp != e.MaxHp || s.Hp != e.Hp || s.AC != e.AC || s.Damage != e.Damage {
			t.Errorf("nv %d vida %d/%d def %d dano %d, want os do Enigma: nv %d vida %d def %d dano %d",
				s.Level, s.Hp, s.MaxHp, s.AC, s.Damage, e.Level, e.MaxHp, e.AC, e.Damage)
		}
		if s.Con <= conj.CurrentScore.Con {
			t.Errorf("CON %d, want maior que a do Conj Caveira (%d), que é o tamanho", s.Con, conj.CurrentScore.Con)
		}
	}
	if b.Resist != enigma.Resist {
		t.Errorf("resistência %v, want a do Enigma %v", b.Resist, enigma.Resist)
	}
	// +11 é EF_SANC 234..237: um 11 cru o cliente lê como +1 (protocol/visual.go).
	if arma := b.Equip[6]; arma.Index != 726 || arma.Effects[0].Effect != efSanc || arma.Effects[0].Value < 234 || arma.Effects[0].Value > 237 {
		t.Errorf("arma %d %+v, want a Foice Esqueleto (726) +11", arma.Index, arma.Effects[0])
	}
	// O 10:1 no primeiro par é o que o cliente exige para desenhar a montaria; o
	// nível vai no byte de efeito do segundo.
	if mt := b.Equip[14]; mt.Index != 2363 || mt.Effects[0] != (savefmt.Effect{Effect: 10, Value: 1}) || mt.Effects[1].Effect != 100 {
		t.Errorf("montaria %d %+v, want Dragão Menor (2363) de nível 100", mt.Index, mt.Effects)
	}
	if b.Exp > 10_000 {
		t.Errorf("XP %d, acima do teto de 10.000 da Dungeon (0091)", b.Exp)
	}
	for i, it := range b.Carry {
		if it.Index != 0 {
			t.Errorf("Carry[%d] = %d, want vazio: o saque é do código", i, it.Index)
		}
	}
}

// A 0144 só cita os dois monstros do spot e itens que existem, e dá a eles os
// dois Restos e exatamente as Armas C que o código carimba.
func TestCaveirasMigracao(t *testing.T) {
	root := releaseDir(t)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	linhas := linhasComZero(t, caveirasMigracao)
	if len(linhas) != 2 {
		t.Errorf("%d monstros na 0144, want Caveira_Lanc e Conj_Caveira", len(linhas))
	}
	armas := slices.Concat(armasCFisicas, armasCMagicas)
	for _, mob := range []string{"Caveira_Lanc", "Conj_Caveira"} {
		if _, _, err := npctemplate.Load(root, mob); err != nil {
			t.Errorf("%s: %v", mob, err)
		}
		porItem := linhas[mob]
		for item, c := range porItem {
			if _, ok := items.Get(int(item)); !ok {
				t.Errorf("%s: item %d não existe no ItemList", mob, item)
			}
			if c <= 0 {
				t.Errorf("%s: item %d a %d, want > 0", mob, item, c)
			}
			if item != 419 && item != 420 && !slices.Contains(armas, item) {
				t.Errorf("%s: item %d não é Resto nem Arma C", mob, item)
			}
		}
		for _, item := range append([]int16{419, 420}, armas...) {
			if _, ok := porItem[item]; !ok {
				t.Errorf("%s: falta o item %d", mob, item)
			}
		}
	}
	// As Armas C são as de EF_ITEMLEVEL 3 das nove famílias comuns.
	for _, idx := range armas {
		it, _ := items.Get(int(idx))
		lvl := ""
		for i, f := range it.Fields[:len(it.Fields)-1] {
			if f == "EF_ITEMLEVEL" {
				lvl = it.Fields[i+1]
			}
		}
		if lvl != "3" {
			t.Errorf("item %d (%s) com EF_ITEMLEVEL %q, want 3 (Arma C)", idx, it.Name, lvl)
		}
	}
}

// Ponta a ponta pela Mesa: a Arma C que a regra solta de uma Caveira Lanc chega na
// bolsa com o add pedido, e a mesma regra num monstro de fora não carimba nada.
func TestCaveirasArmaDaMesaChegaComAdd(t *testing.T) {
	for _, c := range []struct {
		mob    string
		carimb bool
	}{{"Caveira_Lanc", true}, {"Caveira_Lanc_", false}} {
		d, w, killer := mobKilledWorld(t)
		d.dropRules = droprule.NewTable([]droprule.Rule{{Mob: c.mob, Item: 868, Chance: droprule.MaxChance}})
		d.mobKilled(w, killer, spawnNamed(t, w, expMobTemplate(120, 0, 0), c.mob))
		it, ok := carryHas(killer, 868)
		if !ok {
			t.Fatalf("%s: a Lâmina Espiritual da Mesa não chegou", c.mob)
		}
		carimbada := it.Effects[1].Effect == efDamage && slices.Contains([]int{45, 54, 63}, int(it.Effects[1].Value)) && it.Effects[2] == (world.Effect{})
		if carimbada != c.carimb {
			t.Errorf("%s: arma %+v, want carimbada %v", c.mob, it.Effects, c.carimb)
		}
	}
}
