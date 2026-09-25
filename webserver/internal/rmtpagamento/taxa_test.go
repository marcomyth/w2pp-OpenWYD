package rmtpagamento

import (
	"context"
	"testing"
)

// A TAXA ATRAVESSA COMO CHEGOU, e nulo não vira zero em nenhum ponto do caminho.
//
// É o teste mais importante deste arquivo, e não por ser difícil: é porque o defeito
// que ele pega é invisível em produção. Se alguma tradução trocar nulo por zero, o
// repasse ao vendedor nasce com o valor CHEIO, a casa banca a taxa, e nada parece
// errado — um repasse de valor cheio tem a cara de um repasse correto. Não haveria log
// de erro, não haveria linha na fila da staff, e a perda só apareceria conferindo
// extrato contra banco, venda por venda.
func TestATaxaAtravessaComoChegou(t *testing.T) {
	cinquenta := int64(50)
	zero := int64(0)

	casos := []struct {
		nome string
		taxa *int64
	}{
		{"taxa conhecida", &cinquenta},
		// Zero é uma AFIRMAÇÃO e tem de chegar como zero, não como ausência: é o
		// espelho do caso de baixo, e os dois juntos são o que prende a distinção.
		{"taxa zero, que e afirmacao", &zero},
		{"taxa desconhecida", nil},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			p, b := vendaBoa()
			p.t.TaxaCentavos = c.taxa

			if _, err := Novo(p, b, nil, mudo()).ConferirEConcluir(context.Background(), "id-1"); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if !b.taxaRecebida {
				t.Fatal("a confirmacao nao recebeu a taxa")
			}
			switch {
			case c.taxa == nil && b.taxaVista != nil:
				t.Errorf("taxa desconhecida chegou como %d; nulo NAO e zero", *b.taxaVista)
			case c.taxa != nil && b.taxaVista == nil:
				t.Errorf("taxa %d chegou como desconhecida", *c.taxa)
			case c.taxa != nil && *b.taxaVista != *c.taxa:
				t.Errorf("taxa = %d, queria %d", *b.taxaVista, *c.taxa)
			}
		})
	}
}

// A TAXA DESCONHECIDA NÃO IMPEDE A ENTREGA. O comprador pagou, e o item é dele mesmo
// que ninguém saiba a taxa: o que espera é só o pagamento ao VENDEDOR.
//
// Sem este teste, "segurar por taxa desconhecida" poderia crescer para o lado errado —
// alguém lê "segura" e acha que segura a venda inteira, e aí quem pagou fica sem item
// por causa de um número que não é da conta dele.
func TestTaxaDesconhecidaNaoSeguraAEntregaAoComprador(t *testing.T) {
	p, b := vendaBoa()
	p.t.TaxaCentavos = nil
	j := &jogofake{emJogo: true}

	res, err := Novo(p, b, j, mudo()).ConferirEConcluir(context.Background(), "id-1")

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !res.Confirmada || !res.Entregue {
		t.Errorf("resultado = %+v, queria a venda confirmada e entregue", res)
	}
}

// taxaParaLog mostra ausência como -1, e não como 0.
//
// Zero no log seria lido como "a taxa foi zero" por quem estiver conferindo a primeira
// venda real — que é exatamente a leitura errada que este campo existe para evitar, e
// numa medição de dinheiro a confusão custa a decisão do preço mínimo.
func TestTaxaParaLogNaoMostraAusenciaComoZero(t *testing.T) {
	zero := int64(0)
	if got := taxaParaLog(nil); got != -1 {
		t.Errorf("ausencia virou %d no log, queria -1", got)
	}
	if got := taxaParaLog(&zero); got != 0 {
		t.Errorf("taxa zero virou %d no log, queria 0", got)
	}
}
