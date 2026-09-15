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

// TestSpawnNPCsNaoPopulaOsBlocos81a97: os blocos 81-97 do NPCGener.txt (Krill e
// ChaosOrc__ dentro da caixa da Sala Secreta, MinuteGenerate -1) não nascem no
// boot, e os vizinhos 80 e 98, com a mesma receita, continuam nascendo.
//
// O legado nunca os gera: o relógio de minuto pula MinuteGenerate <= 0
// (ProcessSecMinTimer.cpp:2727) e nenhuma faixa de GenerateMob os cobre. O boot que
// popula blocos -1 no resto do mundo é divergência deliberada e fica, por isso o
// teste cobra os dois lados da fronteira.
func TestSpawnNPCsNaoPopulaOsBlocos81a97(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "TMsrv", "run")
	npcDir := filepath.Join(runDir, "npc")
	if err := os.MkdirAll(npcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var gener strings.Builder
	for i := 0; i <= 98; i++ {
		fmt.Fprintf(&gener, "# [%d]\n\tLeader: Grunt\n\tMinuteGenerate: -1\n\tMinGroup: 0\n\tMaxGroup: 0\n\tMaxNumMob: 1\n\tStartX: %d\n\tStartY: %d\n\n",
			i, 4+i%50, 4+i/50*4)
	}
	if err := os.WriteFile(filepath.Join(runDir, "NPCGener.txt"), []byte(gener.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(npcDir, "Grunt"), testMobTemplate("Grunt"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := world.New(world.Config{GridDim: 64}, logger, nil, nil)
	spawnNPCs(w, dir, false, nil, nil, nil, logger)

	for idx := 81; idx <= 97; idx++ {
		g := w.GeneratorAt(idx)
		if g == nil {
			t.Fatalf("o bloco %d não foi registrado", idx)
		}
		if g.CurrentNumMob != 0 {
			t.Errorf("o bloco %d nasceu no boot (contagem %d): o legado nunca gera os blocos da Sala Secreta sem dono", idx, g.CurrentNumMob)
		}
	}
	for _, idx := range []int{80, 98} {
		g := w.GeneratorAt(idx)
		if g == nil {
			t.Fatalf("o bloco vizinho %d não foi registrado", idx)
		}
		if g.CurrentNumMob != 1 {
			t.Errorf("o bloco vizinho %d ficou com contagem %d, queria 1: o boot dos blocos -1 fora da faixa tem que continuar", idx, g.CurrentNumMob)
		}
	}
}
