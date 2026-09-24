package handler

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const geloAmonMigracao = "0128_gelo_amon_armas_d.up.sql"

// A 0128 dá ao Soldado e ao Guerreiro Amon as nove Armas D de topo, e só a eles:
// todo item é uma arma de nível 4 do catálogo, e cada uma está numa das duas
// escadas do add (física ou mágica). Os dois só nascem no Gelo.
func TestGeloAmonArmasD(t *testing.T) {
	root := releaseDir(t)
	items := catalogoDoGelo(t)
	linhas := linhasComZero(t, geloAmonMigracao)
	if len(linhas) != 2 {
		t.Errorf("a 0128 mexe em %d templates, want 2 (Soldado e Guerreiro Amon)", len(linhas))
	}
	for mob, l := range linhas {
		if !geloAmon[droprule.Canonical(mob)] {
			t.Errorf("%s na 0128 não é um dos Amon do add", mob)
		}
		if want := len(armasAmonFisicas) + len(armasAmonMagicas); len(l) != want {
			t.Errorf("%s solta %d armas, want %d", mob, len(l), want)
		}
		for item, c := range l {
			e, ok := items.Get(int(item))
			if !ok {
				t.Errorf("%s: item %d não existe no ItemList", mob, item)
				continue
			}
			if nivel, _ := efeitoDoItem(e, "EF_ITEMLEVEL"); nivel != 4 {
				t.Errorf("%s solta %s, de nível %d; want Arma D (nível 4)", mob, e.Name, nivel)
			}
			if !armasAmonFisicas[item] && !armasAmonMagicas[item] {
				t.Errorf("%s solta %s, que não tem escada de add", mob, e.Name)
			}
			if c == 0 {
				t.Errorf("%s solta %s a 0%%", mob, e.Name)
			}
		}
	}
	gens, err := content.LoadNPCGenerators(filepath.Join(root, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		t.Fatal(err)
	}
	nascem := 0
	for i, g := range gens {
		if _, ok := linhas[g.Leader]; !ok {
			continue
		}
		nascem++
		x, y := int(g.SegX[0]), int(g.SegY[0])
		if x < geloMinX || x > geloMaxX || y < geloMinY || y > geloMaxY {
			t.Errorf("%s nasce no bloco %d em (%d,%d), fora do Gelo", g.Leader, i, x, y)
		}
	}
	if nascem == 0 {
		t.Error("nenhum bloco do NPCGener tem os Amon da 0128")
	}
}

// A escada dos Amon: a física só dá dano (27 a 54) e a mágica só magia (12 a 24),
// todo degrau sai, o slot 0 é refino de +0 a +2, e o slot 2 nunca repete o
// atributo da escada — o segundo add aleatório do legado fica quando é outro.
func TestGeloAmonAddDaEscada(t *testing.T) {
	casos := []struct {
		nome   string
		arma   int16
		tabela []addArma
		outro  world.Effect // segundo add do legado, de outro atributo
	}{
		{"Espada Vorpal", 870, addAmonFisica, world.Effect{Effect: 26, Value: 9}},
		{"Solaris", 911, addAmonFisica, world.Effect{Effect: efSpecialAll, Value: 6}},
		{"Lança do Triunfo", 855, addAmonMagica, world.Effect{Effect: efSpecialAll, Value: 9}},
		{"Fúria Divina", 900, addAmonMagica, world.Effect{Effect: efSpecialAll, Value: 3}},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			d, w, _ := mobKilledWorld(t)
			naTabela := map[world.Effect]bool{}
			for _, l := range tc.tabela {
				naTabela[world.Effect{Effect: l.efeito, Value: uint8(l.valor)}] = true
			}
			visto := map[world.Effect]bool{}
			for _, mob := range []string{"Soldado_Amon", "Guerreiro_Amon"} {
				m := spawnNamed(t, w, expMobTemplate(360, 0, 0), mob)
				m.SpawnX, m.SpawnY = 3700, 2900
				for i := range 400 {
					// O que o bônus de drop comum pode ter deixado: um bônus especial
					// no slot 0 e, no slot 2, o mesmo atributo da escada ou outro.
					repete := world.Effect{Effect: tc.tabela[0].efeito, Value: 36}
					it := world.Item{Index: tc.arma, Effects: [3]world.Effect{{Effect: efDamage, Value: 20}, {}, repete}}
					if i%2 == 1 {
						it.Effects = [3]world.Effect{{Effect: efSanc, Value: 2}, {}, tc.outro}
					}
					d.geloAmonFinish(w, m, &it)
					if it.Effects[0].Effect != efSanc || it.Effects[0].Value > 2 {
						t.Fatalf("slot 0 = %+v, want EF_SANC de +0 a +2", it.Effects[0])
					}
					if !naTabela[it.Effects[1]] {
						t.Fatalf("add %+v fora da escada", it.Effects[1])
					}
					if it.Effects[2].Effect == it.Effects[1].Effect {
						t.Fatalf("slot 2 = %+v repete o atributo do add %+v", it.Effects[2], it.Effects[1])
					}
					if i%2 == 1 && it.Effects[2] != tc.outro {
						t.Fatalf("slot 2 = %+v, want o segundo add do legado %+v intacto", it.Effects[2], tc.outro)
					}
					visto[it.Effects[1]] = true
				}
			}
			if len(visto) != len(tc.tabela) {
				t.Errorf("só %d dos %d degraus saíram: %v", len(visto), len(tc.tabela), visto)
			}
		})
	}
	if addAmonFisica[len(addAmonFisica)-1].valor != 54 || addAmonMagica[len(addAmonMagica)-1].valor != 24 {
		t.Error("o topo da escada deixou de ser 54 de dano e 24 de magia")
	}
	pedida := func(tab []addArma, de int) (p, total int) {
		for _, l := range tab {
			total += l.peso
			if l.valor >= de {
				p += l.peso
			}
		}
		return p, total
	}
	for _, c := range []struct {
		nome string
		tab  []addArma
		de   int
	}{{"física 45-54", addAmonFisica, 45}, {"mágica 20-24", addAmonMagica, 20}} {
		if p, total := pedida(c.tab, c.de); p == 0 || p*2 > total {
			t.Errorf("%s sai em %d de %d: want possível, mas não a maioria", c.nome, p, total)
		}
	}
}

// Fora dos Amon do Gelo nada muda: a mesma arma de outro monstro do Gelo, de um
// Amon nascido fora da caixa, do Capitão Amon, e um item que não é Arma D do
// Amon passam como vieram.
func TestGeloAmonNaoMexeEmOutroDrop(t *testing.T) {
	d, w, _ := mobKilledWorld(t)
	arma := world.Item{Index: 870, Effects: [3]world.Effect{{Effect: efSanc, Value: 1}, {Effect: 26, Value: 3}, {Effect: efDamage, Value: 9}}}
	casos := []struct {
		nome string
		mob  string
		x, y int16
		it   world.Item
	}{
		{"lobo do Gelo", "Lobo_Polar", 3700, 2900, arma},
		{"Amon fora do Gelo", "Soldado_Amon", 2635, 1725, arma},
		{"Capitão Amon", "Capitao_Amon", 3700, 2900, arma},
		{"âmago do Amon", "Guerreiro_Amon", 3700, 2900, world.Item{Index: 2404}},
	}
	for _, c := range casos {
		m := spawnNamed(t, w, expMobTemplate(360, 0, 0), c.mob)
		m.SpawnX, m.SpawnY = c.x, c.y
		it := c.it
		d.geloAmonFinish(w, m, &it)
		if it != c.it {
			t.Errorf("%s: %+v virou %+v", c.nome, c.it, it)
		}
	}
}

// Pelo abate de verdade: a regra da Mesa solta a Arma D do Amon, e ela chega à
// bolsa com o add da escada. Trinta abates, porque o bônus de drop sozinho também
// escreve dano no slot 1 — só numa parte das armas, nunca em todas.
func TestGeloAmonAbateSoltaArmaComAdd(t *testing.T) {
	naEscada := map[world.Effect]bool{}
	for _, l := range addAmonFisica {
		naEscada[world.Effect{Effect: l.efeito, Value: uint8(l.valor)}] = true
	}
	d, w, killer := mobKilledWorld(t)
	d.dropRules = droprule.NewTable([]droprule.Rule{{Mob: "Guerreiro_Amon", Item: 911, Chance: droprule.MaxChance}})
	for i := range 30 {
		killer.Carry = [len(killer.Carry)]world.Item{}
		m := spawnNamed(t, w, expMobTemplate(360, 0, 0), "Guerreiro_Amon")
		m.SpawnX, m.SpawnY = 3700, 2900
		d.mobKilled(w, killer, m)
		it, ok := carryHas(killer, 911)
		if !ok {
			t.Fatalf("abate %d: a Solaris não chegou à bolsa", i)
		}
		if !naEscada[it.Effects[1]] {
			t.Fatalf("abate %d: a Solaris saiu com %+v, want um add da escada dos Amon no slot 1", i, it.Effects)
		}
	}
}

// Depois de o servidor subir, a Sombra e o Verid do Gelo só aparecem 4 h depois,
// pela mesma fila da morte; um chefe de fora do Gelo e o resto do mundo ficam de
// pé. E voltam para ficar: nada os tira de novo sem morte.
func TestGeloChefesNascemHorasDepoisDoBoot(t *testing.T) {
	var agora uint32 = 5_000
	bloco := func(nome string, x, y int16) *world.Generator {
		g := geradorSozinho(2_990_849, 10)
		g.LeaderName = nome
		g.LeaderTmpl = moldeDeMonstro(nome, 2_990_849, 0)
		g.SegX[0], g.SegY[0] = x, y
		return g
	}
	w := world.New(world.Config{GridDim: 4096, Now: func() uint32 { return agora }},
		slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators([]*world.Generator{
		30: bloco("Verid", 3667, 2780),
		31: bloco("Sombra_Negra_", 3817, 2880),
		32: bloco("Sombra_Negra", 2635, 1725),
		33: bloco("Lobo_Polar", 3700, 2900),
	})
	for _, idx := range []int{30, 31, 32, 33} {
		if len(w.GenerateMob(idx)) != 1 {
			t.Fatalf("o boot não levantou o bloco %d", idx)
		}
	}
	vivos := func() map[int]bool {
		out := map[int]bool{}
		for id := world.MaxUser; id < world.MaxMob; id++ {
			if e := w.Entity(id); e != nil {
				out[int(e.GenIndex)] = true
			}
		}
		return out
	}

	dispatcherQuieto().ApplyGeloChefesBoot(w)

	v := vivos()
	if v[30] || v[31] {
		t.Fatalf("chefe do Gelo de pé logo depois do boot: %v", v)
	}
	if !v[32] || !v[33] {
		t.Fatalf("o boot do Gelo tirou quem não é chefe dele: %v", v)
	}
	quatro := uint32(geloChefeHoras * msPorHora)
	if got := w.SpawnDueRespawns(agora + quatro - 1); len(got) != 0 {
		t.Fatalf("chefe voltou antes das 4 h: %v", got)
	}
	agora += quatro
	if got := w.SpawnDueRespawns(agora); len(got) != 2 {
		t.Fatalf("4 h depois do boot voltaram %d chefes, want 2", len(got))
	}
	if v := vivos(); !v[30] || !v[31] {
		t.Fatalf("os chefes do Gelo não estão de pé 4 h depois do boot: %v", v)
	}
	if got := w.SpawnDueRespawns(agora + 48*msPorHora); len(got) != 0 {
		t.Errorf("a fila ainda tinha %d chefes do Gelo depois da volta", len(got))
	}
}
