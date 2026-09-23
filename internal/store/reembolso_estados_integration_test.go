//go:build integration

// A máquina de estados do reembolso: o que pode e, sobretudo, o que NÃO pode.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

// poeEstado força a linha num estado, para o teste partir de onde quiser.
func poeEstado(ctx context.Context, t *testing.T, s *Store, id int64, estado int16) {
	t.Helper()
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET reembolso_status = $2 WHERE id = $1`, id, estado); err != nil {
		t.Fatalf("pondo o estado %d: %v", estado, err)
	}
}

// AS TRANSIÇÕES PROIBIDAS, todas de uma vez.
//
// Elas existem porque as mensagens chegam FORA DE ORDEM — é o normal de quem fala
// com uma processadora pela rede. Sem guarda, o estado anda para trás: a staff
// marca "resolvido na mão" e meio segundo depois chega a resposta atrasada de uma
// tentativa antiga, que devolve a linha para RECUSADO. A página do comprador passa
// a dizer que há algo pendente sobre um dinheiro que já voltou.
//
// A tabela inteira num teste só porque o valor está no CONJUNTO: cada linha
// isolada passaria com uma função que recusa sempre, e a prova de que ela não
// recusa sempre está no teste das transições permitidas, logo abaixo.
func TestAsTransicoesProibidasDoReembolso(t *testing.T) {
	s, ctx := freshStore(t)
	_, cobranca := recusadoNaFila(ctx, t, s, "estados", "erro")

	casos := []struct {
		nome   string
		de     int16
		muda   func(context.Context, int64) error
		porque string
	}{
		{"pedir duas vezes", reembolsoPedido, s.MarcarReembolsoPedido,
			"criaria um segundo pedido sobre o mesmo dinheiro"},
		{"pedir depois de concluido", reembolsoConcluido, s.MarcarReembolsoPedido,
			"o dinheiro ja voltou; pedir de novo e pedir duas vezes"},
		{"pedir depois de recusado", reembolsoRecusado, s.MarcarReembolsoPedido,
			"o recusado passa pela staff, que o poe de volta em pendente"},
		{"concluir sem ter pedido", reembolsoPendente, s.MarcarReembolsoConcluido,
			"nada foi pedido; nao ha o que concluir"},
		{"concluir um recusado", reembolsoRecusado, s.MarcarReembolsoConcluido,
			"a saida do recusado e pela staff, com registro de quem foi"},
		{"recusar o que ja concluiu", reembolsoConcluido, func(ctx context.Context, id int64) error {
			return s.MarcarReembolsoRecusado(ctx, id, "resposta atrasada")
		}, "CONCLUIDO E TERMINAL: o dinheiro voltou, e isso nao se desfaz por uma mensagem atrasada"},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			poeEstado(ctx, t, s, cobranca, c.de)

			err := c.muda(ctx, cobranca)

			var recusada *TransicaoRecusada
			if !errors.As(err, &recusada) {
				t.Fatalf("erro = %v, quero TransicaoRecusada: %s", err, c.porque)
			}
			if recusada.De != c.de {
				t.Errorf("a recusa diz que veio de %d, quero %d — quem for ler o aviso "+
					"precisa do estado encontrado", recusada.De, c.de)
			}
			if st, _ := reembolsoDe(ctx, t, s, cobranca); st != c.de {
				t.Errorf("o estado virou %d apesar da recusa", st)
			}
		})
	}
}

// E AS PERMITIDAS PASSAM, que é o que dá valor ao teste de cima. Sem esta metade,
// uma máquina que recusasse tudo passaria em todas as linhas de lá.
func TestAsTransicoesPermitidasDoReembolso(t *testing.T) {
	s, ctx := freshStore(t)
	_, cobranca := recusadoNaFila(ctx, t, s, "permitidas", "erro")

	poeEstado(ctx, t, s, cobranca, reembolsoPendente)
	if err := s.MarcarReembolsoPedido(ctx, cobranca); err != nil {
		t.Fatalf("pendente -> pedido: %v", err)
	}
	if err := s.MarcarReembolsoConcluido(ctx, cobranca); err != nil {
		t.Fatalf("pedido -> concluido: %v", err)
	}
	if st, _ := reembolsoDe(ctx, t, s, cobranca); st != reembolsoConcluido {
		t.Errorf("estado = %d, quero concluido", st)
	}

	// E a recusa a partir dos dois estados de onde ela pode vir.
	for _, de := range []int16{reembolsoPendente, reembolsoPedido} {
		poeEstado(ctx, t, s, cobranca, de)
		if err := s.MarcarReembolsoRecusado(ctx, cobranca, "403"); err != nil {
			t.Errorf("%d -> recusado: %v", de, err)
		}
	}
}

// A AUDITORIA GUARDA O CÓDIGO DE ERRO DA PROCESSADORA.
//
// É a única coisa que explica POR QUE o reembolso falhou, e a ação da staff apaga
// o campo. Se ela não for lida ANTES da escrita, some exatamente no momento em que
// alguém mexe — e a auditoria fica dizendo "estava recusado" sem dizer de quê.
//
// O bug real que isto pega: o RETURNING de um UPDATE devolve a linha NOVA. Ler o
// `reembolso_erro` no RETURNING de um UPDATE que grava NULL nele devolve o NULL
// que a própria instrução acabou de gravar.
func TestAAuditoriaGuardaOCodigoDeErroDaProcessadora(t *testing.T) {
	s, ctx := freshStore(t)
	const codigo = "403 reembolso nao habilitado para esta conta"
	_, cobranca := recusadoNaFila(ctx, t, s, "auditerro", codigo)

	if err := s.ResolverReembolsoNaMao(ctx, cobranca,
		staffDeTeste(ctx, t, s, "staff_auditerro")); err != nil {
		t.Fatalf("resolvendo na mao: %v", err)
	}

	var antigo string
	if err := s.pool.QueryRow(ctx, `
		SELECT coalesce(old_value::text, '') FROM admin_audit_log
		 WHERE action = 'rmt_reembolso_resolvido_na_mao' ORDER BY id DESC LIMIT 1`).
		Scan(&antigo); err != nil {
		t.Fatalf("lendo a auditoria: %v", err)
	}

	if !contemTexto(antigo, codigo) {
		t.Errorf("a auditoria guardou %q, sem o codigo da processadora; ela fica dizendo "+
			"\"estava recusado\" sem dizer de que", antigo)
	}
}

func contemTexto(todo, pedaco string) bool {
	return len(pedaco) > 0 && len(todo) >= len(pedaco) && indexDe(todo, pedaco) >= 0
}

func indexDe(todo, pedaco string) int {
	for i := 0; i+len(pedaco) <= len(todo); i++ {
		if todo[i:i+len(pedaco)] == pedaco {
			return i
		}
	}
	return -1
}
