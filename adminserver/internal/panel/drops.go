package panel

import (
	"net/http"
	"net/url"
	"sort"

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
			Ordem     ordem
			Extras    url.Values
		}{h.pageFor(r, "drops"), "", "", nil, false, dropsLimit, false,
			ordem{}, r.URL.Query()})
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

	// Sorting applies inside EACH item's mob list, not across them: the page is
	// one table per item, and the question people bring here is "which of these
	// monsters is the easiest source", asked one item at a time.
	//
	// Chance ascending puts the best odds on top, because Divisor is "one in N"
	// — a smaller divisor is a better drop, and sorting the raw number the other
	// way would put the rarest source first for anyone who did not read this.
	o := ordemDe(r, "monstro", "nivel", "chance")
	for _, d := range achados {
		switch o.Por {
		case "monstro":
			sort.SliceStable(d.Mobs, o.Menor(func(i, j int) bool {
				return nomeDeMob(d.Mobs[i]) < nomeDeMob(d.Mobs[j])
			}))
		case "nivel":
			sort.SliceStable(d.Mobs, o.Menor(func(i, j int) bool {
				return d.Mobs[i].MobLevel < d.Mobs[j].MobLevel
			}))
		case "chance":
			sort.SliceStable(d.Mobs, o.Menor(func(i, j int) bool {
				return d.Mobs[i].Divisor < d.Mobs[j].Divisor
			}))
		}
	}

	h.render(w, "drops.html", struct {
		page
		Item, Mob string
		Itens     []gamedata.Drop
		Truncado  bool
		Limite    int
		Pediu     bool
		Ordem     ordem
		Extras    url.Values
	}{h.pageFor(r, "drops"), item, mob, achados, truncado, dropsLimit, true,
		o, r.URL.Query()})
}

// nomeDeMob is what the row shows: the readable name when the catalog has one,
// and the template name when it does not — so sorting by the column orders what
// the reader actually sees rather than a name half the rows do not display.
func nomeDeMob(m gamedata.DropMob) string {
	if m.MobName != "" {
		return m.MobName
	}
	return m.TemplateName
}
