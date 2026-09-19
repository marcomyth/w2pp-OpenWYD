package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Transferência de Cash e RMT entre contas — o pagamento da Loja do Servidor.
//
// A vitrine deixa o vendedor cobrar em três moedas. Ouro é do personagem e o
// tmServer move sozinho, dentro do próprio laço; Cash e RMT são da CONTA e
// moram aqui: `account.donate_balance` (a carteira que a recarga por PIX
// credita) e `account.rmt_balance` (0076_saldo_rmt).
//
// Débito e crédito acontecem na MESMA transação, e as duas linhas são travadas
// em ordem crescente de id. A ordem importa: duas compras cruzadas entre as
// mesmas contas, cada uma travando a sua primeiro, se esperariam para sempre.

// MoedaDeConta é a carteira que a transferência move.
type MoedaDeConta int

const (
	MoedaCash MoedaDeConta = iota + 1
	MoedaRMT
)

// ErrSaldoInsuficiente e ErrMoedaInvalida são respostas previstas, não falhas:
// quem chama traduz em recusa para o jogador.
var (
	ErrSaldoInsuficiente = errors.New("store: saldo insuficiente")
	ErrMoedaInvalida     = errors.New("store: moeda desconhecida")
	ErrValorInvalido     = errors.New("store: valor de transferencia invalido")
)

// tetoDaCarteira é o limite de uma carteira, o mesmo do INTEGER do Postgres.
// Passar disto estouraria a coluna, então a transferência para antes.
const tetoDaCarteira = int32(2_000_000_000)

func colunaDaMoeda(m MoedaDeConta) (string, error) {
	switch m {
	case MoedaCash:
		return "donate_balance", nil
	case MoedaRMT:
		return "rmt_balance", nil
	default:
		return "", ErrMoedaInvalida
	}
}

// TransferePlayerBalance move `valor` de uma conta para outra e devolve os dois
// saldos já atualizados. Nada se move se o pagador não tiver o valor inteiro.
func (s *Store) TransferePlayerBalance(ctx context.Context, deConta, paraConta int64,
	moeda MoedaDeConta, valor int32, motivo string) (saldoDe int32, saldoPara int32, err error) {
	coluna, err := colunaDaMoeda(moeda)
	if err != nil {
		return 0, 0, err
	}
	if valor <= 0 || valor > tetoDaCarteira {
		return 0, 0, ErrValorInvalido
	}
	if deConta == paraConta {
		return 0, 0, ErrValorInvalido
	}

	err = s.inTx(ctx, func(tx pgx.Tx) error {
		// Trava as duas carteiras em ordem crescente de id, sempre.
		primeira, segunda := deConta, paraConta
		if segunda < primeira {
			primeira, segunda = segunda, primeira
		}
		saldos := map[int64]int32{}
		for _, id := range []int64{primeira, segunda} {
			var bal int32
			erro := tx.QueryRow(ctx,
				`SELECT `+coluna+` FROM account WHERE id = $1 FOR UPDATE`, id).Scan(&bal)
			if errors.Is(erro, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if erro != nil {
				return fmt.Errorf("store: transfere: lendo saldo a=%d: %w", id, erro)
			}
			saldos[id] = bal
		}
		if saldos[deConta] < valor {
			return ErrSaldoInsuficiente
		}
		if int64(saldos[paraConta])+int64(valor) > int64(tetoDaCarteira) {
			return ErrValorInvalido
		}

		if erro := tx.QueryRow(ctx,
			`UPDATE account SET `+coluna+` = `+coluna+` - $2 WHERE id = $1 RETURNING `+coluna,
			deConta, valor).Scan(&saldoDe); erro != nil {
			return fmt.Errorf("store: transfere: debitando a=%d: %w", deConta, erro)
		}
		if erro := tx.QueryRow(ctx,
			`UPDATE account SET `+coluna+` = `+coluna+` + $2 WHERE id = $1 RETURNING `+coluna,
			paraConta, valor).Scan(&saldoPara); erro != nil {
			return fmt.Errorf("store: transfere: creditando a=%d: %w", paraConta, erro)
		}

		// Fica no mesmo diário das outras mexidas em carteira, para dar para
		// reconstruir depois de onde saiu cada moeda.
		registro, erro := json.Marshal(map[string]any{
			"de": deConta, "para": paraConta, "moeda": coluna, "valor": valor,
			"saldo_de": saldoDe, "saldo_para": saldoPara, "motivo": motivo,
		})
		if erro != nil {
			return fmt.Errorf("store: transfere: montando o registro: %w", erro)
		}
		return donateAudit(ctx, tx, nil, deConta, "transferencia", nil, registro)
	})
	if err != nil {
		return 0, 0, err
	}
	return saldoDe, saldoPara, nil
}
