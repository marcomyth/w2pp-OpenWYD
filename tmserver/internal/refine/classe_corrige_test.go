package refine

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func peca(idx int16, e0, v0, e1, v1, e2, v2 uint8) world.Item {
	return world.Item{Index: idx, Effects: [3]world.Effect{{Effect: e0, Value: v0}, {Effect: e1, Value: v1}, {Effect: e2, Value: v2}}}
}

// As quatro peças que o log do reroll mostrou aberrantes em 27/09/2026 viram
// Defesa 30 com o mesmo add; o resto não pode ser tocado — em especial as peças
// de loja com Defesa 70 e 99, que também juntam Defesa com Magia ou HP.
func TestCorrigeDefesaCombinada(t *testing.T) {
	casos := []struct {
		nome   string
		nPos   int
		antes  world.Item
		muda   bool
		depois world.Item
	}{
		{"Túnica de Mytril (M): Defesa 35 + Magia 10", nPosChest,
			peca(1345, efSanc, 6, efMagic, 10, efAC, 35), true, peca(1345, efSanc, 6, efMagic, 10, efAC, 30)},
		{"Calça de Mytril (M): Defesa 40 + Magia 10", nPosLegs,
			peca(1348, efSanc, 6, efMagic, 10, efAC, 40), true, peca(1348, efSanc, 6, efMagic, 10, efAC, 30)},
		{"Calça Elemental (M): Defesa 40 + Dano 18", nPosLegs,
			peca(1498, efSanc, 6, efDamageBonus, 18, efAC, 40), true, peca(1498, efSanc, 6, efDamageBonus, 18, efAC, 30)},
		// Luva: a Defesa sai e vira Skill 12 (a Defesa alta é só de peito e calça).
		{"Manoplas Elementais (M): Defesa 50 + Magia 8", nPosGlove,
			peca(1501, efSanc, 4, efMagic, 8, efAC, 50), true, peca(1501, efSanc, 4, efMagic, 8, efSpecialAll, 12)},
		{"luva do print de 27/09: Defesa 45 sozinha", nPosGlove,
			peca(1501, efSanc, 5, efUnique, 111, efAC, 45), true, peca(1501, efSanc, 5, efUnique, 111, efSpecialAll, 12)},
		{"luva que a primeira correção deixou com Defesa 30", nPosGlove,
			peca(1501, efSanc, 4, efMagic, 8, efAC, 30), true, peca(1501, efSanc, 4, efMagic, 8, efSpecialAll, 12)},
		{"luva do item de nível com Defesa 20 fica", nPosGlove,
			peca(1501, efSanc, 3, efDamageBonus, 20, efAC, 20), false, world.Item{}},
		{"Defesa na vaga 1 também", nPosChest,
			peca(1345, efSanc, 2, efAC, 45, efMagic, 6), true, peca(1345, efSanc, 2, efAC, 30, efMagic, 6)},

		{"Defesa alta sozinha fica", nPosChest, peca(1345, efSanc, 6, efUnique, 77, efAC, 50), false, world.Item{}},
		{"Defesa 30 com Magia já está no limite", nPosChest, peca(1345, efSanc, 6, efMagic, 10, efAC, 30), false, world.Item{}},
		{"loja: Set_BM com Magia 14 e Defesa 70", nPosChest, peca(1515, efMagic, 14, efAC, 70, efSanc, 253), false, world.Item{}},
		{"loja: Defesa 99 com HP", nPosChest, peca(1711, efAC, 99, 4, 120, efSanc, 253), false, world.Item{}},
		{"arma com Dano e Defesa 40 não é Repletion", 64, peca(811, efSanc, 6, efDamageBonus, 18, efAC, 40), false, world.Item{}},
		{"elmo não tem Defesa na Repletion", nPosHelm, peca(1193, efSanc, 6, efMagic, 10, efAC, 40), false, world.Item{}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			it := c.antes
			if got := CorrigeDefesaCombinada(&it, c.nPos); got != c.muda {
				t.Fatalf("CorrigeDefesaCombinada = %v, quer %v (%+v)", got, c.muda, it.Effects)
			}
			quer := c.depois
			if !c.muda {
				quer = c.antes
			}
			if it != quer {
				t.Errorf("peça ficou %+v, quer %+v", it.Effects, quer.Effects)
			}
		})
	}
}
