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

// A lança do TK Espada Mágica entra no ramo mágico por ArmaPct; 0 é neutro.
func TestSkillBaseDamageArmaNoRamoMagico(t *testing.T) {
	sp := SkillSpell{InstanceType: 1, InstanceValue: 65} // Ataque da Alma
	caster := SkillCaster{Class: 0, Level: 300, Int: 2500, Special: 255, Magic: 50, DamageMultiPct: 100, LearnedSkill: 1 << 23}
	neutro := SkillBaseDamage(20, sp, caster, 0, 500)
	caster.ArmaPct = 100
	if got := SkillBaseDamage(20, sp, caster, 0, 500); got != neutro {
		t.Fatalf("ArmaPct 100 = %d, want %d (neutro)", got, neutro)
	}
	caster.ArmaPct = 140
	// (255 + 65 + 500 + 300 + 625 + 62) × 300% = 5421; × 1,40 = 7589; × 5/4 = 9486; × 1,15 = 10908.
	if got := SkillBaseDamage(20, sp, caster, 0, 500); got != 10908 {
		t.Fatalf("lança: SkillBaseDamage = %d, want 10908", got)
	}
}
