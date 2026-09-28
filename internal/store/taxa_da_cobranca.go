package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrTaxaImpossivel é uma taxa que não pode ser verdade para aquela venda.
var ErrTaxaImpossivel = errors.New("store: taxa impossivel para o valor da cobranca")

// InformarTaxaDaCobranca é a ÚNICA saída do repasse segurado por taxa desconhecida.
//
// Um estado que segura dinheiro sem porta de saída é uma armadilha, não uma proteção:
// o repasse pararia no 6 para sempre e o vendedor esperaria um dia que não chega. Esta
// função é a porta, e existe no mesmo commit que o estado justamente por isso.
//
// GRAVA A TAXA NA COBRANÇA E RECALCULA O LÍQUIDO, na mesma transação. Separar as duas
// coisas abriria a janela em que a taxa está escrita e o repasse continua com o valor
// cheio — e é uma janela que termina em pagamento errado, porque a fila de pagar não
// pergunta se a taxa já foi aplicada, ela lê valor_centavos e manda.
//
// SÓ MEXE NO ESTADO 6. Não é conservadorismo: um repasse já ENVIADO teria o valor
// alterado depois de a ponte ter recebido a ordem, e um já PAGO viraria um registro que
// contradiz o extrato. A guarda é o que garante que recalcular só acontece onde nada
// saiu ainda.
//
// Devolve false quando não havia repasse segurado naquela cobrança — chamada repetida,
// ou cobrança que nunca segurou nada. Não é erro: é a resposta, e quem chama decide.
func (s *Store) InformarTaxaDaCobranca(ctx context.Context, cobrancaID int64, taxaCentavos int64, ator AtorDoRepasse) (bool, error) {
	if taxaCentavos < 0 {
		return false, fmt.Errorf("%w: taxa %d negativa", ErrTaxaImpossivel, taxaCentavos)
	}
	var liberou bool
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// O BRUTO VEM DA LINHA DO REPASSE, travada, e não do que quem chama informou.
		//
		// Recalcular a partir de um valor recebido de fora deixaria o líquido depender
		// de quem digitou. O bruto está gravado desde que o repasse nasceu; é ele que
		// manda.
		var bruto int64
		err := tx.QueryRow(ctx, `
			SELECT coalesce(r.bruto_centavos, r.valor_centavos)
			  FROM rmt_repasse r
			 WHERE r.cobranca_id = $1 AND r.status = $2
			 FOR UPDATE`, cobrancaID, repasseSemTaxa).Scan(&bruto)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: lendo o repasse segurado da cobranca %d: %w", cobrancaID, err)
		}
		if taxaCentavos >= bruto {
			// Continua segurado. Uma taxa que come a venda inteira não é um líquido
			// pequeno: é zero ou negativo, e nenhum dos dois é um pagamento. Sair
			// daqui com esse número só trocaria o problema de lugar.
			return fmt.Errorf("%w: taxa %d >= bruto %d na cobranca %d",
				ErrTaxaImpossivel, taxaCentavos, bruto, cobrancaID)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_cobranca SET taxa_centavos = $2 WHERE id = $1`,
			cobrancaID, taxaCentavos); err != nil {
			return fmt.Errorf("store: gravando a taxa da cobranca %d: %w", cobrancaID, err)
		}
		// Volta para PENDENTE, que é a fila de pagar. Quem resolveu fica gravado: numa
		// disputa a pergunta é "quem disse que a taxa era essa, e quando", e um valor
		// de dinheiro que muda sem dono não responde.
		if _, err := tx.Exec(ctx, `
			UPDATE rmt_repasse
			   SET valor_centavos = $2, status = $3, resolvido_em = now(), resolvido_por = $4
			 WHERE cobranca_id = $1 AND status = $5`,
			cobrancaID, bruto-taxaCentavos, repassePendente, ator.Nome, repasseSemTaxa); err != nil {
			return fmt.Errorf("store: liberando o repasse da cobranca %d: %w", cobrancaID, err)
		}
		liberou = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return liberou, nil
}
