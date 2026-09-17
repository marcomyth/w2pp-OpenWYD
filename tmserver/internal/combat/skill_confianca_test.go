package combat

import "testing"

// O TK Confiança bate as quatro skills da árvore pela DES e pela INT, sem a Magia,
// com a arma por cima (handler/arvore_confianca.go).
func TestSkillBaseDamageConfianca(t *testing.T) {
	sp := SkillSpell{InstanceType: 4, InstanceValue: 75} // Fanatismo
	caster := SkillCaster{
		Class: 0, Level: 300, Int: 800, Dex: 1500, Special: 255,
		DamageMultiPct: 100, LearnedSkill: 1 << 7,
		Confianca: true, ArmaPct: 140,
	}
	// 3×1000 + 3×(0,6×1500 + 0,4×800) + 300 + 255 + 75 = 7290; ×1,40 = 10206;
	// ×5/4 = 12757; ×1,15 da 8ª = 14670.
	const want = 14670
	for _, magic := range []int{0, 200} {
		caster.Magic = magic
		if got := SkillBaseDamage(4, sp, caster, 0, 1000); got != want {
			t.Fatalf("Magia %d: SkillBaseDamage = %d, want %d (a Magia não entra)", magic, got, want)
		}
	}

	caster.ArmaPct = 100
	if got := SkillBaseDamage(4, sp, caster, 0, 1000); got != 10478 { // 7290 × 5/4 × 1,15
		t.Fatalf("arma neutra: SkillBaseDamage = %d, want 10478", got)
	}

	caster.DamageMultiPct = 120
	if got := SkillBaseDamage(4, sp, caster, 0, 1000); got != 12575 { // 7290 × 1,20 × 5/4 × 1,15
		t.Fatalf("poção de 20%%: SkillBaseDamage = %d, want 12575", got)
	}

	// Sem a regra, a mesma skill segue a fórmula mágica do legado.
	caster.Confianca, caster.DamageMultiPct, caster.Magic = false, 100, 0
	legado := 255 + 75 + 1000 + 300 + 800/4 + 800/40 // × (4×0+100)% × 5/4 × 1,15
	if got, want := SkillBaseDamage(4, sp, caster, 0, 1000), legado*5/4*115/100; got != want {
		t.Fatalf("sem a Confiança: SkillBaseDamage = %d, want %d", got, want)
	}
}
