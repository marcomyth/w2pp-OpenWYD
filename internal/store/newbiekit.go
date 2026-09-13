package store

import (
	"context"
	"fmt"
)

// Newbie kit claim (0062_newbie_kit): the once-per-account gate behind /novato.

// ClaimNewbieKit marks the account as having taken the newbie kit and reports
// whether THIS call is the one that took it. false means the account already had
// it — the caller must hand out nothing.
//
// The gate is the INSERT itself, not a read followed by a write: two characters
// of the same account can type /novato in the same instant on two channels, and
// the window between a SELECT and an UPDATE is exactly where the second kit
// would be born. ON CONFLICT DO NOTHING makes the database settle it — one row
// inserted, one command tag saying zero rows, and only the first caller delivers.
//
// The claim is written BEFORE the items exist in the bag, which is deliberate:
// between the two, failing closed (the player keeps the claim and no kit) needs
// one support ticket, while failing open (no claim, kit delivered) is an
// unlimited item faucet for anyone willing to disconnect at the right moment.
func (s *Store) ClaimNewbieKit(ctx context.Context, accountID int64, characterName string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO newbie_kit_claim (account_id, character_name)
		VALUES ($1, $2)
		ON CONFLICT (account_id) DO NOTHING`, accountID, characterName)
	if err != nil {
		return false, fmt.Errorf("store: registrar kit de novato da conta %d: %w", accountID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// NewbieKitClaimed reports whether the account already took the kit. It exists
// for the staff panel and for tests; the game never asks before claiming,
// because asking first is the race ClaimNewbieKit avoids.
func (s *Store) NewbieKitClaimed(ctx context.Context, accountID int64) (bool, error) {
	var existe bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM newbie_kit_claim WHERE account_id = $1)`, accountID).Scan(&existe)
	if err != nil {
		return false, fmt.Errorf("store: consultar kit de novato da conta %d: %w", accountID, err)
	}
	return existe, nil
}
