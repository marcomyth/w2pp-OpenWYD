//go:build simulacao

package handler

import (
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Abre a conta de um golpe de cada skill da HT no TK, passo a passo.
func TestSimulacaoContaAberta(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	ht := sm.montar(xorimpas, 1, buffsHT())
	tk := sm.montar(porradeiro, 2, nil)
	l := &lado{e: ht, cd: map[int]int64{}}
	t.Logf("arma (weaponDamage) = %d, Special = %v", sm.d.weaponDamage(ht), ht.Special)
	for _, sk := range []int{skillLaminaDasSombras, skillGolpeFelino, skillTempestadeDeFlechas} {
		cast, _ := sm.cast(l, tk, sk)
		sp := combat.SkillSpell{InstanceType: cast.spell.InstanceType, InstanceValue: cast.spell.InstanceValue}
		c := combat.SkillCaster{Class: 3, Level: int(ht.Level), Str: int(effectiveStr(ht)), Damage: int(sm.d.effectiveDamage(ht)), Special: cast.special, Mortal: true, LearnedSkill: ht.LearnedSkill}
		base := combat.SkillBaseDamage(sk, sp, c, 0, int(sm.d.weaponDamage(ht)))
		vals := []int{}
		for range 5 {
			tk.HP = 14277
			vals = append(vals, sm.d.resolveSkillHit(sm.w, ht, tk, tk.ID, sk, cast))
		}
		t.Logf("skill %d: base %d; resolveSkillHit no TK (antes de crítico, perfuração e PvP) = %v", sk, base, vals)
		d := vals[0]
		p := perfuracao(tk, tk.ID, d, 0)
		t.Logf("   perfuracao(%d) = %d; applyPvPRule = %d", d, p, sm.d.applyPvPRule(p, true))
	}
	_ = world.MaxUser
}
