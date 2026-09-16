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

// A porcentagem cresce com o refino, e a Defesa do primeiro espaço de acessório
// não: um Brinco +15 dá 29% (8 × 3,7) e continua dando 150 de Defesa.
func TestBrincoDeHerculesMais15(t *testing.T) {
	d := dispatcherDoBrinco()
	sem := testPlayerEntity()
	d.refreshScore(sem)

	com := testPlayerEntity()
	com.Equip[8] = brincoRefinado(t, 15)
	d.refreshScore(com)

	if com.DanoFisicoPct != 29 {
		t.Errorf("dano físico = %d%%, esperado 29%%", com.DanoFisicoPct)
	}
	if got := com.AC - sem.AC; got != 150 {
		t.Errorf("Defesa do brinco +15 = %d, esperado 150 (não cresce com o refino)", got)
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
