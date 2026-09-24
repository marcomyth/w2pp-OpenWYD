package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jeanluca/w2pp-openwyd/internal/pilha"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Donate top-up persistence (payment-method-agnostic). Postgres owns the payer
// profile (donate_payer_profile) and the top-up orders (donate_topup_order); the
// portal has no database of its own. The credit of a paid order happens inside
// ConfirmTopupOrder, in the same transaction that flips the order to PAID, with
// no moderator id — unlike CreditDonateBalance. UNIQUE(external_reference) plus
// the FOR UPDATE + status check make the credit idempotent: a replayed gateway
// webhook credits exactly once. Crediting account.donate_balance is a partial
// UPDATE, so no tmServer save clobbers it (same as CreditDonateBalance, donate.go).

// Top-up order status values, matching webv1.TopupStatus.
const (
	TopupStatusPending int16 = 1
	TopupStatusPaid    int16 = 2
)

// ErrDuplicateExternalRef is returned by CreateTopupOrder when external_reference
// already exists — the portal reused a UUID.
var ErrDuplicateExternalRef = errors.New("store: duplicate topup external_reference")

// TopupConfirmOutcome is the result of ConfirmTopupOrder, mirroring
// webv1.TopupResult without the wire dependency.
type TopupConfirmOutcome int

const (
	// TopupConfirmed means the order was PENDING and was credited now.
	TopupConfirmed TopupConfirmOutcome = iota
	// TopupAlreadyConfirmed means the order was already PAID — not re-credited.
	TopupAlreadyConfirmed
	// TopupNotFound means no order has that external_reference.
	TopupNotFound
)

// SavePayerProfile upserts the payer's name + CPF by account id. cpf is expected
// pre-normalized to 11 digits by the caller. A missing account (FK violation)
// returns ErrNotFound.
func (s *Store) SavePayerProfile(ctx context.Context, accountID int64, name, cpf string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO donate_payer_profile (account_id, name, cpf, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id) DO UPDATE
			SET name = EXCLUDED.name, cpf = EXCLUDED.cpf, updated_at = now()`,
		accountID, name, cpf)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: save payer profile a=%d: %w", accountID, err)
	}
	return nil
}

// GetPayerProfile returns the payer's name + CPF. found is false (no error) when
// the account has no profile yet.
func (s *Store) GetPayerProfile(ctx context.Context, accountID int64) (name, cpf string, found bool, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT name, cpf FROM donate_payer_profile WHERE account_id = $1`, accountID).Scan(&name, &cpf)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("store: get payer profile a=%d: %w", accountID, err)
	}
	return name, cpf, true, nil
}

// CreateTopupOrder inserts a PENDING order and returns its id. A reused
// external_reference (unique_violation) returns ErrDuplicateExternalRef; a
// missing account (FK violation) returns ErrNotFound.
func (s *Store) CreateTopupOrder(ctx context.Context, o domain.TopupOrder) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO donate_topup_order
			(external_reference, account_id, credits, amount_cents, payment_method, status, pacote_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		o.ExternalReference, o.AccountID, o.Credits, o.AmountCents, o.PaymentMethod,
		TopupStatusPending, textoOuNulo(o.PacoteID),
	).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation on external_reference
			return 0, ErrDuplicateExternalRef
		case "23503": // foreign_key_violation on account_id
			return 0, ErrNotFound
		}
	}
	if err != nil {
		return 0, fmt.Errorf("store: create topup order ref=%q: %w", o.ExternalReference, err)
	}
	return id, nil
}

// ConfirmTopupOrder is the idempotency core: it locks the order by
// external_reference, and only if it is still PENDING credits the account's
// donate balance and flips the order to PAID — all in one transaction. A missing
// order returns (TopupNotFound, 0); an already-PAID order returns
// (TopupAlreadyConfirmed, currentBalance) WITHOUT crediting again.
func (s *Store) ConfirmTopupOrder(ctx context.Context, externalRef string) (TopupConfirmOutcome, int32, error) {
	var outcome TopupConfirmOutcome
	var balance int32
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var orderID, accountID int64
		var credits int32
		var status int16
		var pacoteID *string
		err := tx.QueryRow(ctx, `
			SELECT id, account_id, credits, status, pacote_id
			FROM donate_topup_order WHERE external_reference = $1 FOR UPDATE`,
			externalRef).Scan(&orderID, &accountID, &credits, &status, &pacoteID)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = TopupNotFound
			return nil
		}
		if err != nil {
			return fmt.Errorf("store: confirm topup: load ref=%q: %w", externalRef, err)
		}

		if status == TopupStatusPaid {
			// Already settled: report the current balance, never re-credit.
			if err := tx.QueryRow(ctx,
				`SELECT donate_balance FROM account WHERE id = $1`, accountID).Scan(&balance); err != nil {
				return fmt.Errorf("store: confirm topup: read balance a=%d: %w", accountID, err)
			}
			outcome = TopupAlreadyConfirmed
			return nil
		}

		// PENDING: credit + mark PAID together (partial UPDATE, safe vs tmServer saves).
		if err := tx.QueryRow(ctx,
			`UPDATE account SET donate_balance = donate_balance + $2 WHERE id = $1 RETURNING donate_balance`,
			accountID, credits).Scan(&balance); err != nil {
			return fmt.Errorf("store: confirm topup: credit a=%d: %w", accountID, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE donate_topup_order SET status = $2, confirmed_at = now() WHERE id = $1`,
			orderID, TopupStatusPaid); err != nil {
			return fmt.Errorf("store: confirm topup: mark paid id=%d: %w", orderID, err)
		}

		// OS BRINDES ENTRAM NA MESMA TRANSAÇÃO do crédito, e é essa a única coisa que
		// importa nesta parte.
		//
		// Fora dela abre a janela em que uma queda deixa os Rcoins creditados e os
		// brindes não — e aí ninguém sabe o que faltou, porque a ordem já está PAGA e a
		// repetição não credita de novo (o caminho de cima). O contrário é igual de
		// ruim: brinde entregue e crédito perdido.
		//
		// Ordem antiga sem pacote não tem brinde, e isso é o normal: todo pedido
		// anterior à 0122 tem pacote_id nulo. Nulo aqui quer dizer "veio antes de
		// existir pacote", e não "pacote desconhecido" — quem recusa id desconhecido é
		// a criação da ordem, não a confirmação dela.
		if pacoteID != nil && *pacoteID != "" {
			if err := enfileirarBrindes(ctx, tx, accountID, orderID, *pacoteID); err != nil {
				return err
			}
		}
		outcome = TopupConfirmed
		return nil
	})
	if err != nil {
		return TopupNotFound, 0, err
	}
	return outcome, balance, nil
}

// GetTopupOrder returns an order's status and credits, scoped to its owner: an
// order belonging to another account (or absent) returns ErrNotFound so no
// order leaks across accounts. When the order is PAID, newBalance carries the
// account's current donate balance; otherwise it is 0.
func (s *Store) GetTopupOrder(ctx context.Context, externalRef string, accountID int64) (status int16, credits int32, newBalance int32, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT status, credits FROM donate_topup_order
		WHERE external_reference = $1 AND account_id = $2`,
		externalRef, accountID).Scan(&status, &credits)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, 0, ErrNotFound
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("store: get topup order ref=%q: %w", externalRef, err)
	}
	if status == TopupStatusPaid {
		if err := s.pool.QueryRow(ctx,
			`SELECT donate_balance FROM account WHERE id = $1`, accountID).Scan(&newBalance); err != nil {
			return 0, 0, 0, fmt.Errorf("store: get topup order: read balance a=%d: %w", accountID, err)
		}
	}
	return status, credits, newBalance, nil
}

// enfileirarBrindes põe os itens do pacote na caixa postal, dentro da transação que
// acabou de creditar.
//
// A QUANTIDADE PASSA PELO pilha.Divide, a mesma função que a tela do site usa para
// dizer "ocupa N espaços": o que empilha vira pilha, o que não empilha vira uma linha
// por unidade. Assim o número prometido na página e o que a entrega produz não podem
// divergir, porque são a mesma conta.
//
// A `source` leva a ordem e não só o pacote. Numa investigação a pergunta é "de onde
// veio este item", e "apoiador-supremo" não responde: a mesma pessoa pode ter comprado
// o mesmo pacote duas vezes.
func enfileirarBrindes(ctx context.Context, tx pgx.Tx, accountID, orderID int64, pacoteID string) error {
	rows, err := tx.Query(ctx, `
		SELECT item_index, eff1, effv1, eff2, effv2, eff3, effv3
		  FROM donate_pacote_item WHERE pacote_id = $1 ORDER BY ordem, id`, pacoteID)
	if err != nil {
		return fmt.Errorf("store: confirm topup: lendo os brindes de %q: %w", pacoteID, err)
	}
	var brindes []ItemDoPacote
	for rows.Next() {
		var it ItemDoPacote
		if err := rows.Scan(&it.ItemIndex, &it.Eff1, &it.EffV1,
			&it.Eff2, &it.EffV2, &it.Eff3, &it.EffV3); err != nil {
			rows.Close()
			return fmt.Errorf("store: confirm topup: lendo os brindes de %q: %w", pacoteID, err)
		}
		brindes = append(brindes, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: confirm topup: lendo os brindes de %q: %w", pacoteID, err)
	}

	origem := fmt.Sprintf("donate_pacote:%s:order:%d", pacoteID, orderID)
	for _, it := range brindes {
		for _, pilhaDe := range pilha.Divide(int16(it.ItemIndex), quantidadeDoBrinde(it)) {
			carga, err := payloadDoBrinde(comAmount(it, pilhaDe))
			if err != nil {
				return fmt.Errorf("store: confirm topup: montando o brinde %d: %w", it.ItemIndex, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO delivery_queue (account_id, kind, payload, source)
				VALUES ($1, 'item', $2, $3)`, accountID, carga, origem); err != nil {
				return fmt.Errorf("store: confirm topup: enfileirando o brinde %d: %w",
					it.ItemIndex, err)
			}
		}
	}
	return nil
}

// textoOuNulo grava nulo em vez de texto vazio.
//
// A coluna tem chave estrangeira para donate_pacote: um texto vazio viraria violação
// de FK e o pedido inteiro falharia, em vez de ser gravado como a doação sem pacote que
// ele é. E nulo é o que a coluna significa para todo pedido anterior aos pacotes.
func textoOuNulo(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
