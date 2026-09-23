package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// As transições do reembolso — o caminho de volta do dinheiro que chegou tarde.
//
// O estado PAGA_SEM_ITEM era dívida com uma pessoa e mais nada: a linha ficava
// numa fila para alguém olhar. A decisão da Hanna deu destino a ela — reembolso,
// sem item. Este arquivo é a máquina de estados desse destino.
//
// A TRANSIÇÃO E A AUDITORIA ACONTECEM NA MESMA TRANSAÇÃO, e isso é mais estrito
// do que o resto do painel faz. É de propósito: aqui se mexe no dinheiro de
// alguém, e uma mudança aplicada sem registro é exatamente a mudança que ninguém
// consegue explicar depois. Se o registro falha, a mudança não acontece.

// ErrReembolsoNaoEstaRecusado é a recusa prevista das ações da staff: elas só
// valem sobre um reembolso que a processadora recusou.
//
// Existe para a tela não conseguir "consertar" o que não está quebrado — mandar
// de novo um pedido que está em análise criaria um segundo pedido sobre o mesmo
// dinheiro.
var ErrReembolsoNaoEstaRecusado = errors.New("store: o reembolso nao esta recusado")

// AtorDaStaff é quem fez, para a auditoria. Sem ele não há transição: a
// assinatura obriga o chamador a dizer quem foi, em vez de deixar isso
// opcional e descobrir depois que metade das linhas não tem autor.
type AtorDaStaff struct {
	ContaID int64
	Papel   string
}

// MarcarReembolsoPendente registra que devemos um reembolso, antes de pedir.
//
// Roda junto com a marcação de PAGA_SEM_ITEM, na confirmação do pagamento
// atrasado. Sem este estado, um pedido que nunca chegou a ser feito ficaria
// indistinguível de um que não era devido.
func (s *Store) MarcarReembolsoPendente(ctx context.Context, cobrancaID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET reembolso_status = $2
		 WHERE id = $1 AND reembolso_status IS NULL`, cobrancaID, reembolsoPendente)
	if err != nil {
		return fmt.Errorf("store: marcar reembolso pendente %d: %w", cobrancaID, err)
	}
	return nil
}

// MarcarReembolsoPedido registra que a processadora ACEITOU o pedido.
//
// A data é gravada aqui e não quando decidimos pedir: é dela que a página conta
// os até dois dias úteis da análise, e o relógio que importa para a pessoa começa
// quando o pedido entrou na fila deles, não na nossa.
func (s *Store) MarcarReembolsoPedido(ctx context.Context, cobrancaID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca
		   SET reembolso_status = $2, reembolso_pedido_em = now(), reembolso_erro = NULL
		 WHERE id = $1`, cobrancaID, reembolsoPedido)
	if err != nil {
		return fmt.Errorf("store: marcar reembolso pedido %d: %w", cobrancaID, err)
	}
	return nil
}

// MarcarReembolsoRecusado guarda a recusa da processadora COM O CÓDIGO DELA.
//
// O código vai inteiro e não traduzido: quem for olhar precisa do que eles
// disseram, não da nossa interpretação. A diferença entre "reembolso não
// habilitado para esta conta" e "conta do seller não está aprovada" decide o que
// a Hanna tem de pedir ao suporte, e as duas viram "falhou" se a gente resumir.
//
// NÃO SE TENTA DE NOVO EM LAÇO. Recusado espera uma pessoa — é por isso que existe
// a ação da staff, e é por isso que este estado não sai da página do comprador
// sozinho.
func (s *Store) MarcarReembolsoRecusado(ctx context.Context, cobrancaID int64, codigo string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET reembolso_status = $2, reembolso_erro = $3
		 WHERE id = $1`, cobrancaID, reembolsoRecusado, codigo)
	if err != nil {
		return fmt.Errorf("store: marcar reembolso recusado %d: %w", cobrancaID, err)
	}
	return nil
}

// MarcarReembolsoConcluido registra que o dinheiro voltou.
func (s *Store) MarcarReembolsoConcluido(ctx context.Context, cobrancaID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca
		   SET reembolso_status = $2, reembolso_erro = NULL
		 WHERE id = $1`, cobrancaID, reembolsoConcluido)
	if err != nil {
		return fmt.Errorf("store: marcar reembolso concluido %d: %w", cobrancaID, err)
	}
	return nil
}

// ReabrirReembolsoRecusado é o "tentar de novo" da staff: volta o reembolso para
// PENDENTE, para que o caminho automático o pegue na próxima passada.
//
// Volta para PENDENTE e não direto para PEDIDO porque pedir é falar com a
// processadora, e esta função não fala com ninguém — ela só põe a linha de volta
// na fila. Quem pede é quem sabe pedir.
//
// SÓ VALE SOBRE UM RECUSADO, e o WHERE é quem garante. Reabrir um que está em
// análise criaria um segundo pedido sobre o mesmo dinheiro.
func (s *Store) ReabrirReembolsoRecusado(ctx context.Context, cobrancaID int64, ator AtorDaStaff) error {
	return s.transicaoDaStaff(ctx, cobrancaID, ator, reembolsoPendente,
		"rmt_reembolso_tentar_de_novo")
}

// ResolverReembolsoNaMao é para quando a staff devolveu o dinheiro pelo painel da
// processadora, fora do nosso caminho.
//
// O dinheiro voltou de verdade; o que falta é o nosso registro saber disso. Sem
// esta ação a linha ficaria em RECUSADO para sempre e a página do comprador
// continuaria dizendo que há algo pendente que já não há.
//
// SÓ VALE SOBRE UM RECUSADO, pelo mesmo motivo: marcar como devolvido um
// reembolso que a processadora ainda está analisando faria a página mentir, e
// depois chegaria a devolução de verdade sobre um registro que já dizia
// concluído.
func (s *Store) ResolverReembolsoNaMao(ctx context.Context, cobrancaID int64, ator AtorDaStaff) error {
	return s.transicaoDaStaff(ctx, cobrancaID, ator, reembolsoConcluido,
		"rmt_reembolso_resolvido_na_mao")
}

// transicaoDaStaff move um reembolso RECUSADO para outro estado e registra quem
// fez, na MESMA transação.
//
// Mesma transação e não duas escritas: ação com dinheiro aplicada sem registro é
// a ação que ninguém consegue explicar depois. Se a auditoria falha, a transição
// não acontece.
func (s *Store) transicaoDaStaff(ctx context.Context, cobrancaID int64, ator AtorDaStaff,
	destino int16, acao string,
) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var compradorConta int64
		var erroAntigo *string
		err := tx.QueryRow(ctx, `
			UPDATE rmt_cobranca
			   SET reembolso_status = $2, reembolso_erro = NULL
			 WHERE id = $1 AND reembolso_status = $3
			RETURNING comprador_conta, reembolso_erro`,
			cobrancaID, destino, reembolsoRecusado).Scan(&compradorConta, &erroAntigo)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReembolsoNaoEstaRecusado
		}
		if err != nil {
			return fmt.Errorf("store: %s na cobranca %d: %w", acao, cobrancaID, err)
		}

		// O alvo da auditoria é o COMPRADOR, que é de quem é o dinheiro. A
		// cobrança vai no valor, para a linha do log apontar para qual foi.
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_audit_log
			    (actor_account_id, actor_role, action, target_account_id, old_value, new_value)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			ator.ContaID, ator.Papel, acao, compradorConta,
			fmt.Sprintf(`{"cobranca":%d,"reembolso":"recusado"}`, cobrancaID),
			fmt.Sprintf(`{"cobranca":%d,"reembolso":%d}`, cobrancaID, destino)); err != nil {
			return fmt.Errorf("store: %s: registrando na auditoria: %w", acao, err)
		}
		return nil
	})
}

// ReembolsoRecusadoNaFila é uma linha da tela da staff: dinheiro parado esperando
// gente.
type ReembolsoRecusadoNaFila struct {
	CobrancaID     int64
	CompradorConta int64
	ValorCentavos  int64
	Identifier     string
	Erro           string
}

// ReembolsosRecusados lista o que travou, para a tela da staff.
//
// Devolve o identifier da processadora e o código de erro DELES porque é com
// esses dois que uma pessoa resolve: o identifier acha a venda no painel deles, e
// o código diz o que aconteceu. Sem os dois, a tela só consegue dizer "deu erro".
func (s *Store) ReembolsosRecusados(ctx context.Context) ([]ReembolsoRecusadoNaFila, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, comprador_conta, valor_centavos,
		       coalesce(identifier_syncpay, ''), coalesce(reembolso_erro, '')
		  FROM rmt_cobranca
		 WHERE reembolso_status = $1
		 ORDER BY reembolso_pedido_em NULLS FIRST, id`, reembolsoRecusado)
	if err != nil {
		return nil, fmt.Errorf("store: reembolsos recusados: %w", err)
	}
	defer rows.Close()
	var fila []ReembolsoRecusadoNaFila
	for rows.Next() {
		var r ReembolsoRecusadoNaFila
		if err := rows.Scan(&r.CobrancaID, &r.CompradorConta, &r.ValorCentavos,
			&r.Identifier, &r.Erro); err != nil {
			return nil, fmt.Errorf("store: reembolsos recusados: %w", err)
		}
		fila = append(fila, r)
	}
	return fila, rows.Err()
}
