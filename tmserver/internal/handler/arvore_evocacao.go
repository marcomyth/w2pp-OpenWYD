package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// O GOLPE DA EVOCAÇÃO EM JOGADOR (19/09/2026).
//
// REGRA DE SERVIDOR, não paridade. O legado deixa a evocação bater pela conta
// normal de monstro e corta o resultado em 40% (ProcessSecMinTimer.cpp:2234,
// Server.cpp:9949). Aqui o golpe em JOGADOR não passa pela conta: é um número
// próprio por criatura, e a defesa do alvo entra por uma curva.
//
// O motivo é o pedágio da armadura. BASE_GetDamage cobra `dano − defesa×3/2` POR
// UNIDADE contra jogador, e a evocação bate com muitas cabeças de dano pequeno —
// a pior combinação que existe para uma subtração. Medido: um Tigre calibrado
// para tirar 650 de um alvo de 2.233 de defesa tira 1.110 de um mago de 1.500 e
// 169 de um TransKnight de 3.000. Seis vezes e meia de diferença na mesma
// criatura, e nenhum valor na tabela conserta isso — a subtração é o problema,
// não o número. Cortar 40% no fim só multiplica os três resultados.
//
// A curva abaixo é o mesmo desenho com a defesa entrando de forma proporcional:
// na defesa de referência o número da tabela sai inteiro, e para os lados ele
// anda devagar. O mesmo Tigre passa a tirar 778 / 650 / 555 nos três alvos.
//
// O golpe contra MONSTRO não muda: continua na conta do servidor, com o Damage
// que summonBonus escalona (summon.go). São dois números diferentes de propósito
// — o de PvE está calibrado contra a AC dos chefes e não sobreviveria a servir
// também o PvP.

// evocacaoDefesaRef é a defesa de alvo em que os números de evocacaoDano valem
// exatamente. É a MEDIANA da defesa do elenco medido (2.233 a 2.632), e não a
// ficha de um personagem só: centrada aqui, a tabela cai dentro da faixa pedida
// contra todos eles, e não só contra um.
const evocacaoDefesaRef = 2400

// evocacaoMaestriaCheia é a Evocação em que a criatura entrega o número inteiro
// da tabela. É a mesma marca em que summonHeads atinge o teto de cabeças, para
// as duas metades da árvore — quantidade e força — chegarem juntas ao topo.
const evocacaoMaestriaCheia = 320

// evocacaoDano é quanto CADA cabeça tira de um jogador de defesa
// evocacaoDefesaRef, com a Evocação cheia.
//
// O que vale é a coluna da DIREITA, não a da esquerda: o dano por cabeça sozinho
// não diz nada, porque cada criatura põe um número diferente de cabeças em campo
// (summonHeads) e todas batem na mesma cadência. Duas criaturas com o mesmo
// golpe entregam o dobro uma da outra se uma sai em 8 e a outra em 4.
//
// Tigre 650 é número escolhido por quem opera. Os outros sete saem da progressão
// por segundo, resolvidos de trás para frente pela cabeça e pela curva, para a
// coluna da direita crescer do começo ao fim da árvore.
//
// O TETO DO BANDO (21/09/2026). A Succubus era o DOBRO do bando de Tigres, a
// pedido, e o Dragão Negro fora posto para caber entre os dois. Quando o BM
// Evocador finalmente entrou no torneio, em 21/09, ele fez 117 vitórias em 130
// lutas — quase o dobro do segundo colocado. Medido contra o elenco inteiro, o
// bando respondia por 72% do dano dele e tirava 2.995/s SOZINHO, com o dono
// parado, contra uma poção que levanta 2.000/s.
//
// O teto é essa poção. Um bando que passa dela sozinho decide a luta sem que o
// dono jogue e sem que o alvo tenha resposta, porque o dano do bando é paralelo
// ao do dono: ele não ocupa o turno de ninguém. Os dois últimos degraus foram
// trazidos para baixo dela, e o Evocador passou de 117 para 59 vitórias — a
// faixa da Black (58) e do BM Elemental (52).
//
// A progressão continua crescendo do começo ao fim, que é a regra da tabela, mas
// os degraus do topo ficam apertados (1.832 → 1.840 → 1.853). É a saturação
// esperada: com o Tigre já em 1.733 e o teto em ~1.850, as três últimas
// criaturas dividem 120 pontos. Baixar o topo mais do que isso exigiria
// reescalar a árvore inteira, e as seis primeiras não foram medidas como
// problema — quem não tem a 8ª nem chega à maestria cheia em que a tabela vale.
var evocacaoDano = [9]int{
	300,  // 0 Condor         12 cabeças → 1.200/s
	390,  // 1 Javali         10        → 1.300/s
	425,  // 2 Lobo           10        → 1.417/s
	505,  // 3 Urso            9        → 1.515/s
	650,  // 4 Tigre           8        → 1.733/s  (número do operador)
	785,  // 5 Gorila          7        → 1.832/s
	1104, // 6 Dragão Negro    5        → 1.840/s  (era 1.320 → 2.200/s)
	1390, // 7 Succubus        4        → 1.853/s  (era 2.600 → 3.467/s)
	0,    // 8 Invocação Final: sem regra própria, como em summonBonus
}

// evocacaoCadenciaMs é o intervalo entre golpes de uma evocação em JOGADOR.
//
// O golpe em monstro fica em mobAttackCadence (1 s). A diferença é o número de
// cabeças: oito Tigres a 650 de golpe, batendo de segundo em segundo, são
// 5.200 por segundo — mais que a Black, com o Beast Master parado. A 3 s o
// conjunto fica em 1.733, que é a faixa das outras classes.
//
// O legado deixa cada monstro agir a cada 2 s (ProcessSecMinTimer roda duas
// vezes por segundo e varre um quarto do pMob por vez, :2024). Os 3 s são
// escolha de quem opera, mais lentos que o original de propósito.
// É var porque a simulação o varre (simulacao_torneio_test.go).
var evocacaoCadenciaMs = uint32(3000)

// danoDaEvocacaoEmJogador é o golpe de uma cabeça num jogador: o número da
// criatura, escalado pela Evocação do dono, curvado pela defesa do alvo.
//
// A curva é base × 2R/(R+defesa): vale o número cheio na defesa de referência,
// tende ao dobro contra alvo sem armadura nenhuma e cai devagar contra alvo
// blindado. Nunca chega a zero, que é a diferença toda para a subtração.
func danoDaEvocacaoEmJogador(criatura, evocacao, defesa int) int {
	if criatura < 0 || criatura >= len(evocacaoDano) {
		return 0
	}
	base := evocacaoDano[criatura]
	if base <= 0 {
		return 0
	}
	if evocacao < 0 {
		evocacao = 0
	}
	if evocacao > evocacaoMaestriaCheia {
		evocacao = evocacaoMaestriaCheia
	}
	base = base * evocacao / evocacaoMaestriaCheia
	if defesa < 0 {
		defesa = 0
	}
	// Piso de 1: um golpe que chegou e marca zero é lido como falha pelo dono e
	// pelo alvo — a mesma razão de applyTierDefense e absorbBlow (combat.go).
	return max(base*2*evocacaoDefesaRef/(evocacaoDefesaRef+defesa), 1)
}

// evocacaoDoPet devolve o índice de criatura (0..8) de um pet, ou -1. O pet não
// guarda o índice: o que ele carrega é a FACE do template, que generateSummon
// copia para EquipVisual[0] (summon.go), então a volta é pela mesma tabela.
func (d *Dispatcher) evocacaoDoPet(pet *world.Entity) int {
	if pet == nil || pet.Summoner == 0 || pet.Clan != summonClan {
		return -1
	}
	for i := range len(evocacaoDano) - 1 {
		if i >= len(d.summonMobs) || d.summonMobs[i] == nil {
			continue
		}
		if summonTemplateFace(d.summonMobs[i]) == pet.EquipVisual[0] {
			return i
		}
	}
	return -1
}

// golpeDaEvocacaoEmJogador devolve o dano de um pet num jogador e se a regra se
// aplica. Falso deixa o golpe seguir pela conta normal — é o que acontece com
// monstro, e com qualquer criatura fora das oito da árvore (crias de montaria,
// evocações de item).
func (d *Dispatcher) golpeDaEvocacaoEmJogador(dono, pet, alvo *world.Entity) (int, bool) {
	if dono == nil || pet == nil || alvo == nil || !world.IsPlayer(alvo.ID) {
		return 0, false
	}
	criatura := d.evocacaoDoPet(pet)
	if criatura < 0 || evocacaoDano[criatura] <= 0 {
		return 0, false
	}
	return danoDaEvocacaoEmJogador(criatura, effectiveSpecial(dono, 2), int(effectiveAC(alvo))), true
}

// ---------------------------------------------------------------------------
// A EVOCAÇÃO NA CONTAGEM DO PvP (_MSG_Attack.cpp:1342).
//
// No legado, quem bate numa evocação está batendo no DONO dela para tudo que é
// social: ponto de PK, marca de criminoso, guilda e guerra de guilda. O bloco
// "PK - War - Miss" inteiro roda com `Summoner` no lugar do alvo, e é isso que
// impede a evocação de virar um jeito de ferir alguém sem consequência nenhuma.
//
// O que NÃO muda é o cálculo do dano. O golpe num pet continua sendo golpe em
// mob (AC ×1, com o ÷4 que a perfuração já dá ao Clan 4), e não entra no
// percentual de PvP nem na Defesa de Evolução. São duas perguntas diferentes —
// "quanto dói" e "quem responde por isso" — e só a segunda troca de dono aqui.

// donoDaEvocacao devolve o jogador por trás do alvo: o dono, quando target é uma
// evocação viva de alguém em jogo; senão o próprio target.
func donoDaEvocacao(w *world.World, target *world.Entity) *world.Entity {
	if w == nil || target == nil || target.Clan != summonClan || target.Summoner <= 0 {
		return target
	}
	if !world.IsPlayer(target.Summoner) {
		return target
	}
	if m, ok := w.SessionMode(target.Summoner); !ok || m != world.UserPlay {
		return target
	}
	if dono := w.Entity(target.Summoner); dono != nil {
		return dono
	}
	return target
}

// golpeContaComoPvP diz se este golpe entra na contagem social do PvP: em
// jogador, ou na evocação de um jogador que não seja o próprio atacante.
func golpeContaComoPvP(w *world.World, attackerID, tid int, target *world.Entity) bool {
	if world.IsPlayer(tid) {
		return tid != attackerID
	}
	dono := donoDaEvocacao(w, target)
	return dono != target && dono.ID != attackerID
}

// ---------------------------------------------------------------------------
// O QUE A EVOCAÇÃO APANHA DE JOGADOR.
//
// A vida dos bichos (summonBonus, summon.go) foi calibrada para o PvE, em
// 11/09/2026: "aguentar ~12 golpes do Tauron mais forte". Mexer nela para
// afinar o PvP quebraria a caçada, e é o mesmo problema do dano — um número só
// servindo duas pontas que não se parecem.
//
// Então o ajuste fino do PvP entra aqui: um percentual sobre o golpe que um
// JOGADOR dá numa evocação. 100 deixa como está.
//
// Fica só no caminho do jogador (combat.go). O golpe de MONSTRO em pet corre por
// mobAttack, que não passa por aqui, então a resistência em PvE segue intacta —
// que é o ponto todo de ter um botão separado.
var evocacaoDanoRecebidoPct = 130

// danoEmEvocacao aplica esse percentual quando o alvo é uma evocação.
//
// O piso de 1 é o mesmo do resto do pipeline (perfuracao, applyTierDefense,
// absorbBlow): um golpe que chegou e marca zero é lido como falha pelos dois
// lados.
func danoEmEvocacao(target *world.Entity, dmg int) int {
	if dmg <= 0 || target == nil || target.Clan != summonClan || target.Summoner <= 0 {
		return dmg
	}
	if evocacaoDanoRecebidoPct == 100 {
		return dmg
	}
	return max(dmg*evocacaoDanoRecebidoPct/100, 1)
}
