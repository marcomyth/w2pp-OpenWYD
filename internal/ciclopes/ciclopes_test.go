package ciclopes

import "testing"

func TestMonstroDeCombate(t *testing.T) {
	casos := []struct {
		nome        string
		template    string
		mobMerchant uint8
		quer        bool
	}{
		{"Ciclope Cruel em qualquer lugar", "Ciclope_Cruel", 0, true},
		{"a cópia do spot", "Ciclope_Cruel_Spot", 0, true},
		{"o Lanceiro do spot", "Lanceiro_Zakum_Spot", 0, true},
		{"caixa diferente no nome", "ciclope_cruel", 0, true},
		{"o Lanceiro fora do spot fica como era", "Lanceiro_Zakum", 0, false},
		{"um NPC de verdade pelo byte do legado", "Ciclope_Cruel", 1, false},
		{"outro monstro", "Anciao_Ciclops", 0, false},
	}
	for _, c := range casos {
		if got := MonstroDeCombate(c.template, c.mobMerchant); got != c.quer {
			t.Errorf("%s: MonstroDeCombate(%q, %d) = %v, queria %v", c.nome, c.template, c.mobMerchant, got, c.quer)
		}
	}
}
