package handler

import (
	"fmt"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os blocos do Coliseu saíram do mundo (world.IsEventOwnedGenerator), mas o GM
// ainda os levanta para um evento: "gerar <bloco> aqui" não olha dono de evento.
// Que eles não voltam sozinhos depois de mortos é o world.TestColiseuMorreENaoVolta.
func TestGMGeraBlocoDoColiseu(t *testing.T) {
	gens := make([]*world.Generator, 4855)
	gens[0] = bloco("Ciclope_Forte", 50, 50, plainMobTemplate("Ciclope_Forte"))
	gens[4854] = bloco("Espectro", 52, 50, plainMobTemplate("Espectro"))
	addr, w, stop := startGenServer(t, nil, gens)
	mod := enterWorldAs(t, addr, "mod")
	defer mod.Close()

	for _, idx := range []int{0, 4854} {
		gmFrame(t, mod, fmt.Sprintf("matar bloco %d", idx))
		if _, ok := panelWith(t, mod, fmt.Sprintf("Mortos 1 do bloco #%d.", idx)); !ok {
			t.Fatalf("/gm matar bloco %d não matou", idx)
		}
		gmFrame(t, mod, fmt.Sprintf("gerar %d aqui", idx))
		if _, ok := panelWith(t, mod, fmt.Sprintf("Gerados 1 de #%d", idx)); !ok {
			t.Fatalf("/gm gerar %d aqui não gerou o bloco do Coliseu", idx)
		}
	}
	stop()
	vivos := map[int16]int{}
	w.ForEachMob(func(_ int, e *world.Entity) { vivos[e.GenIndex]++ })
	if vivos[0] != 1 || vivos[4854] != 1 {
		t.Errorf("vivos no bloco 0 = %d e no 4854 = %d, want 1 e 1", vivos[0], vivos[4854])
	}
}
