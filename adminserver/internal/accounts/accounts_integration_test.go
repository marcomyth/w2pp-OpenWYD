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
	"strings"
	"testing"
	"time"

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
	if _, err := s.SetBlocked(ctx, me, me, true, "motivo"); !errors.Is(err, ErrSelf) {
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

	previous, err := s.SetBlocked(ctx, actor, alvo, true, "usou programa de terceiros")
	if err != nil {
		t.Fatalf("SetBlocked: %v", err)
	}
	if previous.Blocked {
		t.Error("previous.Blocked = true, want false — the audit entry depends on this")
	}

	// The row has to carry the whole story: why, when and by whom. Only the
	// panel writes these, so nothing else will fill them in later.
	var motivo string
	var quando *time.Time
	var quem *int64
	if err := pool.QueryRow(ctx,
		`SELECT block_reason, blocked_at, blocked_by FROM account WHERE id = $1`, alvo).
		Scan(&motivo, &quando, &quem); err != nil {
		t.Fatalf("read block row: %v", err)
	}
	if motivo != "usou programa de terceiros" {
		t.Errorf("motivo = %q, want o que foi enviado", motivo)
	}
	if quando == nil {
		t.Error("blocked_at ficou nulo num bloqueio")
	}
	if quem == nil || *quem != actor {
		t.Errorf("blocked_by = %v, want %d", quem, actor)
	}

	previous, err = s.SetBlocked(ctx, actor, alvo, false, "")
	if err != nil {
		t.Fatalf("SetBlocked: %v", err)
	}
	if !previous.Blocked {
		t.Error("previous.Blocked = false, want true after having been blocked")
	}
	if previous.Reason != "usou programa de terceiros" {
		t.Errorf("previous.Reason = %q — o audit precisa do motivo que estava valendo", previous.Reason)
	}

	// Unblocking clears the reason: a reason hanging off an active account would
	// read as a ban to anyone glancing at the row.
	if err := pool.QueryRow(ctx,
		`SELECT block_reason, blocked_at, blocked_by FROM account WHERE id = $1`, alvo).
		Scan(&motivo, &quando, &quem); err != nil {
		t.Fatalf("read block row: %v", err)
	}
	if motivo != "" || quando != nil || quem != nil {
		t.Errorf("desbloquear deixou resto: motivo=%q quando=%v quem=%v", motivo, quando, quem)
	}
}

func TestBloquearSemMotivoERecusado(t *testing.T) {
	// The whole point of the column is that a player who writes in can be told
	// why. A ban with an empty reason is the state this migration removes.
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor_motivo", RoleAdmin)
	alvo := seed(t, pool, "acc_sem_motivo", RolePlayer)

	if _, err := s.SetBlocked(ctx, actor, alvo, true, "   "); !errors.Is(err, ErrMotivo) {
		t.Fatalf("bloquear sem motivo = %v, want ErrMotivo", err)
	}
	if _, err := s.SetBlocked(ctx, actor, alvo, true, strings.Repeat("x", MaxMotivoBytes+1)); !errors.Is(err, ErrMotivo) {
		t.Fatalf("motivo gigante = %v, want ErrMotivo", err)
	}
	var bloqueado bool
	if err := pool.QueryRow(ctx, `SELECT is_blocked FROM account WHERE id = $1`, alvo).Scan(&bloqueado); err != nil {
		t.Fatalf("read: %v", err)
	}
	if bloqueado {
		t.Error("um bloqueio recusado ainda assim entrou")
	}
}

func TestEditarOMotivoDeUmBanEmVigorGrava(t *testing.T) {
	// The old code returned before the UPDATE when the flag already matched, so
	// correcting a reason wrote nothing and reported "nothing changed".
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor_edit", RoleAdmin)
	alvo := seed(t, pool, "acc_edit_motivo", RolePlayer)

	if _, err := s.SetBlocked(ctx, actor, alvo, true, "motivo velho"); err != nil {
		t.Fatalf("primeiro bloqueio: %v", err)
	}
	prev, err := s.SetBlocked(ctx, actor, alvo, true, "motivo corrigido")
	if err != nil {
		t.Fatalf("editar motivo: %v", err)
	}
	if prev.Reason != "motivo velho" {
		t.Errorf("prev.Reason = %q, want o anterior", prev.Reason)
	}
	var motivo string
	if err := pool.QueryRow(ctx, `SELECT block_reason FROM account WHERE id = $1`, alvo).Scan(&motivo); err != nil {
		t.Fatalf("read: %v", err)
	}
	if motivo != "motivo corrigido" {
		t.Errorf("motivo gravado = %q, want o corrigido", motivo)
	}
}

func TestBlockedLeOEstadoAtual(t *testing.T) {
	// requireStaff calls this on every request; if it lagged, a banned moderator
	// would keep working until their session expired.
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "acc_actor_bl", RoleAdmin)
	alvo := seed(t, pool, "acc_bl_estado", RolePlayer)

	if b, err := s.Blocked(ctx, alvo); err != nil || b {
		t.Fatalf("Blocked antes = %v, %v; want false", b, err)
	}
	if _, err := s.SetBlocked(ctx, actor, alvo, true, "qualquer"); err != nil {
		t.Fatalf("SetBlocked: %v", err)
	}
	if b, err := s.Blocked(ctx, alvo); err != nil || !b {
		t.Fatalf("Blocked depois = %v, %v; want true", b, err)
	}
	if _, err := s.Blocked(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Blocked de conta inexistente = %v, want ErrNotFound", err)
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
	if _, err := s.SetBlocked(ctx, actor, 999999999, true, "motivo"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetBlocked: err = %v, want ErrNotFound", err)
	}
}

// --- vip ---

func vipOf(t *testing.T, pool *pgxpool.Pool, id int64) *time.Time {
	t.Helper()
	var v *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT vip_until FROM account WHERE id = $1`, id).Scan(&v); err != nil {
		t.Fatalf("read vip: %v", err)
	}
	return v
}

func TestVipGrantCountsFromToday(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "vip_actor", RoleAdmin)
	alvo := seed(t, pool, "vip_novo", RolePlayer)

	prev, next, err := s.AddVipDays(ctx, actor, alvo, 30)
	if err != nil {
		t.Fatalf("AddVipDays: %v", err)
	}
	if prev != nil {
		t.Errorf("previous = %v, want nil for an account that never had VIP", prev)
	}
	want := time.Now().UTC().AddDate(0, 0, 30)
	if diff := next.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("new expiry = %v, want about %v", next, want)
	}
	if got := vipOf(t, pool, alvo); got == nil {
		t.Fatal("vip_until was not written")
	}
}

func TestVipGrantExtendsInsteadOfReplacing(t *testing.T) {
	// The behaviour that matters commercially: adding 30 days to somebody with 10
	// left gives 40. A naive now()+interval would silently take 10 days from a
	// paying player.
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "vip_actor2", RoleAdmin)
	alvo := seed(t, pool, "vip_ativo", RolePlayer)

	if _, _, err := s.AddVipDays(ctx, actor, alvo, 10); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	_, next, err := s.AddVipDays(ctx, actor, alvo, 30)
	if err != nil {
		t.Fatalf("second grant: %v", err)
	}

	want := time.Now().UTC().AddDate(0, 0, 40)
	if diff := next.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("after 10 + 30 days the expiry is %v, want about %v (40 days)", next, want)
	}
}

func TestVipGrantOnALapsedAccountCountsFromToday(t *testing.T) {
	// The other half of the rule: an expiry in the past must NOT be extended from,
	// or someone who lapsed a year ago would buy 30 days and get nothing.
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "vip_actor3", RoleAdmin)
	alvo := seed(t, pool, "vip_vencido", RolePlayer)

	passado := time.Now().UTC().AddDate(0, 0, -365)
	if _, err := pool.Exec(ctx, `UPDATE account SET vip_until = $2 WHERE id = $1`, alvo, passado); err != nil {
		t.Fatalf("seed lapsed vip: %v", err)
	}

	_, next, err := s.AddVipDays(ctx, actor, alvo, 30)
	if err != nil {
		t.Fatalf("AddVipDays: %v", err)
	}
	want := time.Now().UTC().AddDate(0, 0, 30)
	if diff := next.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("expiry = %v, want about %v — a lapsed date must not be extended from", next, want)
	}
}

func TestVipDayBounds(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "vip_actor4", RoleAdmin)
	alvo := seed(t, pool, "vip_limites", RolePlayer)

	for _, d := range []int{0, -1, MaxVipDays + 1, 1000000} {
		if _, _, err := s.AddVipDays(ctx, actor, alvo, d); !errors.Is(err, ErrVipDays) {
			t.Errorf("days=%d: err = %v, want ErrVipDays", d, err)
		}
	}
	if got := vipOf(t, pool, alvo); got != nil {
		t.Errorf("a refused grant still wrote a date: %v", got)
	}
}

func TestClearVip(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "vip_actor5", RoleAdmin)
	alvo := seed(t, pool, "vip_remover", RolePlayer)

	if _, _, err := s.AddVipDays(ctx, actor, alvo, 30); err != nil {
		t.Fatalf("grant: %v", err)
	}
	prev, err := s.ClearVip(ctx, actor, alvo)
	if err != nil {
		t.Fatalf("ClearVip: %v", err)
	}
	if prev == nil {
		t.Error("previous = nil, want the date it had — the audit entry depends on it")
	}
	if got := vipOf(t, pool, alvo); got != nil {
		t.Errorf("vip_until = %v, want NULL", got)
	}

	// Clearing again is a no-op, not an error, and reports no previous date.
	prev, err = s.ClearVip(ctx, actor, alvo)
	if err != nil {
		t.Fatalf("second ClearVip: %v", err)
	}
	if prev != nil {
		t.Errorf("previous = %v, want nil on an account with no VIP", prev)
	}
}

func TestVipActive(t *testing.T) {
	futuro := time.Now().Add(time.Hour)
	passado := time.Now().Add(-time.Hour)
	if VipActive(nil) {
		t.Error("nil counted as VIP")
	}
	if VipActive(&passado) {
		t.Error("a lapsed date counted as VIP")
	}
	if !VipActive(&futuro) {
		t.Error("a future date did not count as VIP")
	}
}

func TestGetReturnsTheFieldsThePanelShows(t *testing.T) {
	pool := testPool(t)
	s := New(pool)
	ctx := context.Background()
	actor := seed(t, pool, "vip_actor6", RoleAdmin)
	alvo := seed(t, pool, "vip_detalhes", RolePlayer)
	if _, err := pool.Exec(ctx,
		`UPDATE account SET email = $2, donate_balance = $3 WHERE id = $1`,
		alvo, "alvo@exemplo.com", 4200); err != nil {
		t.Fatalf("seed details: %v", err)
	}
	if _, _, err := s.AddVipDays(ctx, actor, alvo, 7); err != nil {
		t.Fatalf("grant: %v", err)
	}

	d, err := s.Get(ctx, alvo)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Email != "alvo@exemplo.com" || d.DonateBalance != 4200 || d.VipUntil == nil {
		t.Fatalf("details = %+v, want the email, balance and expiry", d)
	}

	if _, err := s.Get(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on a missing account: err = %v, want ErrNotFound", err)
	}
}

// TestPendingSinceCountsBothBootBoundTables is the test the home page's restart
// warning rests on. Two tables are read once at boot and need a restart to take
// effect — mob template stats and item base stats — and the count has to include
// both or a moderator rebalances an item, sees no pending badge, and concludes
// the panel did nothing.
//
// It also pins the other half of the rule: the tables that hot-reload must NOT
// be counted. A warning that fires for an edit which already applied is a
// warning people learn to ignore.
func TestPendingSinceCountsBothBootBoundTables(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	s := New(pool)

	if _, err := pool.Exec(ctx, `DELETE FROM item_stat; DELETE FROM mob_template_stat`); err != nil {
		t.Fatalf("limpar: %v", err)
	}

	// A moment strictly before any write below, standing in for a boot time.
	inicio := time.Now().Add(-time.Minute)

	n, ultima, err := s.PendingSince(ctx, inicio)
	if err != nil {
		t.Fatalf("PendingSince: %v", err)
	}
	if n != 0 || !ultima.IsZero() {
		t.Fatalf("com nada editado: %d pendentes, última %v; want 0 e zero", n, ultima)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO mob_template_stat (template_name, level) VALUES ('Kentania', 10)`); err != nil {
		t.Fatalf("editar monstro: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO item_stat (item_index, damage) VALUES (2000, 250)`); err != nil {
		t.Fatalf("editar item: %v", err)
	}

	n, ultima, err = s.PendingSince(ctx, inicio)
	if err != nil {
		t.Fatalf("PendingSince: %v", err)
	}
	if n != 2 {
		t.Errorf("pendentes = %d, want 2 (um monstro e um item)", n)
	}
	if ultima.Before(inicio) {
		t.Errorf("última edição = %v, want depois de %v", ultima, inicio)
	}

	// An edit made before the boot is already applied and must not count.
	if _, err := pool.Exec(ctx,
		`UPDATE item_stat SET updated_at = $1 WHERE item_index = 2000`,
		inicio.Add(-time.Hour)); err != nil {
		t.Fatalf("envelhecer o item: %v", err)
	}
	n, _, err = s.PendingSince(ctx, inicio)
	if err != nil {
		t.Fatalf("PendingSince: %v", err)
	}
	if n != 1 {
		t.Errorf("pendentes = %d, want 1 — a edição anterior ao boot já valeu", n)
	}

	// A price override hot-reloads within ~15s, so it is not pending on anything.
	if _, err := pool.Exec(ctx,
		`INSERT INTO item_price (item_index, price) VALUES (2000, 999)
		 ON CONFLICT (item_index) DO UPDATE SET price = EXCLUDED.price`); err != nil {
		t.Fatalf("mudar preço: %v", err)
	}
	n, _, err = s.PendingSince(ctx, inicio)
	if err != nil {
		t.Fatalf("PendingSince: %v", err)
	}
	if n != 1 {
		t.Errorf("pendentes = %d, want 1 — preço não espera reinício e não pode contar", n)
	}
}
