package npcgener

// Classificação de bloco: o que um índice do NPCGener.txt é no mundo.
//
// Estas regras nasceram em tmserver/internal/world/generator.go, onde o boot as
// usa para decidir o que povoar. Elas vieram para cá porque o painel precisa da
// MESMA resposta e a regra de pacote interno do Go impede o webServer de
// importar de dentro do tmserver — e duas cópias da pergunta "isto nasce?" é
// justamente a doença que a lista de monstros do painel existe para curar.
//
// São funções PURAS do índice do bloco: não leem estado de mundo, não leem
// arquivo, não dependem de ordem de boot. O recorte foi só isso; nenhum
// comportamento mudou na mudança de lugar, e o world continua exportando os
// mesmos nomes, agora delegando para aqui.
//
// O índice é a posição do bloco no arquivo como Load devolve (blocos sem Leader
// são descartados), que é o mesmo generator_index que o npc_definition usa.

// Water-dungeon generator bases (WATER_N/M/A_INITIAL, Basedef.h:361-363). Each
// base owns 12 consecutive NPCGener blocks: +0..+7 are the eight numbered rooms
// and +8..+11 the four boss candidates.
const (
	WaterGenBaseN = 171
	WaterGenBaseM = 10
	WaterGenBaseA = 183

	// waterGenSpan is how many blocks each dungeon owns.
	waterGenSpan = 12
)

const (
	// KefraBossGenIndex is KEFRA_BOSS (Basedef.h:475), the NPCGener block with
	// special fixed-range / fixed-position combat rules in CMob.cpp.
	KefraBossGenIndex = 396
	// KefraGuardLast is KEFRA_MOB_END (Basedef.h:477): the Kefra's guards are
	// blocks KefraBossGenIndex+1 .. KefraGuardLast (397-400).
	KefraGuardLast = 400
)

// SecretRoomGenFirst/Last bound the Sala Secreta blocks (Basedef.h:374-420): the
// three card sets N, M and A, ten blocks each — eight sala blocks, then two boss
// templates. handler/carta.go spawns the whole range when a Carta de Duelo is
// used and the run's own sweep clears it.
const (
	SecretRoomGenFirst = 2395
	SecretRoomGenLast  = 2424
)

// SecretRoomStrayGenFirst/Last são os blocos 81-97 do NPCGener.txt: Krill e
// ChaosOrc__ que começam dentro da caixa da Sala Secreta, com MinuteGenerate -1.
// O legado nunca os gera.
const (
	SecretRoomStrayGenFirst = 81
	SecretRoomStrayGenLast  = 97
)

// CasteloOrcGenFirst/Last bound the Castelo Orc quest blocks, appended at the
// end of NPCGener.txt.
const (
	CasteloOrcGenFirst = 6099
	CasteloOrcGenLast  = 6115
)

// AcampamentoTrollGenFirst/Last delimitam os blocos da quest do Acampamento
// Troll, logo depois dos do Castelo Orc no fim do NPCGener.txt.
const (
	AcampamentoTrollGenFirst = 6116
	AcampamentoTrollGenLast  = 6127
)

// eventOwnedGenerators are NPCGener blocks whose mobs are props of a scripted
// war, not world population: as torres da guerra de guilda, as do RvR, as reais
// e as três do castelo de Noatum.
var eventOwnedGenerators = map[int]bool{
	23:   true,
	24:   true,
	25:   true,
	26:   true,
	1078: true,
	4236: true,
	4237: true,
	4238: true,
	4239: true,
}

// coliseuGenerators são os 26 blocos de população da região Coliseu.
var coliseuGenerators = map[int]bool{
	0: true, 1: true, 2: true, 5: true, 6: true, 7: true,
	4854: true, 4855: true, 4856: true, 4857: true, 4858: true,
	4859: true, 4860: true, 4861: true, 4862: true, 4863: true,
	4865: true, 4866: true, 4867: true, 4868: true, 4869: true,
	4870: true, 4871: true, 4872: true, 4873: true, 4874: true,
}

// IsKefraGenerator reports whether an NPCGener block is the Kefra (396) or one
// of its guards (397-400).
func IsKefraGenerator(idx int) bool {
	return idx >= KefraBossGenIndex && idx <= KefraGuardLast
}

// IsWaterDungeonGenerator reports whether an NPCGener block belongs to a
// Pergaminho da Água room. Estes blocos nascem sob demanda, quando um grupo abre
// a sala com o pergaminho.
func IsWaterDungeonGenerator(idx int) bool {
	for _, base := range [...]int{WaterGenBaseN, WaterGenBaseM, WaterGenBaseA} {
		if idx >= base && idx < base+waterGenSpan {
			return true
		}
	}
	return false
}

// IsSecretRoomGenerator reports whether a block belongs to the Sala Secreta.
func IsSecretRoomGenerator(idx int) bool {
	return idx >= SecretRoomGenFirst && idx <= SecretRoomGenLast
}

// IsSecretRoomStrayGenerator diz se um bloco é um dos 81-97 da Sala Secreta.
func IsSecretRoomStrayGenerator(idx int) bool {
	return idx >= SecretRoomStrayGenFirst && idx <= SecretRoomStrayGenLast
}

// IsColiseuGenerator diz se um bloco é um dos 26 do Coliseu.
func IsColiseuGenerator(idx int) bool { return coliseuGenerators[idx] }

// IsCasteloOrcGenerator reports whether a block belongs to the Castelo Orc quest.
func IsCasteloOrcGenerator(idx int) bool {
	return idx >= CasteloOrcGenFirst && idx <= CasteloOrcGenLast
}

// IsAcampamentoTrollGenerator diz se um bloco é da quest do Acampamento Troll.
func IsAcampamentoTrollGenerator(idx int) bool {
	return idx >= AcampamentoTrollGenFirst && idx <= AcampamentoTrollGenLast
}

// IsEventOwnedGenerator reports whether a block belongs to a scripted event
// rather than to the world population.
func IsEventOwnedGenerator(idx int) bool {
	return eventOwnedGenerators[idx] || IsSecretRoomGenerator(idx) || IsSecretRoomStrayGenerator(idx) ||
		IsColiseuGenerator(idx) || IsCasteloOrcGenerator(idx) || IsAcampamentoTrollGenerator(idx)
}

// Classe é o que um bloco é para quem olha a lista de monstros.
//
// A ordem importa: um bloco cai na PRIMEIRA classe que o reconhece, e as classes
// mais específicas vêm antes. O Kefra é chefe mesmo estando fora de evento; a
// Água é masmorra mesmo nascendo sob demanda como um evento nasce.
type Classe uint8

const (
	// ClasseMundo é o caso comum: o bloco é povoado no boot e reposto pelo
	// relógio, então o monstro está no mapa para quem passar por ali.
	ClasseMundo Classe = iota
	// ClasseMasmorra é o Pergaminho da Água: nasce quando um grupo abre a sala.
	// O monstro EXISTE e paga XP — foi onde a medição de 20/09 foi feita —, só
	// não está de pé o tempo todo.
	ClasseMasmorra
	// ClasseChefe é o Kefra e os quatro guardas, que voltam por semana.
	ClasseChefe
	// ClasseEvento é a Sala Secreta, o Coliseu, as torres de guerra, o Castelo
	// Orc e o Acampamento Troll: só existem enquanto o evento roda. Foi tratar um
	// destes como campo aberto que estragou o modelo de XP em 16/09.
	ClasseEvento
)

// ClasseDoBloco classifica um índice de bloco.
func ClasseDoBloco(idx int) Classe {
	switch {
	case IsKefraGenerator(idx):
		return ClasseChefe
	case IsWaterDungeonGenerator(idx):
		return ClasseMasmorra
	case IsEventOwnedGenerator(idx):
		return ClasseEvento
	default:
		return ClasseMundo
	}
}

// NasceNoMundo diz se o bloco põe monstro no mapa sem que alguém dispare um
// evento. É o filtro padrão da lista de monstros do painel: tira o que só existe
// com carta, com guerra ou com quest, e mantém a masmorra e o chefe, que nascem
// de verdade e pagam XP.
func NasceNoMundo(idx int) bool { return ClasseDoBloco(idx) != ClasseEvento }

// Nome é a classe escrita como o painel a mostra.
func (c Classe) Nome() string {
	switch c {
	case ClasseMasmorra:
		return "masmorra"
	case ClasseChefe:
		return "chefe"
	case ClasseEvento:
		return "evento"
	default:
		return "mundo"
	}
}
