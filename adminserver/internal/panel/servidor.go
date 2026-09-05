package panel

import (
	"errors"
	"fmt"
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

	// The restart card lives here as well as on the home page. This tab is where
	// anyone looks for a server control, and having the only restart button on
	// Início meant the obvious place had kick and broadcast but no way to
	// restart — which reads as the feature being missing.
	h.render(w, "servidor.html", struct {
		page
		Estado   jogo.Estado
		Servidor estadoServidor
		Erro     string
		Aviso    string
	}{h.pageFor(r, "servidor"), estado, h.statusServidor(r), erro, r.URL.Query().Get("aviso")})
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

// avisoPadraoReinicio is what players are told when the operator writes nothing.
const avisoPadraoReinicio = "O servidor vai reiniciar agora. Voce volta em cerca de um minuto."

// reinicioSeguro empties the server, waits for every save, and only then
// restarts.
//
// The plain restart is not unsafe by accident — it saves everyone on the way
// out. But it does that inside the window the platform allows between asking the
// process to stop and killing it, and the saves are one gRPC call per player. A
// full server and a slow database can run past it, and what has not been written
// is lost: items, experience, gold since login.
//
// Emptying first does the same work with no clock attached. The shutdown that
// follows then finds nobody in play.
//
// If the drain fails, this does NOT restart. The sessions are already gone by
// then, so restarting would drop exactly what the drain was waiting to save.
func (h *Handler) reinicioSeguro(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())

	aviso := strings.TrimSpace(r.PostFormValue("aviso"))
	if aviso == "" {
		aviso = avisoPadraoReinicio
	}

	dep, err := h.cfg.Platform.Latest(r.Context())
	if err != nil {
		h.cfg.Logger.Error("safe restart: platform unavailable", "err", err)
		http.Error(w, "Não consegui falar com a hospedagem, então não mexi no servidor.",
			http.StatusBadGateway)
		return
	}

	dren, err := h.cfg.Jogo.Drenar(r.Context(), aviso)
	if err != nil {
		h.cfg.Logger.Error("safe restart: drain failed; NOT restarting", "err", err)
		http.Error(w,
			"Esvaziei o servidor mas as gravações não confirmaram, então NÃO reiniciei. "+
				"Espere um minuto e veja a aba Servidor antes de tentar de novo. Detalhe: "+explicaJogo(err),
			http.StatusBadGateway)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSafeRestart,
		New: map[string]any{
			"deployment": dep.ID, "avisados": dren.Avisados, "derrubados": dren.Derrubados,
		},
	}); err != nil {
		// The players are already out and saved. Refusing to restart now leaves
		// an empty server up, which is recoverable; restarting unaudited is not.
		h.cfg.Logger.Error("safe restart drained but NOT audited; refusing to restart", "err", err)
		http.Error(w,
			"Os jogadores foram salvos e desconectados, mas a auditoria falhou, então não reiniciei. "+
				"O servidor está de pé e vazio.", http.StatusInternalServerError)
		return
	}

	if err := h.cfg.Platform.Restart(r.Context(), dep.ID); err != nil {
		h.cfg.Logger.Error("safe restart: platform refused", "deployment", dep.ID, "err", err)
		http.Error(w,
			"Os jogadores foram salvos e desconectados, mas a hospedagem recusou o reinício. "+
				"Ninguém perdeu nada; o servidor está vazio.", http.StatusBadGateway)
		return
	}

	h.cfg.Logger.Info("safe restart", "actor", sess.AccountName,
		"deployment", dep.ID, "avisados", dren.Avisados, "derrubados", dren.Derrubados)
	h.redirectServidor(w, r, fmt.Sprintf(
		"Reinício seguro: %d sessão(ões) salva(s) e encerrada(s) antes de o servidor sair. Volta em cerca de um minuto.",
		dren.Derrubados))
}
