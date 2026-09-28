package content

import (
	"os"
	"path/filepath"
	"testing"
)

// O Taron Tirano está sozinho no bloco 6165, no meio dos Taron Assassino do
// Deserto Baixo, sem período: a espera de 4 h é da fila individual
// (handler/taron_tirano.go).
func TestTaronTiranoBloco(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	const bloco = 6165
	if len(gens) <= bloco {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), bloco)
	}
	g := gens[bloco]
	if g.Leader != "Taron_Tirano" || g.Follower != "Taron_Tirano" || g.MinuteGenerate != -1 ||
		g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want um Taron_Tirano sozinho, sem período", bloco, g)
	}
	// Dentro da faixa dos Taron Assassino do Deserto Baixo.
	if x, y := g.SegX[0], g.SegY[0]; x < 1410 || x > 1512 || y < 1686 || y > 1768 {
		t.Errorf("Taron_Tirano nasce em (%d,%d), fora dos Taron Assassino", x, y)
	}
	for i, g := range gens {
		if (g.Leader == "Taron_Tirano" || g.Follower == "Taron_Tirano") && i != bloco {
			t.Errorf("Taron_Tirano também no bloco %d", i)
		}
	}
}
