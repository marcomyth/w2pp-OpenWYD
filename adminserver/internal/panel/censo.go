package panel

import (
	"net/http"
	"strconv"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// censoLimite caps one page. The interesting rows are at the ends of the sort;
// nobody reads to the middle, where every item moved by one.
const censoLimite = 120

// censoJanelas are the comparison windows offered, in days. One day catches a
// jump; thirty catches a leak too slow to see day by day.
var censoJanelas = []int{1, 7, 30}

// censoLinha is one row as the page shows it, with the catalog name resolved.
type censoLinha struct {
	domain.ItemCensus
	Nome string // vazio quando o catálogo não está configurado ou não conhece o índice
}

// Cresceu reports whether this row went up, which is what colours it.
func (c censoLinha) Cresceu() bool { return c.Delta > 0 }

// censo shows how many units of each item exist, and what moved.
//
// The census is the only thing here that can notice a duplicate at all: an item
// has no identity — it is an index plus three effect pairs, and two identical
// ones are indistinguishable — so "is this one a copy" has no answer. "How many
// existed last week, how many exist now" does.
//
// Read-only, and there is nothing to press: the screen reports a count. What to
// do about a count that looks wrong is a decision, not a button.
func (h *Handler) censo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	dias := 7
	if n, err := strconv.Atoi(q.Get("dias")); err == nil {
		for _, permitido := range censoJanelas {
			if n == permitido {
				dias = n
			}
		}
	}
	subiu := q.Get("ordem") != "sumiu"
	refinado := q.Get("refino") == "1"

	var falha falhas
	cmp, err := h.cfg.Censo.CensusGrowth(r.Context(), store.CensusQuery{
		Dias: dias, Subiu: subiu, SoRefinado: refinado, Limit: censoLimite,
	})
	if err != nil {
		h.cfg.Logger.Error("census read failed", "dias", dias, "err", err)
		falha.nao("censo")
	}

	// Names are a convenience, not the answer: an index with no name still has
	// a count, and the catalog lives in another server that can be down.
	nomes := map[int32]string{}
	if h.cfg.GameData != nil {
		if catalogo, e := h.cfg.GameData.ItemLookup(r.Context()); e == nil {
			for idx, it := range catalogo {
				nomes[idx] = it.DisplayName
			}
		} else {
			h.cfg.Logger.Warn("census item names unavailable", "err", e)
			falha.nao("nomes")
		}
	}

	linhas := make([]censoLinha, 0, len(cmp.Linha))
	for _, c := range cmp.Linha {
		linhas = append(linhas, censoLinha{ItemCensus: c, Nome: nomes[c.Index]})
	}

	// The window actually compared, which is not always the one asked for: with
	// less history than that, CensusGrowth falls back to the oldest snapshot,
	// and a page that did not say so would report a month's growth as a day's.
	encurtado := !cmp.De.Zero() && !cmp.Ate.Zero() &&
		int(cmp.Ate.Day.Sub(cmp.De.Day).Hours()/24) < dias

	h.render(w, "censo.html", struct {
		page
		Dias      int
		Janelas   []int
		Subiu     bool
		Refinado  bool
		De        domain.CensusRun
		Ate       domain.CensusRun
		Linhas    []censoLinha
		Encurtado bool
		UmDiaSo   bool
		Limite    int
		Cheio     bool
		Falha     falhas
	}{
		h.pageFor(r, "censo"), dias, censoJanelas, subiu, refinado,
		cmp.De, cmp.Ate, linhas, encurtado,
		!cmp.Ate.Zero() && cmp.De.Day.Equal(cmp.Ate.Day),
		censoLimite, len(linhas) >= censoLimite, falha,
	})
}
