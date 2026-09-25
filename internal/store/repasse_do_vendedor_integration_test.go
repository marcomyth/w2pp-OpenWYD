//go:build integration

// O que o vendedor vê na própria página: quanto tem a receber e por que ainda não
// chegou.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
)

// SEM DÍVIDA, ZERO E MOTIVO NENHUM. O motivo só existe para explicar uma espera, e
// quem não espera nada não tem o que explicar.
func TestRepasseDoVendedorSemDividaNaoInventaMotivo(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaPix(ctx, t, s, "sem_divida")

	total, motivo, err := s.RepasseDoVendedor(ctx, conta)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || motivo != SemEspera {
		t.Errorf("total = %d, motivo = %d; quero 0 e SemEspera", total, motivo)
	}
}

// COM O CADASTRO COMPLETO, A RESPOSTA É "A CAMINHO".
func TestRepasseDoVendedorPendenteComCadastroEstaACaminho(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "a-caminho")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	total, motivo, err := s.RepasseDoVendedor(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != precoEmCentavos {
		t.Errorf("total = %d, quero %d", total, precoEmCentavos)
	}
	if motivo != EsperaPagamento {
		t.Errorf("motivo = %d, quero EsperaPagamento", motivo)
	}
}

// SEM DOCUMENTO, A RESPOSTA É A ÚNICA QUE A PESSOA RESOLVE SOZINHA.
//
// É o estado do vendedor antigo, cadastrado antes de o CPF ser obrigatório: a chave
// está lá e o documento não. Dizer "a caminho" para ele seria mentira — não há para
// onde mandar — e dizer "estamos verificando" o deixaria esperando uma providência
// nossa que nunca vem, porque a providência é dele.
func TestRepasseDoVendedorSemDocumentoPedeOCadastro(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "falta-cadastro")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_recebedor SET documento = NULL WHERE account_id = $1`, v.vendedor); err != nil {
		t.Fatal(err)
	}

	total, motivo, err := s.RepasseDoVendedor(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != precoEmCentavos {
		t.Errorf("total = %d, quero %d", total, precoEmCentavos)
	}
	if motivo != EsperaCadastro {
		t.Errorf("motivo = %d, quero EsperaCadastro", motivo)
	}
}

// O INCERTO E O RECUSADO MANDAM OLHAR, e o RECUSADO CONTINUA NO TOTAL.
//
// O recusado é o que eu quase deixei sumir. A recusa não apaga a dívida: o dinheiro
// continua sendo do vendedor, só não achou o caminho. Tirá-lo do total faria o
// número sumir da tela exatamente no momento em que a pessoa mais precisa ver que
// ele existe — e ela concluiria, sozinha, que perdeu.
func TestRepasseDoVendedorRecusadoEIncertoMandamOlhar(t *testing.T) {
	for _, c := range []struct {
		nome  string
		marca func(*Store, context.Context, int64) error
	}{
		{"incerto", func(s *Store, ctx context.Context, id int64) error {
			return s.MarcarRepasseIncerto(ctx, id, "a resposta nao voltou")
		}},
		{"recusado", func(s *Store, ctx context.Context, id int64) error {
			http := int32(422)
			return s.MarcarRepasseRecusado(ctx, id, &http, "PIX_KEY_NOT_FOUND", "chave nao existe")
		}},
	} {
		t.Run(c.nome, func(t *testing.T) {
			s, ctx := freshStore(t)
			v := montaVenda(ctx, t, s, "olhar-"+c.nome)
			_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
			if err != nil {
				t.Fatal(err)
			}
			if err := c.marca(s, ctx, idDoRepasse(ctx, t, s, venda.CobrancaID)); err != nil {
				t.Fatal(err)
			}

			total, motivo, err := s.RepasseDoVendedor(ctx, v.vendedor)
			if err != nil {
				t.Fatal(err)
			}
			if total != precoEmCentavos {
				t.Errorf("total = %d, quero %d: a divida nao deixou de existir", total, precoEmCentavos)
			}
			if motivo != EsperaGente {
				t.Errorf("motivo = %d, quero EsperaGente", motivo)
			}
		})
	}
}

// O QUE FOI PAGO SAI DO TOTAL. Deixá-lo faria a tela dizer para sempre que há
// dinheiro a receber, e a pessoa abriria um chamado por um valor que já está na
// conta dela.
func TestRepasseDoVendedorPagoSaiDoTotal(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "pago-sai")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)
	if err := s.MarcarRepasseEnviado(ctx, id, "saque-1", precoEmCentavos); err != nil {
		t.Fatal(err)
	}
	if err := s.MarcarRepassePago(ctx, "saque-1", precoEmCentavos); err != nil {
		t.Fatal(err)
	}

	total, motivo, err := s.RepasseDoVendedor(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || motivo != SemEspera {
		t.Errorf("total = %d, motivo = %d; o pago ficou pendurado", total, motivo)
	}
}

// O TOTAL É DESTA CONTA, e a dívida de outro vendedor não entra.
func TestRepasseDoVendedorNaoSomaDividaDeOutro(t *testing.T) {
	s, ctx := freshStore(t)
	meu := montaVenda(ctx, t, s, "meu")
	outro := montaVenda(ctx, t, s, "outro")
	for _, v := range []vendaMontada{meu, outro} {
		if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
			t.Fatal(err)
		}
	}

	total, _, err := s.RepasseDoVendedor(ctx, meu.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != precoEmCentavos {
		t.Errorf("total = %d, quero %d: entrou divida de outra conta", total, precoEmCentavos)
	}
}
