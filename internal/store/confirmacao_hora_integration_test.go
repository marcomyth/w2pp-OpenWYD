//go:build integration

// A confirmação decidindo pela HORA DO PAGAMENTO, e a idempotência do aviso que
// se repete.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

// cobrancaPronta monta uma venda pronta para ser paga: vendedor com chave, anúncio
// ativo, item marcado e cobrança aberta.
func cobrancaPronta(ctx context.Context, t *testing.T, s *Store, nome string) (vendedor, comprador int64, ref string) {
	t.Helper()
	vendedor = contaPix(ctx, t, s, "vendedor_"+nome)
	comprador = contaPix(ctx, t, s, "comprador_"+nome)
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	ref = "ref-" + nome
	if res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, ref, 0); err != nil {
		t.Fatal(err)
	} else if res != CobrancaAbertaOK {
		t.Fatalf("a cobranca nao abriu: resultado %d", res)
	}
	return vendedor, comprador, ref
}

// PAGOU DENTRO DO PRAZO, RECEBE — mesmo que o aviso chegue depois de a nossa
// varredura já ter expirado a cobrança.
//
// É o caso que mais dói e o motivo de a decisão ser pela hora do pagamento: o
// comprador paga no minuto 4:59 e o aviso da processadora atrasa. Decidindo pelo
// NOSSO estado, esse pagamento viraria dívida — ele pagou em dia e ficaria sem o
// item.
func TestPagouNoPrazoRecebeMesmoComOAvisoAtrasado(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "avisoatrasado")

	// A varredura roda e expira a cobrança, porque do NOSSO lado o prazo acabou.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET expira_em = now() - interval '1 minute'
		 WHERE referencia_externa = $1`, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExpirarCobrancasRMT(ctx); err != nil {
		t.Fatal(err)
	}

	// E só então o aviso chega, dizendo que o pagamento aconteceu ANTES do prazo.
	pagoEm := time.Now().Add(-2 * time.Minute)
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, ref, pagoEm)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %d, quero confirmada(%d): ele pagou EM DIA e o aviso "+
			"e que se atrasou", res, CobrancaConfirmada)
	}
	if venda.EntregaID == 0 {
		t.Error("nao enfileirou a entrega de quem pagou no prazo")
	}
	if venda.PagoComAtraso {
		t.Error("marcou como atrasado um pagamento feito dentro do prazo")
	}
}

// PAGOU FORA DO PRAZO NÃO RECEBE, mesmo com o item ainda lá.
//
// Parece desperdício — o item está disponível. Mas o prazo é a única coisa que o
// vendedor tem: ele combinou prender o item por cinco minutos, e entregar aos dez
// seria decidir por ele que a venda ainda valia. E o item ainda estar lá é acaso
// de alguns segundos: a esta altura ele podia já ter sido solto e vendido a outra
// pessoa.
func TestPagouForaDoPrazoNaoRecebeNemComItemDisponivel(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "atrasadoreal")

	// Tudo ainda de pé: anúncio ativo, item marcado, cobrança ABERTA. Só o
	// pagamento é que veio tarde.
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, ref, foraDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaPagaSemItem {
		t.Fatalf("resultado = %d, quero paga-sem-item(%d): o prazo e a unica coisa "+
			"que o vendedor tem", res, CobrancaPagaSemItem)
	}
	if venda.EntregaID != 0 {
		t.Error("entregou um pagamento fora do prazo")
	}
	if !venda.PagoComAtraso {
		t.Error("nao marcou o atraso; e a coluna que diz se a janela esta curta demais")
	}
	// E o item continua com o vendedor, livre para a próxima venda.
	var marcado bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM item WHERE owner_kind = 'account_cargo'
		               AND account_id = (SELECT vendedor_conta FROM rmt_anuncio
		                                  WHERE id = (SELECT anuncio_id FROM rmt_cobranca
		                                               WHERE referencia_externa = $1)))`,
		ref).Scan(&marcado); err != nil {
		t.Fatal(err)
	}
	if !marcado {
		t.Error("o item sumiu do bau do vendedor numa venda que nao aconteceu")
	}
}

// SEM A HORA DO PAGAMENTO, NÃO DECIDE. Adivinhar aqui é decidir sobre o dinheiro
// de alguém no escuro.
func TestSemAHoraDoPagamentoNaoConfirma(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "semhora")

	_, _, err := s.ConfirmarCobrancaRMT(ctx, ref, time.Time{})

	if err == nil {
		t.Fatal("confirmou sem saber quando o pagamento aconteceu")
	}
	// E NADA mudou: uma recusa que deixa metade feita é pior do que nenhuma.
	var status int16
	if err := s.pool.QueryRow(ctx,
		`SELECT status FROM rmt_cobranca WHERE referencia_externa = $1`, ref).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != cobrancaAberta {
		t.Errorf("a cobranca virou %d apesar da recusa", status)
	}
}

// O MESMO AVISO DUAS VEZES ENTREGA UMA VEZ SÓ.
//
// Webhook se repete por desenho — é assim que a processadora garante a entrega da
// mensagem. Duas entregas do mesmo pagamento seriam dois itens saindo de um
// anúncio que tinha um.
func TestOMesmoAvisoDuasVezesEntregaUmaVez(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPronta(ctx, t, s, "repetido")
	pagoEm := dentroDoPrazo()

	res1, venda1, err := s.ConfirmarCobrancaRMT(ctx, ref, pagoEm)
	if err != nil {
		t.Fatal(err)
	}
	res2, venda2, err := s.ConfirmarCobrancaRMT(ctx, ref, pagoEm)
	if err != nil {
		t.Fatal(err)
	}

	if res1 != CobrancaConfirmada {
		t.Errorf("a primeira devolveu %d, quero confirmada", res1)
	}
	if res2 != CobrancaJaConfirmada {
		t.Errorf("a segunda devolveu %d, quero ja-confirmada(%d) — repetir e o caminho "+
			"NORMAL, nao uma anomalia", res2, CobrancaJaConfirmada)
	}
	if venda2.EntregaID != venda1.EntregaID {
		t.Errorf("a segunda apontou para a entrega %d; a primeira foi %d",
			venda2.EntregaID, venda1.EntregaID)
	}
	if n := entregasDe(ctx, t, s, comprador); n != 1 {
		t.Errorf("a caixa postal tem %d entrega(s), quero 1: dois itens sairiam de um "+
			"anuncio que tinha um", n)
	}
}

// E DOIS AVISOS AO MESMO TEMPO TAMBÉM ENTREGAM UMA VEZ SÓ.
//
// O repetido em sequência é pego pela leitura do status; o SIMULTÂNEO não — as
// duas transações leem o mesmo estado antigo. Quem resolve é o FOR UPDATE, e é
// este teste que prova que ele está lá.
func TestDoisAvisosAoMesmoTempoEntregamUmaVez(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPronta(ctx, t, s, "paralelo")
	pagoEm := dentroDoPrazo()

	type saida struct {
		res ResultadoCobranca
		err error
	}
	fora := make(chan saida, 2)
	var largada sync.WaitGroup
	largada.Add(1)
	for i := 0; i < 2; i++ {
		go func() {
			largada.Wait() // as duas partem juntas
			res, _, err := s.ConfirmarCobrancaRMT(context.Background(), ref, pagoEm)
			fora <- saida{res, err}
		}()
	}
	largada.Done()

	confirmadas, jaConfirmadas := 0, 0
	for i := 0; i < 2; i++ {
		r := <-fora
		if r.err != nil {
			t.Fatalf("confirmando em paralelo: %v", r.err)
		}
		switch r.res {
		case CobrancaConfirmada:
			confirmadas++
		case CobrancaJaConfirmada:
			jaConfirmadas++
		default:
			t.Errorf("resultado inesperado: %d", r.res)
		}
	}

	if confirmadas != 1 || jaConfirmadas != 1 {
		t.Errorf("confirmadas = %d, ja-confirmadas = %d; quero 1 e 1",
			confirmadas, jaConfirmadas)
	}
	if n := entregasDe(ctx, t, s, comprador); n != 1 {
		t.Errorf("a caixa postal tem %d entrega(s), quero 1", n)
	}
}

// O COMPRADOR FECHA O JOGO PARA PAGAR NO CELULAR, O VENDEDOR DERRUBA A BARRACA, E
// O PIX CAI DENTRO DO PRAZO. O item tem de ser ENTREGUE.
//
// É o caso mais comum que existe, e hoje ele perde dinheiro:
//
//  1. o comprador sai do jogo — o movimento natural de quem vai pagar no celular;
//  2. o logout CANCELA a cobrança dele;
//  3. cancelada, ela deixa de contar como "aberta" para o escrow;
//  4. o vendedor derruba a barraca, e sem cobrança aberta o anúncio é cancelado e
//     o item é solto NA HORA;
//  5. o Pix cai trinta segundos depois, DENTRO do prazo, e não acha item.
//
// O resultado é PAGA_SEM_ITEM: reembolso que custa R$ 1,00 mais as taxas à Hanna,
// de um comprador que fez tudo certo.
//
// A regra: NENHUMA cobrança solta o item antes do prazo acabar, seja qual for o
// jeito que ela fechou.
func TestCompradorQueSaiParaPagarNoCelularRecebe(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor, comprador, ref := cobrancaPronta(ctx, t, s, "celular")

	var anuncio int64
	if err := s.pool.QueryRow(ctx,
		`SELECT anuncio_id FROM rmt_cobranca WHERE referencia_externa = $1`, ref).
		Scan(&anuncio); err != nil {
		t.Fatal(err)
	}

	// 1 e 2: ele sai do jogo, e o logout cancela a cobranca.
	if _, err := s.CancelarCobrancasDoComprador(ctx, comprador); err != nil {
		t.Fatal(err)
	}

	// 4: o vendedor derruba a barraca.
	if _, err := s.EncerrarAnunciosRMT(ctx, []int64{anuncio}); err != nil {
		t.Fatal(err)
	}
	// E entra em jogo de novo, o que dispara a reconciliacao.
	if _, err := s.ReconciliarEscrowRMT(ctx, vendedor); err != nil {
		t.Fatal(err)
	}

	// 5: o Pix cai, DENTRO do prazo.
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %d, quero confirmada(%d). Ele pagou DENTRO do prazo e "+
			"fez tudo certo; isto vira reembolso que custa dinheiro a Hanna",
			res, CobrancaConfirmada)
	}
	if venda.EntregaID == 0 {
		t.Error("nao enfileirou a entrega de quem pagou no prazo")
	}
	if venda.PagoComAtraso {
		t.Error("marcou como atraso um pagamento feito dentro do prazo")
	}
}
