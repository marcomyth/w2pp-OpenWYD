package handler

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// A corrida do Acampamento Troll (corrida.go): o acampamento murado a leste de
// Erion, onde o Troll Enigma ficava na jaula. O líder entrega a Chave do Rei Orc
// ao Xamã Troll, do lado de fora do muro oeste; o grupo cai no meio do acampamento
// com 15 minutos, os mesmos tempos do Castelo Orc.
//
// A chave é a mesma do Castelo Orc desde 16/09/2026, pedido do Marco: o jogador
// escolhe onde gastá-la ("Trolls ou Orcs, o que vamos caçar hoje?"). Por isso não
// há sorteio próprio: a Chave dos Trolls (3223) nunca chegou ao cliente e voltou a
// ser o Cupom da Sorte do catálogo.
//
// O acampamento não tem portão que o servidor desenhe, e ninguém anda até lá
// dentro: o grupo entra por teleporte, e durante a corrida estranhos não passam da
// caixa. A caixa é só o miolo, dentro do muro (x 2638-2670, y 1967-2003 no
// HeightMap), para não varrer quem caça os Trolls do mundo aberto em volta: os
// blocos deles (700-715) ficam nas linhas y 1960 e 2010, do lado de fora.
const (
	// gradeAcampamentoTroll é o EF_GRADE0 do Xamã Troll (Merchant 100); nenhum
	// template do legado usa o 41, e o 40 é o do Xamã Orc.
	gradeAcampamentoTroll = 41

	acampamentoTrollNPCTemplate = "ATroll_Xama"
)

var acampamentoTrollSpec = corridaSpec{
	nome: "acampamento troll",

	chave:       itemChaveCasteloOrc,
	grauNPC:     gradeAcampamentoTroll,
	npcTemplate: acampamentoTrollNPCTemplate,
	npc:         [2]int16{2633, 1979},

	caixa:   areaBox{2638, 1967, 2670, 2003},
	entrada: [2]int16{2651, 1983},
	saida:   [2]int16{2633, 1985},

	genFirst:    world.AcampamentoTrollGenFirst,
	genLast:     world.AcampamentoTrollGenLast,
	genBoss:     world.AcampamentoTrollGenFirst,
	genSeguidor: world.AcampamentoTrollGenFirst + 1,

	duracao:      15 * 60,
	saque:        2 * 60,
	seguidorCada: 30,
	abandono:     60,

	textos: corridaTextos{
		ocupada:  "Um grupo já está no acampamento. Volte em %d min.",
		soLider:  "Só o líder do grupo pode abrir o acampamento.",
		traga:    "Traga a %s para abrir o acampamento.",
		aberta:   "O acampamento é de vocês por 15 minutos.",
		chegada:  "Acampamento Troll: 15 minutos. Derrube o Troll Enigma.",
		estranho: "Um grupo abriu o Acampamento Troll. Volte em 15 minutos.",
		fim:      "A corrida do Acampamento Troll terminou.",
		relogio:  "Acampamento Troll: %d min",
		bossCaiu: "O Troll Enigma caiu! 2 minutos para o saque.",
	},
}
