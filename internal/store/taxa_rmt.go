package store

// A taxa da casa sobre uma venda em dinheiro real.
//
// É o que o servidor fica da venda entre jogadores. NÃO se confunde com a taxa da
// PROCESSADORA (`rmt_cobranca.taxa_centavos`), que é o que o banco retém do que entra e
// que o servidor não escolhe: aquela é descoberta depois do pagamento, esta é conhecida
// antes de o anúncio subir, e é a que o vendedor precisa ver na hora de pôr o preço.
//
// DUAS FAIXAS, por decisão da Hanna: 5% até R$ 100,00 e 6,99% acima, as duas com R$ 0,80
// fixos por venda. O fixo existe porque toda venda custa uma cobrança na processadora,
// independente do valor — sem ele, a venda pequena sairia no prejuízo.
//
// TUDO EM INTEIRO, e nenhum float em lugar nenhum. Dinheiro em float acumula erro de
// arredondamento que ninguém vê até a diferença virar uma reclamação, e "um centavo a
// menos" é a reclamação mais cara que existe, porque destrói a confiança na conta
// inteira. Os percentuais viajam em PONTOS-BASE (centésimos de por cento), que é o
// formato que mantém 6,99% exato: 699.

const (
	// TaxaBaseBps é 5,00% em pontos-base, para vendas até o degrau.
	TaxaBaseBps int64 = 500
	// TaxaAltaBps é 6,99% em pontos-base, acima do degrau.
	TaxaAltaBps int64 = 699
	// TaxaFixaCentavos é R$ 0,80 por venda, nas duas faixas.
	TaxaFixaCentavos int64 = 80
	// DegrauDaTaxaCentavos é R$ 100,00: até aqui vale a base, acima vale a alta.
	//
	// O DEGRAU É INCLUSIVO na faixa de baixo — exatamente R$ 100,00 paga 5%. A regra
	// tinha de escolher um lado, e escolher o lado de baixo é o que faz a frase "5%
	// até R$ 100" ser verdade lida como o jogador a lê.
	DegrauDaTaxaCentavos int64 = 10_000
)

// TaxaDaVendaRMT devolve quanto a casa fica de uma venda, e a regra que usou.
//
// Os dois números da regra voltam junto de propósito: eles são GRAVADOS na venda. Sem
// isso, uma venda de seis meses atrás só se explica com a tabela de hoje, e no dia em
// que a Hanna mudar a taxa toda conta antiga passa a "não bater" — sem ninguém ter
// mexido em nada. Guardar a regra usada é o que deixa o passado continuar fechando.
//
// O ARREDONDAMENTO É PARA CIMA, e é escolha, não descuido. Uma venda de R$ 5,01 dá
// 25,05 centavos de percentual; arredondar para baixo faria a casa pagar essa fração em
// toda venda, e em volume isso é dinheiro de verdade. Para cima, quem paga a fração é
// quem vende, e é um centavo.
func TaxaDaVendaRMT(valorCentavos int64) (taxa, bps, fixo int64) {
	bps, fixo = TaxaBaseBps, TaxaFixaCentavos
	if valorCentavos > DegrauDaTaxaCentavos {
		bps = TaxaAltaBps
	}
	// Teto para cima em aritmética inteira: (a + b - 1) / b. O +9999 é o b-1 de 10000.
	percentual := (valorCentavos*bps + 9_999) / 10_000
	return percentual + fixo, bps, fixo
}

// LiquidoDoVendedorRMT é o que sobra para quem vendeu, depois da taxa da casa.
//
// Existe como função e não como uma subtração solta porque o número aparece em três
// lugares — a linha de valor líquido na montagem da barraca, a conferência da abertura
// da cobrança e o repasse — e três subtrações escritas à mão é como uma delas fica para
// trás no dia em que a regra mudar.
func LiquidoDoVendedorRMT(valorCentavos int64) int64 {
	taxa, _, _ := TaxaDaVendaRMT(valorCentavos)
	return valorCentavos - taxa
}
