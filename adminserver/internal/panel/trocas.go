package panel

import (
	"net/http"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// trocasLimit caps one page. A busy server writes a lot of these and nobody
// reads past the first screen of a scam report.
const trocasLimit = 100

// trocas shows player-to-player trades, newest first, filtered by character.
//
// Read-only, and deliberately so: a trade already happened in the world. The
// screen answers "what changed hands", not "put it back".
func (h *Handler) trocas(w http.ResponseWriter, r *http.Request) {
	nome := strings.TrimSpace(r.URL.Query().Get("personagem"))

	achadas, err := h.cfg.Trocas.ListTrades(r.Context(), store.TradeQuery{
		Char: nome, Limit: trocasLimit,
	})
	if err != nil {
		h.cfg.Logger.Error("trade list failed", "personagem", nome, "err", err)
		http.Error(w, "Erro ao carregar as trocas.", http.StatusInternalServerError)
		return
	}

	h.render(w, "trocas.html", struct {
		page
		Personagem string
		Trocas     []domain.TradeRecord
		Limite     int
		Cheio      bool
	}{h.pageFor(r, "trocas"), nome, achadas, trocasLimit, len(achadas) >= trocasLimit})
}
