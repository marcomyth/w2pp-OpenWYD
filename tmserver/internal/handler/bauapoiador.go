package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os baús do apoiador: o brinde que acompanha cada pacote de apoio, sorteado
// pelo mesmo caminho dos baús do legado (baus.go) mas com tabela própria.
//
// Nada disto existe no legado — é regra deste servidor, decidida com o Marco em
// 18/09/2026. São três tabelas numa escada de três degraus de preço:
//
//	Bronze    (até R$ 49,90)   sem ovo e sem âmago de Equipado: o topo é o donate
//	Apoiador  (R$ 99,90 ao Lenda)  ganha o âmago de Equipado e o ovo
//	Supremo   (R$ 799,90)      o mesmo com o topo em dobro e as pilhas maiores
//
// As chances vivem em CENTÉSIMOS DE POR CENTO, somando 10.000, e não em
// porcentagem inteira como as tabelas do legado. O motivo está em
// chestDenominator: o prêmio mais raro é 0,05%, e a resolução de 1% do legado
// não sabe escrever isso — escreveria 1%, que ao longo dos 64 baús de um Supremo
// vira quase metade das compras com ovo em vez de quase nenhuma.
//
// Um invariante que os testes fixam: o Supremo é maior ou igual ao Apoiador em
// TODO prêmio, por baú. Um baú de topo que pague menos de alguma coisa que o baú
// mais barato é um defeito, e é fácil de introduzir sem querer ao mexer numa
// linha só — a soma tem de fechar 10.000, então subir um prêmio baixa outro.
//
// Ajuste de 20/09/2026, com 128 Supremos abertos em jogo: o âmago de Equipado
// subiu metade nos dois baús pagos — Supremo 3,6% -> 5,4%, Apoiador 1,8% ->
// 2,7%. O que desceu para pagar foi a Poeira no Supremo e o Pergaminho no
// Apoiador, e não o contrário, porque são os dois pontos onde cada baú tinha
// folga sobre o de baixo: mexer no outro quebraria o invariante logo acima.
const (
	itemBauBronze   = 3304
	itemBauApoiador = 3305
	itemBauSupremo  = 3306
)

// Os índices emprestados para o teste em jogo. São três baús do legado que o
// catálogo tem, o cliente desenha e NENHUM código abre: os Grandes Baús do
// Tesouro nunca foram portados e não têm efeito nenhum no catálogo, então não há
// comportamento a atropelar.
//
// CORREÇÃO DE 24/09/2026: o texto aqui dizia "enquanto 3304-3306 não existem no
// cliente", e isso ESTAVA ERRADO para o cliente que está em uso. A
// client-planejadora mediu o `Itemname.bin` e o `ItemList.bin` da Hanna: os
// registros 3304, 3305 e 3306 estão lá, com malha e ícone, e o cliente novo usa
// os mesmos arquivos. O "não existem" foi medido em OUTRO cliente, e ficou aqui
// sem dizer qual — que é o que o fez mentir depois.
//
// A lição, para o próximo comentário sobre arquivo de cliente: dizer QUAL cliente
// e QUANDO. "O cliente" não é um só.
//
// Os emprestados ficam por enquanto porque o mapa aceita os dois e não custa
// nada, e porque um teste que roda num código diferente do que vai para produção
// não testa o que vai para produção.
const (
	itemBauBronzeTeste   = 4900 // Grande_Baú_do_Tesouro_I
	itemBauApoiadorTeste = 4901 // Grande_Baú_do_Tesouro_II
	itemBauSupremoTeste  = 4902 // Grande_Baú_do_Tesouro_III
)

// Os prêmios, com o índice do catálogo ao lado do nome que o jogador lê.
const (
	itemPoeiraLacto = 413  // Poeira de Lactolerium
	itemPergaAguaN1 = 3173 // Pergaminho da Água (N) LV1
	itemAmagoLeveN  = 2398 // Âmago de Cavalo Leve N
	itemAmagoLeveB  = 2403 // Âmago de Cavalo Leve B
	itemAmagoEquipN = 2399 // Âmago de Cavalo Equip N
	itemAmagoEquipB = 2404 // Âmago de Cavalo Equip B
	itemOvoLeveN    = 2308 // Ovo de Cavalo Leve N
	itemOvoLeveB    = 2313 // Ovo de Cavalo Leve B
	itemOvoEquipN   = 2309 // Ovo de Cavalo Equip N
	itemOvoEquipB   = 2314 // Ovo de Cavalo Equip B
	itemMoeda1KK    = 4026 // Moeda de Prata (1Mi)
	itemMoeda5KK    = 4027 // Moeda de Prata (5Mi)
	itemBauExp      = 4140 // Baú de Experiência
)

// bauApoiadorTotal é o denominador das três tabelas.
const bauApoiadorTotal = 10000

// bauBronzeTable atende Iniciante e Bronze. Não dá ovo nem âmago de Equipado
// (decisão do Marco, 18/09): ovo é do Prata para cima, e o teto daqui é o donate.
var bauBronzeTable = []chestPrize{
	stackPrize(2700, itemPoeiraLacto, 3),
	stackPrize(5400, itemPergaAguaN1, 3),
	stackPrize(7100, itemAmagoLeveN, 10),
	stackPrize(8400, itemAmagoLeveB, 10),
	plainPrize(9400, itemMoeda1KK),
	plainPrize(9800, itemMoeda5KK),
	plainPrize(10000, itemRCoin100),
}

// bauApoiadorTable atende do Prata ao Lenda. Sem a moeda de 1KK — no baú pago
// não cai troco — e com o Baú de Experiência entre os principais.
var bauApoiadorTable = []chestPrize{
	stackPrize(2400, itemPoeiraLacto, 3),
	stackPrize(4710, itemPergaAguaN1, 3),
	stackPrize(6210, itemAmagoLeveN, 10),
	stackPrize(7510, itemAmagoLeveB, 10),
	stackPrize(8510, itemBauExp, 3),
	plainPrize(9310, itemMoeda5KK),
	plainPrize(9710, itemRCoin100),
	stackPrize(9890, itemAmagoEquipN, 10),
	stackPrize(9980, itemAmagoEquipB, 10),
	plainPrize(9995, itemOvoEquipN),
	plainPrize(10000, itemOvoEquipB),
}

// bauSupremoTable atende o Supremo: degraus 4 e 5 em dobro, âmago em pilha de 12
// e o Lac só na pilha de 5.
var bauSupremoTable = []chestPrize{
	stackPrize(2400, itemPergaAguaN1, 3),
	stackPrize(4420, itemPoeiraLacto, 5),
	stackPrize(5920, itemAmagoLeveN, 12),
	stackPrize(7220, itemAmagoLeveB, 12),
	stackPrize(8220, itemBauExp, 3),
	plainPrize(9020, itemMoeda5KK),
	plainPrize(9420, itemRCoin100),
	stackPrize(9780, itemAmagoEquipN, 12),
	stackPrize(9960, itemAmagoEquipB, 12),
	plainPrize(9990, itemOvoEquipN),
	plainPrize(10000, itemOvoEquipB),
}

// registerBausApoiador põe as três tabelas no mapa que openChest consulta, sob
// o índice definitivo e sob o emprestado do teste.
func registerBausApoiador() {
	for _, r := range []struct {
		definitivo, teste int16
		tabela            []chestPrize
	}{
		{itemBauBronze, itemBauBronzeTeste, bauBronzeTable},
		{itemBauApoiador, itemBauApoiadorTeste, bauApoiadorTable},
		{itemBauSupremo, itemBauSupremoTeste, bauSupremoTable},
	} {
		chestTables[r.definitivo] = r.tabela
		chestTables[r.teste] = r.tabela
	}
}

func init() { registerBausApoiador() }

// O prêmio de donate é a RCoin 100 (itemRCoin100, rcoin.go): usar credita 100
// na carteira de donate da conta.

// Os ovos de Cavalo Leve saíram das tabelas quando o Bronze perdeu o degrau 5
// (Marco, 18/09). Ficam nomeados porque a decisão de reintroduzi-los no Apoiador,
// como ovo intermediário, continua em aberto.
var _, _ = itemOvoLeveN, itemOvoLeveB

// bauApoiadorItem monta a pilha que a entrega põe na fila, para o servidor não
// depender de quem chama saber escrever EF_AMOUNT.
func bauApoiadorItem(index int16, quantidade uint8) world.Item {
	it := world.Item{Index: index}
	it.Effects[0] = world.Effect{Effect: efAmount, Value: quantidade}
	return it
}
