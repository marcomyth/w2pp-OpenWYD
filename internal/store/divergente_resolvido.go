package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrDivergenteNaoEstaAberta é a recusa prevista da saída do valor divergente:
// alguém já resolveu, ou a linha nunca esteve nesse estado.
var ErrDivergenteNaoEstaAberta = errors.New("store: a cobranca divergente nao esta aberta")

// ResolverDivergenteDevolvido fecha uma cobrança de valor divergente que a staff
// resolveu POR FORA, devolvendo o dinheiro ao comprador.
//
// POR QUE ESTA SAÍDA PRECISA EXISTIR, mesmo sem nenhum caminho automático para o
// dinheiro: a cobrança divergente fica ABERTA, e enquanto ela estiver aberta duas
// pessoas estão presas no jogo.
//
//   - O COMPRADOR não consegue comprar mais nada: o índice de uma cobrança aberta
//     por comprador o recusa em toda tentativa.
//   - O ITEM DO VENDEDOR fica marcado, porque o anúncio continua ativo esperando
//     uma cobrança que nunca fecha.
//
// Sem esta função, a staff resolveria o dinheiro no painel da processadora e as duas
// continuariam travadas para sempre — nada no sistema saberia que acabou.
//
// ELA NÃO MEXE EM DINHEIRO NENHUM, e é isso que a distingue de um botão de
// "devolver": o que ela faz é REGISTRAR que uma pessoa já devolveu, e destravar o
// que estava preso por causa disso. Por isso a nota é obrigatória de quem chama, e
// por isso ela vai inteira para a auditoria.
//
// O ESTADO É PAGA_SEM_ITEM, e não "cancelada", porque é a verdade: o dinheiro
// entrou e o item não saiu. Cancelada diria que a cobrança morreu sem pagamento, o
// que apagaria do registro o fato de ter havido dinheiro no meio. E o reembolso vai
// para CONCLUÍDO porque foi feito — por uma pessoa, e não por nós.
//
// O ITEM VOLTA PELO CAMINHO DE SEMPRE: fechada a cobrança, a reconciliação do login
// do vendedor encerra o anúncio sem barraca e solta o cadeado do baú. Escrever no
// baú daqui, com o vendedor possivelmente em jogo, seria escrever por cima do dono
// dele.
func (s *Store) ResolverDivergenteDevolvido(ctx context.Context, cobrancaID int64,
	ator AtorDaStaff, nota string,
) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var compradorConta int64
		var status int16
		var divergente *int64
		err := tx.QueryRow(ctx, `
			SELECT comprador_conta, status, valor_divergente_centavos
			  FROM rmt_cobranca WHERE id = $1 FOR UPDATE`, cobrancaID).
			Scan(&compradorConta, &status, &divergente)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDivergenteNaoEstaAberta
		}
		if err != nil {
			return fmt.Errorf("store: resolvendo a divergente %d: %w", cobrancaID, err)
		}
		// AS DUAS CONDIÇÕES, e não só uma: a linha tem de ser divergente E estar
		// aberta. Sem a segunda, dois cliques fechariam duas vezes; sem a primeira,
		// esta saída viraria um jeito de fechar qualquer cobrança aberta pela tela
		// errada.
		if divergente == nil || status != cobrancaAberta {
			return ErrDivergenteNaoEstaAberta
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_cobranca
			   SET status = $2, encerrada_em = now(), reembolso_status = $3
			 WHERE id = $1`, cobrancaID, cobrancaPagaSemItem, reembolsoConcluido); err != nil {
			return fmt.Errorf("store: resolvendo a divergente %d: %w", cobrancaID, err)
		}
		// A AUDITORIA NA MESMA TRANSAÇÃO, como no resto do reembolso: aqui se mexe
		// no dinheiro de alguém, e uma mudança aplicada sem registro é exatamente a
		// que ninguém consegue explicar depois. Falhando o registro, a mudança não
		// acontece.
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_audit_log
			    (actor_account_id, actor_role, action, target_account_id, old_value, new_value)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			ator.ContaID, ator.Papel, "rmt_divergente_devolvido", compradorConta,
			fmt.Sprintf(`{"cobranca":%d,"divergente_centavos":%d}`, cobrancaID, *divergente),
			fmt.Sprintf(`{"cobranca":%d,"nota":%q}`, cobrancaID, nota)); err != nil {
			return fmt.Errorf("store: divergente %d: registrando na auditoria: %w", cobrancaID, err)
		}
		return nil
	})
}
