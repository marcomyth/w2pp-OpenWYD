package combat

import "testing"

// Reforma dos acessórios: a porcentagem mágica (Hecate) entra no multiplicador das
// magias, somada às poções; a física (Hércules) entra nas skills que pulam a Magia
// — a 2ª árvore do TK e a Huntress — e só nelas.
func TestPorcentagemDeDanoDosAcessorios(t *testing.T) {
	casos := []struct {
		nome     string
		skillnum int
		sp       SkillSpell
		c        SkillCaster
		weapon   int
		quer     int
	}{
		// 255 → ×110% = 280 → ×5/4 = 350 (sem o brinco: 318).
		{"TK árvore 2 com dano físico 10%", 8, SkillSpell{InstanceType: 1, InstanceValue: 10},
			SkillCaster{Class: 0, Level: 50, Str: 40, Magic: 99, Special: 15, DanoFisicoPct: 10}, 20, 350},
		// 228 → ×120% = 273 → ×110% = 300 → ×5/4 = 375 (sem o brinco: 341).
		{"Foema com dano mágico 10%", 30, SkillSpell{InstanceType: 3, InstanceValue: 8},
			SkillCaster{Class: 1, Level: 60, Int: 300, Magic: 5, Special: 25, DamageMultiPct: 110}, 0, 375},
		// O dano físico do Hércules não vale na magia.
		{"Foema com dano físico ignora", 30, SkillSpell{InstanceType: 3, InstanceValue: 8},
			SkillCaster{Class: 1, Level: 60, Int: 300, Magic: 5, Special: 25, DanoFisicoPct: 50}, 0, 341},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := SkillBaseDamage(tc.skillnum, tc.sp, tc.c, 0, tc.weapon); got != tc.quer {
				t.Errorf("dano = %d, esperado %d", got, tc.quer)
			}
		})
	}
}
