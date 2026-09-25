//go:build integration

// O apagar da chave contra repasse RECUSADO e contra repasse PAGO.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

// TestApagarRecusaComRepasseRecusado.
//
// O DEFEITO QUE ELE PRENDE, e ele era visível na tela: o `RepasseDoVendedor` conta como
// "a receber" todo repasse que não é PAGO, recusado incluído, porque a recusa não apaga a
// dívida — o dinheiro continua sendo do vendedor, só não achou o caminho. Mas o apagar
// não olhava o recusado. Então a tela dizia "você tem R$ X a receber" e o botão Apagar
// funcionava assim mesmo, tirando justamente a chave para onde aquele dinheiro tem de ir.
//
// ESTE TESTE É O QUE PRENDE AS DUAS REGRAS JUNTAS, e é por isso que ele pergunta as duas
// coisas do MESMO vendedor, na mesma situação: quanto a tela diz que há a receber, e o
// que o apagar responde. Uma consulta compartilhada entre os dois lugares seria o jeito
// óbvio de impedir a divergência, mas o `RepasseDoVendedor` precisa do total e de três
// contagens filtradas na mesma consulta — fatorar o predicado numa string esconderia a
// regra em vez de guardá-la. A ligação fica aqui, onde ela é medida.
//
// Decisão da Hanna, 25/09/2026: "não permitir que ele delete se tiver valores a receber;
// se não tiver nada, ele pode remover tranquilamente".
func TestApagarRecusaComRepasseRecusado(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaDoRepasse(ctx, t, s, repasseRecusadoParaAjuste(ctx, t, s, "apagar_recusado"))

	// Primeiro o que a TELA diz, porque é o que faz a recusa ser coerente: se aqui o
	// total fosse zero, travar o apagar é que seria o erro.
	total, _, err := s.RepasseDoVendedor(ctx, conta)
	if err != nil {
		t.Fatal(err)
	}
	if total <= 0 {
		t.Fatalf("a tela diz %d a receber; sem valor a receber esta trava nao faria sentido", total)
	}

	if err := s.ApagarChavePix(ctx, conta); !errors.Is(err, ErrRepasseEmCurso) {
		t.Fatalf("apagar = %v, queria ErrRepasseEmCurso: a tela promete %d e o botao levava a chave",
			err, total)
	}
}

// TestApagarPassaQuandoSoHaRepassePago.
//
// O OUTRO LADO DA MESMA DECISÃO, e sem ele a trava não estaria medida: "se não tiver
// nada, ele pode remover tranquilamente". Um guard escrito como "existe QUALQUER repasse"
// passaria no teste de cima e prenderia a chave para sempre — quem vendeu uma vez e
// recebeu nunca mais conseguiria tirar o cadastro.
func TestApagarPassaQuandoSoHaRepassePago(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "apagar_pago")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)
	// PAGO PELO CAMINHO DA STAFF, que desde 25/09/2026 é o único que existe: o par
	// enviado/aviso-de-saque saiu junto com o saque automático. O teste ganhou com a
	// troca — ele passou a chegar ao estado final pelo caminho de verdade, em vez de
	// montá-lo com duas escritas que ninguém mais faz.
	if err := s.MarcarRepassePagoAMao(ctx, id, staffQuePaga(), "pago no app, comprovante E1"); err != nil {
		t.Fatal(err)
	}

	// A tela já não promete nada, e o apagar acompanha.
	total, _, err := s.RepasseDoVendedor(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("o pago ficou pendurado como %d a receber", total)
	}
	if err := s.ApagarChavePix(ctx, v.vendedor); err != nil {
		t.Fatalf("apagar = %v; com tudo pago a chave tem de sair", err)
	}
}

// contaDoRepasse devolve o vendedor daquele repasse. O `repasseRecusadoParaAjuste`
// entrega só o id do repasse, e as duas perguntas deste arquivo são sobre a CONTA.
func contaDoRepasse(ctx context.Context, t *testing.T, s *Store, repasseID int64) int64 {
	t.Helper()
	var conta int64
	if err := s.pool.QueryRow(ctx,
		`SELECT vendedor_conta FROM rmt_repasse WHERE id = $1`, repasseID).Scan(&conta); err != nil {
		t.Fatal(err)
	}
	return conta
}
