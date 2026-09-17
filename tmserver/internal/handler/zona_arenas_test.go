package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
)

// TestZonaDasArenasCobreAsCincoAreas: a zona de XP das arenas
// (level.ZoneArenas) é exatamente o chão que o guarda das arenas guarda. Cada
// tile de cada área de quest256Steps cai na zona, e nenhum tile da borda ou de
// fora dela cai. Se alguém mexer numa área sem mexer no retângulo da zona, a XP
// passa a ser paga pela tabela errada lá dentro, e este teste acusa.
func TestZonaDasArenasCobreAsCincoAreas(t *testing.T) {
	for i, step := range quest256Steps {
		a := step.area
		dentro := 0
		for x := a.x1 - 1; x <= a.x2+1; x++ {
			for y := a.y1 - 1; y <= a.y2+1; y++ {
				z := level.ZoneForTile(int32(x), int32(y))
				switch {
				case a.contains(x, y) && z != level.ZoneArenas:
					t.Errorf("arena %d: %d,%d está dentro e caiu em %s", i, x, y, z.Name())
				case !a.contains(x, y) && z == level.ZoneArenas:
					t.Errorf("arena %d: %d,%d está fora e caiu nas arenas", i, x, y)
				}
				if a.contains(x, y) {
					dentro++
				}
			}
		}
		if dentro == 0 {
			t.Errorf("arena %d: área vazia, o teste não conferiu nada", i)
		}
		if z := level.ZoneForKill(int32(step.x), int32(step.y), int32(step.x)+1, int32(step.y)); z != level.ZoneArenas {
			t.Errorf("arena %d: morte com os dois dentro = %s, quero as arenas", i, z.Name())
		}
	}
}
