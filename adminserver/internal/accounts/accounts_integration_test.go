//go:build integration

// Integration tests for the panel's account writes, against a real PostgreSQL.
//
//	W2PP_TEST_DSN=postgres://postgres@localhost:5432/postgres go test -tags=integration ./adminserver/internal/accounts/
//
// The guards are the point. Each one prevents a state the panel cannot get
// itself out of afterwards, so each is tested against the database rather than
// against a stub that agrees with the code.
package accounts

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("W2PP_TEST_DSN")
	if dsn == "" {
		t.Skip("W2PP_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := store.Pool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seed inserts (or resets) an account and returns its id.
func seed(t *testing.T, pool *pgxpool.Pool, name, role string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO account (name, pass_hash, role) VALUES ($1, 'x', $2)
		 ON CONFLICT (name) DO UPDATE SET role = EXCLUDED.role, is_blocked = false
		 RETURNING id`, name, role).Scan(&id)
	if err != nil {
		t.Fatalf("seed %q: %v", name, err)
	}
	return id
}

func roleOf(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var role string
	if err := pool.QueryRow(context.Background(),
		`SELECT role FROM account WHERE id = $1`, id).Scan(&role); err != nil {
		t.Fatalf("read role: %v", err)
	}
	return role
}

func TestSetRoleChangesAndReportsThePrevious(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()

	actor := seed(t, pool, "acc_actor", RoleAdmin)
	_ = seed(t, pool, "acc_second_admin", RoleAdmin) // so the last-admin guard is not in play
	alvo := seed(t, pool, "acc_alvo", RolePlayer)

	previous, err := s.SetRole(ctx, actor, alvo, RoleModerator)
	if err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if previous != RolePlayer {
		t.Errorf("previous = %q, want player — the audit entry depends on this", previous)
	}
	if got := roleOf(t, pool, alvo); got != RoleModerator {
		t.Errorf("role after = %q, want moderator", got)
	}
}

func TestSetRoleIsANoOpWhenNothingChanges(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor2", RoleAdmin)
	_ = seed(t, pool, "acc_second_admin2", RoleAdmin)
	alvo := seed(t, pool, "acc_igual", RoleModerator)

	previous, err := s.SetRole(ctx, actor, alvo, RoleModerator)
	if err != nil {
		t.Fatalf("setting the role it already has must not be an error: %v", err)
	}
	if previous != RoleModerator {
		t.Errorf("previous = %q, want moderator", previous)
	}
}

func TestCannotChangeYourOwnAccess(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	me := seed(t, pool, "acc_eu", RoleAdmin)

	if _, err := s.SetRole(ctx, me, me, RolePlayer); !errors.Is(err, ErrSelf) {
		t.Errorf("SetRole on self: err = %v, want ErrSelf", err)
	}
	if _, err := s.SetBlocked(ctx, me, me, true); !errors.Is(err, ErrSelf) {
		t.Errorf("SetBlocked on self: err = %v, want ErrSelf", err)
	}
	if got := roleOf(t, pool, me); got != RoleAdmin {
		t.Errorf("role changed despite the refusal: %q", got)
	}
}

func TestTheLastAdminCannotBeDemoted(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()

	// Exactly one admin in the whole table.
	if _, err := pool.Exec(ctx, `UPDATE account SET role = $1 WHERE role = $2`,
		RolePlayer, RoleAdmin); err != nil {
		t.Fatalf("clear admins: %v", err)
	}
	unico := seed(t, pool, "acc_unico_admin", RoleAdmin)
	actor := seed(t, pool, "acc_outro_actor", RoleModerator)

	if _, err := s.SetRole(ctx, actor, unico, RolePlayer); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if got := roleOf(t, pool, unico); got != RoleAdmin {
		t.Fatalf("the last admin was demoted anyway: role = %q", got)
	}

	// With a second admin, the same demotion is allowed.
	segundo := seed(t, pool, "acc_segundo_admin", RoleAdmin)
	if _, err := s.SetRole(ctx, actor, unico, RolePlayer); err != nil {
		t.Fatalf("with two admins the demotion must succeed: %v", err)
	}
	if got := roleOf(t, pool, segundo); got != RoleAdmin {
		t.Errorf("the other admin was disturbed: %q", got)
	}
}

func TestUnknownRoleIsRefusedBeforeTouchingTheDatabase(t *testing.T) {
	// world.ParseAccess fails closed on anything it does not recognise, so a typo
	// written through would silently strip authority instead of erroring.
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor3", RoleAdmin)
	alvo := seed(t, pool, "acc_alvo3", RoleModerator)

	for _, bad := range []string{"", "Admin", "superadmin", "ADMIN", "gm"} {
		if _, err := s.SetRole(ctx, actor, alvo, bad); !errors.Is(err, ErrUnknownRole) {
			t.Errorf("role %q: err = %v, want ErrUnknownRole", bad, err)
		}
	}
	if got := roleOf(t, pool, alvo); got != RoleModerator {
		t.Errorf("a refused role still changed the row: %q", got)
	}
}

func TestSetBlockedRoundTrip(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor4", RoleAdmin)
	alvo := seed(t, pool, "acc_bloqueio", RolePlayer)

	previous, err := s.SetBlocked(ctx, actor, alvo, true)
	if err != nil {
		t.Fatalf("SetBlocked: %v", err)
	}
	if previous {
		t.Error("previous = true, want false — the audit entry depends on this")
	}

	previous, err = s.SetBlocked(ctx, actor, alvo, false)
	if err != nil {
		t.Fatalf("SetBlocked: %v", err)
	}
	if !previous {
		t.Error("previous = false, want true after having been blocked")
	}
}

func TestMissingAccount(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor5", RoleAdmin)

	if _, err := s.SetRole(ctx, actor, 999999999, RolePlayer); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetRole: err = %v, want ErrNotFound", err)
	}
	if _, err := s.SetBlocked(ctx, actor, 999999999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetBlocked: err = %v, want ErrNotFound", err)
	}
}
