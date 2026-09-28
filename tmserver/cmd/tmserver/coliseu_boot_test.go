package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestSpawnNPCsNaoPopulaOColiseu: os 36 blocos de monstro do Coliseu (as ondas 0-2
// e 5-7, os Espectros 4854-4874 e os dez chefes sozinhos) não nascem no boot. O
// Guarda_Carga, a Prona e os vizinhos de número, com a mesma receita -1, continuam
// nascendo: o boot dos blocos -1 é divergência deliberada e fica.
//
// Os blocos fora da conta têm um Leader sem molde; o boot os pula e o número de
// cada bloco continua sendo a posição no arquivo.
func TestSpawnNPCsNaoPopulaOColiseu(t *testing.T) {
	coliseu := []int{0, 1, 2, 5, 6, 7,
		4854, 4855, 4856, 4857, 4858, 4859, 4860, 4861, 4862, 4863,
		4865, 4866, 4867, 4868, 4869, 4870, 4871, 4872, 4873, 4874,
		103, 104, 105, 106, 4853, 4864, 4885, 4886, 4887, 4888} // chefes sozinhos
	doMundo := []int{974, 4232, // Guarda_Carga e Prona
		3, 4, 8, 4852, 4875} // vizinhos
	usados := map[int]bool{}
	for _, idx := range append(append([]int(nil), coliseu...), doMundo...) {
		usados[idx] = true
	}

	dir := t.TempDir()
	runDir := filepath.Join(dir, "TMsrv", "run")
	npcDir := filepath.Join(runDir, "npc")
	if err := os.MkdirAll(npcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var gener strings.Builder
	k := 0
	for i := 0; i <= 4888; i++ {
		leader, x, y := "Nada", 1, 1
		if usados[i] {
			leader, x, y = "Grunt", 4+k%50*2, 4+k/50*4
			k++
		}
		fmt.Fprintf(&gener, "# [%d]\n\tLeader: %s\n\tMinuteGenerate: -1\n\tMinGroup: 0\n\tMaxGroup: 0\n\tMaxNumMob: 1\n\tStartX: %d\n\tStartY: %d\n\n",
			i, leader, x, y)
	}
	if err := os.WriteFile(filepath.Join(runDir, "NPCGener.txt"), []byte(gener.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(npcDir, "Grunt"), testMobTemplate("Grunt"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := world.New(world.Config{GridDim: 128}, logger, nil, nil)
	spawnNPCs(w, dir, false, nil, nil, nil, logger)

	for _, idx := range coliseu {
		g := w.GeneratorAt(idx)
		if g == nil {
			t.Fatalf("o bloco %d não foi registrado", idx)
		}
		if g.CurrentNumMob != 0 {
			t.Errorf("o bloco %d do Coliseu nasceu no boot (contagem %d): sem o evento ele não existe", idx, g.CurrentNumMob)
		}
	}
	for _, idx := range doMundo {
		g := w.GeneratorAt(idx)
		if g == nil {
			t.Fatalf("o bloco %d não foi registrado", idx)
		}
		if g.CurrentNumMob != 1 {
			t.Errorf("o bloco %d ficou com contagem %d, queria 1: fora dos 36, o boot dos blocos -1 continua", idx, g.CurrentNumMob)
		}
	}
}
