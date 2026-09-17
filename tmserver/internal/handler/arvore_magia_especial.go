package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// Árvore MAGIA ESPECIAL da Foema (skills 40-47, maestria Special[3]) — REGRAS DO
// SERVIDOR, decididas pelo Marco em 17/09/2026.
//
// A "FM Cancelamento" é a Foema com o Cancelamento (47, a 8ª da árvore)
// aprendido: a Foema física, que bufa e cancela. O dano dela passa a sair da
// INTELIGÊNCIA com a Destreza, no lugar da Força, porque a mana alta é o que
// alimenta o Controle de Mana (46) — e com a 8ª o Controle segura ainda mais.
// Duas espadas, dois machados ou garra cancelam 3 alvos; arma de 1 mão com
// escudo cancela 1. Os buffs dela valem em dobro nela mesma e o normal nos
// aliados, e a Velocidade só dá velocidade de ataque e crítico nela.
//
// Os nomes 41 e 42 estão trocados no SkillData.csv: pelos livros, a 41 é a
// Velocidade e a 42 é o Teleporte.
const (
	skillNevoaVenenosa = 40
	skillVelocidade    = 41
	skillEscudoMagico  = 43
	skillArmaMagica    = 44
	skillToqueDeAthena = 45
	skillCancelamento  = 47

	learnedCancelamento = 1 << 23 // skill 47

	wtypeUmaMao          = 1  // espadas, adagas e maças de 1 mão
	wtypeMachadoUmaMaoFM = 11 // machados e martelos de 1 mão

	affectVelocidade   = 2
	affectArmaMagica   = 9
	affectEscudoMagico = 11
	affectToqueAthena  = 15
)

// fmCancelamento diz se as regras da árvore valem para e.
func fmCancelamento(e *world.Entity) bool {
	return e != nil && e.Class == 1 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedCancelamento != 0
}

// ---------------------------------------------------------------------------
// Quantos alvos o Cancelamento (47) pega:
//
//	duas espadas, dois machados ou garra ... 3
//	uma arma de 1 mão com escudo ........... 1
//	qualquer outra coisa ................... 2
//
// e um a menos se a Destreza dela for menor que um quarto da INT — a trava da
// "black-cancel", a full INT que só quer cancelar. A CanceleiVoce de referência
// (INT 2.147, DES 712) passa; uma full INT de DES 12 não. O mínimo é sempre 1.
//
// A garra é arma de Huntress: só conta do Arch para cima, que é quem o
// BASE_CanEquip do legado deixa equipar arma de outra classe.
const (
	cancelAlvosDuasArmas = 3
	cancelAlvosPadrao    = 2
	cancelAlvosEscudo    = 1
	cancelDestrezaFator  = 4 // a Destreza precisa ser pelo menos um quarto da INT
)

func alvosDoCancelamento(e *world.Entity, itemAbility func(world.Item, uint8) int) int {
	if itemAbility == nil {
		return cancelAlvosPadrao
	}
	direita := itemAbility(e.Equip[weaponSlotR], efWType)
	esquerda := itemAbility(e.Equip[weaponSlotL], efWType)
	garra := direita == wtypeGarra || esquerda == wtypeGarra
	duasArmas := direita == esquerda && (direita == wtypeUmaMao || direita == wtypeMachadoUmaMaoFM)
	alvos := cancelAlvosPadrao
	switch {
	case duasArmas, garra && e.ClassMaster != classMasterMortal:
		alvos = cancelAlvosDuasArmas
	case direita != 0 && esquerda == 0 && !e.Equip[weaponSlotL].Empty():
		alvos = cancelAlvosEscudo // arma de 1 mão com escudo: o escudo não tem EF_WTYPE
	}
	if int32(effectiveDex(e))*cancelDestrezaFator < int32(effectiveInt(e)) {
		alvos-- // pouca Destreza para o tamanho da INT
	}
	return max(alvos, 1)
}

// ---------------------------------------------------------------------------
// Buffs em dobro nela: Velocidade (2), Arma Mágica (9), Escudo Mágico (11) e
// Toque de Athena (15) valem 200% na FM Cancelamento e 100% em qualquer outro
// jogador. A conta é a mesma do applyAffectScore, somada uma segunda vez.
//
// Só nela, e só com a 8ª, a Velocidade dá também velocidade de ataque e crítico.
const (
	velocidadeAtaque  = 30 // pontos de velocidade de ataque
	velocidadeCritico = 25 // byte do crítico: +10% na janela
)

func applyPassivasDaEspecial(e *world.Entity, bitDaFoema bool) {
	if !fmCancelamento(e) {
		return
	}
	for i := range e.Affect {
		af := e.Affect[i]
		value, level := int32(af.Value), int32(af.Level)
		switch af.Type {
		case affectVelocidade:
			e.AffRunSpeed += value
			e.AffAttackSpeed += velocidadeAtaque
			e.AffCritical += velocidadeCritico
		case affectArmaMagica:
			add := (level*5/20 + value) * 3 / 2
			if bitDaFoema {
				add *= 3
			}
			e.AffDamage += add
			e.AffMagic += 5
		case affectEscudoMagico:
			e.AffAC += level/3 + value
		case affectToqueAthena:
			v := int16(level/10 + value)
			for k := range e.AffSpecial {
				e.AffSpecial[k] += v
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Controle de Mana (46) com o Cancelamento aprendido: o divisor sobe, e a parte
// do golpe que chega à vida cai de ~30% para ~20%. O resto continua saindo da
// mana, que é o que a INT alta dela paga.
const manaControlDivisorCancel = 82
