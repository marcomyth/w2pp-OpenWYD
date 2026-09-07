package panel

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// chatJanelas are the periods offered, in days. Zero means "everything that is
// still kept", which is bounded by the retention anyway.
var chatJanelas = []int{1, 7, 0}

// chatLinha is one line as the page shows it.
type chatLinha struct {
	domain.ChatLinha
	Idade string
}

// Sussurro reports whether this was a private message rather than public speech.
func (c chatLinha) Sussurro() bool { return c.Tipo == domain.ChatSussurro }

// chat shows what was said (0034_chat_log).
//
// It exists for one question, and it is the one support gets every day:
// somebody opens a ticket saying they were insulted, threatened or talked into
// a scam. The report queue keeps the moment and who was nearby; it did not keep
// what was said.
//
// EVERY SEARCH IS AUDITED, including the ones that find nothing. This is
// people's private conversation, and the check on that is not a permission — a
// moderator has to be able to read it, or reports cannot be worked — but a
// record of who read whose messages. The audit write is not best-effort: a read
// that could not be recorded does not happen.
func (h *Handler) chat(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	q := r.URL.Query()

	nome := strings.TrimSpace(q.Get("personagem"))
	texto := strings.TrimSpace(q.Get("texto"))
	tipo := domain.ChatTipo(strings.TrimSpace(q.Get("tipo")))
	if tipo != domain.ChatPublico && tipo != domain.ChatSussurro {
		tipo = ""
	}
	dias := 7
	if n, err := strconv.Atoi(q.Get("dias")); err == nil {
		for _, permitido := range chatJanelas {
			if n == permitido {
				dias = n
			}
		}
	}

	// Nothing asked, nothing read. The page opens on an empty search on purpose:
	// browsing everybody's conversation is not a thing this screen should make
	// easy, and an audit entry for merely opening the menu would bury the
	// entries that mean something.
	buscou := nome != "" || texto != ""
	pag := paginaDe(r, "pagina")

	var falha falhas
	var linhas []chatLinha
	agora := time.Now()

	if buscou {
		if err := h.cfg.Audit.Write(r.Context(), audit.Record{
			ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
			Action: "chat.ler",
			New: map[string]any{
				"personagem": nome, "texto": texto,
				"tipo": string(tipo), "dias": dias,
			},
		}); err != nil {
			// Refused, not degraded: reading private messages without leaving a
			// trace is the one outcome this page must not produce.
			h.cfg.Logger.Error("chat read not audited; refusing", "actor", sess.AccountName, "err", err)
			http.Error(w, "Não consegui registrar esta consulta na auditoria, "+
				"e leitura de conversa privada só acontece com registro. Tente de novo.",
				http.StatusServiceUnavailable)
			return
		}

		cq := store.ChatQuery{
			Char: nome, Texto: texto, Tipo: tipo,
			Limit: pag.Pedir(), Offset: pag.Offset(),
		}
		if dias > 0 {
			cq.Desde = agora.AddDate(0, 0, -dias)
		}
		achadas, err := h.cfg.Chat.ListChat(r.Context(), cq)
		if err != nil {
			h.cfg.Logger.Error("chat list failed", "personagem", nome, "err", err)
			falha.nao("chat")
		}
		achadas = Corta(&pag, achadas)
		linhas = make([]chatLinha, 0, len(achadas))
		for _, l := range achadas {
			linhas = append(linhas, chatLinha{ChatLinha: l, Idade: idade(l.At, agora)})
		}
	}

	// The retention the sweep is actually applying, read from the database
	// rather than from this process's own configuration: the number lives in the
	// dbServer's environment, and a screen announcing one nobody enforces would
	// be worse than one that says nothing.
	varredura, err := h.cfg.Chat.ChatSweep(r.Context())
	if err != nil {
		h.cfg.Logger.Error("chat sweep read failed", "err", err)
		falha.nao("prazo")
	}

	h.render(w, "chat.html", struct {
		page
		Personagem string
		Texto      string
		Tipo       string
		Dias       int
		Janelas    []int
		Buscou     bool
		Linhas     []chatLinha
		Varredura  domain.ChatVarredura
		VarridoHa  string
		Pagina     pagina
		Extras     url.Values
		Falha      falhas
	}{
		h.pageFor(r, "chat"), nome, texto, string(tipo), dias, chatJanelas,
		buscou, linhas, varredura, idade(varredura.VarridoEm, agora),
		pag, r.URL.Query(), falha,
	})
}
