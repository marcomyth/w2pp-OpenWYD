package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
)

// servidor shows who the game server is holding right now, and offers the two
// live actions: kick an account and send a notice.
//
// It does NOT refresh on a timer, and must not learn to. Every call here crosses
// into the single-owner game loop, whose callback queue is drained ahead of
// player input — a page polling this every few seconds would front-run the
// people playing.
func (h *Handler) servidor(w http.ResponseWriter, r *http.Request) {
	estado, err := h.cfg.Jogo.Estado(r.Context())
	erro := ""
	if err != nil {
		h.cfg.Logger.Warn("live server state unavailable", "err", err)
		erro = explicaJogo(err)
	}

	h.render(w, "servidor.html", struct {
		page
		Estado jogo.Estado
		Erro   string
		Aviso  string
	}{h.pageFor(r, "servidor"), estado, erro, r.URL.Query().Get("aviso")})
}

// derrubarConta ends every session of one account.
func (h *Handler) derrubarConta(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	conta := strings.TrimSpace(r.PostFormValue("conta"))
	if conta == "" {
		http.Error(w, "Informe a conta.", http.StatusBadRequest)
		return
	}

	n, err := h.cfg.Jogo.Derrubar(r.Context(), conta)
	if err != nil {
		h.cfg.Logger.Error("kick failed", "conta", conta, "err", err)
		http.Error(w, explicaJogo(err), http.StatusBadGateway)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionKick,
		New:    map[string]any{"conta": conta, "sessoes": n},
	}); err != nil {
		h.cfg.Logger.Error("kick done but NOT audited", "conta", conta, "err", err)
		http.Error(w, "A conta foi derrubada, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("account kicked", "actor", sess.AccountName, "conta", conta, "sessoes", n)
	msg := conta + " não estava conectada."
	if n > 0 {
		msg = conta + " foi derrubada."
	}
	http.Redirect(w, r, "/servidor?aviso="+urlQuery(msg), http.StatusSeeOther)
}

// avisarTodos sends a notice to everyone in play.
func (h *Handler) avisarTodos(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	msg := strings.TrimSpace(r.PostFormValue("mensagem"))
	if msg == "" {
		http.Error(w, "Escreva a mensagem.", http.StatusBadRequest)
		return
	}

	n, err := h.cfg.Jogo.Avisar(r.Context(), msg)
	if errors.Is(err, jogo.ErrInvalido) {
		http.Error(w, explicaJogo(err), http.StatusBadRequest)
		return
	}
	if err != nil {
		h.cfg.Logger.Error("broadcast failed", "err", err)
		http.Error(w, explicaJogo(err), http.StatusBadGateway)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionBroadcast,
		// The message is recorded: an aviso goes to everyone at once, and "who
		// said that" is the first question when one lands badly.
		New: map[string]any{"mensagem": msg, "destinatarios": n},
	}); err != nil {
		h.cfg.Logger.Error("broadcast sent but NOT audited", "err", err)
		http.Error(w, "O aviso foi enviado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("broadcast sent", "actor", sess.AccountName, "destinatarios", n)
	h.redirectServidor(w, r, "Aviso enviado para quem está jogando.")
}

func (h *Handler) redirectServidor(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/servidor?aviso="+urlQuery(msg), http.StatusSeeOther)
}

// explicaJogo turns a link failure into something a moderator can act on. The
// three cases lead to different actions, so they must not collapse into one.
func explicaJogo(err error) string {
	switch {
	case errors.Is(err, jogo.ErrRecusado):
		return "O servidor do jogo recusou nossa senha de controle. As duas pontas estão com segredos diferentes."
	case errors.Is(err, jogo.ErrForaDoAr):
		return "O servidor do jogo não respondeu. Ele pode estar reiniciando."
	case errors.Is(err, jogo.ErrInvalido):
		return "O servidor do jogo recusou: " + err.Error()
	default:
		return "Erro ao falar com o servidor do jogo."
	}
}
