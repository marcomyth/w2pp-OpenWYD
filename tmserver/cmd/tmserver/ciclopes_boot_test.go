package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Em produção o boot roda com -npc-editing, e aí todo bloco cujo líder tem byte
// de loja no 104 passa a ser do painel (npc_definition). Os Ciclopes Cruéis e as
// cópias do spot têm 64 nesse byte: sem a exceção, os blocos deles viravam
// definições, que nascem sem nome de template, e a correção de 26/09 — que
// reconhece o monstro pelo nome — nunca os alcançou. O print de 27/09 mostrou o
// Ciclope ainda sem levar dano, com a release já no ar.
//
// Os templates são os reais. O Lanceiro_Zakum de fora do spot e o Coveiro seguem
// do painel, como antes.
func TestBootComEdicaoDeNPCDeixaOsCiclopesNoNPCGener(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "TMsrv", "run")
	npcDir := filepath.Join(runDir, "npc")
	if err := os.MkdirAll(npcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	blocos := []struct {
		leader  string
		x, y    int
		combate bool
	}{
		{"Ciclope_Cruel", 2300, 1400, true},
		{"Ciclope_Cruel_Spot", 2239, 1333, true},
		{"Lanceiro_Zakum_Spot", 2245, 1335, true},
		{"Lanceiro_Zakum", 2320, 1420, false},
		{"Coveiro", 2375, 2104, false},
	}
	// Os blocos do teste começam no 30: 0-7 têm o Coliseu e 10-21 a masmorra da
	// Água, que não nascem no boot. O enchimento é do Coveiro, que é do painel.
	const primeiro = 30
	var gener string
	for i := 0; i < primeiro; i++ {
		gener += "# [" + strconv.Itoa(i) + "]\n\tLeader: Coveiro\n\tStartX: " + strconv.Itoa(100+2*i) + "\n\tStartY: 100\n\n"
	}
	for i, b := range blocos {
		gener += "# [" + strconv.Itoa(primeiro+i) + "]\n\tLeader: " + b.leader + "\n\tMinGroup: 0\n\tMaxGroup: 0\n\tMaxNumMob: 1\n\tStartX: " +
			strconv.Itoa(b.x) + "\n\tStartY: " + strconv.Itoa(b.y) + "\n\n"
	}
	if err := os.WriteFile(filepath.Join(runDir, "NPCGener.txt"), []byte(gener), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, b := range blocos {
		src, err := os.ReadFile(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "npc", b.leader))
		if err != nil {
			t.Fatalf("template real %s: %v", b.leader, err)
		}
		if err := os.WriteFile(filepath.Join(npcDir, b.leader), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := world.New(world.Config{GridDim: 4096}, logger, nil, nil)
	spawnNPCs(w, dir, true, nil, nil, nil, logger) // true: -npc-editing, como em produção

	for i, b := range blocos {
		g := w.GeneratorAt(primeiro + i)
		if g == nil {
			t.Fatalf("%s: bloco %d não registrado", b.leader, primeiro+i)
		}
		if g.DBManaged == b.combate {
			t.Errorf("%s: DBManaged = %v, want %v", b.leader, g.DBManaged, !b.combate)
			continue
		}
		if !b.combate {
			continue
		}
		if g.CurrentNumMob != 1 {
			t.Errorf("%s: %d no mundo, want 1 nascido do NPCGener", b.leader, g.CurrentNumMob)
		}
		for id := world.MaxUser; id < world.MaxMob; id++ {
			if e := w.Entity(id); e != nil && int(e.GenIndex) == primeiro+i {
				if e.NonCombatNPC {
					t.Errorf("%s: nasceu intocável", b.leader)
				}
				if e.TemplateName != b.leader {
					t.Errorf("%s: nasceu com o nome %q; a Mesa de Drops não o acharia", b.leader, e.TemplateName)
				}
			}
		}
	}
}
