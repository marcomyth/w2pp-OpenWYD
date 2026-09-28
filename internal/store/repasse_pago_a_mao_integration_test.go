//go:build integration

// O pagamento à mão: a staff fecha a dívida que pagou fora do sistema.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// repassePendenteDeVenda deixa uma venda confirmada e devolve o repasse pendente dela.
func repassePendenteDeVenda(ctx context.Context, t *testing.T, s *Store, sufixo string) (int64, int64) {
	t.Helper()
	v := montaVenda(ctx, t, s, sufixo)
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	return idDoRepasse(ctx, t, s, venda.CobrancaID), v.vendedor
}

func staffQuePaga() AtorDoAjuste {
	return AtorDoAjuste{ContaID: 1, Papel: "admin", Nome: "hanna"}
}

// TestPagamentoAMaoFechaADividaEGuardaQuemPagou.
//
// Esta é a única escrita de dinheiro do servidor SEM comprovante nosso atrás dela: não
// houve chamada a processadora nenhuma, só uma pessoa clicando. Então o que este teste
// mede não é só o estado — é se sobrou registro de quem clicou e do que ela disse.
func TestPagamentoAMaoFechaADividaEGuardaQuemPagou(t *testing.T) {
	s, ctx := freshStore(t)
	id, _ := repassePendenteDeVenda(ctx, t, s, "pago_a_mao")

	const nota = "pago no app do Nubank, comprovante E1234567202609251530"
	if err := s.MarcarRepassePagoAMao(ctx, id, staffQuePaga(), nota); err != nil {
		t.Fatalf("pagar a mao: %v", err)
	}

	var estado int16
	var chegou *int64
	var guardouNota, guardouPor *string
	if err := s.pool.QueryRow(ctx, `
		SELECT status, chegou_centavos, pago_a_mao_nota, pago_a_mao_por
		  FROM rmt_repasse WHERE id = $1`, id).
		Scan(&estado, &chegou, &guardouNota, &guardouPor); err != nil {
		t.Fatal(err)
	}
	if EstadoRepasse(estado) != RepassePago {
		t.Errorf("estado = %d, queria PAGO", estado)
	}
	// O líquido da própria linha, e não um número digitado: sem saque não há taxa de
	// saque, e deixar nulo faria toda conta de receita tratar "pago sem valor" como
	// caso especial para sempre.
	if chegou == nil || *chegou != liquidoDaVendaDeTeste() {
		t.Errorf("chegou_centavos = %v, queria %d", chegou, liquidoDaVendaDeTeste())
	}
	if guardouNota == nil || *guardouNota != nota {
		t.Errorf("a observacao nao foi guardada: %v", guardouNota)
	}
	if guardouPor == nil || *guardouPor != "hanna" {
		t.Errorf("quem pagou nao foi guardado: %v", guardouPor)
	}

	// A auditoria, na mesma transação.
	var acao string
	var alvo int64
	if err := s.pool.QueryRow(ctx, `
		SELECT action, target_account_id FROM admin_audit_log
		 WHERE action = $1 ORDER BY id DESC LIMIT 1`, AcaoRepassePagoAMao).Scan(&acao, &alvo); err != nil {
		t.Fatalf("a auditoria do pagamento a mao nao foi gravada: %v", err)
	}
	if alvo == 0 {
		t.Error("a auditoria nao diz de quem era o dinheiro")
	}
}

// TestPagamentoAMaoSoSaiDePendente.
//
// O caso que importa é o SEGUNDO CLIQUE no mesmo botão: ele tem de recusar, e não pagar
// de novo. E recusado e incerto também não passam — o incerto é justamente o caso em que
// o dinheiro PODE já ter saído, e fechá-lo à mão esconderia a dúvida que ele existe para
// levantar.
func TestPagamentoAMaoSoSaiDePendente(t *testing.T) {
	s, ctx := freshStore(t)
	id, _ := repassePendenteDeVenda(ctx, t, s, "so_pendente")
	if err := s.MarcarRepassePagoAMao(ctx, id, staffQuePaga(), "primeira vez, comprovante X"); err != nil {
		t.Fatal(err)
	}
	// Segundo clique.
	err := s.MarcarRepassePagoAMao(ctx, id, staffQuePaga(), "segunda vez, comprovante Y")
	if !errors.Is(err, ErrRepasseInexistente) {
		t.Fatalf("segundo clique = %v, queria recusa: pagou duas vezes", err)
	}

	// E um recusado não passa por aqui.
	recusado := repasseRecusadoParaAjuste(ctx, t, s, "pago_a_mao_recusado")
	if err := s.MarcarRepassePagoAMao(ctx, recusado, staffQuePaga(),
		"quis fechar o recusado"); !errors.Is(err, ErrRepasseInexistente) {
		t.Fatalf("recusado = %v, queria recusa", err)
	}
}

// TestPagamentoAMaoExigeObservacao: sem comprovante do nosso lado, a observação é o
// único lugar onde fica escrito ONDE o dinheiro foi pago. Vazia, ela não serve, e
// espaços em branco não valem como resposta.
func TestPagamentoAMaoExigeObservacao(t *testing.T) {
	s, ctx := freshStore(t)
	id, _ := repassePendenteDeVenda(ctx, t, s, "sem_nota")

	for _, nota := range []string{"", "   ", strings.Repeat("x", MaxNotaDoPagamento+1)} {
		if err := s.MarcarRepassePagoAMao(ctx, id, staffQuePaga(), nota); !errors.Is(err, ErrPagamentoSemNota) {
			t.Errorf("nota de %d caracteres: erro = %v, queria ErrPagamentoSemNota", len(nota), err)
		}
	}
	var estado int16
	if err := s.pool.QueryRow(ctx, `SELECT status FROM rmt_repasse WHERE id = $1`, id).Scan(&estado); err != nil {
		t.Fatal(err)
	}
	if EstadoRepasse(estado) != RepassePendente {
		t.Errorf("estado = %d; a recusa mexeu na linha", estado)
	}
}

// TestPendenteComCadastroDizQueEstaACaminho.
//
// COM O PAGAMENTO À MÃO, O PENDENTE DURA HORAS, e antes durava segundos. O que o
// jogador lê nesse tempo é o que este teste prende: "a caminho", e não "a staff está
// olhando o seu caso".
//
// A diferença não é cosmética. NEEDS_STAFF faz o vendedor abrir ticket no caso NORMAL —
// ele esperaria um atendimento que ninguém precisa fazer, e a fila de suporte encheria
// de gente cujo dinheiro está só aguardando o horário de pagamento.
//
// Conferido no corpo de quem decide o motivo (RepasseDoVendedor): nenhum prazo
// transforma pendente velho em EsperaGente — os baldes são por ESTADO, não por tempo.
// Este teste é o que impede alguém de acrescentar esse prazo sem perceber.
func TestPendenteComCadastroDizQueEstaACaminho(t *testing.T) {
	s, ctx := freshStore(t)
	_, vendedor := repassePendenteDeVenda(ctx, t, s, "a_caminho")

	total, motivo, err := s.RepasseDoVendedor(ctx, vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if total != liquidoDaVendaDeTeste() {
		t.Errorf("total = %d, queria %d", total, liquidoDaVendaDeTeste())
	}
	if motivo != EsperaPagamento {
		t.Errorf("motivo = %d, queria EsperaPagamento (IN_PROGRESS): o vendedor veria "+
			"ticket no caso normal", motivo)
	}
}
