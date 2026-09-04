// Package panel serves the staff admin panel: its routes, its session gate and
// its embedded UI.
//
// Everything here is deliberately ordinary web-server code. The single-owner
// loop rule that governs the game path does not apply — this service never
// touches world state, and its own state (sessions, login buckets) is guarded by
// mutexes because it is reached from one goroutine per request.
package panel

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

//go:embed ui/*.html
var uiFS embed.FS

// cookieName is deliberately not the game site's cookie: an admin session must
// never be interchangeable with a player session (SDD §5).
const cookieName = "w2pp_admin_session"

// Staff roles. Anything else — including an unrecognised value — is refused, so
// a typo in the database locks people out rather than letting them in.
const (
	roleModerator = "moderator"
	roleAdmin     = "admin"
)

// loginFailed is what every rejected login says, whatever the real reason. The
// reason goes to the log, never to the browser: distinguishing "no such account"
// from "wrong password" hands an attacker a way to enumerate staff accounts.
const loginFailed = "Usuário ou senha inválidos, ou a conta não tem acesso ao painel."

// searchLimit caps a listing. Staff search by name; the cap exists so a blank
// query cannot pull the whole account table into one page.
const searchLimit = 50

// Accounts is the slice of the store this package needs. Narrow on purpose, so
// the handlers can be tested without a database.
//
// Every method here already existed for other callers. That is deliberate: the
// panel reads a schema it does not own, and adding to internal/store would drag
// the game services into this feature's deploys for a read-only screen.
type Accounts interface {
	AccountByName(ctx context.Context, name string) (store.AccountAuth, error)
	AccountRole(ctx context.Context, id int64) (string, error)
	SearchAccountsByNamePrefix(ctx context.Context, prefix string, limit int) ([]domain.AccountSummary, error)
	ListCharacters(ctx context.Context, accountID int64) ([]domain.Character, error)
}

// AuditLog is the panel's view of the action log. Reading is all this file
// needs; the write side belongs to whichever handler performs the action.
type AuditLog interface {
	List(ctx context.Context, targetID int64) ([]audit.Entry, error)
	Limit() int
}

// Config wires the handler.
type Config struct {
	Accounts   Accounts
	Audit      AuditLog
	Sessions   *session.Store
	Logger     *slog.Logger
	SecureOnly bool // Secure flag on the cookie; false only for local HTTP dev
}

// Handler is the panel's HTTP surface.
type Handler struct {
	cfg   Config
	tmpl  *template.Template
	limit *limiter
	decoy string // see newDecoyHash
}

// New builds the handler and parses the embedded templates.
func New(cfg Config) (*Handler, error) {
	tmpl, err := template.ParseFS(uiFS, "ui/*.html")
	if err != nil {
		return nil, err
	}
	return &Handler{cfg: cfg, tmpl: tmpl, limit: newLimiter(), decoy: newDecoyHash()}, nil
}

// newDecoyHash returns a real argon2id hash of a value nobody knows.
//
// It exists so a login for a non-existent account still pays the cost of a hash
// verification. Without it, "no such account" returns in microseconds while a
// wrong password takes the ~100ms argon2id needs, and that gap alone tells an
// attacker which staff names are real — the same thing the generic error message
// is there to hide.
func newDecoyHash() string {
	h, err := secret.HashSecret("decoy-" + time.Now().Format(time.RFC3339Nano))
	if err != nil {
		// Hashing cannot realistically fail; if it did, an empty decoy only
		// costs the timing defence, so refusing to boot over it would be worse.
		return ""
	}
	return h
}

// Routes builds the mux.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Liveness. No session, no database, no disk — see the note in cmd/.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.Handle("GET /{$}", h.requireStaff(http.HandlerFunc(h.home)))
	mux.Handle("GET /contas", h.requireStaff(http.HandlerFunc(h.contas)))
	mux.Handle("GET /contas/{nome}", h.requireStaff(http.HandlerFunc(h.conta)))
	mux.Handle("GET /auditoria", h.requireStaff(h.onlyAdmin(http.HandlerFunc(h.auditoria))))

	return securityHeaders(mux)
}

// page carries what every signed-in template needs: who is looking, with what
// authority, and which nav entry to mark.
type page struct {
	Account string
	Role    string
	Nav     string
	IsAdmin bool // hides nav entries the viewer would only be refused from
}

func pageFor(r *http.Request, nav string) page {
	sess, _ := staffFrom(r.Context())
	role := roleFrom(r.Context())
	return page{Account: sess.AccountName, Role: role, Nav: nav, IsAdmin: role == roleAdmin}
}

// --- pages ---

func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	// Already signed in: skip the form rather than inviting a second login.
	if _, _, ok := h.currentSession(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.render(w, "login.html", map[string]any{"Error": r.URL.Query().Get("erro")})
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	h.render(w, "index.html", struct{ page }{pageFor(r, "inicio")})
}

// contas lists accounts whose name starts with the query. A blank query lists the
// first page alphabetically, which is the useful default on a small server and
// still bounded by searchLimit.
func (h *Handler) contas(w http.ResponseWriter, r *http.Request) {
	// Lowercased for the same reason login is: names are stored canonical, so a
	// staff member typing "Chefe" must find "chefe" rather than nothing.
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	// One over the cap, so "there is more" is known rather than guessed at.
	found, err := h.cfg.Accounts.SearchAccountsByNamePrefix(r.Context(), q, searchLimit+1)
	if err != nil {
		h.cfg.Logger.Error("account search failed", "query", q, "err", err)
		http.Error(w, "Erro ao buscar contas.", http.StatusInternalServerError)
		return
	}
	truncado := len(found) > searchLimit
	if truncado {
		found = found[:searchLimit]
	}

	h.render(w, "contas.html", struct {
		page
		Query    string
		Contas   []domain.AccountSummary
		Truncado bool
		Limite   int
	}{pageFor(r, "contas"), q, found, truncado, searchLimit})
}

// conta shows one account and its characters.
func (h *Handler) conta(w http.ResponseWriter, r *http.Request) {
	nome := strings.ToLower(strings.TrimSpace(r.PathValue("nome")))

	auth, err := h.cfg.Accounts.AccountByName(r.Context(), nome)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.cfg.Logger.Error("account lookup failed", "account", nome, "err", err)
		http.Error(w, "Erro ao carregar a conta.", http.StatusInternalServerError)
		return
	}

	chars, err := h.cfg.Accounts.ListCharacters(r.Context(), auth.ID)
	if err != nil {
		// The account is worth showing even if the character list fails; losing
		// the roster should not blank out the role and block status too.
		h.cfg.Logger.Error("character list failed", "account", nome, "id", auth.ID, "err", err)
		chars = nil
	}

	h.render(w, "conta.html", struct {
		page
		Conta       contaView
		Personagens []domain.Character
	}{
		pageFor(r, "contas"),
		contaView{ID: auth.ID, Name: nome, Role: auth.Role, IsBlocked: auth.IsBlocked},
		chars,
	})
}

// onlyAdmin narrows a staff route to the admin tier. It runs INSIDE requireStaff,
// so the role it reads is the one fetched from the database for this request.
//
// A moderator gets 403 rather than a redirect: they are signed in and their
// session is fine, so bouncing them to the login form would read as a bug.
func (h *Handler) onlyAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if roleFrom(r.Context()) != roleAdmin {
			sess, _ := staffFrom(r.Context())
			h.cfg.Logger.Warn("admin-only route refused",
				"account", sess.AccountName, "role", roleFrom(r.Context()), "path", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			h.render(w, "negado.html", struct{ page }{pageFor(r, "")})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// auditoria shows the action log, optionally narrowed to one account.
func (h *Handler) auditoria(w http.ResponseWriter, r *http.Request) {
	var alvo int64
	if v := r.URL.Query().Get("conta"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Conta inválida.", http.StatusBadRequest)
			return
		}
		alvo = id
	}

	entradas, err := h.cfg.Audit.List(r.Context(), alvo)
	if err != nil {
		h.cfg.Logger.Error("audit list failed", "target", alvo, "err", err)
		http.Error(w, "Erro ao carregar a auditoria.", http.StatusInternalServerError)
		return
	}

	h.render(w, "auditoria.html", struct {
		page
		Entradas []audit.Entry
		Alvo     int64
		Limite   int
		Truncado bool
	}{
		pageFor(r, "auditoria"), entradas, alvo, h.cfg.Audit.Limit(),
		len(entradas) == h.cfg.Audit.Limit(),
	})
}

// contaView is what the detail page shows. It exists so the password hash that
// rides along in store.AccountAuth never reaches a template.
type contaView struct {
	ID        int64
	Name      string
	Role      string
	IsBlocked bool
}

// --- login / logout ---

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.failLogin(w, r, "form ilegível", "")
		return
	}
	name := strings.ToLower(strings.TrimSpace(r.PostFormValue("usuario")))
	pass := r.PostFormValue("senha")
	ip := clientIP(r)

	// Throttle BEFORE hashing. Argon2id is configured to cost 64 MiB per call;
	// verifying first would let anyone turn the login form into a memory
	// exhaustion tool.
	if !h.limit.allow("ip:"+ip, "conta:"+name) {
		h.cfg.Logger.Warn("login rate limited", "ip", ip, "account", name)
		http.Error(w, "Muitas tentativas. Espere um minuto.", http.StatusTooManyRequests)
		return
	}

	auth, err := h.cfg.Accounts.AccountByName(r.Context(), name)
	if errors.Is(err, store.ErrNotFound) {
		// Spend the same time a real account would, then fail identically.
		_, _ = secret.VerifySecret(pass, h.decoy)
		h.failLogin(w, r, "conta inexistente", name)
		return
	}
	if err != nil {
		h.cfg.Logger.Error("login lookup failed", "account", name, "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}

	ok, err := secret.VerifySecret(pass, auth.PassHash)
	if err != nil {
		h.cfg.Logger.Error("login verify failed", "account", name, "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}
	if !ok {
		h.failLogin(w, r, "senha incorreta", name)
		return
	}
	if auth.IsBlocked {
		h.failLogin(w, r, "conta bloqueada", name)
		return
	}
	if !isStaff(auth.Role) {
		h.failLogin(w, r, "cargo sem acesso ao painel", name)
		return
	}

	token, _, err := h.cfg.Sessions.Create(auth.ID, name)
	if err != nil {
		h.cfg.Logger.Error("session create failed", "account", name, "err", err)
		http.Error(w, "Erro interno.", http.StatusInternalServerError)
		return
	}
	h.setCookie(w, token)
	h.cfg.Logger.Info("login ok", "account", name, "id", auth.ID, "role", auth.Role, "ip", ip)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		h.cfg.Sessions.Delete(c.Value)
	}
	h.clearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// failLogin answers every rejection the same way and records why on the server.
func (h *Handler) failLogin(w http.ResponseWriter, r *http.Request, reason, account string) {
	h.cfg.Logger.Warn("login denied", "reason", reason, "account", account, "ip", clientIP(r))
	w.WriteHeader(http.StatusUnauthorized)
	h.render(w, "login.html", map[string]any{"Error": loginFailed})
}

// --- session gate ---

type ctxKey int

const (
	ctxSession ctxKey = iota
	ctxRole
)

// requireStaff admits a request only if it carries a live session whose account
// STILL holds a staff role.
//
// The role is read from the database here, on every request, rather than stored
// in the session at login. That is the whole point: demoting someone has to take
// effect now, not whenever their session happens to lapse — and demotion is the
// one action whose delay actually matters. The cost is one primary-key lookup per
// request, which is nothing at staff-panel volume.
func (h *Handler) requireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, sess, ok := h.currentSession(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		role, err := h.cfg.Accounts.AccountRole(r.Context(), sess.AccountID)
		if err != nil {
			// Includes ErrNotFound: an account deleted mid-session is not staff.
			h.cfg.Logger.Warn("session role lookup failed; ending session",
				"account", sess.AccountName, "id", sess.AccountID, "err", err)
			h.cfg.Sessions.Delete(token)
			h.clearCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !isStaff(role) {
			h.cfg.Logger.Info("session ended: role no longer staff",
				"account", sess.AccountName, "id", sess.AccountID, "role", role)
			h.cfg.Sessions.Delete(token)
			h.clearCookie(w)
			http.Redirect(w, r, "/login?erro=Seu+acesso+foi+revogado.", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxSession, sess)
		ctx = context.WithValue(ctx, ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) currentSession(r *http.Request) (string, session.Session, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", session.Session{}, false
	}
	sess, ok := h.cfg.Sessions.Get(c.Value)
	if !ok {
		return "", session.Session{}, false
	}
	return c.Value, sess, true
}

// staffFrom returns the signed-in session a requireStaff-wrapped handler runs
// under.
func staffFrom(ctx context.Context) (session.Session, bool) {
	s, ok := ctx.Value(ctxSession).(session.Session)
	return s, ok
}

// roleFrom returns the role read for this request.
func roleFrom(ctx context.Context) string {
	r, _ := ctx.Value(ctxRole).(string)
	return r
}

func isStaff(role string) bool { return role == roleModerator || role == roleAdmin }

// --- cookies, headers, helpers ---

func (h *Handler) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.SecureOnly,
		// Strict, not Lax: the panel is never linked to from elsewhere, and
		// Strict is what keeps a cross-site form post from acting as the
		// signed-in user. It stands in for a CSRF token while every route that
		// changes something is still to come; the first of those adds one.
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionCookieMaxAge.Seconds()),
	})
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: h.cfg.SecureOnly,
		SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

// sessionCookieMaxAge bounds the cookie itself. The server-side session decides
// validity; this only stops a browser holding a cookie that can no longer work.
const sessionCookieMaxAge = 2 * time.Hour

func (h *Handler) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := h.tmpl.ExecuteTemplate(w, page, data); err != nil {
		h.cfg.Logger.Error("render failed", "page", page, "err", err)
	}
}

// securityHeaders applies the few headers that matter for a page which is public
// on the internet and shows account data. The CSP is restrictive because the UI
// is self-contained: no external script, style or font is loaded, so nothing
// legitimate is blocked by forbidding them.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// clientIP prefers the forwarded address: the service always runs behind the
// platform's proxy, so RemoteAddr is that proxy for every request and would
// throttle every user as one. The header is caller-controlled, which is fine for
// a rate-limit key and for logs, and is never used for authorization.
func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		if i := strings.IndexByte(f, ','); i >= 0 {
			return strings.TrimSpace(f[:i]) // left-most entry is the original client
		}
		return strings.TrimSpace(f)
	}
	return r.RemoteAddr
}
