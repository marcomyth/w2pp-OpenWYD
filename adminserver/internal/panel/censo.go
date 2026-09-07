package panel

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// censoJanelas are the comparison windows offered, in days. One day catches a
// jump; thirty catches a leak too slow to see day by day.
var censoJanelas = []int{1, 7, 30}

// copiaGrupo is one serial and every item row carrying it. Two rows is a copy;
// three is a copy that was copied.
type copiaGrupo struct {
	Serial int64
	Index  int16
	Nome   string
	Copias []censoCopia
}

// censoCopia is where one copy sits, in the words the panel uses.
type censoCopia struct {
	Conta      string
	Personagem string
	Onde       string
}

// ondeItem translates owner_kind. The database words are the ones the game uses
// internally; a moderator reads "baú".
func ondeItem(kind string) string {
	switch kind {
	case "char_equip":
		return "equipado"
	case "char_carry":
		return "mochila"
	case "account_cargo":
		return "baú"
	}
	return kind
}

// agrupaCopias turns the flat rows into one entry per serial. They arrive
// ordered by serial, so a single pass is enough.
func agrupaCopias(rows []domain.ItemDup, nomes map[int32]string) []copiaGrupo {
	var out []copiaGrupo
	for _, r := range rows {
		c := censoCopia{Conta: r.Account, Personagem: r.Character, Onde: ondeItem(r.Onde)}
		if n := len(out); n > 0 && out[n-1].Serial == r.Serial {
			out[n-1].Copias = append(out[n-1].Copias, c)
			continue
		}
		out = append(out, copiaGrupo{
			Serial: r.Serial, Index: r.Index, Nome: nomes[int32(r.Index)],
			Copias: []censoCopia{c},
		})
	}
	return out
}

// censoLinha is one row as the page shows it, with the catalog name resolved.
type censoLinha struct {
	domain.ItemCensus
	Nome string // vazio quando o catálogo não está configurado ou não conhece o índice
}

// Cresceu reports whether this row went up, which is what colours it.
func (c censoLinha) Cresceu() bool { return c.Delta > 0 }

// copiasLimite caps the duplicate list. If there are more than this many, the
// problem is not one scammer and the list is not the tool for it.
const copiasLimite = 100

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

	pag := paginaDe(r, "pagina")

	var falha falhas
	cmp, err := h.cfg.Censo.CensusGrowth(r.Context(), store.CensusQuery{
		Dias: dias, Subiu: subiu, SoRefinado: refinado,
		Limit: pag.Pedir(), Offset: pag.Offset(),
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

	// The duplicates come after the names because they are rendered with them.
	var copias []copiaGrupo
	var marcados, semMarca int64
	if brutas, e := h.cfg.Censo.ListDupes(r.Context(), copiasLimite); e != nil {
		h.cfg.Logger.Error("dupe list failed", "err", e)
		falha.nao("copias")
	} else {
		copias = agrupaCopias(brutas, nomes)
		if marcados, semMarca, e = h.cfg.Censo.CountMarked(r.Context()); e != nil {
			h.cfg.Logger.Error("marked count failed", "err", e)
			falha.nao("copias")
		}
	}

	cmp.Linha = Corta(&pag, cmp.Linha)
	linhas := make([]censoLinha, 0, len(cmp.Linha))
	for _, c := range cmp.Linha {
		linhas = append(linhas, censoLinha{ItemCensus: c, Nome: nomes[c.Index]})
	}

	// The default is the variation, which is why the page exists — the store
	// already returned it in that order, so no sort at all is the fastest path
	// to the answer people come for. The columns are for the follow-up
	// questions: "how many of this exist in total", "show me the refined ones".
	o := ordemDe(r, "item", "refino", "variacao", "agora")
	switch o.Por {
	case "item":
		sort.SliceStable(linhas, o.Menor(func(i, j int) bool {
			if linhas[i].Nome != linhas[j].Nome {
				return linhas[i].Nome < linhas[j].Nome
			}
			return linhas[i].Index < linhas[j].Index
		}))
	case "refino":
		sort.SliceStable(linhas, o.Menor(func(i, j int) bool { return linhas[i].Sanc < linhas[j].Sanc }))
	case "variacao":
		sort.SliceStable(linhas, o.Menor(func(i, j int) bool { return linhas[i].Delta < linhas[j].Delta }))
	case "agora":
		sort.SliceStable(linhas, o.Menor(func(i, j int) bool { return linhas[i].Units < linhas[j].Units }))
	}

	// The window actually compared, which is not always the one asked for: with
	// less history than that, CensusGrowth falls back to the oldest snapshot,
	// and a page that did not say so would report a month's growth as a day's.
	encurtado := !cmp.De.Zero() && !cmp.Ate.Zero() &&
		int(cmp.Ate.Day.Sub(cmp.De.Day).Hours()/24) < dias

	h.render(w, "censo.html", struct {
		page
		Copias    []copiaGrupo
		Marcados  int64
		SemMarca  int64
		Dias      int
		Janelas   []int
		Subiu     bool
		Refinado  bool
		De        domain.CensusRun
		Ate       domain.CensusRun
		Linhas    []censoLinha
		Encurtado bool
		UmDiaSo   bool
		Pagina    pagina
		Ordem     ordem
		Extras    url.Values
		Falha     falhas
	}{
		h.pageFor(r, "censo"), copias, marcados, semMarca,
		dias, censoJanelas, subiu, refinado,
		cmp.De, cmp.Ate, linhas, encurtado,
		!cmp.Ate.Zero() && cmp.De.Day.Equal(cmp.Ate.Day),
		pag, o, r.URL.Query(), falha,
	})
}
