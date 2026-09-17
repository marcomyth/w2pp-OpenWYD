package panel

import (
	"fmt"
	"net/http"

	"github.com/jeanluca/w2pp-openwyd/internal/mapaevento"
)

// mapaEventoView é uma linha da tela de mapas de evento.
type mapaEventoView struct {
	Nome    string
	Area    string
	Comando string
	Nota    string
}

// mapasEvento mostra os mapas guardados para evento. A lista é a mesma que o
// tmServer usa para não gerar mob nenhum lá (internal/mapaevento), então a tela
// não lê banco nem jogo: não tem como mostrar um mapa que o jogo não limpa.
func (h *Handler) mapasEvento(w http.ResponseWriter, r *http.Request) {
	mapas := make([]mapaEventoView, 0, len(mapaevento.Todos))
	for _, m := range mapaevento.Todos {
		mapas = append(mapas, mapaEventoView{
			Nome:    m.Nome,
			Area:    fmt.Sprintf("%d, %d – %d, %d", m.X1, m.Y1, m.X2, m.Y2),
			Comando: fmt.Sprintf("/gm pos %d %d", m.EntradaX, m.EntradaY),
			Nota:    m.Nota,
		})
	}
	h.render(w, "mapas-evento.html", struct {
		page
		Mapas []mapaEventoView
	}{h.pageFor(r, "mapas-evento"), mapas})
}
