package panel

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// limiteDenuncias bounds one page of the queue.
const limiteDenuncias = 100

// esperaHa turns a timestamp into how long somebody has been waiting.
//
// A queue page that prints timestamps makes the reader do the subtraction, and
// the number they actually want is the age: "há 3 horas" is the thing that says
// whether the queue is being worked.
func esperaHa(quando time.Time, agora time.Time) string {
	if quando.IsZero() {
		return ""
	}
	d := agora.Sub(quando)
	switch {
	case d < time.Minute:
		return "agora há pouco"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " min"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + " d"
	}
}

// denunciaView is one report as the page shows it.
type denunciaView struct {
	domain.PlayerReport
	Espera string
	Regiao string
}

// denuncias shows the report queue.
func (h *Handler) denuncias(w http.ResponseWriter, r *http.Request) {
	// The queue opens on what is still open. Everything is one click away, but
	// the page exists to answer "what needs me now".
	todas := r.URL.Query().Get("todas") != ""

	rs, err := h.cfg.Denuncias.ListReports(r.Context(), store.ReportQuery{
		SoAbertos: !todas, Limit: limiteDenuncias,
	})
	if err != nil {
		h.cfg.Logger.Error("report list failed", "err", err)
		http.Error(w, "Erro ao ler as denúncias.", http.StatusInternalServerError)
		return
	}
	contagem, err := h.cfg.Denuncias.CountReports(r.Context())
	if err != nil {
		// The list is the page; losing the summary must not blank it.
		h.cfg.Logger.Error("report count failed", "err", err)
		contagem = store.ReportCounts{}
	}

	agora := time.Now()
	vistas := make([]denunciaView, 0, len(rs))
	for _, d := range rs {
		vistas = append(vistas, denunciaView{
			PlayerReport: d,
			Espera:       esperaHa(d.At, agora),
			Regiao:       regiao(d.X, d.Y),
		})
	}

	h.render(w, "denuncias.html", struct {
		page
		Denuncias []denunciaView
		Contagem  store.ReportCounts
		EsperaMax string
		Todas     bool
		Prazo     int
		Aviso     string
	}{
		h.pageFor(r, "denuncias"), vistas, contagem,
		esperaHa(contagem.MaisAntigo, agora), todas,
		domain.ReportRetentionDays, r.URL.Query().Get("aviso"),
	})
}

// tratarDenuncia closes one report.
//
// Staff, not admin: reading and answering reports is the job, and a queue only
// the admin can clear is a queue nobody clears.
func (h *Handler) tratarDenuncia(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("denuncia"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	if err := h.cfg.Denuncias.MarkReportHandled(r.Context(), id, sess.AccountID); err != nil {
		h.cfg.Logger.Error("mark report handled failed", "denuncia", id, "err", err)
		http.Error(w, "Erro ao marcar a denúncia.", http.StatusInternalServerError)
		return
	}

	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionHandleReport,
		New:    map[string]any{"denuncia": id},
	}); err != nil {
		h.cfg.Logger.Error("report handled but NOT audited", "denuncia", id, "err", err)
		http.Error(w, "A denúncia foi marcada, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("report handled", "actor", sess.AccountName, "denuncia", id)
	destino := "/denuncias?aviso=" + urlQuery("Marcada como tratada.")
	if r.PostFormValue("todas") != "" {
		destino = "/denuncias?todas=1&aviso=" + urlQuery("Marcada como tratada.")
	}
	http.Redirect(w, r, destino, http.StatusSeeOther)
}
