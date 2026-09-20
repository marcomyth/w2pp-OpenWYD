package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A ÁRVORE ELEMENTAL DO BEAST MASTER (skills 48-55, 19/09/2026).
//
// O BM Elemental é o TK Espada Mágica e a FM Magia Negra misturados, com UMA
// diferença deliberada: ele NÃO tem crítico de mago. Quem tem são só aquelas
// duas (magoCritico, arvore_magia_negra.go), e o preço que o BM paga por isso é
// alto — o crítico sai em 25% dos golpes com INT cheia e multiplica de ×2 a ×4,
// o que vale cerca de +50% de dano médio. É a compensação por ele trazer o
// bando de evocações junto (arvore_evocacao.go).
//
// O que ele ganha no lugar é a ESCOLHA DA ARMA, e cada uma dá um jogo:
//
//   - LANÇA: 120% de dano e roubo de VIDA. O modo de aguentar.
//   - CAJADO DE DUAS MÃOS: 140% de dano e roubo de MANA. O modo de derrubar
//     mago — a Black vive da barra de mana, e drená-la é o que faz dela um
//     alvo em vez de um carrasco.
//
// A arma decide, não a evolução: um BM Mortal que queira o roubo de mana pega o
// cajado, e um Arch que queira se curar pega a lança. Amarrar isso à evolução
// deixaria o Mortal sem resposta nenhuma contra a Black a vida inteira, e não
// por escolha dele.

const (
	skillFeraFlamejante   = 48
	skillEspiritoVingador = 55

	// learnedEspiritoVingador é a 8ª da Elemental (skill 55, bit 55%24 = 7).
	learnedEspiritoVingador = 1 << 7

	elementalLancaPct       = 120
	elementalCajado2MaosPct = 140
	elementalArmaPct        = 100
)

// bmElemental diz se as regras da árvore valem para e: Beast Master, jogador,
// com o Espírito Vingador aprendido. Mesmo portão de tkEspadaMagica e
// fmMagiaNegra — a árvore só vira uma identidade com a 8ª na mão.
func bmElemental(e *world.Entity) bool {
	return e != nil && e.Class == 2 && world.IsPlayer(e.ID) && e.LearnedSkill&learnedEspiritoVingador != 0
}

// skillDeDanoDaElemental são as skills de dano da árvore. Ficam de fora a
// Proteção Elemental (53) e a Aura Bestial (54), que são buff, e o Enfraquecer
// (51), que é debuff puro.
func skillDeDanoDaElemental(skillnum int) bool {
	switch skillnum {
	case 51, 53, 54:
		return false
	}
	return skillnum >= skillFeraFlamejante && skillnum <= skillEspiritoVingador
}

// armaPctElemental: lança 120%, cajado de duas mãos 140%, qualquer outra arma
// 100%. O cajado de UMA mão não entra — o BM que quiser escudo abre mão do
// bônus, que é o custo de carregar a defesa a mais.
func armaPctElemental(e *world.Entity, itemAbility func(world.Item, uint8) int) int {
	if itemAbility == nil {
		return elementalArmaPct
	}
	switch itemAbility(e.Equip[weaponSlotR], efWType) {
	case wtypeLanca:
		return elementalLancaPct
	case wtypeCajadoDuasMaos:
		return elementalCajado2MaosPct
	}
	return elementalArmaPct
}

// rouboDaElemental devolve o que o golpe repõe ao BM e em qual barra.
//
// Reusa as duas funções que já existem, com os mesmos números das árvores de
// origem: rouboDeVida (arvore_espada_magica.go) e rouboDeMana
// (arvore_magia_negra.go). A arma escolhe qual das duas roda, e NUNCA as duas —
// é a escolha que dá ao BM dois jogos em vez de um acúmulo.
//
// restanteMana é quanto ainda cabe no teto do lançamento, como na Magia Negra:
// uma skill que pega vários alvos não enche a barra de uma vez.
func (d *Dispatcher) rouboDaElemental(r combat.Rand, e *world.Entity, dano int, restanteMana int32) (vida, mana int32) {
	if !bmElemental(e) || dano <= 0 {
		return 0, 0
	}
	switch armaPctElemental(e, d.itemAbility) {
	case elementalLancaPct:
		return rouboDeVida(r, e, dano), 0
	case elementalCajado2MaosPct:
		return 0, rouboDeMana(r, e, dano, restanteMana)
	}
	return 0, 0
}

// ---------------------------------------------------------------------------
// A FORÇA ELEMENTAL EM JOGADOR (skill 54, TickType 23).
//
// Ela lança uma Fúria de Gaia sintética de 8 em 8 segundos em todos os inimigos
// de um 3x3 (affect_tick.go). No port nasceu SÓ contra monstro, com o comentário
// "PK-mode/arena attributes are not modeled" — a trava existia porque um tique
// automático que fere jogador precisa das mesmas perguntas que um golpe, e elas
// não estavam prontas na época.
//
// Agora estão, e são estas. Um tique só alcança um jogador quando:
//
//   - o lançador optou pelo PvP (modo PK), como em qualquer golpe;
//   - o alvo não está em cidade nem em zona segura — um tique de área não pode
//     ferir quem entrou na cidade para se proteger;
//   - os dois não são do mesmo grupo nem da mesma guilda;
//   - o lançador não é chaoticamente pior que o alvo (a mesma conta de PKPoint
//     do golpe normal, _MSG_Attack.cpp "PK - War - Miss").
//
// O duelo fica de FORA de propósito: um tique automático que continua batendo
// dentro de uma arena de duelo decide a luta sozinho, e o duelo tem varredura
// própria (duel.go).

// A ordem é das perguntas BARATAS e locais para as caras: o que dá para decidir
// só com as duas entidades vem antes do que precisa consultar o mundo. Além de
// ser mais rápido no caminho quente (um tique de 8 em 8 segundos varrendo até
// nove casas), é o que deixa cada guarda verificável isoladamente — com o gate
// de sessão na frente, todas as outras ficavam inalcançáveis em teste.
func (d *Dispatcher) tiqueAlcancaJogador(w *world.World, caster, target *world.Entity) bool {
	if caster == nil || target == nil || caster.ID == target.ID {
		return false
	}
	if !caster.PKMode {
		return false // o PvP é opt-in, como em qualquer golpe
	}
	if world.Village(target.X, target.Y) >= 0 || world.Village(caster.X, caster.Y) >= 0 {
		return false // ninguém fere nem apanha de dentro da cidade
	}
	if int(caster.PKPoint) <= 10 && int(target.PKPoint) > 10 {
		return false // a mesma conta de caos do golpe comum
	}
	if d.dueling(caster.ID, target.ID) {
		return false // o duelo tem varredura própria (duel.go)
	}
	if m, ok := w.SessionMode(target.ID); !ok || m != world.UserPlay {
		return false
	}
	return !skillSameLeaderOrGuild(w, caster, target)
}

// aplicarBlocoPvP passa um golpe já resolvido pelo mesmo bloco de PvP do golpe
// comum (combat.go): a perfuração do legado, o percentual do painel, a força, a
// Defesa de Evolução, os atributos de PvP, a Garnet e a absorção.
//
// Existe porque a Força Elemental fere por um TIQUE, fora do laço de dano do
// _MSG_Attack, e sem isto ela entregaria o dano CHEIO em jogador — sem o ÷4 e
// sem os 37%, o que a tornaria de longe a coisa mais forte do jogo.
func (d *Dispatcher) aplicarBlocoPvP(_ *world.World, e, target *world.Entity, dmg int, skill bool) int {
	if dmg <= 0 || !world.IsPlayer(target.ID) {
		return dmg
	}
	dmg = perfuracao(target, target.ID, dmg, 0)
	dmg = d.applyPvPRule(dmg, skill)
	dmg = danoDoTransContraHT(e, target, dmg)
	dmg = applyForceDamage(e, target, target.ID, dmg)
	dmg = applyTierDefense(e.ClassMaster, target.ClassMaster, dmg)
	dmg = d.applyPvPStats(e, target, dmg)
	dmg = d.absorverGarnet(e, target, dmg)
	return dmg
}

// ---------------------------------------------------------------------------
// A ABSORÇÃO DA PROTEÇÃO ELEMENTAL (skill 53, com a 8ª aprendida).
//
// A skill já dava resistência aos três elementos — fogo, trovão e gelo, nunca
// sagrado (afeto 25, affect_score.go). Isso é o que ela sempre fez e continua
// fazendo para qualquer BM que a aprenda.
//
// REGRA NOVA, não do legado: quem tem o Espírito Vingador, a 8ª da Elemental,
// ganha por cima uma absorção plana de todo golpe enquanto a proteção estiver de
// pé. Resistência elemental defende de um tipo de dano; a absorção defende de
// todos, e é o que dá ao BM Elemental uma resposta contra o físico — que a
// resistência não cobre.
//
// Vale em PvP e em PvE: é uma defesa do personagem, não uma regra de arena.
const (
	protecaoElementalAfeto  = 25
	protecaoElementalAbsPct = 20
)

// absorcaoDaProtecaoElemental tira a fatia da Proteção Elemental de um golpe.
//
// Exige as DUAS coisas: a 8ª aprendida (bmElemental) e a proteção no ar. Sem o
// afeto não há absorção nenhuma — ela é um buff que se lança e expira, não uma
// passiva que a 8ª liga para sempre.
func absorcaoDaProtecaoElemental(vitima *world.Entity, dano int) int {
	if dano <= 0 || !bmElemental(vitima) {
		return dano
	}
	for i := range vitima.Affect {
		if vitima.Affect[i].Type == protecaoElementalAfeto {
			// Piso de 1, como o resto do pipeline: um golpe que chegou e marca
			// zero é lido como falha pelos dois lados.
			return max(dano-dano*protecaoElementalAbsPct/100, 1)
		}
	}
	return dano
}
