package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/combatrule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// tkDaJanela é a TK da captura de 2026-09-11: FOR 2.802, DES 712, Espada de 2
// mãos (nUnique 48). A captura trazia as três 8ªs, mas um personagem só aprende
// uma; aqui ficam a Trans e a Espada Mágica, porque o bit 7 faria dela uma TK
// Confiança, que perde a DES no bônus (arvore_confianca.go).
func tkDaJanela() (*Dispatcher, *world.Entity) {
	const espada2m = 900
	d := New(Config{ItemUnique: map[int]int{espada2m: 48}})
	e := testPlayerEntity()
	e.Class = 0
	e.Str, e.Dex = 2802, 712
	e.LearnedSkill = 1<<15 | 1<<23
	e.Equip[weaponSlotR] = world.Item{Index: espada2m}
	return d, e
}

// TestBonusDeArmaContaUmaVez: com a regra decidida o bônus de arma entra uma
// vez, seja qual for o número de evoluções; o legado (Kersef) soma uma por
// evolução aprendida.
func TestBonusDeArmaContaUmaVez(t *testing.T) {
	d, e := tkDaJanela()
	uma := dexStrWeaponBonus(2802, 712, 0.56, 0.60)
	if got := d.classWeaponDamage(e); got != uma {
		t.Errorf("regra padrão: bônus de arma = %d, want %d (uma vez)", got, uma)
	}

	d.combatRules.WeaponDamageGrants = 2
	if got := d.classWeaponDamage(e); got != 2*uma {
		t.Errorf("duas vezes: bônus de arma = %d, want %d", got, 2*uma)
	}

	d.combatRules = combatrule.Kersef()
	if got := d.classWeaponDamage(e); got != 2*uma {
		t.Errorf("Kersef: bônus de arma = %d, want %d (uma por evolução)", got, 2*uma)
	}
}

// TestBonusDeArmaNaoInventaEvolucao: o teto nunca dá mais do que as evoluções
// aprendidas — com só a Trans, 3× no painel continua sendo uma vez.
func TestBonusDeArmaNaoInventaEvolucao(t *testing.T) {
	d, e := tkDaJanela()
	e.LearnedSkill = 1 << 15
	d.combatRules = combatrule.Kersef()
	if got, uma := d.classWeaponDamage(e), dexStrWeaponBonus(2802, 712, 0.56, 0.60); got != uma {
		t.Errorf("uma evolução com o teto 3: bônus = %d, want %d", got, uma)
	}
	e.LearnedSkill = 0
	if got := d.classWeaponDamage(e); got != 0 {
		t.Errorf("sem evolução: bônus = %d, want 0", got)
	}
}

// TestBonusDeArmaMudaOAtaqueAoVivo: trocar o botão no painel refaz o Ataque de
// quem está em jogo, sem relogar.
func TestBonusDeArmaMudaOAtaqueAoVivo(t *testing.T) {
	d, e := tkDaJanela()
	e.BaseStr, e.BaseDex = 2802, 712
	d.combatRules = combatrule.Kersef()
	d.refreshScore(e)
	legado := e.Damage

	d.combatRules.WeaponDamageGrants = 1
	d.refreshScore(e)
	if want := legado - dexStrWeaponBonus(2802, 712, 0.56, 0.60); e.Damage != want {
		t.Errorf("Ataque com o bônus 1× = %d, want %d (legado %d menos uma vez o bônus)", e.Damage, want, legado)
	}
}
