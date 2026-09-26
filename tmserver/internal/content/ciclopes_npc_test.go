package content

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCiclopesDoSpotBlocos prende o spot dos Ciclopes Cruéis e dos Lanceiros
// Zakum (x 2195-2285, y 1300-1380; handler/ciclopes.go e migração 0169): nele, e
// só nele, os blocos usam as cópias Ciclope_Cruel_Spot e Lanceiro_Zakum_Spot, e os
// blocos de um bicho só viraram um grupo cheio de 3. O Ciclope Tirano está sozinho
// no bloco 6164, dentro do spot, sem período: a espera de 4 h é da fila individual.
func TestCiclopesDoSpotBlocos(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) < 6165 {
		t.Fatalf("NPCGener has %d blocks, want 6165 or more", len(gens))
	}
	spot := func(x, y int16) bool { return x >= 2195 && x <= 2285 && y >= 1300 && y <= 1380 }
	copia := map[string]bool{"Ciclope_Cruel_Spot": true, "Lanceiro_Zakum_Spot": true}
	original := map[string]bool{"Ciclope_Cruel": true, "Lanceiro_Zakum": true}

	g := gens[6164]
	if g.Leader != "Ciclope_Tirano" || g.Follower != "Ciclope_Tirano" || g.MinuteGenerate != -1 ||
		g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco 6164 = %+v, want um Ciclope_Tirano sozinho, sem período", g)
	}
	if !spot(g.SegX[0], g.SegY[0]) {
		t.Errorf("Ciclope_Tirano nasce em (%d,%d), fora do spot", g.SegX[0], g.SegY[0])
	}

	var comCopia, triplos int
	for i, g := range gens {
		if g.Leader == "Ciclope_Tirano" && i != 6164 {
			t.Errorf("Ciclope_Tirano também no bloco %d", i)
		}
		dentro := spot(g.SegX[0], g.SegY[0])
		if copia[g.Leader] || copia[g.Follower] {
			comCopia++
			if !dentro {
				t.Errorf("bloco %d: %s/%s fora do spot em (%d,%d); a Mesa da 0169 vale por template", i, g.Leader, g.Follower, g.SegX[0], g.SegY[0])
			}
			if g.Leader == g.Follower {
				triplos++
				if g.MaxNumMob != 3 || g.MinGroup != 2 || g.MaxGroup != 2 {
					t.Errorf("bloco %d = %+v, want um grupo cheio de 3 %s", i, g, g.Leader)
				}
			}
		}
		if dentro && (original[g.Leader] || original[g.Follower]) {
			t.Errorf("bloco %d: %s/%s original dentro do spot", i, g.Leader, g.Follower)
		}
	}
	// 20 Ciclopes e 11 Lanceiros sozinhos; 8 Ciclopes que lideram Anciões.
	if comCopia != 39 || triplos != 31 {
		t.Errorf("%d blocos com cópia (%d triplos), want 39 (31 triplos)", comCopia, triplos)
	}
}
