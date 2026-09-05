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
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

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

// --- VIP ---

// VIP grant bounds. A day count outside this is a typo, not an intention: one
// day is the smallest useful grant, and ten years is longer than the server is
// likely to outlive. Refusing beats writing a date nobody meant.
const (
	MinVipDays = 1
	MaxVipDays = 3650
)

// ErrVipDays is returned for a grant length outside the bounds above.
var ErrVipDays = errors.New("accounts: vip day count out of range")

// Details is what the panel shows about an account beyond its auth row.
//
// It exists here rather than in internal/store for the same reason the writes
// do — and it carries the email and donate balance the account page had to go
// without while there was no panel-owned read.
type Details struct {
	Email         string
	DonateBalance int32
	VipUntil      *time.Time // nil means the account has never been VIP
}

// Get reads the panel-facing fields of one account.
func (s *Store) Get(ctx context.Context, id int64) (Details, error) {
	var d Details
	err := s.pool.QueryRow(ctx,
		`SELECT email, donate_balance, vip_until FROM account WHERE id = $1`, id).
		Scan(&d.Email, &d.DonateBalance, &d.VipUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return Details{}, ErrNotFound
	}
	if err != nil {
		return Details{}, fmt.Errorf("accounts: get %d: %w", id, err)
	}
	return d, nil
}

// AddVipDays extends an account's VIP and returns the dates before and after.
//
// Extension counts from whichever is later: now, or the current expiry. Adding
// thirty days to somebody who still has ten left gives forty, not thirty — the
// other reading silently takes time away from a paying player, and it is the
// reading a naive `now() + interval` produces.
//
// There is deliberately no self-grant guard, which is why the actor is accepted
// and then ignored here. VIP is an entitlement, not authority: granting it to
// yourself cannot lock anyone out or escalate what you can do, and the audit
// entry the caller writes names who did it. Blocking it would also stop staff
// testing their own change, which is a real thing they need to do.
//
// The parameter stays for symmetry with SetRole and SetBlocked, where it is load
// bearing: a guard added here later should not have to change the signature and
// every caller with it.
func (s *Store) AddVipDays(ctx context.Context, _, targetID int64, days int) (prev, next *time.Time, err error) {
	if days < MinVipDays || days > MaxVipDays {
		return nil, nil, ErrVipDays
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("accounts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE so two grants landing together both extend, instead of the
	// second computing its new date from the value the first has already
	// replaced — which would silently drop one of the two grants.
	var current *time.Time
	err = tx.QueryRow(ctx, `SELECT vip_until FROM account WHERE id = $1 FOR UPDATE`, targetID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("accounts: read vip: %w", err)
	}

	from := time.Now().UTC()
	if current != nil && current.After(from) {
		from = current.UTC()
	}
	novo := from.AddDate(0, 0, days)

	if _, err := tx.Exec(ctx, `UPDATE account SET vip_until = $2 WHERE id = $1`, targetID, novo); err != nil {
		return nil, nil, fmt.Errorf("accounts: set vip: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("accounts: commit: %w", err)
	}
	return current, &novo, nil
}

// ClearVip removes VIP immediately and returns the date it had.
//
// It writes NULL rather than a past date: "never had VIP" and "had VIP, taken
// away" read the same to anything that compares against now(), and the audit log
// is where the difference is preserved.
func (s *Store) ClearVip(ctx context.Context, _, targetID int64) (*time.Time, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("accounts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current *time.Time
	err = tx.QueryRow(ctx, `SELECT vip_until FROM account WHERE id = $1 FOR UPDATE`, targetID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("accounts: read vip: %w", err)
	}
	if current == nil {
		return nil, nil // already not VIP
	}

	if _, err := tx.Exec(ctx, `UPDATE account SET vip_until = NULL WHERE id = $1`, targetID); err != nil {
		return nil, fmt.Errorf("accounts: clear vip: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("accounts: commit: %w", err)
	}
	return current, nil
}

// VipActive reports whether a stored expiry means the account is VIP right now.
// Comparing against now() is the whole expiry mechanism: nothing sweeps the
// column, so a lapsed date simply stops counting.
func VipActive(until *time.Time) bool {
	return until != nil && until.After(time.Now())
}

// --- pendências de reinício ---

// PendingSince reports how many boot-bound overrides were edited after the
// given moment, and when the most recent edit was.
//
// It counts the two boot-bound tables and nothing else. NPC definitions, shops
// and item PRICES are polled by the tmServer every ~15 seconds and apply live;
// mob template stats and item base stats are read once at boot and there is no
// hot reload — the code says so, and notes that the legacy EDITAPPMOB behaved
// the same way. Counting the live ones would make the warning cry wolf, and a
// warning people learn to ignore is worse than none.
func (s *Store) PendingSince(ctx context.Context, since time.Time) (n int, last time.Time, err error) {
	var lastNull *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT count(*), max(updated_at) FROM (
			SELECT updated_at FROM mob_template_stat WHERE updated_at > $1
			UNION ALL
			SELECT updated_at FROM item_stat WHERE updated_at > $1
		) AS pendentes`, since).Scan(&n, &lastNull)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("accounts: pending overrides: %w", err)
	}
	if lastNull != nil {
		last = *lastNull
	}
	return n, last, nil
}

// --- senha ---

// Password rules, all of them imposed by the game client rather than by taste.
//
// The login packet carries AccountPassword as a fixed [12]byte
// (tmserver/internal/protocol/messages.go:28), so anything longer verifies in
// the panel and then never works in game — the worst possible failure, because
// it looks like the reset worked. Spaces are out for the same reason: the
// decoder trims trailing spaces off fixed-width fields, so a password that ends
// in one cannot be typed back.
const (
	MaxSenhaBytes = 12
	MinSenhaBytes = 4
)

// Refusals, as values so a handler can say which rule was broken.
var (
	ErrSenhaVazia     = errors.New("accounts: empty password")
	ErrSenhaLonga     = errors.New("accounts: password longer than the client can carry")
	ErrSenhaCurta     = errors.New("accounts: password too short")
	ErrSenhaEspaco    = errors.New("accounts: password contains a space")
	ErrSenhaCaractere = errors.New("accounts: password has a character the client cannot type")
)

// ValidarSenha checks a password against what the client can carry and type.
//
// Empty is refused loudly rather than silently accepted. secret.HashSecret("")
// returns an empty hash on purpose — it means "no secret set" — and
// secret.VerifySecret then matches it against an empty password. That is correct
// for an unset block PIN and catastrophic for a login password: the account
// would sign in with no password at all, on the panel, the web and the game. The
// guard belongs here, before the hash, not after it.
func ValidarSenha(s string) error {
	switch {
	case s == "":
		return ErrSenhaVazia
	case len(s) > MaxSenhaBytes:
		return ErrSenhaLonga
	case len(s) < MinSenhaBytes:
		return ErrSenhaCurta
	}
	for _, r := range s {
		if r == ' ' {
			return ErrSenhaEspaco
		}
		// The wire is bytes, and the client is a Windows program from 2003. Stay
		// inside printable ASCII rather than discover which code page it uses.
		if r < '!' || r > '~' {
			return ErrSenhaCaractere
		}
	}
	return nil
}

// senhaAlfabeto omits the character pairs people mistype when reading a password
// off a screen and typing it into a game client: 0/O, 1/l/I.
const senhaAlfabeto = "abcdefghijkmnpqrstuvwxyzACDEFGHJKLMNPQRTUVWXY2345679"

// senhaGerada is shorter than the 12-byte ceiling on purpose: the moderator has
// to read it out and the player has to type it, and the two extra characters buy
// less than the transcription errors they cost.
const senhaGerada = 10

// GerarSenha returns a random password that satisfies ValidarSenha.
//
// The panel offers this as the default rather than an empty field, so the empty
// case is not something a distracted moderator can reach by pressing enter.
func GerarSenha() (string, error) {
	out := make([]byte, senhaGerada)
	limite := big.NewInt(int64(len(senhaAlfabeto)))
	for i := range out {
		n, err := rand.Int(rand.Reader, limite)
		if err != nil {
			return "", fmt.Errorf("accounts: generate password: %w", err)
		}
		out[i] = senhaAlfabeto[n.Int64()]
	}
	return string(out), nil
}

// SetPassword replaces an account's password hash.
//
// The hash is computed by the caller so this package never holds the plaintext
// beyond validation, and so the empty-hash case cannot arrive here by accident:
// an empty hash is refused outright rather than written, because it would mean
// "any empty password logs in".
//
// There is no self guard. Changing your own password is a normal thing to do and
// cannot lock anyone else out; the caller decides who may target whom, because
// that rule is about rank and lives with the rest of the panel's authorization.
func (s *Store) SetPassword(ctx context.Context, targetID int64, hash string) error {
	if hash == "" {
		return ErrSenhaVazia
	}
	tag, err := s.pool.Exec(ctx, `UPDATE account SET pass_hash = $2 WHERE id = $1`, targetID, hash)
	if err != nil {
		return fmt.Errorf("accounts: set password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
