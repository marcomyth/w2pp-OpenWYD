package panel

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
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
	mu   sync.Mutex
	rows map[string]store.AccountAuth
}

func newFakeAccounts(role string) *fakeAccounts {
	return &fakeAccounts{rows: map[string]store.AccountAuth{
		"chefe": {ID: 42, PassHash: hashOnce(), Role: role},
	}}
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

// newTestPanel builds a handler over the given accounts, with logs discarded.
func newTestPanel(t *testing.T, acc Accounts) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts:   acc,
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
