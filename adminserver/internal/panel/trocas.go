package panel

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// trocasLimit caps one page. A busy server writes a lot of these and nobody
// reads past the first screen of a scam report.
const trocasLimit = 100

// chaoLimit is higher than trocasLimit because dropping things is constant:
// every fight ends with somebody discarding loot, so the same hundred rows
// cover far less time on the floor than in the trade window.
const chaoLimit = 200

// chaoParMax is how long after a drop a pickup still counts as the same event.
//
// The floor slot is reused, so a "pegou" on slot 7 an hour after a "largou" on
// slot 7 is almost certainly a different item. Ten minutes is long enough for
// somebody to walk over and take it, short enough that the pairing stays a fact
// instead of a guess.
const chaoParMax = 10 * time.Minute

// chaoLinha is one floor row as the page shows it, with the other half of the
// pair filled in when there is one.
type chaoLinha struct {
	domain.GroundEvent
	// De is who dropped it, on a pickup that pairs with a drop. Empty when the
	// drop is not in this page (it decayed, or fell outside the window).
	De string
	// Levou is how long the item sat there before somebody took it.
	Levou time.Duration
	// TrocouMao is the case worth reading: two different names on the pair, so
	// the item went from one player to another with the floor in between.
	TrocouMao bool
}

// Espera renders Levou for the table.
func (c chaoLinha) Espera() string {
	switch {
	case c.Levou <= 0:
		return ""
	case c.Levou < time.Minute:
		return fmt.Sprintf("%ds depois", int(c.Levou.Seconds()))
	default:
		return fmt.Sprintf("%dmin depois", int(c.Levou.Minutes()))
	}
}

// Largou reports whether this row is the drop half.
func (c chaoLinha) Largou() bool { return c.Acao == domain.GroundLargou }

// emparelhaChao resolves each pickup against the drop that preceded it on the
// same floor slot.
//
// Rows arrive newest first, so the drop for a pickup at i sits somewhere after
// i. Scanning forward for the first drop on the same slot is the whole
// algorithm; the list is one page long, so the quadratic worst case is a few
// thousand comparisons.
func emparelhaChao(rows []domain.GroundEvent) []chaoLinha {
	out := make([]chaoLinha, 0, len(rows))
	for i, r := range rows {
		l := chaoLinha{GroundEvent: r}
		if r.Acao == domain.GroundPegou {
			for j := i + 1; j < len(rows); j++ {
				d := rows[j]
				if d.GroundID != r.GroundID || d.Acao != domain.GroundLargou {
					continue
				}
				espera := r.At.Sub(d.At)
				if espera < 0 || espera > chaoParMax {
					break // the slot was reused; this is a different item
				}
				l.De, l.Levou = d.Character, espera
				l.TrocouMao = !strings.EqualFold(d.Character, r.Character)
				break
			}
		}
		out = append(out, l)
	}
	return out
}

// soMaos keeps only the rows where an item went from one player to another.
func soMaos(linhas []chaoLinha) []chaoLinha {
	var out []chaoLinha
	for _, l := range linhas {
		if l.TrocouMao {
			out = append(out, l)
		}
	}
	return out
}

// trocas shows what changed hands, newest first, filtered by character: the
// trade window on top, the floor below.
//
// Read-only, and deliberately so: it already happened in the world. The screen
// answers "what changed hands", not "put it back".
func (h *Handler) trocas(w http.ResponseWriter, r *http.Request) {
	nome := strings.TrimSpace(r.URL.Query().Get("personagem"))
	maos := r.URL.Query().Get("maos") == "1"

	var falha falhas

	achadas, err := h.cfg.Trocas.ListTrades(r.Context(), store.TradeQuery{
		Char: nome, Limit: trocasLimit,
	})
	if err != nil {
		h.cfg.Logger.Error("trade list failed", "personagem", nome, "err", err)
		falha.nao("trocas")
	}

	brutas, err := h.cfg.Trocas.ListGround(r.Context(), store.GroundQuery{
		Char: nome, Limit: chaoLimit,
	})
	if err != nil {
		h.cfg.Logger.Error("ground list failed", "personagem", nome, "err", err)
		falha.nao("chao")
	}
	// Pairing runs on the full page, then the filter: a hand-off is only
	// visible once both halves are matched, so filtering first would hide it.
	chao := emparelhaChao(brutas)
	total := len(chao)
	if maos {
		chao = soMaos(chao)
	}

	h.render(w, "trocas.html", struct {
		page
		Personagem string
		Trocas     []domain.TradeRecord
		Chao       []chaoLinha
		ChaoTotal  int
		SoMaos     bool
		Limite     int
		ChaoLimite int
		Cheio      bool
		ChaoCheio  bool
		Falha      falhas
	}{
		h.pageFor(r, "trocas"), nome, achadas, chao, total, maos,
		trocasLimit, chaoLimit,
		len(achadas) >= trocasLimit, total >= chaoLimit, falha,
	})
}
