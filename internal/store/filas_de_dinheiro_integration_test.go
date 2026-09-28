//go:build integration

// O contador do menu tem de dar exatamente o que cada tela lista.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
)

// O CONTADOR BATE COM AS QUATRO LISTAS, item por item.
//
// É o teste que sustenta o contador inteiro. Um número no menu que diverge da tela é pior
// do que não ter número: ou manda a pessoa abrir uma tela vazia, ou — muito pior — diz
// zero enquanto há dinheiro parado, e aí ninguém abre.
//
// Ele compara contra as MESMAS funções que as telas chamam, e não contra um número
// escrito à mão: um valor esperado fixo passaria a mentir junto com o código no dia em que
// alguém mudasse o que uma fila significa.
func TestOContadorBateComAsQuatroListas(t *testing.T) {
	s, ctx := freshStore(t)
	montaAsQuatroFilas(ctx, t, s)

	c, err := s.ContarFilasDeDinheiro(ctx)
	if err != nil {
		t.Fatal(err)
	}

	repasses, err := s.RepassesQuePrecisamDeGente(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reembolsos, err := s.ReembolsosRecusados(ctx)
	if err != nil {
		t.Fatal(err)
	}
	orfaos, err := s.PagamentosOrfaos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	divergentes, err := s.ValoresDivergentes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	casos := []struct {
		nome    string
		contado int
		listado int
	}{
		{"repasses", c.Repasses, len(repasses)},
		{"reembolsos", c.Reembolsos, len(reembolsos)},
		{"orfaos", c.Orfaos, len(orfaos)},
		{"divergentes", c.Divergentes, len(divergentes)},
	}
	for _, caso := range casos {
		if caso.contado != caso.listado {
			t.Errorf("%s: o menu conta %d e a tela lista %d",
				caso.nome, caso.contado, caso.listado)
		}
	}
	if soma := len(repasses) + len(reembolsos) + len(orfaos) + len(divergentes); c.Total() != soma {
		t.Errorf("Total() = %d, a soma das quatro e %d", c.Total(), soma)
	}
}

// E COM AS FILAS VAZIAS O CONTADOR É ZERO, sem erro.
//
// Parece óbvio e não é: a consulta junta quatro subselects, e um deles devolvendo NULL em
// vez de 0 faria o Scan falhar — a tela do menu quebraria em TODA página de um servidor
// novo, que é justamente onde ninguém está olhando.
func TestContadorComTudoVazioEhZero(t *testing.T) {
	s, ctx := freshStore(t)

	c, err := s.ContarFilasDeDinheiro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c.Total() != 0 {
		t.Errorf("com tudo vazio o total = %d, queria 0: %+v", c.Total(), c)
	}
}

// montaAsQuatroFilas põe uma linha em cada fila, incluindo a que já foi esquecida uma vez.
func montaAsQuatroFilas(ctx context.Context, t *testing.T, s *Store) {
	t.Helper()

	// 1) Um repasse RECUSADO: a fila de gente mais comum.
	v1 := montaVenda(ctx, t, s, "fila_recusado")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v1.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	var idRecusado int64
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM rmt_repasse WHERE vendedor_conta = $1`, v1.vendedor).Scan(&idRecusado); err != nil {
		t.Fatal(err)
	}
	http403 := int32(403)
	if err := s.MarcarRepasseRecusado(ctx, idRecusado, &http403, "", "403"); err != nil {
		t.Fatal(err)
	}

	// 2) Um repasse PENDENTE SEM DOCUMENTO: a linha que sumia do JOIN normal, e que o
	// contador precisa achar pelo mesmo caminho que a tela.
	v2 := montaVenda(ctx, t, s, "fila_sem_doc")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v2.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_recebedor SET documento = NULL WHERE account_id = $1`, v2.vendedor); err != nil {
		t.Fatal(err)
	}

	// 3) Um pagamento órfão.
	quinhentos := int64(500)
	if err := s.RegistrarPagamentoOrfao(ctx, PagamentoOrfao{
		Identifier: "orfao-do-contador", ValorCentavos: &quinhentos, Motivo: MotivoOrfaoContestado,
	}); err != nil {
		t.Fatal(err)
	}

	// 4) Um reembolso recusado e 5) um valor divergente, os dois na tabela de cobrança.
	v3 := montaVenda(ctx, t, s, "fila_reemb")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v3.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET reembolso_status = $2
		 WHERE referencia_externa = $1`, v3.ref, reembolsoRecusado); err != nil {
		t.Fatal(err)
	}

	v4 := montaVenda(ctx, t, s, "fila_diverg")
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v4.ref, dentroDoPrazo(), HoraDaProcessadora,
		precoEmCentavos+1, taxaZero()); err != nil {
		t.Fatal(err)
	}
}
