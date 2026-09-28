package refine

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// classePares runs every combination of draws classeAdds can make and returns
// the exact odds of each (first add, second add) pair.
func classePares(t *testing.T, nPos int) map[[2]world.Effect]float64 {
	t.Helper()
	pares := map[[2]world.Effect]float64{}
	var anda func(prefixo []int, prob float64)
	anda = func(prefixo []int, prob float64) {
		i := 0
		var mods []int
		funda := false
		roll := func(n int) int {
			mods = append(mods, n)
			if i < len(prefixo) {
				v := prefixo[i]
				i++
				return v
			}
			funda = true
			return 0
		}
		a1, a2 := classeAdds(nPos, roll)
		if !funda {
			// O valor do EF_UNIQUE é só ruído: agrupa todos numa chave.
			if a1.Effect == efUnique {
				a1.Value = 0
			}
			pares[[2]world.Effect{a1, a2}] += prob
			return
		}
		n := mods[len(prefixo)]
		for v := 0; v < n; v++ {
			anda(append(append([]int{}, prefixo...), v), prob/float64(n))
		}
	}
	anda(nil, 1)
	return pares
}

// A Repletion não junta Defesa alta com outro add (Marco, 27/09/2026): uma
// túnica saiu com Defesa 50 e Magia 10%. Acima de 30 a peça é só Defesa; os adds
// combinados ficam no limite do jogo (Defesa até 30 com Crítico ou Magia).
func TestRepletionDefesaAltaVemSozinha(t *testing.T) {
	for _, c := range []struct {
		nome string
		nPos int
	}{{"peito", nPosChest}, {"calça", nPosLegs}} {
		t.Run(c.nome, func(t *testing.T) {
			sozinha, total := 0.0, 0.0
			for par, p := range classePares(t, c.nPos) {
				total += p
				a1, a2 := par[0], par[1]
				if a2.Effect == efAC && a2.Value > classeDefesaSozinhaAcima {
					if a1.Effect != efUnique {
						t.Errorf("Defesa %d veio junto com o add %d/%d", a2.Value, a1.Effect, a1.Value)
					}
					sozinha += p
					continue
				}
				// Fora da Defesa alta, o primeiro add continua sendo Dano ou Magia.
				if a1.Effect != efDamageBonus && a1.Effect != efMagic {
					t.Errorf("sem Defesa alta o primeiro add saiu %d/%d, quer Dano ou Magia", a1.Effect, a1.Value)
				}
			}
			if d := total - 1; d > 1e-9 || d < -1e-9 {
				t.Fatalf("as chances somam %.6f, quer 1", total)
			}
			// Desde 28/09 a Defesa alta é a chance extra sobre a tabela do legado.
			if d := sozinha - classeDefesaAltaPct/100.0; d > 1e-9 || d < -1e-9 {
				t.Errorf("peças só com Defesa: %.4f, quer %.2f", sozinha, classeDefesaAltaPct/100.0)
			}
		})
	}
}

// A luva da Repletion não sorteia Defesa (Marco, 27/09/2026): saiu uma luva com
// Defesa 45 somada aos 17 de base do catálogo. O segundo add dela é sempre Skill,
// e o primeiro Dano ou Magia — em todas as combinações de sorteio.
func TestRepletionLuvaNuncaTemDefesa(t *testing.T) {
	for par := range classePares(t, nPosGlove) {
		a1, a2 := par[0], par[1]
		if a1.Effect == efAC || a2.Effect == efAC {
			t.Errorf("luva saiu com Defesa: %d/%d e %d/%d", a1.Effect, a1.Value, a2.Effect, a2.Value)
		}
		if a2.Effect != efSpecialAll {
			t.Errorf("o segundo add da luva saiu %d/%d, quer Skill", a2.Effect, a2.Value)
		}
		if a1.Effect != efDamageBonus && a1.Effect != efMagic {
			t.Errorf("o primeiro add da luva saiu %d/%d, quer Dano ou Magia", a1.Effect, a1.Value)
		}
	}
}

// A fórmula do legado não se apaga (Marco, 28/09/2026): toda linha de
// g_pBonusValue2 continua saindo no peito e na calça, cada uma com a mesma
// chance, fora a parte da Defesa alta. Os pools de 26/09 tinham tirado todas —
// Magia 10 + Defesa 30 e Dano 24 + Crítico 7% deixaram de existir na Repletion.
func TestRepletionMantemATabelaDoLegado(t *testing.T) {
	porLinha := (1 - classeDefesaAltaPct/100.0) / float64(len(bonusValue2))
	for _, c := range []struct {
		nome string
		nPos int
	}{{"peito", nPosChest}, {"calça", nPosLegs}} {
		t.Run(c.nome, func(t *testing.T) {
			pares := classePares(t, c.nPos)
			for _, row := range bonusValue2 {
				par := [2]world.Effect{
					{Effect: uint8(row[0]), Value: uint8(row[1])},
					{Effect: uint8(row[2]), Value: uint8(row[3])},
				}
				if d := pares[par] - porLinha; d > 1e-9 || d < -1e-9 {
					t.Errorf("linha %v do legado sai com %.5f, quer %.5f", row, pares[par], porLinha)
				}
			}
			for _, alvo := range []struct {
				nome string
				par  [2]world.Effect
			}{
				{"Magia 10 + Defesa 30", [2]world.Effect{{Effect: efMagic, Value: 10}, {Effect: efAC, Value: 30}}},
				{"Dano 24 + Crítico 7%", [2]world.Effect{{Effect: efDamageBonus, Value: 24}, {Effect: efCritical2, Value: 70}}},
				{"Dano 30 + Crítico 7%", [2]world.Effect{{Effect: efDamageBonus, Value: 30}, {Effect: efCritical2, Value: 70}}},
			} {
				if pares[alvo.par] == 0 {
					t.Errorf("%s não sai mais da Repletion", alvo.nome)
				}
			}
		})
	}
}
