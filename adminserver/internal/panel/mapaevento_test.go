package panel

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/mapaevento"
)

// A tela mostra cada mapa da lista com o comando para chegar lá, e existe — com
// entrada no menu — mesmo num painel sem banco e sem jogo.
func TestMapasEventoMostraALista(t *testing.T) {
	get := signedIn(t, newTestPanel(t, withTarget(roleModerator)))
	rec := get("/mapas-evento")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	corpo := rec.Body.String()
	for _, m := range mapaevento.Todos {
		if !strings.Contains(corpo, m.Nome) {
			t.Errorf("a tela não mostra %q", m.Nome)
		}
	}
	for _, trecho := range []string{"/gm pos 1085 1467", "/gm pos 321 309", `href="/mapas-evento"`} {
		if !strings.Contains(corpo, trecho) {
			t.Errorf("a tela não tem %q", trecho)
		}
	}
}
