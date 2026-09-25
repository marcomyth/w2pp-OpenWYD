package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// MaxNotaDoPagamento é o teto da observação obrigatória: cabe onde o dinheiro foi pago
// e como achar o comprovante, e não um relatório.
const MaxNotaDoPagamento = 500

// ErrPagamentoSemNota é a recusa de fechar a dívida sem dizer onde ela foi paga.
var ErrPagamentoSemNota = errors.New("store: pagamento a mao exige observacao")

// AcaoRepassePagoAMao é o nome desta ação no log de auditoria.
const AcaoRepassePagoAMao = "REPASSE_PAGO_A_MAO"

// MarcarRepassePagoAMao fecha a dívida que a staff pagou fora do sistema.
//
// POR QUE ELA EXISTE, e por que não deu para ajustar a MarcarRepassePago: aquela casa a
// linha pelo `identifier_saque` e só sai de ENVIADO, porque foi escrita para o aviso da
// processadora. Pagamento à mão não tem identifier de saque nenhum — não passou por
// processadora — e a linha está em PENDENTE. São duas portas diferentes para o mesmo
// estado final, e juntá-las numa só significaria afrouxar a guarda de estado da outra,
// que é o que protege o aviso de saque de fechar a linha errada.
//
// SÓ DE PENDENTE PARA PAGO. Recusado e incerto NÃO passam por aqui: o recusado pede a
// fila de quem precisa de gente, e o incerto é o caso em que o dinheiro PODE já ter
// saído — marcá-lo pago à mão esconderia justamente a dúvida que ele existe para
// levantar. Quem quiser fechá-los passa antes pelo caminho que os resolve.
//
// O QUE VAI EM chegou_centavos é o valor da própria linha, e não um número digitado.
// Essa coluna nasceu para medir a taxa do saque, que era a diferença entre o enviado e o
// que chegou; com pagamento manual não há saque nem taxa de saque, e o que a staff
// mandou é o líquido que a linha diz. Deixá-la nula faria toda conta de receita ter de
// tratar "pago mas sem valor" como caso especial, para sempre.
//
// A OBSERVAÇÃO É OBRIGATÓRIA e a auditoria vai na MESMA transação. Esta é a escrita que
// diz "essa pessoa foi paga", sem nenhum comprovante do nosso lado atrás dela: se for
// errada — vendedor trocado, valor trocado, ou paga duas vezes — a linha da auditoria é
// a única coisa que reconstrói quem clicou e o que ela disse que fez.
//
// A LINHA É TRAVADA antes de mudar, e não é zelo vazio: o estorno do comprador decide
// pelo estado do repasse, e sem o FOR UPDATE as duas ações se cruzariam — o pagamento
// veria pendente, o estorno veria pendente, e o vendedor receberia o dinheiro de uma
// venda cancelada.
func (s *Store) MarcarRepassePagoAMao(ctx context.Context, id int64, ator AtorDoAjuste,
	nota string,
) error {
	nota = strings.TrimSpace(nota)
	if nota == "" || len(nota) > MaxNotaDoPagamento {
		return ErrPagamentoSemNota
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var valor, vendedor int64
		err := tx.QueryRow(ctx, `
			SELECT valor_centavos, vendedor_conta FROM rmt_repasse
			 WHERE id = $1 AND status = $2
			 FOR UPDATE`, id, repassePendente).Scan(&valor, &vendedor)
		if errors.Is(err, pgx.ErrNoRows) {
			// Não existe, ou não está pendente. É a mesma resposta de propósito, como no
			// ajuste: a tela não deve poder descobrir o estado de uma linha tentando
			// mexer nela. E o caso que importa é o segundo clique no mesmo botão — ele
			// tem de recusar, e não pagar de novo.
			return fmt.Errorf("repasse %d: %w", id, ErrRepasseInexistente)
		}
		if err != nil {
			return fmt.Errorf("store: lendo o repasse %d para pagar a mao: %w", id, err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_repasse
			   SET status = $2, chegou_centavos = $3, pago_em = now(),
			       pago_a_mao_nota = $4, pago_a_mao_por = $5
			 WHERE id = $1 AND status = $6`,
			id, repassePago, valor, nota, ator.Nome, repassePendente); err != nil {
			return fmt.Errorf("store: marcando pago a mao o repasse %d: %w", id, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_audit_log
			    (actor_account_id, actor_role, action, target_account_id, old_value, new_value)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			ator.ContaID, ator.Papel, AcaoRepassePagoAMao, vendedor,
			fmt.Sprintf(`{"repasse":%d,"status":%d}`, id, repassePendente),
			fmt.Sprintf(`{"repasse":%d,"status":%d,"centavos":%d,"nota":%q}`,
				id, repassePago, valor, nota)); err != nil {
			return fmt.Errorf("store: pagamento a mao do repasse %d: registrando na auditoria: %w", id, err)
		}
		return nil
	})
}
