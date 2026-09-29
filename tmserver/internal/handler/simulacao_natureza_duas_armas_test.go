//go:build simulacao

package handler

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O PRÊMIO DAS DUAS ARMAS DA NATUREZA (29/09/2026).
//
// O pedido veio de um print: o BateNeles, BM Natureza nível 353, FOR 1.722,
// DES 637, set do Corvo +9 e duas espadas +9, marcava 2.313 de Ataque, e o
// operador quer entre 3.200 e 3.600.
//
// A ficha é montada pelo EQUIPAMENTO, e não calibrada pela janela, porque a
// pergunta é justamente de onde sai o número. O simulador comum não carrega
// nUnique nem nPos, e sem eles o bônus de arma da classe e os +40 do refino
// saem zero; aqui os dois mapas entram. O que ainda falta (acessórios,
// montaria, buffs) é resolvido como Damage plano até a janela bater com o
// print, com o prêmio em zero, e fica fixo durante a varredura.
func TestSimulacaoNaturezaDuasArmas(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	items, err := content.LoadItemList(filepath.Join(root, "Common", "ItemList.csv"))
	if err != nil {
		t.Skipf("ItemList: %v", err)
	}
	sm.d.itemPos, sm.d.itemUnique, sm.d.itemNames = items.Positions(), items.Uniques(), items.Names()
	atual := naturezaDanoDuasArmasAtual
	defer func() { naturezaDanoDuasArmasAtual = atual }()

	type ficha struct {
		nome          string
		nivel         int32
		str, dex, con int16
		special       [4]int16
		refino        uint8
		forma         uint8 // Value do afeto 16; 0 = humano
		janela        int32 // o Ataque do print, com o prêmio em zero
		min, max      int32 // onde a ficha tem de cair com o prêmio de hoje
	}
	fichas := []ficha{
		// O print de 29/09: humano, montado.
		{"BateNeles", 353, 1722, 637, 300, [4]int16{125, 146, 234, 289}, 9, 0, 2313, 3200, 3600},
		// O print de 20/09 (simulacao_natureza_test.go): a ponta de Destreza,
		// em Éden, é a que mais se aproxima do teto de 9.000.
		{"DanoPRZ", 400, 6, 2569, 805, [4]int16{198, 294, 294, 349}, simRefino11, 5, 4787, 0, 9000},
	}
	sanc := func(idx int16, v uint8) world.Item {
		return world.Item{Index: idx, Effects: [3]world.Effect{{Effect: simEfSanc, Value: v}}}
	}
	monta := func(f ficha, plano int32) *world.Entity {
		e := &world.Entity{ID: 1, Class: 2, ClassMaster: classMasterMortal, Level: f.nivel,
			Str: f.str, Int: 12, Dex: f.dex, Con: f.con, LearnedSkill: simBMLearned | learnedEden,
			Special: f.special, BaseSpecial: f.special}
		e.Equip[0] = world.Item{Index: 21}
		for i := range int16(5) {
			e.Equip[1+i] = sanc(1510+i, f.refino) // set do Corvo
		}
		e.Equip[weaponSlotR] = sanc(simCaliburn, f.refino)
		e.Equip[weaponSlotL] = sanc(simBalmung, f.refino)
		sm.d.deriveBaseScore(e)
		e.BaseDamage += plano
		sm.d.refreshScore(e)
		if f.forma != 0 {
			e.Affect[1] = world.Affect{Type: affectTransform, Value: f.forma, Level: uint16(f.special[3]), Time: 5000}
		}
		sm.d.applyAffectScore(e)
		return e
	}

	naturezaDanoDuasArmasAtual = 0
	planos := make([]int32, len(fichas))
	for i, f := range fichas {
		lo, hi := int32(0), int32(100_000)
		for lo < hi {
			mid := (lo + hi) / 2
			if sm.d.effectiveDamage(monta(f, mid)) < f.janela {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		planos[i] = lo
		fmt.Printf("%s: %d de Damage plano fora do modelo (acessórios, montaria, buffs)\n", f.nome, lo)
	}

	for _, v := range []int{0, 40, 60, 80, atual} {
		naturezaDanoDuasArmasAtual = v
		fmt.Printf("duas armas %3d:", v)
		for i, f := range fichas {
			fmt.Printf("  %s %5d", f.nome, sm.d.effectiveDamage(monta(f, planos[i])))
		}
		fmt.Println()
	}

	naturezaDanoDuasArmasAtual = atual
	for i, f := range fichas {
		if got := sm.d.effectiveDamage(monta(f, planos[i])); got < f.min || got > f.max {
			t.Errorf("%s com o prêmio em %d: janela %d, want %d..%d", f.nome, atual, got, f.min, f.max)
		}
	}
}
