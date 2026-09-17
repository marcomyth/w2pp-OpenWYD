package combine

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Reforma dos acessórios (2026-09-16). Nada disto existe no legado: a +10 e o
// +12 do Odin só aceitam as peças de armadura e as armas (nPos 2-192), e não há
// evolução de acessório em máquina nenhuma.
//
// A liberação é por lista de índices, nunca pelo espaço do equipamento. Os
// espaços de acessório também guardam orbs, pedras de quest, Sephirot, a Pedra
// Amunra e as Pedras Espirituais — liberar o nPos inteiro levaria todos juntos.

// acessoriosAteMais15 são os acessórios que a +10 aceita e o Odin leva até +15.
// Os brincos entram com os status de hoje; a reforma deles vem depois. Os seis de
// bracelete (507, 510-514) e os Amuletos de Prata e de Ouro entraram em 17/09, a pedido do Marco.
var acessoriosAteMais15 = map[int16]bool{
	591: true, 592: true, 593: true, 594: true, 595: true, // Brincos
	507: true, 510: true, 511: true, 512: true, 513: true, 514: true, // Bracelete de Hércules, Athena, Titã, Gaia, Zeus e Hecate
	551: true, 552: true, 553: true, 554: true, // Amuletos de Prata
	555: true, 556: true, 557: true, 558: true, // Amuletos de Ouro
	559: true, 560: true, 561: true, 562: true, // Amuletos Místicos
	567: true, 568: true, 569: true, 570: true, // Amuletos Arcanos
	661: true, 662: true, 663: true, // Ankhs
	762: true, 763: true, 764: true, 765: true, 766: true, 767: true, 768: true, // Planetas
	1738: true, // Amuleto dos Amantes
}

// AcessorioAteMais15 diz se o item é um acessório que a +10 e o Odin aceitam.
func AcessorioAteMais15(index int16) bool { return acessoriosAteMais15[index] }

// Joias que um acessório aceita na +10: as quatro, sempre quatro iguais. Toda
// peça +10 guarda a joia usada, e o bônus dela vale em qualquer espaço do
// equipamento (gem_bonus.go). Até 17/09 Esmeralda e Garnet ficavam de fora; o
// Marco liberou as quatro, e a Gema troca a joia gravada depois (useBaseGem).
const (
	joiaDiamante  int16 = 2441 // +8% de drop por peça
	joiaEsmeralda int16 = 2442 // perfuração
	joiaCoral     int16 = 2443 // +2% de XP por peça
	joiaGarnet    int16 = 2444 // absorção
	pedraDoSabio  int16 = 1774
)

// AcessorioMais10Recipe é a receita da +10 para acessório: dois iguais em +9,
// a Pedra do Sábio e quatro joias iguais, de qualquer uma das quatro.
//
// A +10 das armas não confere o +9 porque o legado deixa isso para a janela do
// cliente. Aqui o servidor confere: é uma receita nova, e não há janela que
// tenha sido testada com acessório.
func AcessorioMais10Recipe(items []world.Item) bool {
	if !validItems(items, 7) || !AcessorioAteMais15(items[0].Index) || items[1].Index != items[0].Index {
		return false
	}
	if refine.Level(items[0]) != 9 || refine.Level(items[1]) != 9 || items[2].Index != pedraDoSabio {
		return false
	}
	return quatroJoiasIguais(items, joiaDiamante, joiaEsmeralda, joiaCoral, joiaGarnet)
}

// evolucoes leva cada degrau da escada ao seguinte, na mesma linha: a mesma
// árvore de skill nos amuletos, o mesmo status (MP, HP, crítico) nas pedras.
var evolucoes = map[int16]int16{
	563: 559, 564: 560, 565: 561, 566: 562, // Amuleto de Cristal → Místico
	654: 658, 655: 659, 656: 660, // Pedra Necromântica → Gema da Siren
	658: 661, 659: 662, 660: 663, // Gema da Siren → Ankh
}

// EvolucaoAcessorio devolve o item em que um acessório +9 evolui na +10.
func EvolucaoAcessorio(index int16) (int16, bool) {
	next, ok := evolucoes[index]
	return next, ok
}

// EvolucaoRecipe é a evolução na +10: o item em +9, uma cópia dele como
// sacrifício (em qualquer refino), a Pedra do Sábio e quatro joias iguais. A forma é a da +10 para o jogador não aprender outra janela; a
// joia não fica gravada, porque o item evoluído sai em +0.
func EvolucaoRecipe(items []world.Item) bool {
	if !validItems(items, 7) || items[1].Index != items[0].Index || items[2].Index != pedraDoSabio {
		return false
	}
	if _, ok := EvolucaoAcessorio(items[0].Index); !ok || refine.Level(items[0]) != 9 {
		return false
	}
	return quatroJoiasIguais(items, joiaDiamante, joiaEsmeralda, joiaCoral, joiaGarnet)
}

// quatroJoiasIguais confere as células 3-6: a mesma joia nas quatro, e uma das
// permitidas.
func quatroJoiasIguais(items []world.Item, permitidas ...int16) bool {
	want := items[3].Index
	ok := false
	for _, p := range permitidas {
		if want == p {
			ok = true
		}
	}
	if !ok {
		return false
	}
	for i := 4; i < 7; i++ {
		if items[i].Index != want {
			return false
		}
	}
	return true
}

// arcanoDoMistico leva o Amuleto Místico de cada árvore ao Arcano da mesma.
var arcanoDoMistico = map[int16]int16{559: 567, 560: 568, 561: 569, 562: 570}

// OdinArcanoRecipe é a evolução do Místico +15 em Arcano no Odin: o Místico na
// célula 0, as células 1 e 2 vazias e as quatro Pedras Secretas (Água, Terra,
// Sol, Vento) nas células 3 a 6. Devolve o Arcano da mesma árvore.
//
// Não colide com nenhuma receita do Odin: as numeradas pedem outra coisa na
// célula 0 ou na 2, e a Composição de Sets exige a célula 1 ocupada.
func OdinArcanoRecipe(items []world.Item) (int16, bool) {
	if len(items) < 8 {
		return 0, false
	}
	arcano, ok := arcanoDoMistico[items[0].Index]
	if !ok || refine.Level(items[0]) != 15 || !items[1].Empty() || !items[2].Empty() || !items[7].Empty() {
		return 0, false
	}
	for i, pedra := range [4]int16{5334, 5335, 5336, 5337} {
		if items[3+i].Index != pedra {
			return 0, false
		}
	}
	return arcano, true
}
