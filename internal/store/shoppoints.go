package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrPontosInsuficientes é resposta prevista, não falha: o gasto não cabe no saldo,
// e nada se moveu. Quem gasta precisa distinguir isso de um banco fora do ar — um
// é "você não tem pontos", o outro é "tente de novo" —, e essa distinção nasce
// aqui, no único lugar que fala com a tabela.
var ErrPontosInsuficientes = errors.New("store: pontos de lojinha insuficientes")

// Shop-points wallet (0060_shop_points): the currency an open personal shop pays
// its owner, 3 points per quarter-hour and 7 with a Fada Azul.
//
// Separate from account.donate_balance on purpose — that wallet is money somebody
// paid and is what the revenue panel sums; this one is time. See the migration.

// AddShopPoints credits (or debits, with a negative delta) the account's
// shop-points wallet and appends the movement to the audit trail, in one
// transaction. It returns the balance AFTER the movement.
//
// The balance is computed by the database (balance + $2), never written back as a
// total the caller worked out: two characters on the same account can finish a
// quarter-hour in the same instant, and a read-modify-write would silently drop
// one of the two credits.
//
// A SPEND (negative delta) takes a different statement, and the reason is a bug
// the Loja de Honra hit on its first live purchase. The upsert below cannot spend:
// PostgreSQL evaluates the table CHECK on the tuple it is about to INSERT
// (balance = delta) BEFORE it discovers the conflict and switches to the UPDATE
// path, so a delta of -30 raises check_violation whatever the balance is. The
// comment that used to be here claimed the CHECK was the floor; the CHECK was the
// wall, and no spend ever got through it.
//
// So a spend is a plain conditional UPDATE, and the floor is in the WHERE: no row,
// or a row that cannot pay, matches nothing and the whole transaction rolls back,
// audit row included — a spend can never leave a receipt for points that were not
// actually taken.
func (s *Store) AddShopPoints(ctx context.Context, accountID int64, delta int32, characterName, reason string) (int32, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: abrir transação de pontos de lojinha: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var saldo int32
	if delta < 0 {
		// Gasto: o piso mora no WHERE. Nada casou significa carteira que nao
		// existe ou que nao alcanca o preco - as duas coisas sao "nao tem pontos".
		err = tx.QueryRow(ctx, `
			UPDATE shop_points
			   SET balance = balance + $2, updated_at = now()
			 WHERE account_id = $1 AND balance + $2 >= 0
			RETURNING balance`, accountID, delta).Scan(&saldo)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrPontosInsuficientes
		}
		if err != nil {
			return 0, fmt.Errorf("store: gastar pontos de lojinha: %w", err)
		}
	} else {
		// The wallet row is created on first credit: an account that never kept a
		// shop open has no row, and requiring a seed would mean every account ever
		// created carries one.
		err = tx.QueryRow(ctx, `
			INSERT INTO shop_points (account_id, balance, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (account_id) DO UPDATE
				SET balance = shop_points.balance + $2, updated_at = now()
			RETURNING balance`, accountID, delta).Scan(&saldo)
		if err != nil {
			return 0, fmt.Errorf("store: creditar pontos de lojinha: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO shop_points_audit (account_id, character_name, delta, balance_after, reason)
		VALUES ($1, $2, $3, $4, $5)`,
		accountID, characterName, delta, saldo, reason); err != nil {
		return 0, fmt.Errorf("store: gravar extrato de pontos de lojinha: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("store: confirmar pontos de lojinha: %w", err)
	}
	return saldo, nil
}

// ShopPoints reads one account's balance. A missing row is zero, not an error:
// an account that never opened a shop simply has no points.
func (s *Store) ShopPoints(ctx context.Context, accountID int64) (int32, error) {
	var saldo int32
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE((SELECT balance FROM shop_points WHERE account_id = $1), 0)`,
		accountID).Scan(&saldo)
	if err != nil {
		return 0, fmt.Errorf("store: ler pontos de lojinha: %w", err)
	}
	return saldo, nil
}

// SpendShopPoints debits cost points from the account and records the movement,
// atomically. ok is false when the wallet does not cover the cost — including the
// account that has no wallet row at all, which is simply an account with zero
// points.
//
// A conditional UPDATE rather than AddShopPoints with a negative delta: that one
// leans on the table's CHECK to reject an overdraft, and a constraint violation
// arrives here indistinguishable from a connection failure. The caller has to
// tell the player "you don't have the points" apart from "try again", and one of
// those must not consume the item.
//
// cost must be positive; a zero or negative cost is refused rather than quietly
// turned into a credit.
func (s *Store) SpendShopPoints(ctx context.Context, accountID int64, cost int32, characterName, reason string) (int32, bool, error) {
	if cost <= 0 {
		return 0, false, fmt.Errorf("store: gastar pontos de lojinha: custo %d não é positivo", cost)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("store: abrir transação de gasto de pontos: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var saldo int32
	err = tx.QueryRow(ctx, `
		UPDATE shop_points SET balance = balance - $2, updated_at = now()
		WHERE account_id = $1 AND balance >= $2
		RETURNING balance`, accountID, cost).Scan(&saldo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil // saldo insuficiente, ou conta sem carteira
		}
		return 0, false, fmt.Errorf("store: debitar pontos de lojinha: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO shop_points_audit (account_id, character_name, delta, balance_after, reason)
		VALUES ($1, $2, $3, $4, $5)`,
		accountID, characterName, -cost, saldo, reason); err != nil {
		return 0, false, fmt.Errorf("store: gravar extrato de gasto de pontos: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("store: confirmar gasto de pontos: %w", err)
	}
	return saldo, true, nil
}
