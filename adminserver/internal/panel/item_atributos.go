package panel

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
)

// atributosItem shows one item's numbers, grouped.
func (h *Handler) atributosItem(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	indice, ok := indiceDoCaminho(w, r)
	if !ok {
		return
	}

	stat, err := h.cfg.GameData.ItemStat(r.Context(), sess.AccountID, indice)
	if err != nil {
		h.recusaGameData(w, r, "carregar os atributos do item", err)
		return
	}

	porGrupo := map[string][]gamedata.MobField{}
	for _, c := range stat.Fields() {
		porGrupo[c.Grupo] = append(porGrupo[c.Grupo], c)
	}
	grupos := make([]grupoCampos, 0, len(gamedata.GruposItem()))
	for _, g := range gamedata.GruposItem() {
		grupos = append(grupos, grupoCampos{Nome: g, Campos: porGrupo[g]})
	}

	h.render(w, "item_atributos.html", struct {
		page
		Indice     int32
		Exibido    string
		Overridden bool
		Grupos     []grupoCampos
		Aviso      string
	}{
		pageFor(r, "itens", true), stat.Index(), stat.DisplayName(), stat.Overridden(),
		grupos, r.URL.Query().Get("aviso"),
	})
}

// setAtributosItem saves the override.
//
// Like the monster editor, it reads the current values first and writes onto
// them. The webServer replaces the whole override on save, so a field this form
// does not carry would be zeroed by building a fresh message from the request.
func (h *Handler) setAtributosItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	indice, ok := indiceDoCaminho(w, r)
	if !ok {
		return
	}

	stat, err := h.cfg.GameData.ItemStat(r.Context(), sess.AccountID, indice)
	if err != nil {
		h.recusaGameData(w, r, "carregar os atributos do item", err)
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
		// Set refuses anything the 16-bit column cannot hold. Saying so beats
		// storing a number that reads back as a different one.
		if !stat.Set(c.Nome, v) {
			http.Error(w, "O valor de "+c.Rotulo+" está fora do que cabe no jogo (-32768 a 32767).",
				http.StatusBadRequest)
			return
		}
		if v != c.Valor {
			mudados[c.Nome] = v
		}
	}

	if err := h.cfg.GameData.SaveItemStat(r.Context(), sess.AccountID, stat); err != nil {
		h.recusaGameData(w, r, "gravar os atributos do item", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetItemStat,
		New:    map[string]any{"item": indice, "campos": mudados},
	}); err != nil {
		h.cfg.Logger.Error("item stat changed but NOT audited", "item", indice, "err", err)
		http.Error(w, "O item foi alterado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}

	h.cfg.Logger.Info("item stat changed", "actor", sess.AccountName, "item", indice, "campos", len(mudados))
	h.redirectAtributosItem(w, r, indice,
		"Gravado. Só entra em jogo depois de reiniciar o servidor — o aviso está na página inicial.")
}

// limparAtributosItem drops the override, restoring the catalog's values.
func (h *Handler) limparAtributosItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	indice, ok := indiceDoCaminho(w, r)
	if !ok {
		return
	}

	if err := h.cfg.GameData.ClearItemStat(r.Context(), sess.AccountID, indice); err != nil {
		h.recusaGameData(w, r, "limpar os atributos do item", err)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionClearItemStat,
		Old:    map[string]any{"item": indice},
	}); err != nil {
		h.cfg.Logger.Error("item stat cleared but NOT audited", "item", indice, "err", err)
		http.Error(w, "O item foi restaurado, mas a auditoria falhou. Avise quem cuida do servidor.",
			http.StatusInternalServerError)
		return
	}
	h.redirectAtributosItem(w, r, indice,
		"Valores do catálogo restaurados. Só entra em jogo depois de reiniciar o servidor.")
}

// indiceDoCaminho reads the item index out of the path, answering 404 for
// anything that is not one — a bad index is a wrong URL, not a bad request.
func indiceDoCaminho(w http.ResponseWriter, r *http.Request) (int32, bool) {
	v, err := strconv.ParseInt(r.PathValue("indice"), 10, 32)
	if err != nil || v < 0 {
		http.NotFound(w, r)
		return 0, false
	}
	return int32(v), true
}

func (h *Handler) redirectAtributosItem(w http.ResponseWriter, r *http.Request, indice int32, msg string) {
	http.Redirect(w, r,
		"/itens/"+strconv.FormatInt(int64(indice), 10)+"/atributos?aviso="+urlQuery(msg),
		http.StatusSeeOther)
}
