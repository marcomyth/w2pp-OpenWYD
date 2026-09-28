//go:build integration

// A taxa da casa no repasse: quem decide o que o vendedor recebe.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import "testing"

// TestOLiquidoSaiDaTaxaDaCasaENaoDaProcessadora.
//
// A MUDANÇA DE REGRA QUE ESTE TESTE PRENDE, nas palavras da Hanna: "5% da venda + R$ 0,80,
// descontados do que o vendedor recebe". Antes, o líquido era `bruto - taxa da
// PROCESSADORA` — o que o banco reteve, descoberto só depois do pagamento. Agora a taxa
// da processadora é custo da casa e não entra na conta de quem vendeu.
//
// Com o preço de teste (R$ 50,00): 5% de 5000 = 250, mais os 80 fixos, dá 330 de taxa e
// 4670 de líquido. Os números estão escritos à mão de propósito — calculá-los com a mesma
// função que está sendo testada faria o teste concordar com qualquer fórmula.
func TestOLiquidoSaiDaTaxaDaCasaENaoDaProcessadora(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "taxa_da_casa")

	// A processadora reteve 999 — um número que NÃO pode aparecer em lugar nenhum da
	// conta do vendedor. Se ele aparecer, a regra velha continua de pé.
	retido := int64(999)
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, &retido)
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	var liquido, bruto, taxaCasa, fixo int64
	var bps int16
	var estado int16
	if err := s.pool.QueryRow(ctx, `
		SELECT valor_centavos, bruto_centavos, status,
		       taxa_casa_centavos, taxa_casa_bps, taxa_casa_fixa
		  FROM rmt_repasse WHERE id = $1`, id).
		Scan(&liquido, &bruto, &estado, &taxaCasa, &bps, &fixo); err != nil {
		t.Fatal(err)
	}

	if bruto != precoEmCentavos {
		t.Errorf("bruto = %d, queria %d", bruto, precoEmCentavos)
	}
	if taxaCasa != 330 {
		t.Errorf("taxa da casa = %d, queria 330 (5%% de 5000 = 250, mais 80)", taxaCasa)
	}
	if liquido != 4_670 {
		t.Errorf("liquido = %d, queria 4670: o vendedor recebe o bruto menos a taxa DA CASA",
			liquido)
	}
	if liquido == precoEmCentavos-retido {
		t.Error("o liquido saiu do desconto da PROCESSADORA: a regra velha continua de pe")
	}

	// A REGRA FICA GRAVADA NA LINHA. Sem ela, uma venda de seis meses atrás só se
	// explica com a tabela de hoje, e no dia em que a Hanna mudar a taxa toda conta
	// antiga passa a "não bater" sem ninguém ter mexido em nada.
	if bps != 500 {
		t.Errorf("faixa gravada = %d bps, queria 500", bps)
	}
	if fixo != 80 {
		t.Errorf("fixo gravado = %d, queria 80", fixo)
	}
	if EstadoRepasse(estado) != RepassePendente {
		t.Errorf("estado = %d, queria PENDENTE", estado)
	}
}

// TestVendaNovaSemTaxaDaProcessadoraNaoFicaSegurada.
//
// O repasseSemTaxa existia porque o líquido dependia de um número que só se conhecia
// depois do pagamento: sem ele, pagar o bruto faria a casa bancar a taxa em silêncio.
//
// AGORA O LÍQUIDO É CONHECIDO ANTES DE O ANÚNCIO SUBIR, então não há o que esperar. Uma
// venda cuja taxa da processadora nunca chegou tem de nascer PENDENTE e entrar na fila de
// pagamento como qualquer outra — segurá-la seria deixar o vendedor esperando por um
// número que não interessa mais a ele.
//
// O estado continua existindo para as linhas ANTIGAS, gravadas sob a regra anterior, que
// ainda saem da espera pelo caminho de lá.
func TestVendaNovaSemTaxaDaProcessadoraNaoFicaSegurada(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "sem_taxa_da_processadora")

	// Nulo: ninguém sabe o que a processadora reteve, e agora tanto faz.
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	var estado int16
	var liquido int64
	if err := s.pool.QueryRow(ctx,
		`SELECT status, valor_centavos FROM rmt_repasse WHERE id = $1`, id).
		Scan(&estado, &liquido); err != nil {
		t.Fatal(err)
	}
	if EstadoRepasse(estado) == RepasseSemTaxa {
		t.Fatal("a venda nova ficou SEGURADA esperando a taxa da processadora, que ja nao " +
			"decide nada: o vendedor esperaria por um numero que nao interessa mais a ele")
	}
	if EstadoRepasse(estado) != RepassePendente {
		t.Errorf("estado = %d, queria PENDENTE", estado)
	}
	if liquido != 4_670 {
		t.Errorf("liquido = %d, queria 4670 mesmo sem a taxa da processadora", liquido)
	}
}

// TestOSiteEAFilaMostramOMesmoQueOJogoPrometeu.
//
// TRÊS TELAS, UM NÚMERO. O jogo diz "você recebe R$ X,XX" ao montar a barraca, o site
// mostra o total a receber, e a lista da staff mostra o valor a pagar. Se os três não
// saírem da mesma conta, o vendedor lê um valor, confere outro e recebe um terceiro — e
// a diferença só aparece depois da venda, que é o pior momento possível.
//
// Aqui se prova que a conta do JOGO (LiquidoDoVendedorRMT, a mesma que a linha ao montar
// usa) é exatamente o que o banco gravou e o que o site soma.
func TestOSiteEAFilaMostramOMesmoQueOJogoPrometeu(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "tres_telas")

	prometidoNoJogo := LiquidoDoVendedorRMT(precoEmCentavos)

	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	// O que o SITE mostra ao vendedor.
	noSite, _, err := s.RepasseDoVendedor(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if noSite != prometidoNoJogo {
		t.Errorf("o site mostra %d e o jogo prometeu %d", noSite, prometidoNoJogo)
	}

	// O que a FILA DA STAFF manda pagar.
	fila, err := s.FilaDePagamentoAMao(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 {
		t.Fatalf("linhas na fila = %d, queria 1", len(fila))
	}
	if int64(fila[0].ValorCentavos) != prometidoNoJogo {
		t.Errorf("a fila da staff manda pagar %d e o jogo prometeu %d",
			fila[0].ValorCentavos, prometidoNoJogo)
	}
}

// liquidoDaVendaDeTeste é o que o vendedor recebe da venda que os testes montam.
//
// EXISTE PARA OS TESTES ANTIGOS, escritos quando o líquido era o bruto e que por isso
// esperavam precoEmCentavos. A regra mudou — a taxa da casa desconta 5% + R$ 0,80 — e eles
// passaram a falhar dizendo "total = 4670, quero 5000". É a regra velha presa no teste, e
// não um defeito no código: conferi as vinte falhas uma a uma e todas são esta.
//
// CALCULA PELA MESMA FUNÇÃO em vez de trazer 4670 escrito. Com o número digitado, mudar a
// taxa quebraria vinte testes de uma vez, o conserto seria reescrever vinte números, e um
// deles ficaria para trás sem ninguém saber qual.
//
// O que NÃO virou isto, de propósito: as chamadas que dizem quanto foi COBRADO. Ali o
// número continua sendo o preço cheio, porque é o que o comprador paga — e é justamente a
// diferença entre os dois que este PR introduz.
func liquidoDaVendaDeTeste() int64 { return LiquidoDoVendedorRMT(precoEmCentavos) }
