package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/accounts"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/secret"
)

// setSenha replaces a player's password and shows the new one once.
//
// Two things shape this handler.
//
// The result is rendered, not redirected. Every other write on this page answers
// 303 with the message in the query string, which is right for "Gravado" and
// wrong for a password: it would land in the browser history, in the proxy logs
// and in anything that records URLs. The password is shown on a page that is
// never bookmarkable and never repeated.
//
// An empty field means GENERATE, never empty. secret.HashSecret("") returns an
// empty hash meaning "no secret set", which VerifySecret then matches against an
// empty password — so a blank form would silently make the account log in with
// no password at all. Structuring the default as "generate" makes that state
// unreachable rather than merely guarded against.
func (h *Handler) setSenha(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	conta, auth, ok := h.alvo(w, r)
	if !ok {
		return
	}

	// Rank guard. Any staff may reset a player; only an admin may reset another
	// staff member. Without this a moderator could reset the admin's password and
	// sign in as them, which turns "moderator" into "admin" in one click — and the
	// panel has no other rank check to lean on.
	if auth.Role != accounts.RolePlayer && roleFrom(r.Context()) != roleAdmin {
		http.Error(w, "Só um admin pode trocar a senha de outro membro da equipe.",
			http.StatusForbidden)
		return
	}

	nova := strings.TrimSpace(r.PostFormValue("senha"))
	gerada := nova == ""
	if gerada {
		var err error
		if nova, err = accounts.GerarSenha(); err != nil {
			h.cfg.Logger.Error("password generation failed", "err", err)
			http.Error(w, "Erro ao gerar a senha.", http.StatusInternalServerError)
			return
		}
	}
	if err := accounts.ValidarSenha(nova); err != nil {
		http.Error(w, explicaSenha(err), http.StatusBadRequest)
		return
	}

	hash, err := secret.HashSecret(nova)
	if err != nil {
		h.cfg.Logger.Error("password hash failed", "account", conta, "err", err)
		http.Error(w, "Erro ao preparar a senha.", http.StatusInternalServerError)
		return
	}
	if err := h.cfg.Writer.SetPassword(r.Context(), auth.ID, hash); err != nil {
		h.cfg.Logger.Error("password write failed", "account", conta, "err", err)
		http.Error(w, "Erro ao gravar a senha.", http.StatusInternalServerError)
		return
	}

	// Whoever was signed into the panel on that account is now signed in with a
	// password that no longer exists. For a player this deletes nothing; for a
	// staff member whose password was just reset, it is the point.
	encerradas := h.cfg.Sessions.DeleteByAccount(auth.ID)

	// The audit records that it happened and nothing about what it is. Neither
	// the password nor its hash goes in: the log is readable by every admin, and
	// a hash in it is a hash to attack offline.
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetPassword, TargetID: auth.ID,
		New: map[string]any{"gerada": gerada, "sessoes_encerradas": encerradas},
	}); err != nil {
		h.cfg.Logger.Error("password changed but NOT audited", "account", conta, "err", err)
		http.Error(w, "A senha foi trocada, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("password reset",
		"actor", sess.AccountName, "account", conta, "gerada", gerada, "sessoes", encerradas)

	h.render(w, "senha.html", struct {
		page
		Conta      string
		Senha      string
		Gerada     bool
		Encerradas int
	}{pageFor(r, "contas", h.cfg.GameData != nil), conta, nova, gerada, encerradas})
}

// explicaSenha turns a rule into something a moderator can act on. The rules
// come from the game client, so the message says which one and why.
func explicaSenha(err error) string {
	switch {
	case errors.Is(err, accounts.ErrSenhaLonga):
		return "A senha não pode passar de 12 caracteres — é o que o cliente do jogo carrega."
	case errors.Is(err, accounts.ErrSenhaCurta):
		return "A senha precisa de pelo menos 4 caracteres."
	case errors.Is(err, accounts.ErrSenhaEspaco):
		return "A senha não pode ter espaço: o jogo corta espaço no fim e o jogador não conseguiria digitar de volta."
	case errors.Is(err, accounts.ErrSenhaCaractere):
		return "Use só letras, números e sinais comuns do teclado — o cliente é antigo e não lida com acento."
	default:
		return "Senha inválida."
	}
}
