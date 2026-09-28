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

// A luva da Repletion segue o legado com o Skill por cima (Marco, 28/09/2026):
// toda linha de g_pBonusValue4 — Dano ou Magia com a defesa extra EF_ACADD2 —
// sai com a mesma chance, e em classeSkillLuvaPct das luvas o Skill toma o lugar
// do Dano ou da Magia e fica com a defesa da linha. Nunca sai Skill junto de
// Dano ou Magia, nunca sai luva vazia, e a defesa nunca é EF_AC: a EF_AC somava
// com a base do catálogo (Defesa 45 + os 17 das Manoplas, 27/09), a EF_ACADD2
// fica no lugar dela.
func TestRepletionLuvaSegueOLegadoComSkill(t *testing.T) {
	pares := classePares(t, nPosGlove)
	porLinha := (1 - classeSkillLuvaPct/100.0) / float64(len(bonusValue4))
	for _, row := range bonusValue4 {
		par := [2]world.Effect{
			{Effect: uint8(row[0]), Value: uint8(row[1])},
			{Effect: uint8(row[2]), Value: uint8(row[3])},
		}
		if d := pares[par] - porLinha; d > 1e-9 || d < -1e-9 {
			t.Errorf("linha %v do legado sai com %.5f, quer %.5f", row, pares[par], porLinha)
		}
	}
	skill, total := 0.0, 0.0
	for par, p := range pares {
		total += p
		a1, a2 := par[0], par[1]
		if a2.Effect != efAcAdd2 || a2.Value < 10 || a2.Value > 30 {
			t.Errorf("o segundo add da luva saiu %d/%d, quer defesa extra 10-30", a2.Effect, a2.Value)
		}
		switch a1.Effect {
		case efSpecialAll:
			skill += p
		case efDamageBonus, efMagic:
		default:
			t.Errorf("o primeiro add da luva saiu %d/%d, quer Dano, Magia ou Skill", a1.Effect, a1.Value)
		}
	}
	if d := total - 1; d > 1e-9 || d < -1e-9 {
		t.Fatalf("as chances somam %.6f, quer 1: há luva sem add", total)
	}
	if d := skill - classeSkillLuvaPct/100.0; d > 1e-9 || d < -1e-9 {
		t.Errorf("luvas com Skill: %.4f, quer %.2f", skill, classeSkillLuvaPct/100.0)
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
