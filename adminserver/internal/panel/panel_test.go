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
	mu         sync.Mutex
	roleCall   []string
	blkCall    []bool
	lastActor  int64 // who the handler said was acting
	lastTarget int64 // and on whom
	prevRole   string
	prevBlk    bool
	err        error
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
