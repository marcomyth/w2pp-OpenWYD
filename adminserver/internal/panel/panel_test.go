package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/plataforma"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

const testPassword = "senha-de-teste"

// hashOnce keeps the argon2id cost (64 MiB, ~100ms) to a single call for the
// whole package instead of once per test.
var hashOnce = sync.OnceValue(func() string {
	h, err := secret.HashSecret(testPassword)
	if err != nil {
		panic(err)
	}
	return h
})

// fakeAccounts is an in-memory stand-in for the store, keyed by canonical name.
type fakeAccounts struct {
	mu    sync.Mutex
	rows  map[string]store.AccountAuth
	chars map[int64][]domain.Character
}

func newFakeAccounts(role string) *fakeAccounts {
	return &fakeAccounts{
		rows: map[string]store.AccountAuth{
			"chefe": {ID: 42, PassHash: hashOnce(), Role: role},
		},
		chars: map[int64][]domain.Character{},
	}
}

// add registers another account, so listing and search have something to sort.
func (f *fakeAccounts) add(name string, id int64, role string, blocked bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[name] = store.AccountAuth{ID: id, PassHash: hashOnce(), Role: role, IsBlocked: blocked}
}

func (f *fakeAccounts) addChar(accountID int64, c domain.Character) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chars[accountID] = append(f.chars[accountID], c)
}

func (f *fakeAccounts) SearchAccountsByNamePrefix(_ context.Context, prefix string, limit int) ([]domain.AccountSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]string, 0, len(f.rows))
	for n := range f.rows {
		if strings.HasPrefix(n, prefix) {
			names = append(names, n)
		}
	}
	sort.Strings(names) // the real query is ORDER BY name
	out := make([]domain.AccountSummary, 0, len(names))
	for _, n := range names {
		if len(out) == limit {
			break
		}
		a := f.rows[n]
		out = append(out, domain.AccountSummary{
			ID: a.ID, Name: n, Role: a.Role, IsBlocked: a.IsBlocked,
		})
	}
	return out, nil
}

func (f *fakeAccounts) ListCharacters(_ context.Context, accountID int64) ([]domain.Character, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chars[accountID], nil
}

func (f *fakeAccounts) AccountByName(_ context.Context, name string) (store.AccountAuth, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.rows[name]
	if !ok {
		return store.AccountAuth{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeAccounts) AccountRole(_ context.Context, id int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.rows {
		if a.ID == id {
			return a.Role, nil
		}
	}
	return "", store.ErrNotFound
}

func (f *fakeAccounts) setRole(name, role string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.rows[name]
	a.Role = role
	f.rows[name] = a
}

func (f *fakeAccounts) setBlocked(name string, blocked bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.rows[name]
	a.IsBlocked = blocked
	f.rows[name] = a
}

// fakeAudit is an in-memory action log.
type fakeAudit struct {
	mu        sync.Mutex
	entries   []audit.Entry
	written   []audit.Record
	failWrite error
	limit     int
}

func newFakeAudit() *fakeAudit { return &fakeAudit{limit: 100} }

func (f *fakeAudit) Limit() int { return f.limit }

func (f *fakeAudit) List(_ context.Context, targetID int64) ([]audit.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]audit.Entry, 0, len(f.entries))
	for _, e := range f.entries {
		if targetID == 0 || e.TargetID == targetID {
			out = append(out, e)
		}
	}
	return out, nil
}

// Write records what a handler asked to log, and can be made to fail so the
// "changed but not audited" path is exercised rather than assumed.
func (f *fakeAudit) Write(_ context.Context, r audit.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWrite != nil {
		return f.failWrite
	}
	f.written = append(f.written, r)
	return nil
}

func (f *fakeAudit) recorded() []audit.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]audit.Record(nil), f.written...)
}

func (f *fakeAudit) add(e audit.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

// newTestPanel builds a handler over the given accounts, with logs discarded.
func newTestPanel(t *testing.T, acc Accounts) http.Handler {
	t.Helper()
	return newTestPanelWith(t, acc, newFakeAudit())
}

// fakeWriter records the writes asked of it and can be made to refuse.
type fakeWriter struct {
	mu           sync.Mutex
	roleCall     []string
	blkCall      []bool
	lastActor    int64 // who the handler said was acting
	lastTarget   int64 // and on whom
	vipDays      []int
	vipCleared   int
	prevRole     string
	prevBlk      bool
	prevVip      *time.Time
	pendentes    int
	ultimaEdicao time.Time
	details      accounts.Details
	err          error
}

func newFakeWriter() *fakeWriter { return &fakeWriter{prevRole: "player"} }

func (f *fakeWriter) SetRole(_ context.Context, actorID, targetID int64, role string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastActor, f.lastTarget = actorID, targetID
	if f.err != nil {
		return "", f.err
	}
	f.roleCall = append(f.roleCall, role)
	return f.prevRole, nil
}

func (f *fakeWriter) PendingSince(_ context.Context, _ time.Time) (int, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pendentes, f.ultimaEdicao, nil
}

func (f *fakeWriter) Get(_ context.Context, _ int64) (accounts.Details, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.details, nil
}

func (f *fakeWriter) AddVipDays(_ context.Context, actorID, targetID int64, days int) (*time.Time, *time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastActor, f.lastTarget = actorID, targetID
	if f.err != nil {
		return nil, nil, f.err
	}
	f.vipDays = append(f.vipDays, days)
	// Mirror the real extension rule so the handler's message can be tested:
	// count from the later of now and the current expiry.
	from := time.Now()
	if f.prevVip != nil && f.prevVip.After(from) {
		from = *f.prevVip
	}
	novo := from.AddDate(0, 0, days)
	return f.prevVip, &novo, nil
}

func (f *fakeWriter) ClearVip(_ context.Context, actorID, targetID int64) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastActor, f.lastTarget = actorID, targetID
	if f.err != nil {
		return nil, f.err
	}
	f.vipCleared++
	return f.prevVip, nil
}

func (f *fakeWriter) SetBlocked(_ context.Context, actorID, targetID int64, blocked bool) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastActor, f.lastTarget = actorID, targetID
	if f.err != nil {
		return false, f.err
	}
	f.blkCall = append(f.blkCall, blocked)
	return f.prevBlk, nil
}

func newTestPanelWith(t *testing.T, acc Accounts, log AuditLog) http.Handler {
	t.Helper()
	return newTestPanelFull(t, acc, log, newFakeWriter())
}

func newTestPanelFull(t *testing.T, acc Accounts, log AuditLog, wr Writer) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts:   acc,
		Writer:     wr,
		Audit:      log,
		Sessions:   session.New(time.Hour),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// postLogin submits the login form and returns the response recorder.
func postLogin(h http.Handler, user, pass string) *httptest.ResponseRecorder {
	form := url.Values{"usuario": {user}, "senha": {pass}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// sessionCookie pulls the session cookie out of a response, or "" if unset.
func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestHealthzNeedsNoSession(t *testing.T) {
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Fatalf("body = %q, want ok", got)
	}
}

func TestHomeRedirectsWhenSignedOut(t *testing.T) {
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestLoginSucceedsForStaff(t *testing.T) {
	for _, role := range []string{roleModerator, roleAdmin} {
		t.Run(role, func(t *testing.T) {
			h := newTestPanel(t, newFakeAccounts(role))
			rec := postLogin(h, "chefe", testPassword)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", rec.Code)
			}
			c := sessionCookie(rec)
			if c == nil {
				t.Fatal("no session cookie set")
			}
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie flags = HttpOnly:%v Secure:%v SameSite:%v; want all strict",
					c.HttpOnly, c.Secure, c.SameSite)
			}
		})
	}
}

func TestLoginIsRefusedAndLeaksNothing(t *testing.T) {
	cases := []struct {
		name  string
		user  string
		pass  string
		setup func(*fakeAccounts)
	}{
		{name: "senha errada", user: "chefe", pass: "outra-coisa"},
		{name: "conta inexistente", user: "ninguem", pass: testPassword},
		{name: "cargo de jogador", user: "chefe", pass: testPassword,
			setup: func(f *fakeAccounts) { f.setRole("chefe", "player") }},
		{name: "cargo desconhecido", user: "chefe", pass: testPassword,
			setup: func(f *fakeAccounts) { f.setRole("chefe", "superadmin") }},
		{name: "conta bloqueada", user: "chefe", pass: testPassword,
			setup: func(f *fakeAccounts) { f.setBlocked("chefe", true) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := newFakeAccounts(roleAdmin)
			if tc.setup != nil {
				tc.setup(acc)
			}
			h := newTestPanel(t, acc)
			rec := postLogin(h, tc.user, tc.pass)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if c := sessionCookie(rec); c != nil {
				t.Fatal("a refused login set a session cookie")
			}
			// Every rejection says the same thing: the page must not hint at
			// which of the five reasons applied.
			if !strings.Contains(rec.Body.String(), loginFailed) {
				t.Fatalf("body does not carry the generic message: %q", rec.Body.String())
			}
		})
	}
}

func TestLoginNameIsCanonicalisedToLowercase(t *testing.T) {
	// The game lowercases the account name at login (handler/login.go), so the
	// panel must match or staff typing "Chefe" would be told their account does
	// not exist.
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	rec := postLogin(h, "  CHEFE  ", testPassword)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	for i := 0; i < loginBurst; i++ {
		if rec := postLogin(h, "chefe", "errada"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}
	if rec := postLogin(h, "chefe", "errada"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d: status = %d, want 429", loginBurst+1, rec.Code)
	}
}

func TestRateLimitAlsoStopsSprayingManyNames(t *testing.T) {
	// A different account name each time must still be stopped: the source
	// address is charged too, so the per-account bucket cannot be dodged.
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	for i := 0; i < loginBurst; i++ {
		postLogin(h, "conta"+string(rune('a'+i)), "errada")
	}
	if rec := postLogin(h, "outronome", "errada"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
}

func TestDemotionEndsTheSessionOnTheNextRequest(t *testing.T) {
	acc := newFakeAccounts(roleAdmin)
	h := newTestPanel(t, acc)

	c := sessionCookie(postLogin(h, "chefe", testPassword))
	if c == nil {
		t.Fatal("login did not set a cookie")
	}

	get := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := get(); rec.Code != http.StatusOK {
		t.Fatalf("signed-in request: status = %d, want 200", rec.Code)
	}

	// Demote in the database, touching nothing about the live session.
	acc.setRole("chefe", "player")

	rec := get()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("after demotion: status = %d, want 303", rec.Code)
	}
	// And the session is gone, not merely refused: restoring the role must not
	// silently bring the old session back to life.
	acc.setRole("chefe", roleAdmin)
	if rec := get(); rec.Code != http.StatusSeeOther {
		t.Fatalf("session survived demotion: status = %d, want 303", rec.Code)
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	c := sessionCookie(postLogin(h, "chefe", testPassword))
	if c == nil {
		t.Fatal("login did not set a cookie")
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want 303", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("session still valid after logout: status = %d", rec.Code)
	}
}

func TestSecurityHeadersAreSet(t *testing.T) {
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	for _, hdr := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy"} {
		if rec.Header().Get(hdr) == "" {
			t.Errorf("%s not set", hdr)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP does not forbid framing: %q", csp)
	}
}

func TestClientIPPrefersLeftmostForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 70.41.3.18")
	if got := clientIP(req); got != "203.0.113.7" {
		t.Fatalf("clientIP = %q, want the original client 203.0.113.7", got)
	}
}

// --- contas ---

// signedIn logs in and returns a getter that carries the session cookie.
func signedIn(t *testing.T, h http.Handler) func(path string) *httptest.ResponseRecorder {
	t.Helper()
	c := sessionCookie(postLogin(h, "chefe", testPassword))
	if c == nil {
		t.Fatal("login did not set a cookie")
	}
	return func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
}

func TestContasNeedsASession(t *testing.T) {
	h := newTestPanel(t, newFakeAccounts(roleAdmin))
	for _, path := range []string{"/contas", "/contas/chefe"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s: status = %d, want 303 to /login", path, rec.Code)
		}
	}
}

func TestContasListsAndFiltersByPrefix(t *testing.T) {
	acc := newFakeAccounts(roleAdmin)
	acc.add("ana", 2, "player", false)
	acc.add("andre", 3, "moderator", false)
	acc.add("bruno", 4, "player", true)
	h := newTestPanel(t, acc)
	get := signedIn(t, h)

	all := get("/contas").Body.String()
	for _, name := range []string{"ana", "andre", "bruno", "chefe"} {
		if !strings.Contains(all, name) {
			t.Errorf("listing without a query omitted %q", name)
		}
	}

	// The blocked account has to be visible as blocked; that is the point of the
	// column for someone triaging a support ticket.
	if !strings.Contains(all, "bloqueada") {
		t.Error("listing does not mark the blocked account")
	}

	filtered := get("/contas?q=an").Body.String()
	if !strings.Contains(filtered, "andre") || !strings.Contains(filtered, "ana") {
		t.Error("prefix search dropped a matching account")
	}
	if strings.Contains(filtered, "bruno") {
		t.Error("prefix search returned a non-matching account")
	}
}

func TestContasSearchIsCaseInsensitive(t *testing.T) {
	// Names are stored canonical, so typing the name as it looks in game must work.
	acc := newFakeAccounts(roleAdmin)
	acc.add("ana", 2, "player", false)
	get := signedIn(t, newTestPanel(t, acc))
	if !strings.Contains(get("/contas?q=ANA").Body.String(), "ana") {
		t.Error("uppercase query found nothing")
	}
}

func TestContasCapsTheListingAndSaysSo(t *testing.T) {
	acc := newFakeAccounts(roleAdmin)
	for i := 0; i < searchLimit+10; i++ {
		acc.add(fmt.Sprintf("conta%03d", i), int64(1000+i), "player", false)
	}
	body := signedIn(t, newTestPanel(t, acc))("/contas").Body.String()
	if !strings.Contains(body, "Refine a busca") {
		t.Error("over the cap, the page does not say the list is partial")
	}
	if strings.Count(body, "<tr>") > searchLimit+1 { // +1 for the header row
		t.Error("more rows rendered than the cap allows")
	}
}

func TestContaShowsAccountAndCharacters(t *testing.T) {
	acc := newFakeAccounts(roleAdmin)
	acc.addChar(42, domain.Character{Slot: 0, Name: "Hanteste", Level: 7, Coin: 900000, Hp: 105, MaxHp: 105})
	body := signedIn(t, newTestPanel(t, acc))("/contas/chefe").Body.String()

	for _, want := range []string{"chefe", "Hanteste", "900000", "105"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
}

func TestContaPathIsCaseInsensitive(t *testing.T) {
	get := signedIn(t, newTestPanel(t, newFakeAccounts(roleAdmin)))
	if rec := get("/contas/CHEFE"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestContaUnknownIs404(t *testing.T) {
	get := signedIn(t, newTestPanel(t, newFakeAccounts(roleAdmin)))
	if rec := get("/contas/ninguem"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestContaNeverRendersThePasswordHash(t *testing.T) {
	// store.AccountAuth carries pass_hash, so the detail handler projects into a
	// view type instead of handing the row to the template. If that ever gets
	// undone, the hash lands in staff browsers and in any page cache.
	acc := newFakeAccounts(roleAdmin)
	acc.addChar(42, domain.Character{Slot: 0, Name: "Hanteste", Level: 7})
	body := signedIn(t, newTestPanel(t, acc))("/contas/chefe").Body.String()

	if strings.Contains(body, "$argon2id$") || strings.Contains(body, hashOnce()) {
		t.Fatal("the password hash was rendered into the page")
	}
}

func TestContaWithNoCharacters(t *testing.T) {
	body := signedIn(t, newTestPanel(t, newFakeAccounts(roleAdmin)))("/contas/chefe").Body.String()
	if !strings.Contains(body, "ainda não criou personagem") {
		t.Error("empty roster does not say so")
	}
}

// --- auditoria ---

func TestAuditoriaIsAdminOnly(t *testing.T) {
	// A moderator is signed in and their session is fine, so they get 403 with an
	// explanation — not a redirect to login, which would read as a bug.
	acc := newFakeAccounts(roleModerator)
	get := signedIn(t, newTestPanel(t, acc))
	rec := get("/auditoria")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("moderator status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "só para administradores") {
		t.Error("403 page does not explain why")
	}
}

func TestAuditoriaNavIsHiddenFromModerators(t *testing.T) {
	mod := signedIn(t, newTestPanel(t, newFakeAccounts(roleModerator)))("/")
	if strings.Contains(mod.Body.String(), `href="/auditoria"`) {
		t.Error("a moderator is offered a link they will be refused from")
	}
	adm := signedIn(t, newTestPanel(t, newFakeAccounts(roleAdmin)))("/")
	if !strings.Contains(adm.Body.String(), `href="/auditoria"`) {
		t.Error("an admin is not offered the audit link")
	}
}

func TestAuditoriaListsEntries(t *testing.T) {
	log := newFakeAudit()
	log.add(audit.Entry{
		ID: 1, ActorID: 42, ActorName: "chefe", ActorRole: roleAdmin,
		Action: audit.ActionSetRole, TargetID: 7, TargetName: "ana",
		Old: `{"role":"player"}`, New: `{"role":"moderator"}`,
		CreatedAt: time.Date(2026, 9, 4, 17, 30, 0, 0, time.UTC),
	})
	body := signedIn(t, newTestPanelWith(t, newFakeAccounts(roleAdmin), log))("/auditoria").Body.String()

	for _, want := range []string{"chefe", "SET_ROLE", "ana", "player", "moderator", "04/09 17:30"} {
		if !strings.Contains(body, want) {
			t.Errorf("audit page missing %q", want)
		}
	}
}

func TestAuditoriaFiltersByAccount(t *testing.T) {
	log := newFakeAudit()
	log.add(audit.Entry{ID: 1, ActorName: "chefe", ActorRole: roleAdmin,
		Action: audit.ActionSetRole, TargetID: 7, TargetName: "ana"})
	log.add(audit.Entry{ID: 2, ActorName: "chefe", ActorRole: roleAdmin,
		Action: audit.ActionSetBlocked, TargetID: 9, TargetName: "bruno"})
	get := signedIn(t, newTestPanelWith(t, newFakeAccounts(roleAdmin), log))

	all := get("/auditoria").Body.String()
	if !strings.Contains(all, "ana") || !strings.Contains(all, "bruno") {
		t.Fatal("unfiltered page is missing an entry")
	}

	one := get("/auditoria?conta=7").Body.String()
	if !strings.Contains(one, "ana") {
		t.Error("filtered page dropped the matching entry")
	}
	if strings.Contains(one, "bruno") {
		t.Error("filtered page kept a non-matching entry")
	}
}

func TestAuditoriaRejectsABadAccountFilter(t *testing.T) {
	get := signedIn(t, newTestPanel(t, newFakeAccounts(roleAdmin)))
	for _, q := range []string{"abc", "-1", "0"} {
		if rec := get("/auditoria?conta=" + q); rec.Code != http.StatusBadRequest {
			t.Errorf("conta=%s: status = %d, want 400", q, rec.Code)
		}
	}
}

func TestAuditoriaEmpty(t *testing.T) {
	body := signedIn(t, newTestPanel(t, newFakeAccounts(roleAdmin)))("/auditoria").Body.String()
	if !strings.Contains(body, "Nenhuma ação administrativa registrada") {
		t.Error("empty log does not say so")
	}
}

// --- escritas: cargo e bloqueio ---

// signedInPost logs in and returns a poster that carries the cookie and, unless
// told otherwise, the session's real CSRF token.
func signedInPost(t *testing.T, h http.Handler) (func(path string, form url.Values) *httptest.ResponseRecorder, string) {
	t.Helper()
	c := sessionCookie(postLogin(h, "chefe", testPassword))
	if c == nil {
		t.Fatal("login did not set a cookie")
	}
	// The token is rendered into every form; read it back the way a browser would.
	req := httptest.NewRequest(http.MethodGet, "/contas/ana", nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	token := csrfFrom(rec.Body.String())

	return func(path string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}, token
}

func csrfFrom(body string) string {
	const marker = `name="csrf" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// withTarget gives the panel an account other than the signed-in one to act on.
func withTarget(role string) *fakeAccounts {
	acc := newFakeAccounts(role)
	acc.add("ana", 7, "player", false)
	return acc
}

func TestSetCargoAppliesAndAudits(t *testing.T) {
	acc := withTarget(roleAdmin)
	log := newFakeAudit()
	wr := newFakeWriter()
	h := newTestPanelFull(t, acc, log, wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/cargo", url.Values{"csrf": {token}, "cargo": {"moderator"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(wr.roleCall) != 1 || wr.roleCall[0] != "moderator" {
		t.Fatalf("writer calls = %v, want one moderator", wr.roleCall)
	}
	// The actor comes from the session, never from the form: a request that could
	// name its own actor would let anyone attribute a change to someone else, and
	// the self-change and last-admin guards both key off this value.
	if wr.lastActor != 42 {
		t.Errorf("actor passed to the writer = %d, want the signed-in account (42)", wr.lastActor)
	}
	if wr.lastTarget != 7 {
		t.Errorf("target passed to the writer = %d, want the account in the path (7)", wr.lastTarget)
	}

	recs := log.recorded()
	if len(recs) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(recs))
	}
	if recs[0].Action != audit.ActionSetRole || recs[0].TargetID != 7 {
		t.Fatalf("audit entry = %+v, want SET_ROLE on 7", recs[0])
	}
	if recs[0].ActorRole != roleAdmin {
		t.Errorf("actor role = %q, want the role at the time (%s)", recs[0].ActorRole, roleAdmin)
	}
}

func TestWritesRequireTheCSRFToken(t *testing.T) {
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), newFakeWriter())
	post, token := signedInPost(t, h)

	cases := []struct {
		name string
		form url.Values
	}{
		{"sem token", url.Values{"cargo": {"moderator"}}},
		{"token errado", url.Values{"csrf": {"nao-e-o-token"}, "cargo": {"moderator"}}},
		{"token de outra sessao", url.Values{"csrf": {token + "x"}, "cargo": {"moderator"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/contas/ana/cargo", "/contas/ana/bloqueio"} {
				if rec := post(path, tc.form); rec.Code != http.StatusForbidden {
					t.Errorf("%s: status = %d, want 403", path, rec.Code)
				}
			}
		})
	}
}

func TestSetCargoIsAdminOnly(t *testing.T) {
	// A moderator may block, but not promote.
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), newFakeWriter())
	post, token := signedInPost(t, h)

	if rec := post("/contas/ana/cargo", url.Values{"csrf": {token}, "cargo": {"admin"}}); rec.Code != http.StatusForbidden {
		t.Errorf("moderator changing role: status = %d, want 403", rec.Code)
	}
	if rec := post("/contas/ana/bloqueio", url.Values{"csrf": {token}, "bloquear": {"1"}}); rec.Code != http.StatusSeeOther {
		t.Errorf("moderator blocking: status = %d, want 303", rec.Code)
	}
}

func TestRefusalsAreExplained(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"propria conta", accounts.ErrSelf, "próprio acesso"},
		{"ultimo admin", accounts.ErrLastAdmin, "último admin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wr := newFakeWriter()
			wr.err = tc.err
			h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
			post, token := signedInPost(t, h)

			rec := post("/contas/ana/cargo", url.Values{"csrf": {token}, "cargo": {"player"}})
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303 back to the account page", rec.Code)
			}
			loc := rec.Header().Get("Location")
			decoded, _ := url.QueryUnescape(loc)
			if !strings.Contains(decoded, tc.want) {
				t.Fatalf("redirect %q does not explain the refusal (want %q)", decoded, tc.want)
			}
		})
	}
}

func TestAChangeThatCannotBeAuditedIsReportedAsAFailure(t *testing.T) {
	// The audit write failing must not pass silently. Staff have to know the log
	// has a hole rather than trust a green screen.
	log := newFakeAudit()
	log.failWrite = errors.New("banco fora do ar")
	h := newTestPanelFull(t, withTarget(roleAdmin), log, newFakeWriter())
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/cargo", url.Values{"csrf": {token}, "cargo": {"moderator"}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "auditoria falhou") {
		t.Errorf("the page does not say the audit failed: %q", rec.Body.String())
	}
}

func TestOwnAccountShowsNoControls(t *testing.T) {
	get := signedIn(t, newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), newFakeWriter()))

	mine := get("/contas/chefe").Body.String()
	if strings.Contains(mine, `action="/contas/chefe/cargo"`) {
		t.Error("the panel offers a control on your own account that always fails")
	}
	if !strings.Contains(mine, "sua própria conta") {
		t.Error("no explanation of why the controls are absent")
	}

	other := get("/contas/ana").Body.String()
	if !strings.Contains(other, `action="/contas/ana/cargo"`) {
		t.Error("the role form is missing on another account")
	}
}

func TestBlockSaysItOnlyAppliesAtLogin(t *testing.T) {
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), newFakeWriter())
	post, token := signedInPost(t, h)
	rec := post("/contas/ana/bloqueio", url.Values{"csrf": {token}, "bloquear": {"1"}})
	decoded, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(decoded, "continua até sair") {
		t.Errorf("staff are not told a blocked player stays online: %q", decoded)
	}
}

// --- vip ---

func TestVipGrantIsAuditedAndReported(t *testing.T) {
	wr := newFakeWriter()
	log := newFakeAudit()
	h := newTestPanelFull(t, withTarget(roleModerator), log, wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/vip", url.Values{"csrf": {token}, "dias": {"30"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(wr.vipDays) != 1 || wr.vipDays[0] != 30 {
		t.Fatalf("writer calls = %v, want one grant of 30", wr.vipDays)
	}

	recs := log.recorded()
	if len(recs) != 1 || recs[0].Action != audit.ActionSetVip {
		t.Fatalf("audit entries = %+v, want one SET_VIP", recs)
	}
}

func TestVipGrantOnTopOfAnActiveOneSaysItExtended(t *testing.T) {
	// Staff who read "VIP até <date>" on an already-active account cannot tell
	// whether the remaining days survived. Saying "estendido de X para Y" is what
	// stops somebody granting the days a second time.
	wr := newFakeWriter()
	ate := time.Now().AddDate(0, 0, 10)
	wr.prevVip = &ate
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/vip", url.Values{"csrf": {token}, "dias": {"30"}})
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "estendido") {
		t.Fatalf("message %q does not say the grant extended the existing one", loc)
	}
}

func TestVipRemoval(t *testing.T) {
	wr := newFakeWriter()
	ate := time.Now().AddDate(0, 0, 5)
	wr.prevVip = &ate
	log := newFakeAudit()
	h := newTestPanelFull(t, withTarget(roleModerator), log, wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/vip", url.Values{"csrf": {token}, "remover": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if wr.vipCleared != 1 {
		t.Fatalf("ClearVip calls = %d, want 1", wr.vipCleared)
	}
	if len(log.recorded()) != 1 {
		t.Error("removing VIP was not audited")
	}
}

func TestVipRemovalOnAnAccountWithoutVipChangesNothing(t *testing.T) {
	wr := newFakeWriter() // prevVip stays nil
	log := newFakeAudit()
	h := newTestPanelFull(t, withTarget(roleModerator), log, wr)
	post, token := signedInPost(t, h)

	post("/contas/ana/vip", url.Values{"csrf": {token}, "remover": {"1"}})
	if len(log.recorded()) != 0 {
		t.Error("a no-op removal was written to the audit log")
	}
}

func TestVipRejectsABadDayCount(t *testing.T) {
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), newFakeWriter())
	post, token := signedInPost(t, h)
	for _, d := range []string{"", "abc", "1,5"} {
		if rec := post("/contas/ana/vip", url.Values{"csrf": {token}, "dias": {d}}); rec.Code != http.StatusBadRequest {
			t.Errorf("dias=%q: status = %d, want 400", d, rec.Code)
		}
	}
}

func TestVipOutOfRangeIsExplained(t *testing.T) {
	wr := newFakeWriter()
	wr.err = accounts.ErrVipDays
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/ana/vip", url.Values{"csrf": {token}, "dias": {"99999"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "3650") {
		t.Errorf("the message does not say the allowed range: %q", rec.Body.String())
	}
}

func TestVipNeedsTheCSRFToken(t *testing.T) {
	h := newTestPanelFull(t, withTarget(roleModerator), newFakeAudit(), newFakeWriter())
	post, _ := signedInPost(t, h)
	if rec := post("/contas/ana/vip", url.Values{"dias": {"30"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAccountPageShowsVipEmailAndBalance(t *testing.T) {
	wr := newFakeWriter()
	ate := time.Now().AddDate(0, 0, 12)
	wr.details = accounts.Details{Email: "chefe@exemplo.com", DonateBalance: 4200, VipUntil: &ate}
	body := signedIn(t, newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr))("/contas/ana").Body.String()

	for _, want := range []string{"chefe@exemplo.com", "4200", ate.Local().Format("02/01/2006")} {
		if !strings.Contains(body, want) {
			t.Errorf("account page missing %q", want)
		}
	}
}

// --- itens ---

// fakeGameData stands in for the webServer link.
type fakeGameData struct {
	mu          sync.Mutex
	itens       []gamedata.Item
	setCalls    [][2]int64 // index, price
	listErr     error
	setErr      error
	versao      string
	npcs        []gamedata.NPC
	npcsErr     error
	shopErr     error
	shopSaves   [][]gamedata.ShopItem
	saved       []gamedata.NPC
	visible     []bool
	visibleFor  []int64
	deleted     []int64
	npcWriteErr error
	mobs        []gamedata.MobTemplate
	mobRows     map[string]mobRow
	mobSaved    []gamedata.MobStat
	mobCleared  []string
	mobErr      error
	mobWriteErr error
}

func newFakeGameData() *fakeGameData {
	return &fakeGameData{
		versao: "abc123",
		itens: []gamedata.Item{
			{Index: 1415, Name: "Sapatos_Pele_de_Animal(A)", DisplayName: "Sapatos Pele de Animal (A)",
				Grade: 1, Slots: []string{"boots"}},
			{Index: 2000, Name: "Espada_Longa", DisplayName: "Espada Longa",
				Grade: 2, Slots: []string{"weapon"}, Price: 5000, Overridden: true},
		},
		npcs: []gamedata.NPC{{
			ID: 5, Slug: "mercador-armia", DisplayName: "Mercador de Armia",
			TemplateName: "Mercador", Enabled: true, MapID: 0, X: 2100, Y: 2100,
			Origin: "content",
			Shop: []gamedata.ShopItem{
				{Slot: 0, ItemIndex: 1415, Quantity: 1, Eff: [3][2]int32{{7, 42}, {0, 0}, {0, 0}}},
			},
		}},
		mobs: []gamedata.MobTemplate{
			{Name: "Kentania", DisplayName: "Kentania"},
			{Name: "Mercador", DisplayName: "Mercador de Armia", Merchant: 8},
		},
		mobRows: map[string]mobRow{
			"Kentania": {
				exibido: "Kentania Velha", overridden: true,
				valores: map[string]int64{"level": 10, "exp": 5000, "resist1": 7},
			},
			"Mercador": {valores: map[string]int64{"level": 1}},
		},
	}
}

func (f *fakeGameData) CatalogVersion() string { return f.versao }

func (f *fakeGameData) Items(_ context.Context, _ int64, query string) ([]gamedata.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	if query == "" {
		return f.itens, nil
	}
	out := []gamedata.Item{}
	for _, it := range f.itens {
		if strings.Contains(strings.ToLower(it.DisplayName), strings.ToLower(query)) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeGameData) NPCs(_ context.Context, _ int64, query string) ([]gamedata.NPC, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.npcsErr != nil {
		return nil, f.npcsErr
	}
	out := []gamedata.NPC{}
	for _, n := range f.npcs {
		if query == "" || strings.Contains(strings.ToLower(n.DisplayName), strings.ToLower(query)) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakeGameData) NPC(_ context.Context, _ int64, id int64) (gamedata.NPC, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.npcs {
		if n.ID == id {
			return n, nil
		}
	}
	return gamedata.NPC{}, gamedata.ErrNotFound
}

func (f *fakeGameData) SetShop(_ context.Context, _ int64, npcID int64, items []gamedata.ShopItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.shopErr != nil {
		return f.shopErr
	}
	f.shopSaves = append(f.shopSaves, items)
	return nil
}

func (f *fakeGameData) SaveNPC(_ context.Context, _ int64, n gamedata.NPC) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.npcWriteErr != nil {
		return f.npcWriteErr
	}
	f.saved = append(f.saved, n)
	for i := range f.npcs {
		if f.npcs[i].Slug == n.Slug {
			f.npcs[i] = n
		}
	}
	return nil
}

func (f *fakeGameData) SetNPCVisible(_ context.Context, _ int64, npcID int64, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.npcWriteErr != nil {
		return f.npcWriteErr
	}
	// The id is recorded, not ignored: a handler that hides the wrong NPC would
	// otherwise pass, since the visibility flag alone looks identical.
	f.visibleFor = append(f.visibleFor, npcID)
	f.visible = append(f.visible, enabled)
	return nil
}

func (f *fakeGameData) DeleteNPC(_ context.Context, _ int64, npcID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.npcWriteErr != nil {
		return f.npcWriteErr
	}
	f.deleted = append(f.deleted, npcID)
	return nil
}

func (f *fakeGameData) SetPrice(_ context.Context, _ int64, index int32, price int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.setCalls = append(f.setCalls, [2]int64{int64(index), price})
	return nil
}

// newTestPanelGame builds a panel with the webServer link present.
func newTestPanelGame(t *testing.T, log AuditLog, game GameData) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts:   withTarget(roleAdmin),
		Writer:     newFakeWriter(),
		GameData:   game,
		Audit:      log,
		Sessions:   session.New(time.Hour),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

func TestItensPageIsHiddenWithoutAWebServer(t *testing.T) {
	// The panel has to run standalone; a missing webServer hides the pages
	// rather than serving one that errors on every load.
	get := signedIn(t, newTestPanel(t, withTarget(roleAdmin)))
	if rec := get("/itens"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no webServer is configured", rec.Code)
	}
	if strings.Contains(get("/").Body.String(), `href="/itens"`) {
		t.Error("the nav offers a link to a page that does not exist")
	}
}

func TestItensListsAndFilters(t *testing.T) {
	game := newFakeGameData()
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), game))

	all := get("/itens").Body.String()
	if !strings.Contains(all, "Sapatos Pele de Animal") || !strings.Contains(all, "Espada Longa") {
		t.Error("catalog listing is missing an item")
	}
	if !strings.Contains(all, "exceção") {
		t.Error("an overridden price is not marked as such")
	}
	if !strings.Contains(all, game.versao) {
		t.Error("the catalog version is not shown, so a stale page is unrecognisable")
	}
	if strings.Contains(all, `href="/itens"`) == false {
		t.Error("the nav does not offer the items page when it exists")
	}

	filtrado := get("/itens?q=espada").Body.String()
	if strings.Contains(filtrado, "Sapatos") {
		t.Error("search returned a non-matching item")
	}
}

func TestSetPrecoSendsAndAudits(t *testing.T) {
	game := newFakeGameData()
	log := newFakeAudit()
	h := newTestPanelGame(t, log, game)
	post, token := signedInPost(t, h)

	rec := post("/itens/1415/preco", url.Values{"csrf": {token}, "preco": {"250"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(game.setCalls) != 1 || game.setCalls[0] != [2]int64{1415, 250} {
		t.Fatalf("SetPrice calls = %v, want one {1415, 250}", game.setCalls)
	}
	recs := log.recorded()
	if len(recs) != 1 || recs[0].Action != audit.ActionSetItemPrice {
		t.Fatalf("audit = %+v, want one SET_ITEM_PRICE", recs)
	}
}

func TestEmptyPriceClearsTheOverride(t *testing.T) {
	// Empty and zero are different: one removes the exception, the other makes
	// the item free. A number box cannot express the first, so blank means clear.
	game := newFakeGameData()
	h := newTestPanelGame(t, newFakeAudit(), game)
	post, token := signedInPost(t, h)

	post("/itens/2000/preco", url.Values{"csrf": {token}, "preco": {""}})
	if len(game.setCalls) != 1 || game.setCalls[0][1] != -1 {
		t.Fatalf("SetPrice calls = %v, want the clear sentinel (-1)", game.setCalls)
	}

	post("/itens/2000/preco", url.Values{"csrf": {token}, "preco": {"0"}})
	if len(game.setCalls) != 2 || game.setCalls[1][1] != 0 {
		t.Fatalf("SetPrice calls = %v, want an explicit zero as the second", game.setCalls)
	}
}

func TestSetPrecoRejectsBadInput(t *testing.T) {
	h := newTestPanelGame(t, newFakeAudit(), newFakeGameData())
	post, token := signedInPost(t, h)
	for _, p := range []string{"abc", "-5", "1.5"} {
		if rec := post("/itens/1415/preco", url.Values{"csrf": {token}, "preco": {p}}); rec.Code != http.StatusBadRequest {
			t.Errorf("preco=%q: status = %d, want 400", p, rec.Code)
		}
	}
	if rec := post("/itens/0/preco", url.Values{"csrf": {token}, "preco": {"10"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("item 0: status = %d, want 400", rec.Code)
	}
}

func TestSetPrecoNeedsTheCSRFToken(t *testing.T) {
	h := newTestPanelGame(t, newFakeAudit(), newFakeGameData())
	post, _ := signedInPost(t, h)
	if rec := post("/itens/1415/preco", url.Values{"preco": {"250"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestWebServerFailureIsNotAPanelFailure(t *testing.T) {
	// The webServer redeploys on its own schedule. A page that reads as broken
	// during that window sends staff looking in the wrong place.
	game := newFakeGameData()
	game.listErr = errors.New("connection refused")
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), game))

	rec := get("/itens")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 — the failure is upstream, not here", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "reiniciando") {
		t.Error("the message does not point at the right service")
	}
}

// --- estado do servidor e reinicio ---

// fakePlatform stands in for the hosting API.
type fakePlatform struct {
	mu        sync.Mutex
	dep       plataforma.Deployment
	latestErr error
	restartEr error
	restarts  []string
}

func newFakePlatform() *fakePlatform {
	return &fakePlatform{dep: plataforma.Deployment{
		ID: "dep-1", Status: "SUCCESS", CreatedAt: time.Now().Add(-3 * time.Hour),
	}}
}

func (f *fakePlatform) Latest(context.Context) (plataforma.Deployment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dep, f.latestErr
}

func (f *fakePlatform) Restart(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.restartEr != nil {
		return f.restartEr
	}
	f.restarts = append(f.restarts, id)
	return nil
}

func newTestPanelPlat(t *testing.T, log AuditLog, wr Writer, plat Platform) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts:   withTarget(roleAdmin),
		Writer:     wr,
		Platform:   plat,
		Audit:      log,
		Sessions:   session.New(time.Hour),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

func TestHomeShowsUptimeAndNoPending(t *testing.T) {
	body := signedIn(t, newTestPanelPlat(t, newFakeAudit(), newFakeWriter(), newFakePlatform()))("/").Body.String()
	if !strings.Contains(body, "no ar há") {
		t.Error("uptime not shown")
	}
	if !strings.Contains(body, "nenhuma edição pendente") {
		t.Error("the no-pending state is not stated")
	}
	// The reassurance that matters: staff must not think every edit needs this.
	if !strings.Contains(body, "15 segundos") {
		t.Error("the page does not say NPC and price edits apply on their own")
	}
}

func TestHomeWarnsAboutPendingEdits(t *testing.T) {
	wr := newFakeWriter()
	wr.pendentes = 3
	wr.ultimaEdicao = time.Now().Add(-20 * time.Minute)
	body := signedIn(t, newTestPanelPlat(t, newFakeAudit(), wr, newFakePlatform()))("/").Body.String()

	if !strings.Contains(body, "3 edição") {
		t.Error("the pending count is not shown")
	}
	if !strings.Contains(body, "não existe recarga ao vivo") {
		t.Error("the page does not explain why a restart is needed")
	}
}

func TestRestartIsAdminOnlyAndAudited(t *testing.T) {
	plat := newFakePlatform()
	log := newFakeAudit()
	h := newTestPanelPlat(t, log, newFakeWriter(), plat)
	post, token := signedInPost(t, h)

	rec := post("/servidor/reiniciar", url.Values{"csrf": {token}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(plat.restarts) != 1 || plat.restarts[0] != "dep-1" {
		t.Fatalf("restarts = %v, want one for dep-1", plat.restarts)
	}
	recs := log.recorded()
	if len(recs) != 1 || recs[0].Action != audit.ActionRestartGame {
		t.Fatalf("audit = %+v, want one RESTART_GAME", recs)
	}
}

func TestRestartRefusedForModerators(t *testing.T) {
	plat := newFakePlatform()
	h := newTestPanelPlat(t, newFakeAudit(), newFakeWriter(), plat)
	// Rebuild with a moderator session by using the moderator account set.
	hm, err := New(Config{
		Accounts: withTarget(roleModerator), Writer: newFakeWriter(), Platform: plat,
		Audit: newFakeAudit(), Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	post, token := signedInPost(t, hm.Routes())
	if rec := post("/servidor/reiniciar", url.Values{"csrf": {token}}); rec.Code != http.StatusForbidden {
		t.Fatalf("moderator restart: status = %d, want 403", rec.Code)
	}
	if len(plat.restarts) != 0 {
		t.Fatal("a refused request still restarted the server")
	}
	_ = h
}

func TestRestartIsNotAttemptedWhenItCannotBeAudited(t *testing.T) {
	// Refusing outright, rather than restarting and logging nothing: a restart
	// nobody can explain is the exact thing the log exists to prevent, and it is
	// the one action that also takes the panel's own reporting offline.
	plat := newFakePlatform()
	log := newFakeAudit()
	log.failWrite = errors.New("banco fora do ar")
	h := newTestPanelPlat(t, log, newFakeWriter(), plat)
	post, token := signedInPost(t, h)

	rec := post("/servidor/reiniciar", url.Values{"csrf": {token}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if len(plat.restarts) != 0 {
		t.Fatal("the server was restarted even though the action could not be recorded")
	}
}

func TestRestartNeedsTheCSRFToken(t *testing.T) {
	plat := newFakePlatform()
	h := newTestPanelPlat(t, newFakeAudit(), newFakeWriter(), plat)
	post, _ := signedInPost(t, h)
	if rec := post("/servidor/reiniciar", url.Values{}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(plat.restarts) != 0 {
		t.Fatal("a request without the token restarted the server")
	}
}

func TestHostingFailureDoesNotBreakTheHomePage(t *testing.T) {
	plat := newFakePlatform()
	plat.latestErr = errors.New("token expirado")
	rec := signedIn(t, newTestPanelPlat(t, newFakeAudit(), newFakeWriter(), plat))("/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — the home page must survive a hosting outage", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "continua funcionando normalmente") {
		t.Error("the page does not reassure that the panel itself is fine")
	}
}

func TestRestartCardIsHiddenWithoutTheHostingAPI(t *testing.T) {
	get := signedIn(t, newTestPanel(t, withTarget(roleAdmin)))
	body := get("/").Body.String()
	if strings.Contains(body, "Servidor de jogo") {
		t.Error("the card is shown with no hosting API configured")
	}
	if rec := get("/servidor/reiniciar"); rec.Code != http.StatusNotFound {
		t.Errorf("route exists without the hosting API: status = %d", rec.Code)
	}
}

// --- npcs e lojas ---

func TestNpcsListsAndFilters(t *testing.T) {
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))
	body := get("/npcs").Body.String()
	if !strings.Contains(body, "Mercador de Armia") {
		t.Error("the NPC listing is empty")
	}
	if strings.Contains(get("/npcs?q=zzz").Body.String(), "Mercador de Armia") {
		t.Error("search returned a non-matching NPC")
	}
}

func TestNpcPageRendersEveryStockSlot(t *testing.T) {
	// Occupied slots only would leave no way to ADD stock — the empty rows are
	// the input, not decoration.
	body := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))("/npcs/5").Body.String()
	for _, name := range []string{`name="item0"`, `name="item26"`, `name="qtd26"`} {
		if !strings.Contains(body, name) {
			t.Errorf("missing form field %s", name)
		}
	}
	if strings.Contains(body, `name="item27"`) {
		t.Error("rendered a slot the service does not accept")
	}
	if !strings.Contains(body, `value="1415"`) {
		t.Error("the existing stock is not filled in")
	}
}

func TestNpcPageCarriesEffectsThrough(t *testing.T) {
	body := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))("/npcs/5").Body.String()
	if !strings.Contains(body, `name="eff0_0" value="7"`) || !strings.Contains(body, `name="effv0_0" value="42"`) {
		t.Fatal("the effect pair of an existing stock item is not carried in the form")
	}
}

func TestSetLojaSavesAndAudits(t *testing.T) {
	game := newFakeGameData()
	log := newFakeAudit()
	h := newTestPanelGame(t, log, game)
	post, token := signedInPost(t, h)

	form := url.Values{"csrf": {token}, "item0": {"1415"}, "qtd0": {"3"},
		"eff0_0": {"7"}, "effv0_0": {"42"}, "item1": {"2000"}, "qtd1": {"1"}}
	rec := post("/npcs/5/loja", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(game.shopSaves) != 1 {
		t.Fatalf("shop saves = %d, want 1", len(game.shopSaves))
	}
	saved := game.shopSaves[0]
	if len(saved) != 2 {
		t.Fatalf("saved %d items, want 2", len(saved))
	}
	if saved[0].ItemIndex != 1415 || saved[0].Quantity != 3 {
		t.Errorf("first slot = %+v, want item 1415 x3", saved[0])
	}
	if saved[0].Eff[0] != [2]int32{7, 42} {
		t.Errorf("the effect pair was lost on save: %v", saved[0].Eff[0])
	}
	if len(log.recorded()) != 1 || log.recorded()[0].Action != audit.ActionSetNpcShop {
		t.Error("the shop change was not audited")
	}
}

func TestEmptySlotsAreSimplyNotSent(t *testing.T) {
	game := newFakeGameData()
	h := newTestPanelGame(t, newFakeAudit(), game)
	post, token := signedInPost(t, h)

	// Everything blank: the shop is emptied rather than left alone.
	post("/npcs/5/loja", url.Values{"csrf": {token}})
	if len(game.shopSaves) != 1 || len(game.shopSaves[0]) != 0 {
		t.Fatalf("saves = %v, want one empty list", game.shopSaves)
	}
}

func TestSetLojaRejectsABadItem(t *testing.T) {
	h := newTestPanelGame(t, newFakeAudit(), newFakeGameData())
	post, token := signedInPost(t, h)
	if rec := post("/npcs/5/loja", url.Values{"csrf": {token}, "item0": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSetLojaNeedsTheCSRFToken(t *testing.T) {
	game := newFakeGameData()
	h := newTestPanelGame(t, newFakeAudit(), game)
	post, _ := signedInPost(t, h)
	if rec := post("/npcs/5/loja", url.Values{"item0": {"1415"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(game.shopSaves) != 0 {
		t.Fatal("a request without the token changed the shop")
	}
}

func TestUnknownNpcIs404(t *testing.T) {
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))
	if rec := get("/npcs/999"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestWebServerRefusalIsExplained(t *testing.T) {
	// The panel already checked the role, so a FORBIDDEN from the webServer means
	// the two disagree — worth saying, not hiding behind a generic error.
	game := newFakeGameData()
	game.npcsErr = gamedata.ErrForbidden
	rec := signedIn(t, newTestPanelGame(t, newFakeAudit(), game))("/npcs")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "não reconhece esta conta") {
		t.Errorf("the message does not explain the disagreement: %q", rec.Body.String())
	}
}

// --- escrita de NPC ---

func TestSetLugarPreservesFieldsTheFormDoesNotCarry(t *testing.T) {
	// The service replaces the whole row, keyed on slug. If the handler built an
	// NPC from the form instead of editing the one it read, route type and
	// merchant kind would silently become zero.
	game := newFakeGameData()
	game.npcs[0].RouteType = 3
	game.npcs[0].Merchant = 8
	h := newTestPanelGame(t, newFakeAudit(), game)
	post, token := signedInPost(t, h)

	rec := post("/npcs/5/lugar", url.Values{
		"csrf": {token}, "mapa": {"1"}, "x": {"500"}, "y": {"600"}, "nome": {"Novo Nome"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(game.saved) != 1 {
		t.Fatalf("saves = %d, want 1", len(game.saved))
	}
	got := game.saved[0]
	if got.MapID != 1 || got.X != 500 || got.Y != 600 || got.DisplayName != "Novo Nome" {
		t.Errorf("saved = %+v, want the form's values", got)
	}
	if got.RouteType != 3 || got.Merchant != 8 {
		t.Errorf("route type / merchant lost: got %d and %d, want 3 and 8", got.RouteType, got.Merchant)
	}
	if got.Slug != "mercador-armia" {
		t.Errorf("slug = %q, want the original — the service keys on it", got.Slug)
	}
}

func TestSetLugarRejectsBadCoordinates(t *testing.T) {
	h := newTestPanelGame(t, newFakeAudit(), newFakeGameData())
	post, token := signedInPost(t, h)
	for _, f := range []url.Values{
		{"csrf": {token}, "mapa": {"0"}, "x": {"-1"}, "y": {"10"}},
		{"csrf": {token}, "mapa": {"0"}, "x": {"abc"}, "y": {"10"}},
		{"csrf": {token}, "mapa": {""}, "x": {"1"}, "y": {"1"}},
	} {
		if rec := post("/npcs/5/lugar", f); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: status = %d, want 400", f, rec.Code)
		}
	}
}

func TestVisibilityTogglesAndIsAudited(t *testing.T) {
	game := newFakeGameData() // starts Enabled
	log := newFakeAudit()
	h := newTestPanelGame(t, log, game)
	post, token := signedInPost(t, h)

	rec := post("/npcs/5/visibilidade", url.Values{"csrf": {token}, "visivel": {"0"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if len(game.visible) != 1 || game.visible[0] {
		t.Fatalf("visibility calls = %v, want one false", game.visible)
	}
	if len(game.visibleFor) != 1 || game.visibleFor[0] != 5 {
		t.Fatalf("hid NPC %v, want the one in the path (5)", game.visibleFor)
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "some do mapa") {
		t.Errorf("the message does not say what happens in game: %q", loc)
	}
	if len(log.recorded()) != 1 {
		t.Error("hiding an NPC was not audited")
	}
}

func TestDeleteIsAdminOnly(t *testing.T) {
	game := newFakeGameData()
	hm, err := New(Config{
		Accounts: withTarget(roleModerator), Writer: newFakeWriter(), GameData: game,
		Audit: newFakeAudit(), Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	post, token := signedInPost(t, hm.Routes())
	if rec := post("/npcs/5/apagar", url.Values{"csrf": {token}}); rec.Code != http.StatusForbidden {
		t.Fatalf("moderator delete: status = %d, want 403", rec.Code)
	}
	if len(game.deleted) != 0 {
		t.Fatal("a refused request still deleted the NPC")
	}
}

func TestDeleteIsNotAttemptedWhenItCannotBeAudited(t *testing.T) {
	// Once deleted there is no row left to describe, so the record has to exist
	// first or the action becomes unexplainable.
	game := newFakeGameData()
	log := newFakeAudit()
	log.failWrite = errors.New("banco fora do ar")
	h := newTestPanelGame(t, log, game)
	post, token := signedInPost(t, h)

	rec := post("/npcs/5/apagar", url.Values{"csrf": {token}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if len(game.deleted) != 0 {
		t.Fatal("the NPC was deleted even though the action could not be recorded")
	}
}

func TestContentOwnedDeleteIsExplained(t *testing.T) {
	game := newFakeGameData()
	game.npcWriteErr = gamedata.ErrContentOwned
	h := newTestPanelGame(t, newFakeAudit(), game)
	post, token := signedInPost(t, h)

	rec := post("/npcs/5/apagar", url.Values{"csrf": {token}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "deixe oculto") {
		t.Errorf("the message does not offer the way out: %q", rec.Body.String())
	}
}

func TestNpcWritesNeedTheCSRFToken(t *testing.T) {
	game := newFakeGameData()
	h := newTestPanelGame(t, newFakeAudit(), game)
	post, _ := signedInPost(t, h)
	for _, path := range []string{"/npcs/5/lugar", "/npcs/5/visibilidade", "/npcs/5/apagar"} {
		if rec := post(path, url.Values{"mapa": {"0"}, "x": {"1"}, "y": {"1"}}); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", path, rec.Code)
		}
	}
	if len(game.saved)+len(game.visible)+len(game.deleted) != 0 {
		t.Fatal("a request without the token changed something")
	}
}

// --- monstros ---

// mobRow is what the fake knows about one template, as plain numbers.
//
// A fresh gamedata.MobStat is built on every read, the way the real client gets
// a fresh message from each RPC reply. Handing out one shared value would let a
// handler's edits reach the fake's own state even when the save never happened,
// and a test asserting on the save would then pass for the wrong reason.
type mobRow struct {
	exibido    string
	overridden bool
	valores    map[string]int64
}

func (f *fakeGameData) MobTemplates(_ context.Context, _ int64, query string) ([]gamedata.MobTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mobErr != nil {
		return nil, f.mobErr
	}
	q := strings.ToLower(query)
	out := []gamedata.MobTemplate{}
	for _, m := range f.mobs {
		if q == "" || strings.Contains(strings.ToLower(m.DisplayName), q) ||
			strings.Contains(strings.ToLower(m.Name), q) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeGameData) MobStat(_ context.Context, _ int64, name string) (gamedata.MobStat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mobErr != nil {
		return gamedata.MobStat{}, f.mobErr
	}
	row, ok := f.mobRows[name]
	if !ok {
		return gamedata.MobStat{}, gamedata.ErrNotFound
	}
	s := gamedata.NewMobStat(name, row.overridden)
	s.SetDisplayName(row.exibido)
	for nome, v := range row.valores {
		if !s.Set(nome, v) {
			return gamedata.MobStat{}, fmt.Errorf("fake: campo %q nao existe no formulario", nome)
		}
	}
	return s, nil
}

func (f *fakeGameData) SaveMobStat(_ context.Context, _ int64, m gamedata.MobStat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mobWriteErr != nil {
		return f.mobWriteErr
	}
	f.mobSaved = append(f.mobSaved, m)
	return nil
}

func (f *fakeGameData) ClearMobStat(_ context.Context, _ int64, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mobWriteErr != nil {
		return f.mobWriteErr
	}
	f.mobCleared = append(f.mobCleared, name)
	return nil
}

// campoDe reads one field back out of a saved stat, by its form name.
func campoDe(t *testing.T, m gamedata.MobStat, nome string) int64 {
	t.Helper()
	for _, c := range m.Fields() {
		if c.Nome == nome {
			return c.Valor
		}
	}
	t.Fatalf("campo %q não existe no formulário", nome)
	return 0
}

func TestMonstrosListaEFiltra(t *testing.T) {
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))

	body := get("/monstros").Body.String()
	for _, want := range []string{"Kentania", "Mercador de Armia"} {
		if !strings.Contains(body, want) {
			t.Errorf("a lista não traz %q", want)
		}
	}

	body = get("/monstros?q=kentania").Body.String()
	if !strings.Contains(body, "Kentania") {
		t.Error("a busca perdeu o monstro procurado")
	}
	if strings.Contains(body, "Mercador de Armia") {
		t.Error("a busca trouxe quem não bate com o termo")
	}
}

func TestMonstroMostraOsNumerosEOAvisoDeReinicio(t *testing.T) {
	// The warning is the whole reason this screen differs from itens and npcs:
	// there an edit lands within ~15s, here it waits for a boot. A moderator who
	// does not read that will report the panel as broken.
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))
	body := get("/monstros/Kentania").Body.String()

	for _, want := range []string{
		"Nível",
		`name="level" type="number" step="1"`,
		`value="10"`,
		"Kentania Velha",
		"só vale depois de reiniciar o servidor",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a página do monstro não traz %q", want)
		}
	}
}

func TestMonstroDesconhecidoE404(t *testing.T) {
	get := signedIn(t, newTestPanelGame(t, newFakeAudit(), newFakeGameData()))
	if rec := get("/monstros/NaoExiste"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSetMonstroPreservaOsCamposQueOFormularioNaoCarrega(t *testing.T) {
	// Upsert replaces the whole override row. If the handler built a fresh
	// message from the request instead of editing the one it read, every field
	// the form leaves out — the equipment list above all, which has its own RPC
	// and no place on this screen — would be silently zeroed.
	game := newFakeGameData()
	log := newFakeAudit()
	post, token := signedInPost(t, newTestPanelGame(t, log, game))

	rec := post("/monstros/Kentania", url.Values{"csrf": {token}, "level": {"42"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(game.mobSaved) != 1 {
		t.Fatalf("saves = %d, want 1", len(game.mobSaved))
	}
	got := game.mobSaved[0]
	if v := campoDe(t, got, "level"); v != 42 {
		t.Errorf("level = %d, want 42", v)
	}
	if v := campoDe(t, got, "exp"); v != 5000 {
		t.Errorf("exp = %d, want 5000 — o formulário não mandou, tinha que ficar como estava", v)
	}
	if v := campoDe(t, got, "resist1"); v != 7 {
		t.Errorf("resist1 = %d, want 7 — o formulário não mandou, tinha que ficar como estava", v)
	}
	if got.Name() != "Kentania" {
		t.Errorf("template = %q, want Kentania — o serviço grava por esse nome", got.Name())
	}
	if len(log.recorded()) != 1 {
		t.Error("a edição do monstro não foi auditada")
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if !strings.Contains(loc, "reiniciar") {
		t.Errorf("o aviso não diz que falta reiniciar: %q", loc)
	}
}

func TestSetMonstroNaoApagaONomeQuandoOCampoNaoVem(t *testing.T) {
	// Absent means "leave it"; present-but-empty means "clear it". Treating both
	// as empty would let a partial post wipe the in-game name, which is the
	// opposite of how every number on the form behaves.
	game := newFakeGameData()
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	if rec := post("/monstros/Kentania", url.Values{"csrf": {token}, "level": {"42"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := game.mobSaved[0].DisplayName(); got != "Kentania Velha" {
		t.Errorf("nome exibido = %q, want Kentania Velha", got)
	}

	if rec := post("/monstros/Kentania", url.Values{
		"csrf": {token}, "level": {"42"}, "display_name": {""},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := game.mobSaved[1].DisplayName(); got != "" {
		t.Errorf("nome exibido = %q, want vazio — o campo veio em branco de propósito", got)
	}
}

func TestSetMonstroRecusaValorInvalido(t *testing.T) {
	game := newFakeGameData()
	post, token := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))

	rec := post("/monstros/Kentania", url.Values{"csrf": {token}, "level": {"abc"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(game.mobSaved) != 0 {
		t.Fatal("um valor ilegível ainda assim gravou")
	}
}

func TestSetMonstroPrecisaDoCSRF(t *testing.T) {
	game := newFakeGameData()
	post, _ := signedInPost(t, newTestPanelGame(t, newFakeAudit(), game))
	if rec := post("/monstros/Kentania", url.Values{"level": {"42"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(game.mobSaved) != 0 {
		t.Fatal("uma requisição sem o token gravou o monstro")
	}
}

func TestLimparMonstroChamaOServicoEAudita(t *testing.T) {
	game := newFakeGameData()
	log := newFakeAudit()
	post, token := signedInPost(t, newTestPanelGame(t, log, game))

	rec := post("/monstros/Kentania/limpar", url.Values{"csrf": {token}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(game.mobCleared) != 1 || game.mobCleared[0] != "Kentania" {
		t.Fatalf("limpou %v, want o monstro do caminho (Kentania)", game.mobCleared)
	}
	if len(log.recorded()) != 1 {
		t.Error("a limpeza não foi auditada")
	}
}

func TestMonstroAlteradoMasNaoAuditadoFalha(t *testing.T) {
	// The write already happened, so the response has to say so rather than read
	// as a plain failure the operator would retry.
	game := newFakeGameData()
	log := newFakeAudit()
	log.failWrite = errors.New("banco fora do ar")
	post, token := signedInPost(t, newTestPanelGame(t, log, game))

	rec := post("/monstros/Kentania", url.Values{"csrf": {token}, "level": {"42"}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if len(game.mobSaved) != 1 {
		t.Fatal("a gravação não aconteceu — a mensagem de erro estaria mentindo")
	}
	if !strings.Contains(rec.Body.String(), "foi alterado") {
		t.Errorf("a resposta não avisa que a mudança já valeu: %q", rec.Body.String())
	}
}

func TestMonstrosSomeSemWebServer(t *testing.T) {
	get := signedIn(t, newTestPanel(t, withTarget(roleAdmin)))
	for _, p := range []string{"/monstros", "/monstros/Kentania"} {
		if rec := get(p); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 sem webServer", p, rec.Code)
		}
	}
	if strings.Contains(get("/").Body.String(), `href="/monstros"`) {
		t.Error("o menu oferece um link para uma página que não existe")
	}
}
