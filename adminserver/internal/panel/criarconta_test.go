package panel

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
)

func TestCriarContaGravaEAudita(t *testing.T) {
	log := newFakeAudit()
	wr := newFakeWriter()
	h := newTestPanelFull(t, withTarget(roleAdmin), log, wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/criar", url.Values{
		"csrf": {token}, "nome": {"jogador1"}, "senha": {"segredo12"}, "email": {"a@b.com"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(wr.criadas) != 1 {
		t.Fatalf("contas criadas = %d, want 1", len(wr.criadas))
	}
	c := wr.criadas[0]
	if c.Nome != "jogador1" || c.Email != "a@b.com" {
		t.Errorf("conta = %+v", c)
	}
	// The password reaches the database hashed, never in the clear.
	if c.Hash == "segredo12" || c.Hash == "" {
		t.Fatalf("hash = %q, a senha não pode ir em claro nem vazia", c.Hash)
	}
	ok, err := secret.VerifySecret("segredo12", c.Hash)
	if err != nil || !ok {
		t.Errorf("o hash gravado não confere com a senha: ok=%v err=%v", ok, err)
	}
	if len(log.written) != 1 || log.written[0].Action != audit.ActionCreateAccount {
		t.Fatalf("auditoria = %+v", log.written)
	}
}

// The password is shown once, on a rendered page — never in a redirect, where it
// would land in the browser history and in every proxy log on the way.
func TestCriarContaMostraASenhaUmaVezSemRedirect(t *testing.T) {
	wr := newFakeWriter()
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/criar", url.Values{"csrf": {token}, "nome": {"jogador1"}, "senha": {"segredo12"}})
	if rec.Code == http.StatusSeeOther {
		t.Fatal("respondeu com redirect: a senha iria parar na URL")
	}
	if !strings.Contains(rec.Body.String(), "segredo12") {
		t.Error("a página deveria mostrar a senha uma vez")
	}
	if loc := rec.Header().Get("Location"); strings.Contains(loc, "segredo12") {
		t.Errorf("a senha vazou para o Location: %q", loc)
	}
}

// An empty password field means GENERATE, never empty: secret.HashSecret("")
// yields the "no secret set" hash, which VerifySecret matches against an empty
// password — the account would log in with no password at all.
func TestCriarContaComSenhaVaziaGeraUma(t *testing.T) {
	wr := newFakeWriter()
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/criar", url.Values{"csrf": {token}, "nome": {"jogador1"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if len(wr.criadas) != 1 {
		t.Fatalf("contas criadas = %d, want 1", len(wr.criadas))
	}
	if h := wr.criadas[0].Hash; h == "" {
		t.Fatal("hash vazio: a conta entraria sem senha nenhuma")
	}
	// And an empty password must NOT verify against what was stored.
	if ok, _ := secret.VerifySecret("", wr.criadas[0].Hash); ok {
		t.Error("senha vazia confere com o hash gravado — a conta entra sem senha")
	}
}

// The name rules come from the game login; a name the panel accepts and the
// client cannot type is an account nobody can use.
func TestCriarContaRecusaNomeInvalido(t *testing.T) {
	cases := []struct {
		nome   string
		porque string
	}{
		{"", "vazio"},
		{"ab", "curto demais"},
		{"contamuitolonga", "longo demais"},
		{"João", "acento"},
		{"com espaco", "espaço"},
		{"user_1", "sublinhado"},
	}
	for _, c := range cases {
		t.Run(c.porque, func(t *testing.T) {
			wr := newFakeWriter()
			h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
			post, token := signedInPost(t, h)

			rec := post("/contas/criar", url.Values{"csrf": {token}, "nome": {c.nome}, "senha": {"segredo12"}})
			if rec.Code != http.StatusBadRequest {
				t.Errorf("nome %q (%s): status = %d, want 400", c.nome, c.porque, rec.Code)
			}
			if len(wr.criadas) != 0 {
				t.Errorf("nome %q não deveria criar nada", c.nome)
			}
		})
	}
}

// The stored login is lowercase: the game looks accounts up that way, so
// "Jogador1" and "jogador1" must not become two accounts.
func TestCriarContaGuardaONomeEmMinusculas(t *testing.T) {
	wr := newFakeWriter()
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	post("/contas/criar", url.Values{"csrf": {token}, "nome": {"  JoGaDoR1  "}, "senha": {"segredo12"}})
	if len(wr.criadas) != 1 || wr.criadas[0].Nome != "jogador1" {
		t.Errorf("conta = %+v, want nome canônico jogador1", wr.criadas)
	}
}

func TestCriarContaRecusaNomeEmUso(t *testing.T) {
	wr := newFakeWriter()
	wr.criarErr = accounts.ErrNomeEmUso
	h := newTestPanelFull(t, withTarget(roleAdmin), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/criar", url.Values{"csrf": {token}, "nome": {"jogador1"}, "senha": {"segredo12"}})
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

// Creating an account hands out a login, so it is admin-only — a moderator
// could otherwise mint themselves an account outside the role rules.
func TestCriarContaEhSoDeAdmin(t *testing.T) {
	wr := newFakeWriter()
	h := newTestPanelFull(t, newFakeAccounts(roleModerator), newFakeAudit(), wr)
	post, token := signedInPost(t, h)

	rec := post("/contas/criar", url.Values{"csrf": {token}, "nome": {"jogador1"}, "senha": {"segredo12"}})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if len(wr.criadas) != 0 {
		t.Error("um moderador não deveria criar conta")
	}
}

// Neither the password nor its hash may reach the audit: every admin can read it,
// and a hash sitting there is a hash to attack offline.
func TestAuditoriaDaCriacaoNaoGuardaSenha(t *testing.T) {
	log := newFakeAudit()
	wr := newFakeWriter()
	h := newTestPanelFull(t, withTarget(roleAdmin), log, wr)
	post, token := signedInPost(t, h)

	post("/contas/criar", url.Values{"csrf": {token}, "nome": {"jogador1"}, "senha": {"segredo12"}})
	if len(log.written) != 1 {
		t.Fatalf("auditoria = %d registros, want 1", len(log.written))
	}
	registro := strings.ToLower(dumpJSON(t, log.written[0].New))
	for _, proibido := range []string{"segredo12", "argon2", wr.criadas[0].Hash} {
		if proibido != "" && strings.Contains(registro, strings.ToLower(proibido)) {
			t.Errorf("a auditoria guardou %q: %s", proibido, registro)
		}
	}
}

// dumpJSON renders an audit payload the way the log stores it, so a test can
// assert on what is actually written rather than on the Go value.
func dumpJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
