package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// Na cidade dos Reinos o exército e os Reis deixam de ser definições de NPC
// (internal/reinos): a poda do seed apaga as linhas que eles têm em produção, e
// eles voltam a nascer do NPCGener como monstros. O Guarda_Real (byte 17 = 100)
// e o oráculo (clã 6) continuam definições, e a mesma Bruxa fora dos Reinos
// também.
func TestBuildNPCDefinitionsDeixaDeForaOExercitoDosReinos(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "TMsrv", "run")
	npcDir := filepath.Join(runDir, "npc")
	if err := os.MkdirAll(npcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const npcGener = `# [0]
	Leader: Bruxa
	StartX: 1706
	StartY: 1686
	RouteType: 2

# [1]
	Leader: Rei_Harabard
	StartX: 1750
	StartY: 1574
	RouteType: 0

# [2]
	Leader: Guarda_Real
	StartX: 1720
	StartY: 1600
	RouteType: 0

# [3]
	Leader: BlackOracle
	StartX: 1720
	StartY: 1610
	RouteType: 0

# [4]
	Leader: Bruxa
	StartX: 2600
	StartY: 1700
	RouteType: 2
`
	if err := os.WriteFile(filepath.Join(runDir, "NPCGener.txt"), []byte(npcGener), 0o644); err != nil {
		t.Fatal(err)
	}
	escrever := func(nome string, clan, mobMerchant, scoreMerchant byte) {
		writeDBNPCMobBytes(t, npcDir, nome, mobMerchant, scoreMerchant)
		p := filepath.Join(npcDir, nome)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		b[16] = clan
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escrever("Bruxa", 7, 0, 64)
	escrever("Rei_Harabard", 7, 15, 111)
	escrever("Guarda_Real", 7, 100, 100)
	escrever("BlackOracle", 6, 20, 20)

	defs, err := buildNPCDefinitions(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("buildNPCDefinitions: %v", err)
	}
	slugs := make(map[string]bool, len(defs))
	for _, d := range defs {
		slugs[d.Slug] = true
	}
	for _, s := range []string{"Bruxa-0", "Rei_Harabard-1"} {
		if slugs[s] {
			t.Errorf("%s virou definição de NPC: continua imortal", s)
		}
	}
	for _, s := range []string{"Guarda_Real-2", "BlackOracle-3", "Bruxa-4"} {
		if !slugs[s] {
			t.Errorf("%s saiu das definições, want NPC como antes", s)
		}
	}
}
