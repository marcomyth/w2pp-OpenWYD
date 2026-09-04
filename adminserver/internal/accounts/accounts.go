// Package accounts performs the panel's writes to the account table.
//
// The queries live here rather than in internal/store for the same reason the
// audit ones do: every service embeds internal/, so adding there redeploys the
// game to ship a panel change. Nothing in the game writes account.role or
// account.is_blocked from a panel, so there is nothing to share.
//
// Every write here is a two-step: check the guards, then apply. The guards are
// not cosmetic — each one prevents a state the panel could not get itself out
// of afterwards.
package accounts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Roles the panel is allowed to set. A value outside this set is refused rather
// than written: world.ParseAccess fails closed on anything unrecognised, so a
// typo would silently strip someone's authority instead of erroring.
const (
	RolePlayer    = "player"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// Refusals. They are values, not strings, so a handler can tell them apart and
// say something useful instead of "erro".
var (
	ErrUnknownRole = errors.New("accounts: unknown role")
	ErrNotFound    = errors.New("accounts: account not found")
	ErrSelf        = errors.New("accounts: an account cannot change its own access")
	ErrLastAdmin   = errors.New("accounts: this is the last admin")
)

// Store performs the writes.
type Store struct{ pool *pgxpool.Pool }

// New wraps a pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// ValidRole reports whether role is one the panel may set.
func ValidRole(role string) bool {
	return role == RolePlayer || role == RoleModerator || role == RoleAdmin
}

// SetRole changes an account's role and returns the previous one.
//
// actorID is the signed-in staff member. Two refusals matter more than they
// look:
//
//	Self — an admin demoting themselves is the panel's own foot-gun: the change
//	takes effect on their next request, and if they were the last admin nobody
//	can undo it from the panel at all.
//
//	Last admin — leaving the server with no admin means the only way back is a
//	hand-written UPDATE against the database, which is exactly the thing this
//	panel exists to stop being normal.
func (s *Store) SetRole(ctx context.Context, actorID, targetID int64, role string) (string, error) {
	if !ValidRole(role) {
		return "", ErrUnknownRole
	}
	if actorID == targetID {
		return "", ErrSelf
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("accounts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE so the last-admin count below cannot be raced by a second
	// demotion running at the same instant. Two panels demoting the two
	// remaining admins concurrently would otherwise both see a count of 2.
	var current string
	err = tx.QueryRow(ctx, `SELECT role FROM account WHERE id = $1 FOR UPDATE`, targetID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("accounts: read role: %w", err)
	}
	if current == role {
		return current, nil // nothing to do; not an error
	}

	if current == RoleAdmin && role != RoleAdmin {
		var admins int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM account WHERE role = $1`, RoleAdmin).Scan(&admins); err != nil {
			return "", fmt.Errorf("accounts: count admins: %w", err)
		}
		if admins <= 1 {
			return "", ErrLastAdmin
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE account SET role = $2 WHERE id = $1`, targetID, role); err != nil {
		return "", fmt.Errorf("accounts: set role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("accounts: commit: %w", err)
	}
	return current, nil
}

// SetBlocked blocks or unblocks an account and returns the previous state.
//
// Blocking yourself is refused for the same reason as demoting yourself: the
// block applies to the game AND to the panel login, so it locks the door with
// the key inside.
func (s *Store) SetBlocked(ctx context.Context, actorID, targetID int64, blocked bool) (bool, error) {
	if actorID == targetID {
		return false, ErrSelf
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("accounts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Read then write, rather than one UPDATE ... RETURNING with a subquery for
	// the old value: whether such a subquery sees the row before or after the
	// update depends on the statement snapshot, and an audit entry that records
	// the wrong "before" is worse than no audit entry.
	var previous bool
	err = tx.QueryRow(ctx, `SELECT is_blocked FROM account WHERE id = $1 FOR UPDATE`, targetID).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("accounts: read blocked: %w", err)
	}
	if previous == blocked {
		return previous, nil // already in the requested state
	}

	if _, err := tx.Exec(ctx, `UPDATE account SET is_blocked = $2 WHERE id = $1`, targetID, blocked); err != nil {
		return false, fmt.Errorf("accounts: set blocked: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("accounts: commit: %w", err)
	}
	return previous, nil
}
