package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// acessorioFixture chama a +10 direto no handler, com a entidade na mão, como o
// odinFixture faz com o Odin.
type acessorioFixture struct {
	d      *Dispatcher
	w      *world.World
	s      *world.Session
	e      *world.Entity
	it     [protocol.MaxCombine]world.Item
	sl     [protocol.MaxCombine]int
	active []int
}

func newAcessorioFixture(t *testing.T, mesa combine.RateConfig, itens ...world.Item) *acessorioFixture {
	t.Helper()
	f := &acessorioFixture{
		d: New(Config{Log: slog.New(slog.DiscardHandler), CombineRates: mesa}),
		w: world.New(world.Config{GridDim: 16}, slog.New(slog.DiscardHandler), nil, nil),
		s: &world.Session{Conn: 1, Mode: world.UserPlay},
		e: &world.Entity{ID: 1, HP: 100, Coin: ailynCost},
	}
	for i, it := range itens {
		f.e.Carry[i] = it
		f.it[i] = it
		f.sl[i] = i
		f.active = append(f.active, i)
	}
	return f
}

func (f *acessorioFixture) enviar(t *testing.T) {
	t.Helper()
	if !f.d.combineAcessorioAilyn(f.w, f.s, f.e, f.it, f.sl, f.active) {
		t.Fatal("a +10 não reconheceu o acessório e mandaria para a receita das armas")
	}
}

func itemRefinado(t *testing.T, index int16, level int) world.Item {
	t.Helper()
	it := world.Item{Index: index, Effects: [3]world.Effect{{Effect: efSanc}}}
	if level > 0 && !refine.Set(&it, level, 0) {
		t.Fatalf("não deu para gravar +%d no item %d", level, index)
	}
	return it
}

func receitaAilyn(t *testing.T, alvo, copia world.Item, joia int16) []world.Item {
	t.Helper()
	return []world.Item{alvo, copia, {Index: itemPedraDoSabio}, {Index: joia}, {Index: joia}, {Index: joia}, {Index: joia}}
}

func mesaMaquina(familia, chave string, chance int32) combine.RateConfig {
	return combine.NewRateConfig(1, []combine.RateRow{{Family: familia, Key: chave, Rate: chance}}, nil)
}

func TestBrincoMais10ComCoral(t *testing.T) {
	brinco := itemRefinado(t, 595, 9)
	brinco.Effects[1] = world.Effect{Effect: efDamage, Value: 7}
	f := newAcessorioFixture(t, mesaMaquina("Ailyn", chaveMais10Chance, 100), receitaAilyn(t, brinco, itemRefinado(t, 595, 9), 2443)...)
	f.enviar(t)

	got := f.e.Carry[0]
	if got.Index != 595 || refine.Level(got) != 10 {
		t.Fatalf("resultado = %d +%d, esperado Brinco de Hércules +10", got.Index, refine.Level(got))
	}
	if gem := refine.Gem(got); gem != 2 {
		t.Errorf("joia gravada = %d, esperado 2 (Coral, XP)", gem)
	}
	for i := 1; i < 7; i++ {
		if !f.e.Carry[i].Empty() {
			t.Errorf("célula %d não foi consumida: %+v", i, f.e.Carry[i])
		}
	}
	if f.e.Coin != 0 {
		t.Errorf("ouro = %d, esperado 0: a +10 de acessório custa o mesmo que a das armas", f.e.Coin)
	}
}

// TestMais10DeAcessorioJuntaOsAdds: a +10 passa os adds dos dois pela regra de
// 17/09 (combine/acessorio_adds.go). O add do item da célula 0 não some mais, como
// sumia no legado, e o refino continua no primeiro espaço.
func TestMais10DeAcessorioJuntaOsAdds(t *testing.T) {
	cases := []struct {
		nome string
		add0 world.Effect
		add1 world.Effect
		quer []world.Effect
	}{
		{"o add do primeiro fica", world.Effect{Effect: efDamage, Value: 7}, world.Effect{},
			[]world.Effect{{Effect: efDamage, Value: 7}, {}}},
		{"junção de magia", world.Effect{Effect: efMagic, Value: 8}, world.Effect{Effect: efMagic, Value: 6},
			[]world.Effect{{Effect: efMagic, Value: 14}, {}}},
		{"junção de HP no teto", world.Effect{Effect: efHp, Value: 70}, world.Effect{Effect: efHp, Value: 70},
			[]world.Effect{{Effect: efHp, Value: 105}, {}}},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			alvo, copia := itemRefinado(t, 551, 9), itemRefinado(t, 551, 9)
			alvo.Effects[1], copia.Effects[1] = tc.add0, tc.add1
			f := newAcessorioFixture(t, mesaMaquina("Ailyn", chaveMais10Chance, 100), receitaAilyn(t, alvo, copia, 2442)...)
			f.enviar(t)

			got := f.e.Carry[0]
			if got.Index != 551 || refine.Level(got) != 10 || refine.Gem(got) != 1 {
				t.Fatalf("resultado = %d +%d joia %d, esperado Amuleto de Prata +10 com Esmeralda", got.Index, refine.Level(got), refine.Gem(got))
			}
			if got.Effects[0].Effect != efSanc {
				t.Errorf("o refino saiu do primeiro espaço: %+v", got.Effects)
			}
			if g := []world.Effect{got.Effects[1], got.Effects[2]}; g[0] != tc.quer[0] || g[1] != tc.quer[1] {
				t.Errorf("adds = %+v, quero %+v", g, tc.quer)
			}
		})
	}
}

// Joia fora das quatro em acessório: recusa sem gastar nada.
func TestBrincoMais10RecusaJoiaErradaSemCobrar(t *testing.T) {
	itens := receitaAilyn(t, itemRefinado(t, 595, 9), itemRefinado(t, 595, 9), 2445)
	f := newAcessorioFixture(t, mesaMaquina("Ailyn", chaveMais10Chance, 100), itens...)
	f.enviar(t)

	for i, want := range itens {
		if f.e.Carry[i] != want {
			t.Errorf("célula %d mudou na recusa: %+v, esperado %+v", i, f.e.Carry[i], want)
		}
	}
	if f.e.Coin != ailynCost {
		t.Errorf("ouro = %d, esperado %d intacto", f.e.Coin, ailynCost)
	}
}

// Um item fora da reforma segue para a +10 das armas, sem ser tocado aqui.
func TestAilynDeixaArmaParaAReceitaAntiga(t *testing.T) {
	f := newAcessorioFixture(t, combine.RateConfig{}, receitaAilyn(t, itemRefinado(t, aylinTarget, 9), itemRefinado(t, aylinTarget, 9), anctDiamond)...)
	if f.d.combineAcessorioAilyn(f.w, f.s, f.e, f.it, f.sl, f.active) {
		t.Fatal("a receita de acessório tomou uma arma")
	}
}

func TestEvolucaoCristalViraMisticoEmMaisZero(t *testing.T) {
	f := newAcessorioFixture(t, mesaMaquina("Ailyn", chaveEvolucaoAcessorio, 100), receitaAilyn(t, itemRefinado(t, 564, 9), itemRefinado(t, 564, 2), 2441)...)
	f.enviar(t)

	got := f.e.Carry[0]
	if got.Index != 560 {
		t.Fatalf("resultado = %d, esperado 560 (Místico da mesma árvore)", got.Index)
	}
	if lvl := refine.Level(got); lvl != 0 {
		t.Errorf("Místico saiu +%d, esperado +0", lvl)
	}
	if !f.e.Carry[1].Empty() {
		t.Errorf("a cópia sacrificada ficou: %+v", f.e.Carry[1])
	}
}

// Na falha da evolução o item e a cópia ficam; a pedra e as joias vão.
func TestEvolucaoFalhaGuardaOItem(t *testing.T) {
	alvo, copia := itemRefinado(t, 655, 9), itemRefinado(t, 655, 0)
	// Chance 1 na Mesa. O mundo novo sempre começa na mesma semente, então o
	// sorteio desta +10 é o mesmo que o de um mundo recém-criado aqui embaixo.
	f := newAcessorioFixture(t, mesaMaquina("Ailyn", chaveEvolucaoAcessorio, 1), receitaAilyn(t, alvo, copia, 2441)...)
	roll, _ := combine.Roll(world.New(world.Config{GridDim: 16}, slog.New(slog.DiscardHandler), nil, nil).Rand(), 1)
	if roll <= 1 {
		t.Skipf("a semente do teste sorteia %d, que passaria na chance 1", roll)
	}
	f.enviar(t)

	if f.e.Carry[0] != alvo || f.e.Carry[1] != copia {
		t.Fatalf("a falha levou o item: carry0=%+v carry1=%+v", f.e.Carry[0], f.e.Carry[1])
	}
	for i := 2; i < 7; i++ {
		if !f.e.Carry[i].Empty() {
			t.Errorf("célula %d não foi consumida na falha", i)
		}
	}
}

func arcanoFixture(t *testing.T, mesa combine.RateConfig) *odinFixture {
	t.Helper()
	f := newOdinFixture(t, combine.Catalog{}, nil)
	f.d.combineRates = mesa
	f.place(0, itemRefinado(t, 562, 15))
	f.place(3, world.Item{Index: 5334})
	f.place(4, world.Item{Index: 5335})
	f.place(5, world.Item{Index: 5336})
	f.place(6, world.Item{Index: 5337})
	return f
}

func TestOdinMisticoMais15ViraArcano(t *testing.T) {
	f := arcanoFixture(t, mesaMaquina("Odin", chaveEvolucaoArcano, 100))
	f.send()
	if got := f.e.Carry[0]; got.Index != 570 || refine.Level(got) != 0 {
		t.Fatalf("resultado = %d +%d, esperado Amuleto Arcano 570 +0", got.Index, refine.Level(got))
	}
	for i := 3; i <= 6; i++ {
		if !f.e.Carry[i].Empty() {
			t.Errorf("pedra no slot %d não foi consumida", i)
		}
	}
}

// O primeiro sorteio do Odin nesta semente é 41 (odin_mesa_test.go): 30 perde.
func TestOdinArcanoFalhaDevolveOMistico(t *testing.T) {
	f := arcanoFixture(t, mesaMaquina("Odin", chaveEvolucaoArcano, 30))
	f.send()
	if got := f.e.Carry[0]; got.Index != 562 || refine.Level(got) != 15 {
		t.Fatalf("depois da falha = %d +%d, esperado o Místico +15 de volta", got.Index, refine.Level(got))
	}
	for i := 3; i <= 6; i++ {
		if !f.e.Carry[i].Empty() {
			t.Errorf("pedra no slot %d não foi consumida", i)
		}
	}
}

func TestOdinLevaBrincoDeMais11ParaMais12(t *testing.T) {
	f := newOdinFixture(t, combine.Catalog{Pos: map[int]int{595: 256}}, nil)
	f.place(0, world.Item{Index: 4043})
	f.place(1, world.Item{Index: 4043})
	f.place(2, itemRefinado(t, 595, 11))
	f.place(3, world.Item{Index: 5334})
	f.place(4, world.Item{Index: 5335})
	f.place(5, world.Item{Index: 5336})
	f.place(6, world.Item{Index: 5337})
	f.send()
	if got := f.e.Carry[2]; got.Index != 595 || refine.Level(got) != 12 {
		t.Fatalf("brinco = %d +%d, esperado +12", got.Index, refine.Level(got))
	}
}

// A Resistência a todos do Amuleto dos Amantes não cresce com o refino; a dos
// planetas cresce como o tooltip mostra (×4,0 no +15), e o resto do equipamento
// segue o legado.
func TestResistenciaDoEspaco4NoRefino(t *testing.T) {
	d := New(Config{
		Log:     slog.New(slog.DiscardHandler),
		ItemPos: map[int]int{762: nPosAcessorio4, 1738: nPosAcessorio4, 612: 1024},
		ItemEffects: map[int][]content.BaseEffect{
			762:  {{Eff: efResistAll, Val: 10}},
			1738: {{Eff: efResistAll, Val: 10}},
			612:  {{Eff: efResist1, Val: 7}},
		},
	})
	casos := []struct {
		nome string
		item world.Item
		fogo int32
		gelo int32
	}{
		{"Netuno +15", itemRefinado(t, 762, 15), 40, 40},
		{"Amantes +9", itemRefinado(t, 1738, 9), 10, 10},
		// Orb de fogo +9 (espaço 3): segue dobrando como no legado.
		{"Defesa contra Fogo +9", itemRefinado(t, 612, 9), 14, 0},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := d.itemResist(tc.item, 0); got != tc.fogo {
				t.Errorf("resistência a fogo = %d, esperado %d", got, tc.fogo)
			}
			if got := d.itemResist(tc.item, 1); got != tc.gelo {
				t.Errorf("resistência a gelo = %d, esperado %d", got, tc.gelo)
			}
		})
	}
}

func TestCuraDaFoemaComAmantes(t *testing.T) {
	foema := func(renascimento bool, equip ...int16) *world.Entity {
		e := &world.Entity{Class: classFoema}
		if renascimento {
			e.LearnedSkill = learnedSkillBit(skillRenascimento)
		}
		for i, idx := range equip {
			e.Equip[i+8] = world.Item{Index: idx}
		}
		return e
	}
	casos := []struct {
		nome   string
		caster *world.Entity
		skill  int
		quer   int
	}{
		{"Cura com Renascimento e Amantes", foema(true, itemAmuletoAmantes), skillCura, 1300},
		{"Recuperar com Renascimento e Amantes", foema(true, itemAmuletoAmantes), skillRecuperar, 1300},
		{"sem Renascimento", foema(false, itemAmuletoAmantes), skillCura, 1000},
		{"sem Amantes", foema(true, 661), skillCura, 1000},
		{"outra skill de cura", foema(true, itemAmuletoAmantes), 13, 1000},
		{"outra classe", &world.Entity{Class: 0, LearnedSkill: learnedSkillBit(skillRenascimento), Equip: [world.MaxEquip]world.Item{11: {Index: itemAmuletoAmantes}}}, skillCura, 1000},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := foemaAmantesHeal(tc.caster, tc.skill, 1000); got != tc.quer {
				t.Errorf("cura = %d, esperado %d", got, tc.quer)
			}
		})
	}
}

// O bônus tem de chegar à cura de verdade, não só à função que o calcula, e
// entra antes do teto: quem já cura no teto não ganha nada.
func TestCuraDaFoemaComAmantesPassaPeloTeto(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.DiscardHandler), nil, nil)
	caster := &world.Entity{ID: 1, Class: classFoema, ClassMaster: classMasterMortal, LearnedSkill: learnedSkillBit(skillRenascimento)}
	caster.Equip[11] = world.Item{Index: itemAmuletoAmantes}
	target := &world.Entity{ID: 2, HP: 100, MaxHP: 5000}

	// Cura com special 200: 2×200 + 100 = 500, e 650 com os 30%.
	cast := castInfo{isSkill: true, special: 200, spell: content.Spell{InstanceType: 6, InstanceValue: 100}}
	if got := d.resolveSkillHit(w, caster, target, target.ID, skillCura, cast); got != -650 {
		t.Errorf("Cura com Amantes = %d, esperado -650", got)
	}
	// Special 450: 1000, e 1300 com os 30%. O teto desta FM é 2.500, porque ela tem
	// o Renascimento (arvore_magia_branca.go); sem ele seriam os 1.100 do legado.
	cast.special = 450
	if got := d.resolveSkillHit(w, caster, target, target.ID, skillCura, cast); got != -1300 {
		t.Errorf("Cura com os 30%% = %d, esperado -1300", got)
	}
	// Special 1400: 3000, 3900 com os 30%, e o teto da 8ª corta em 2.500.
	cast.special = 1400
	if got := d.resolveSkillHit(w, caster, target, target.ID, skillCura, cast); got != -2500 {
		t.Errorf("Cura no teto da 8ª = %d, esperado -2500", got)
	}
}
