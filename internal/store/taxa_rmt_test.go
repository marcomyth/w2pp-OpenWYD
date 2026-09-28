package store

import "testing"

// TestATaxaDaVendaRMT é a tabela acertada com a Hanna, valor por valor.
//
// OS NÚMEROS ESTÃO ESCRITOS À MÃO, e não calculados a partir das constantes. Derivar o
// esperado da mesma fórmula que está sendo testada faz o teste concordar com qualquer
// coisa — inclusive com a fórmula errada. Aqui o esperado veio da decisão, e o código
// tem de chegar nele.
//
// Os casos não são exemplos: cada um responde uma pergunta.
func TestATaxaDaVendaRMT(t *testing.T) {
	casos := []struct {
		nome   string
		valor  int64
		taxa   int64
		bps    int64
		porque string
	}{
		{
			nome: "o minimo", valor: 500, taxa: 105, bps: TaxaBaseBps,
			porque: "5% de 500 = 25, mais os 80 fixos",
		},
		{
			nome: "um centavo acima do minimo", valor: 501, taxa: 106, bps: TaxaBaseBps,
			porque: "5% de 501 = 25,05 e arredonda PARA CIMA, virando 26; a fracao e " +
				"de quem vende, nao da casa",
		},
		{
			nome: "o degrau, exatamente", valor: 10_000, taxa: 580, bps: TaxaBaseBps,
			porque: "R$ 100,00 redondos ainda paga 5%: o degrau e inclusivo na faixa de " +
				"baixo, senao a frase '5% ate R$ 100' seria falsa justamente em R$ 100",
		},
		{
			nome: "um centavo depois do degrau", valor: 10_001, taxa: 780, bps: TaxaAltaBps,
			porque: "6,99% de 10001 = 699,0699, que arredonda para 700, mais 80. O salto " +
				"de 580 para 780 num centavo e real e e a regra combinada",
		},
		{
			nome: "o dobro do degrau", valor: 20_000, taxa: 1_478, bps: TaxaAltaBps,
			porque: "6,99% de 20000 = 1398 exato, mais 80",
		},
		{
			nome: "o teto", valor: 50_000, taxa: 3_575, bps: TaxaAltaBps,
			porque: "6,99% de 50000 = 3495 exato, mais 80",
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			taxa, bps, fixo := TaxaDaVendaRMT(c.valor)
			if taxa != c.taxa {
				t.Errorf("taxa de %d = %d, queria %d — %s", c.valor, taxa, c.taxa, c.porque)
			}
			if bps != c.bps {
				t.Errorf("faixa de %d = %d bps, queria %d", c.valor, bps, c.bps)
			}
			if fixo != TaxaFixaCentavos {
				t.Errorf("fixo de %d = %d, queria %d em toda faixa", c.valor, fixo, TaxaFixaCentavos)
			}
		})
	}
}

// TestATaxaNuncaComeAVendaInteira.
//
// A pergunta que importa para o vendedor: sobra alguma coisa? Com R$ 0,80 fixos, uma
// venda pequena demais sairia com líquido zero ou negativo — e é por isso que o preço
// mínimo existe e é R$ 5,00, não R$ 1,00.
//
// Este teste é o que liga as duas decisões: se alguém baixar o mínimo sem olhar a taxa,
// ele quebra aqui em vez de quebrar no bolso de quem vendeu.
func TestATaxaNuncaComeAVendaInteira(t *testing.T) {
	for valor := int64(PrecoMinimoRMTCentavos); valor <= TetoDaVendaRMTCentavos; valor += 97 {
		if liquido := LiquidoDoVendedorRMT(valor); liquido <= 0 {
			t.Fatalf("venda de %d deixa o vendedor com %d: a taxa comeu a venda", valor, liquido)
		}
	}
	// E nas duas pontas exatas, que o passo de 97 não visita.
	for _, valor := range []int64{PrecoMinimoRMTCentavos, TetoDaVendaRMTCentavos} {
		if liquido := LiquidoDoVendedorRMT(valor); liquido <= 0 {
			t.Errorf("venda de %d deixa o vendedor com %d", valor, liquido)
		}
	}
}

// TestOLiquidoEhOValorMenosATaxa: a conta que o vendedor faz de cabeça tem de fechar.
func TestOLiquidoEhOValorMenosATaxa(t *testing.T) {
	for _, valor := range []int64{500, 501, 10_000, 10_001, 20_000, 50_000} {
		taxa, _, _ := TaxaDaVendaRMT(valor)
		if liquido := LiquidoDoVendedorRMT(valor); liquido != valor-taxa {
			t.Errorf("liquido de %d = %d, queria %d", valor, liquido, valor-taxa)
		}
	}
}

// TestATaxaCresceComOValor.
//
// Uma venda maior nunca pode render à casa MENOS que uma menor. O degrau é o lugar onde
// isso quase quebra: entre 10.000 e 10.001 a faixa muda, e uma fórmula escrita ao
// contrário faria o valor maior pagar menos.
func TestATaxaCresceComOValor(t *testing.T) {
	anterior := int64(-1)
	for valor := int64(PrecoMinimoRMTCentavos); valor <= TetoDaVendaRMTCentavos; valor++ {
		taxa, _, _ := TaxaDaVendaRMT(valor)
		if taxa < anterior {
			t.Fatalf("venda de %d paga %d, menos que a de %d", valor, taxa, valor-1)
		}
		anterior = taxa
	}
}
