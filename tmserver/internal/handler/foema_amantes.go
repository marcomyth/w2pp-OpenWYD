package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// Cura da Foema com o Amuleto dos Amantes (reforma dos acessórios, 2026-09-16).
// Não existe no legado.
//
// A Foema que aprendeu Renascimento — a oitava skill da árvore de cura — e está
// com o Amuleto dos Amantes equipado cura 30% a mais com Cura e Recuperar. O
// bônus entra ANTES do teto de 1100/2200, por decisão da equipe: quem já cura no
// teto não ganha nada com ele.
const (
	classFoema          = 1
	skillCura           = 27
	skillRecuperar      = 29
	skillRenascimento   = 31
	itemAmuletoAmantes  = 1738
	curaAmantesPercento = 130
)

// foemaAmantesHeal aplica o bônus à cura bruta de uma skill, antes do teto.
func foemaAmantesHeal(caster *world.Entity, skillnum, heal int) int {
	if caster == nil || caster.Class != classFoema || (skillnum != skillCura && skillnum != skillRecuperar) {
		return heal
	}
	if caster.LearnedSkill&learnedSkillBit(skillRenascimento) == 0 || !vesteItem(caster, itemAmuletoAmantes) {
		return heal
	}
	return heal * curaAmantesPercento / 100
}

// vesteItem diz se o item está em algum espaço do equipamento.
func vesteItem(e *world.Entity, index int16) bool {
	for _, it := range e.Equip {
		if it.Index == index {
			return true
		}
	}
	return false
}
