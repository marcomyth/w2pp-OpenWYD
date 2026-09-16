package content

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/itemeffect"
)

// Reforma dos acessórios (2026-09-16): bracelete, pingente, brinco e colar de
// Hércules, Hecate e Zeus dão a mesma Defesa por degrau (50/100/150/200). Hércules
// e Hecate trocaram o dano fixo por porcentagem; Zeus mantém a velocidade de
// ataque. O teste lê o catálogo real, porque é ele que o jogo e o gerador do
// cliente usam.
func TestAcessoriosDasFamiliasNoCatalogo(t *testing.T) {
	const efAC, efAttSpeed = 3, 26
	full, err := LoadItemList(release(t, "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	eff := full.BaseEffects()
	for _, tc := range []struct {
		idx          int
		nome         string
		principal    uint8
		valor, defes int16
	}{
		{507, "Bracelete de Hércules", itemeffect.DanoFisicoPct, 4, 50},
		{521, "Pingente de Hércules", itemeffect.DanoFisicoPct, 6, 100},
		{595, "Brinco de Hércules", itemeffect.DanoFisicoPct, 8, 150},
		{643, "Colar da Força", itemeffect.DanoFisicoPct, 10, 200},
		{514, "Bracelete de Hecate", itemeffect.DanoMagicoPct, 4, 50},
		{520, "Pingente de Hecate", itemeffect.DanoMagicoPct, 6, 100},
		{594, "Brinco de Hecate", itemeffect.DanoMagicoPct, 8, 150},
		{642, "Colar Mágico", itemeffect.DanoMagicoPct, 10, 200},
		{513, "Bracelete de Zeus", efAttSpeed, 15, 50},
		{519, "Pingente de Zeus", efAttSpeed, 18, 100},
		{593, "Brinco de Zeus", efAttSpeed, 21, 150},
	} {
		got := map[uint8]int16{}
		for _, e := range eff[tc.idx] {
			got[e.Eff] += e.Val
		}
		if got[tc.principal] != tc.valor {
			t.Errorf("%s (%d): efeito %d = %d, esperado %d", tc.nome, tc.idx, tc.principal, got[tc.principal], tc.valor)
		}
		if got[efAC] != tc.defes {
			t.Errorf("%s (%d): Defesa = %d, esperado %d", tc.nome, tc.idx, got[efAC], tc.defes)
		}
	}
}
