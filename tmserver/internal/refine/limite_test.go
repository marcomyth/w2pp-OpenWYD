package refine

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func TestLimitaAddsDeArmadura(t *testing.T) {
	casos := []struct {
		nome   string
		nPos   int
		antes  world.Item
		depois world.Item
	}{
		{"elmo do drop: Magia 12 com Defesa 15 vira Magia 10", posElmo,
			peca(1193, 8, 11, efMagic, 12, efAC, 15), peca(1193, 8, 11, efMagic, 10, efAC, 15)},
		{"bônus especial de Defesa somando 31 com Crítico: sai da vaga 0", posArmadura,
			peca(1345, efAC, 6, efCritical2, 70, efAC, 25), peca(1345, efAC, 5, efCritical2, 70, efAC, 25)},
		{"bônus especial que zera vira vaga vazia", posArmadura,
			peca(1345, efAC, 5, efCritical2, 70, efAC, 30), peca(1345, efUnique, 0, efCritical2, 70, efAC, 30)},
		{"Crítico 80 com Defesa corta para 7%", posCalca,
			peca(1348, efSanc, 2, efCritical2, 80, efAC, 20), peca(1348, efSanc, 2, efCritical2, 70, efAC, 20)},
		{"Defesa 50 com Magia (Repletion antiga) vira 30", posArmadura,
			peca(1345, efSanc, 6, efMagic, 10, efAC, 50), peca(1345, efSanc, 6, efMagic, 10, efAC, 30)},

		{"Defesa 50 sozinha fica", posArmadura, peca(1345, efSanc, 6, efUnique, 9, efAC, 50), world.Item{}},
		{"Defesa 30 com Crítico 7% é o máximo e fica", posArmadura, peca(1345, efSanc, 6, efCritical2, 70, efAC, 30), world.Item{}},
		{"Defesa 30 com Magia 10% é o máximo e fica", posArmadura, peca(1345, efSanc, 6, efMagic, 10, efAC, 30), world.Item{}},
		{"Magia 12 sozinha num elmo fica (não é combinado)", posElmo, peca(1193, efSanc, 2, efMagic, 12, 4, 60), world.Item{}},
		{"arma fica de fora", 64, peca(811, efSanc, 6, efMagic, 20, efAC, 40), world.Item{}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			it := c.antes
			mudou := LimitaAddsDeArmadura(&it, c.nPos)
			quer := c.depois
			if quer.Index == 0 {
				quer = c.antes
			}
			if mudou != (quer != c.antes) {
				t.Errorf("mudou = %v, quer %v", mudou, quer != c.antes)
			}
			if it != quer {
				t.Errorf("peça ficou %+v, quer %+v", it.Effects, quer.Effects)
			}
			if estouraLimite(it) != "" && ehArmadura(c.nPos) {
				t.Errorf("depois do teto ainda estoura: %s", estouraLimite(it))
			}
		})
	}
}
