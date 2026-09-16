package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	brincoHercules = 595
	espadaDeTeste  = 900
)

func dispatcherDoBrinco() *Dispatcher {
	return New(Config{
		ItemEffects: map[int][]content.BaseEffect{
			brincoHercules: {{Eff: efDanoFisico, Val: 8}, {Eff: efAc, Val: 150}},
			espadaDeTeste:  {{Eff: efDamage, Val: 200}},
		},
		ItemPos: map[int]int{brincoHercules: nPosAcessorio1, espadaDeTeste: nPosWeapon1},
	})
}

func brincoRefinado(t *testing.T, level int) world.Item {
	t.Helper()
	it := world.Item{Index: brincoHercules, Effects: [3]world.Effect{{Effect: efSanc}}}
	if level > 0 && !refine.Set(&it, level, 0) {
		t.Fatalf("não gravou +%d", level)
	}
	return it
}

// A porcentagem e a Defesa crescem com o refino pela conta do WYD.exe: um Brinco
// +15 dá 32% (8 × 4,0) e 600 de Defesa, os números que o tooltip mostra.
func TestBrincoDeHerculesMais15(t *testing.T) {
	d := dispatcherDoBrinco()
	sem := testPlayerEntity()
	d.refreshScore(sem)

	com := testPlayerEntity()
	com.Equip[8] = brincoRefinado(t, 15)
	d.refreshScore(com)

	if com.DanoFisicoPct != 32 {
		t.Errorf("dano físico = %d%%, esperado 32%%", com.DanoFisicoPct)
	}
	if got := com.AC - sem.AC; got != 600 {
		t.Errorf("Defesa do brinco +15 = %d, esperado 600", got)
	}
}

// A porcentagem soma com as poções no dano e também pega a arma, que o
// multiplicador das poções nunca pegou.
func TestPorcentagemFisicaNoAtaque(t *testing.T) {
	d := dispatcherDoBrinco()
	// A regra de combate do painel escala o físico de jogador; aqui fica neutra.
	d.combatRules.PhysicalDamagePct = 100
	e := &world.Entity{ID: 1, Damage: 1000, AffDamageMultiPct: 105, DanoFisicoPct: 10}
	e.Equip[weaponSlotR] = world.Item{Index: espadaDeTeste}
	// 1000 × (105+10)% = 1150, mais a arma 200 × 110% = 220.
	if got := d.effectiveDamage(e); got != 1370 {
		t.Errorf("ataque = %d, esperado 1370", got)
	}
	e.DanoFisicoPct = 0
	// Sem o brinco: 1000 × 105% + 200.
	if got := d.effectiveDamage(e); got != 1250 {
		t.Errorf("ataque sem brinco = %d, esperado 1250", got)
	}
}

// O resto do equipamento segue o legado: a Defesa de uma armadura cresce com o refino.
func TestDefesaForaDoAcessorioSegueORefino(t *testing.T) {
	const armadura = 1331
	d := New(Config{
		ItemEffects: map[int][]content.BaseEffect{armadura: {{Eff: efAc, Val: 100}}},
		ItemPos:     map[int]int{armadura: nPosDef1},
	})
	it := world.Item{Index: armadura, Effects: [3]world.Effect{{Effect: efSanc}}}
	refine.Set(&it, 15, 0)
	if got := d.itemAbilityRefined(it, efAc); got != 370 {
		t.Errorf("Defesa da armadura +15 = %d, esperado 370", got)
	}
}

// Fator de refino de acessório nível a nível, contra a tabela do WYD.exe
// (0x537F31 e 0x5380F2). A armadura continua no legado: +15 é ×3,7.
func TestFatorDeRefinoDoAcessorioSegueOCliente(t *testing.T) {
	d := dispatcherDoBrinco()
	want := map[int]int{0: 10, 5: 15, 8: 18, 9: 20, 10: 22, 11: 25, 12: 28, 13: 32, 14: 37, 15: 40}
	for level, f := range want {
		if got := d.refineFactor(brincoRefinado(t, level)); got != f {
			t.Errorf("brinco +%d: fator %d, esperado %d", level, got, f)
		}
	}
}
