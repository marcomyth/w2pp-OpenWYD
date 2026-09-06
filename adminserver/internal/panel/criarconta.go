package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
)

// criarConta creates a player account from the panel and shows its password once.
//
// It is the counterpart of setSenha and follows the same two rules, for the same
// reasons:
//
// The result is rendered, not redirected — a password in a redirect lands in the
// browser history and in every proxy log along the way.
//
// An empty password field means GENERATE, never empty. secret.HashSecret("")
// returns an empty hash meaning "no secret set", which VerifySecret then matches
// against an empty password: the account would log in with no password at all.
// Making "generate" the default puts that state out of reach instead of merely
// guarding it.
//
// The new account is always a player. Creating staff from a form is a different
// decision with a different blast radius, and the role page already exists for
// it — one deliberate step, with its own audit line.
func (h *Handler) criarConta(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())

	// Canonical form first: the game looks accounts up lowercased, so that is
	// what uniqueness and validation are both about.
	nome := accounts.CanonicalNome(r.PostFormValue("nome"))
	if err := accounts.ValidarNome(nome); err != nil {
		http.Error(w, explicaNome(err), http.StatusBadRequest)
		return
	}

	senha := strings.TrimSpace(r.PostFormValue("senha"))
	gerada := senha == ""
	if gerada {
		var err error
		if senha, err = accounts.GerarSenha(); err != nil {
			h.cfg.Logger.Error("password generation failed", "err", err)
			http.Error(w, "Erro ao gerar a senha.", http.StatusInternalServerError)
			return
		}
	}
	if err := accounts.ValidarSenha(senha); err != nil {
		http.Error(w, explicaSenha(err), http.StatusBadRequest)
		return
	}

	hash, err := secret.HashSecret(senha)
	if err != nil {
		h.cfg.Logger.Error("password hash failed", "account", nome, "err", err)
		http.Error(w, "Erro ao preparar a senha.", http.StatusInternalServerError)
		return
	}

	id, err := h.cfg.Writer.Criar(r.Context(), nome, hash, r.PostFormValue("email"))
	switch {
	case errors.Is(err, accounts.ErrNomeEmUso):
		http.Error(w, "Já existe uma conta com esse nome.", http.StatusConflict)
		return
	case errors.Is(err, accounts.ErrNomeVazio), errors.Is(err, accounts.ErrNomeCurto),
		errors.Is(err, accounts.ErrNomeLongo), errors.Is(err, accounts.ErrNomeCaractere):
		http.Error(w, explicaNome(err), http.StatusBadRequest)
		return
	case err != nil:
		h.cfg.Logger.Error("account creation failed", "account", nome, "err", err)
		http.Error(w, "Erro ao criar a conta.", http.StatusInternalServerError)
		return
	}

	// Neither the password nor its hash goes in the log: every admin can read
	// the audit, and a hash sitting there is a hash to attack offline.
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionCreateAccount, TargetID: id,
		New: map[string]any{"conta": nome, "senha_gerada": gerada},
	}); err != nil {
		h.cfg.Logger.Error("account created but NOT audited", "account", nome, "err", err)
		http.Error(w, "A conta foi criada, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("account created", "actor", sess.AccountName, "account", nome, "gerada", gerada)

	// Same screen as a password reset: the credential is shown once and never
	// again, so it reuses that template rather than growing a near-copy.
	h.render(w, "senha.html", struct {
		page
		Conta      string
		Senha      string
		Gerada     bool
		Encerradas int
		Criada     bool
	}{h.pageFor(r, "contas"), nome, senha, gerada, 0, true})
}

// explicaNome turns a name rule into something a moderator can act on. The rules
// come from the game login, so the message says which one and why.
func explicaNome(err error) string {
	switch {
	case errors.Is(err, accounts.ErrNomeVazio):
		return "Informe o nome da conta."
	case errors.Is(err, accounts.ErrNomeCurto):
		return "O nome da conta precisa de pelo menos 4 caracteres."
	case errors.Is(err, accounts.ErrNomeLongo):
		return "O nome da conta não pode passar de 12 caracteres — é o que o cliente do jogo carrega."
	case errors.Is(err, accounts.ErrNomeCaractere):
		return "Use só letras e números, sem espaço e sem acento: é o que o jogador consegue digitar no cliente."
	default:
		return "Nome de conta inválido."
	}
}
