package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// Os Ciclopes Cruéis e as cópias do spot (internal/ciclopes) deixam de ser
// definições de NPC: a poda do seed apaga as linhas que eles têm em produção, e
// eles voltam a nascer do NPCGener, com nome, apanhando e na Mesa de Drops. O
// Lanceiro_Zakum de fora do spot, com o mesmo 64, continua definição.
func TestBuildNPCDefinitionsDeixaDeForaOsCiclopes(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "TMsrv", "run")
	npcDir := filepath.Join(runDir, "npc")
	if err := os.MkdirAll(npcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const npcGener = `# [0]
	Leader: Ciclope_Cruel
	StartX: 2300
	StartY: 1400

# [1]
	Leader: Ciclope_Cruel_Spot
	StartX: 2239
	StartY: 1333

# [2]
	Leader: Lanceiro_Zakum_Spot
	StartX: 2245
	StartY: 1335

# [3]
	Leader: Lanceiro_Zakum
	StartX: 2320
	StartY: 1420
`
	if err := os.WriteFile(filepath.Join(runDir, "NPCGener.txt"), []byte(npcGener), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, nome := range []string{"Ciclope_Cruel", "Ciclope_Cruel_Spot", "Lanceiro_Zakum_Spot", "Lanceiro_Zakum"} {
		writeDBNPCMobBytes(t, npcDir, nome, 0, 64) // os bytes dos templates reais: @17 0, @104 64
	}

	defs, err := buildNPCDefinitions(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("buildNPCDefinitions: %v", err)
	}
	slugs := make(map[string]bool, len(defs))
	for _, d := range defs {
		slugs[d.Slug] = true
	}
	for _, s := range []string{"Ciclope_Cruel-0", "Ciclope_Cruel_Spot-1", "Lanceiro_Zakum_Spot-2"} {
		if slugs[s] {
			t.Errorf("%s virou definição de NPC: nasce sem nome e continua imortal", s)
		}
	}
	if !slugs["Lanceiro_Zakum-3"] {
		t.Error("Lanceiro_Zakum-3 saiu das definições, want NPC como antes")
	}
}
