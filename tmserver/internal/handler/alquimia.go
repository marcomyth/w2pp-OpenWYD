package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A passiva Alquimia da Huntress.
//
// REGRA DO SERVIDOR, decidida pelo Marco em 16/09/2026 — não é legado. No
// original a Alquimia (skill 84) só abre a máquina de Alquimia da HT; aqui ela
// também vira sorte: a Huntress que aprendeu a skill soma pontos percentuais à
// chance de sucesso de toda máquina de composição (Compositor, +10, acessórios,
// Agatha, Tiny, Shany, Lindy quando sorteia, Ehre, Odin, Alquimia e Extração),
// do refino com poeira, da Pedra Arch e do Âmago de crescimento.
//
// O bônus dobra quando a HT também aprendeu a 8ª skill da mesma árvore (Troca
// de Espírito, skill 87): é o fim da árvore que a Alquimia abre.
//
// Fica de fora a Pedra da Fúria de uso: ela vence em rand()%115 dobrado < 100,
// isto é, já sorteia contra 100 — o teto desta regra —, e o bônus não teria onde
// entrar sem passar de 100.
const (
	alquimiaSkillBit      = 1 << 12 // skill 84, Alquimia
	alquimiaBonus         = 2
	alquimiaBonusCompleto = 4
	alquimiaChanceMax     = 100
)

// bonusAlquimia devolve os pontos de chance que a passiva Alquimia soma para e:
// 0 fora da Huntress ou sem a skill, 2 com ela, 4 com ela e a Troca de Espírito.
func bonusAlquimia(e *world.Entity) int {
	if e == nil || e.Class != 3 || e.LearnedSkill&alquimiaSkillBit == 0 {
		return 0
	}
	if temOitavaDaTroca(e) { // arvore_troca.go
		return alquimiaBonusCompleto
	}
	return alquimiaBonus
}

// chanceComAlquimia soma a Alquimia a uma chance em %, com teto 100, e devolve
// a chance final e quantos pontos o bônus de fato somou.
//
// Chance <= 0 fica como está: 0 é "impossível" no refino e "sem receita" nas
// máquinas, e a Alquimia não inventa possibilidade. Chance que já é 100 ou mais
// também fica como está — já não falha, e baixá-la ao teto mudaria um número que
// o anúncio de quem não é HT mostra hoje. Perto do teto o bônus somado é o que
// coube (99 vira 100 com +1), e é esse +1 que o anúncio imprime.
func chanceComAlquimia(e *world.Entity, chance int) (final, bonus int) {
	b := bonusAlquimia(e)
	if b == 0 || chance <= 0 || chance >= alquimiaChanceMax {
		return chance, 0
	}
	final = chance + b
	if final > alquimiaChanceMax {
		final = alquimiaChanceMax
	}
	return final, final - chance
}

// anuncioPainelMax é o que cabe numa linha do painel (SendClientMessage); o que
// passa disso o cliente corta.
const anuncioPainelMax = protocol.MessageLength - 2

// marcaAlquimia é o "(+2 Alquimia)" que o anúncio põe logo depois do número:
// quem lê "30/43" precisa saber que 41 era a máquina e 2 eram da HT. curta é a
// forma de reserva para quando a linha inteira não cabe no painel.
func marcaAlquimia(bonus int, curta bool) string {
	if curta {
		return fmt.Sprintf(" (+%d Alq.)", bonus)
	}
	return fmt.Sprintf(" (+%d Alquimia)", bonus)
}
