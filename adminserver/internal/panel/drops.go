package panel

import (
	"net/http"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/gamedata"
)

// dropsLimit caps how many items one page renders. The report covers the whole
// catalog crossed with every mob template, so an unfiltered request is tens of
// thousands of rows — enough to make the page unusable and the request slow.
const dropsLimit = 60

// drops shows which mobs drop an item, at what slot, and how likely it is.
//
// The screen is read-only on purpose. Drop tables live in the mob template
// files, which are mounted read-only in production; changing one is a content
// change, not a config change, and there is no override table behind it the way
// there is for stats and prices.
func (h *Handler) drops(w http.ResponseWriter, r *http.Request) {
	sess, _ := staffFrom(r.Context())
	item := r.URL.Query().Get("item")
	mob := r.URL.Query().Get("mob")

	// An empty search would fetch the whole cross product and render a page
	// nobody can read. Asking for a term first is cheaper than truncating it.
	if item == "" && mob == "" {
		h.render(w, "drops.html", struct {
			page
			Item, Mob string
			Itens     []gamedata.Drop
			Truncado  bool
			Limite    int
			Pediu     bool
		}{pageFor(r, "drops", true), "", "", nil, false, dropsLimit, false})
		return
	}

	achados, err := h.cfg.GameData.Drops(r.Context(), sess.AccountID, item, mob)
	if err != nil {
		h.recusaGameData(w, r, "listar os drops", err)
		return
	}
	truncado := len(achados) > dropsLimit
	if truncado {
		achados = achados[:dropsLimit]
	}

	h.render(w, "drops.html", struct {
		page
		Item, Mob string
		Itens     []gamedata.Drop
		Truncado  bool
		Limite    int
		Pediu     bool
	}{pageFor(r, "drops", true), item, mob, achados, truncado, dropsLimit, true})
}
