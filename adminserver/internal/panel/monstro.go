package panel

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
)

// monstros lists the template files a moderator can rebalance.
func (h *Handler) monstros(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	q := r.URL.Query().Get("q")

	achados, err := h.cfg.GameData.MobTemplates(r.Context(), sess.AccountID, q)
	if err != nil {
		h.recusaGameData(w, r, "listar os monstros", err)
		return
	}
	truncado := len(achados) > itensLimit
	if truncado {
		achados = achados[:itensLimit]
	}

	h.render(w, "monstros.html", struct {
		page
		Query     string
		Templates []gamedata.MobTemplate
		Truncado  bool
		Limite    int
		Aviso     string
	}{pageFor(r, "monstros", true), q, achados, truncado, itensLimit, r.URL.Query().Get("aviso")})
}

// monstro shows one template's numbers, grouped.
func (h *Handler) monstro(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	stat, err := h.cfg.GameData.MobStat(r.Context(), sess.AccountID, nome)
	if err != nil {
		h.recusaGameData(w, r, "carregar o monstro", err)
		return
	}

	// Group the fields for rendering: one section per group, in table order.
	campos := stat.Fields()
	porGrupo := map[string][]gamedata.MobField{}
	for _, c := range campos {
		porGrupo[c.Grupo] = append(porGrupo[c.Grupo], c)
	}
	grupos := make([]grupoCampos, 0, len(gamedata.GruposMob()))
	for _, g := range gamedata.GruposMob() {
		grupos = append(grupos, grupoCampos{Nome: g, Campos: porGrupo[g]})
	}

	h.render(w, "monstro.html", struct {
		page
		Nome       string
		Exibido    string
		Overridden bool
		Grupos     []grupoCampos
		Aviso      string
	}{
		pageFor(r, "monstros", true), stat.Name(), stat.DisplayName(), stat.Overridden(),
		grupos, r.URL.Query().Get("aviso"),
	})
}

// grupoCampos is one titled section of the stat form.
type grupoCampos struct {
	Nome   string
	Campos []gamedata.MobField
}

// setMonstro saves the override.
//
// It reads the current values first and writes onto them. Upsert replaces the
// whole row, so a field the form does not carry — the equipment list above all,
// which has its own RPC and no place in this form — would be zeroed by building
// a fresh message from the request.
func (h *Handler) setMonstro(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	stat, err := h.cfg.GameData.MobStat(r.Context(), sess.AccountID, nome)
	if err != nil {
		h.recusaGameData(w, r, "carregar o monstro", err)
		return
	}

	mudados := map[string]any{}
	for _, c := range stat.Fields() {
		bruto := strings.TrimSpace(r.PostFormValue(c.Nome))
		if bruto == "" {
			continue // field absent from the form: leave it as it was
		}
		v, err := strconv.ParseInt(bruto, 10, 64)
		if err != nil {
			http.Error(w, "Valor inválido em "+c.Rotulo+".", http.StatusBadRequest)
			return
		}
		if v != c.Valor {
			mudados[c.Nome] = v
		}
		stat.Set(c.Nome, v)
	}
	// Present-but-empty clears the name; absent leaves it alone. Without the
	// Has check any partial post would silently wipe it, which is the opposite
	// of how every number above behaves.
	if r.PostForm.Has("display_name") {
		exibido := strings.TrimSpace(r.PostFormValue("display_name"))
		if exibido != stat.DisplayName() {
			mudados["display_name"] = exibido
		}
		stat.SetDisplayName(exibido)
	}

	if err := h.cfg.GameData.SaveMobStat(r.Context(), sess.AccountID, stat); err != nil {
		h.recusaGameData(w, r, "gravar o monstro", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetMobStat,
		New:    map[string]any{"template": nome, "campos": mudados},
	}); err != nil {
		h.cfg.Logger.Error("mob stat changed but NOT audited", "template", nome, "err", err)
		http.Error(w, "O monstro foi alterado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("mob stat changed", "actor", sess.AccountName, "template", nome, "campos", len(mudados))
	h.redirectMonstro(w, r, nome,
		"Gravado. Só entra em jogo depois de reiniciar o servidor — o aviso está na página inicial.")
}

// limparMonstro drops the override, restoring the template file's values.
func (h *Handler) limparMonstro(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	nome := r.PathValue("nome")

	if err := h.cfg.GameData.ClearMobStat(r.Context(), sess.AccountID, nome); err != nil {
		h.recusaGameData(w, r, "limpar o monstro", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionClearMobStat,
		Old:    map[string]any{"template": nome},
	}); err != nil {
		h.cfg.Logger.Error("mob stat cleared but NOT audited", "template", nome, "err", err)
		http.Error(w, "O monstro foi restaurado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	h.redirectMonstro(w, r, nome,
		"Valores do arquivo restaurados. Só entra em jogo depois de reiniciar o servidor.")
}

func (h *Handler) redirectMonstro(w http.ResponseWriter, r *http.Request, nome, msg string) {
	http.Redirect(w, r, "/monstros/"+urlPath(nome)+"?aviso="+urlQuery(msg), http.StatusSeeOther)
}
