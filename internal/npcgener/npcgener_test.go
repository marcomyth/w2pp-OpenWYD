package npcgener

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "NPCGener.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	gens, err := Load(write(t, strings.Join([]string{
		"// comentário de topo",
		"#Bloco",
		"Leader:\tAki",
		"Follower:\t0",
		"MinuteGenerate:\t5",
		"MaxNumMob:\t2",
		"RouteType:\t2",
		"StartX:\t1309",
		"StartY:\t312",
		"DestX:\t1320",
		"DestY:\t320",
		"Segment1X:\t1315",
		"Segment1Y:\t316",
		"Segment1Wait:\t4",
		"",
		"#Outro",
		"Leader:\tGuarda_Carga",
		"Follower:\tSoldado",
		"StartX:\t2102",
		"StartY:\t2116",
		"FightAction:\tAo ataque!",
		"DieAction2:\tArgh",
	}, "\n")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(gens) != 2 {
		t.Fatalf("got %d blocks, want 2", len(gens))
	}

	g := gens[0]
	if g.Leader != "Aki" {
		t.Errorf("Leader = %q, want Aki", g.Leader)
	}
	// "0" is not a template name; the original rejects it (ParseString).
	if g.Follower != "" {
		t.Errorf("Follower = %q, want empty for the literal \"0\"", g.Follower)
	}
	// Waypoints map Start→[0], Segment1..3→[1..3], Dest→[4].
	if g.SegX[0] != 1309 || g.SegY[0] != 312 {
		t.Errorf("start waypoint = (%d,%d), want (1309,312)", g.SegX[0], g.SegY[0])
	}
	if g.SegX[1] != 1315 || g.SegY[1] != 316 || g.SegWait[1] != 4 {
		t.Errorf("segment 1 = (%d,%d) wait %d, want (1315,316) wait 4", g.SegX[1], g.SegY[1], g.SegWait[1])
	}
	if g.SegX[4] != 1320 || g.SegY[4] != 320 {
		t.Errorf("dest waypoint = (%d,%d), want (1320,320)", g.SegX[4], g.SegY[4])
	}
	if g.MinuteGenerate != 5 || g.MaxNumMob != 2 || g.RouteType != 2 {
		t.Errorf("block 0 = %+v, want MinuteGenerate 5 / MaxNumMob 2 / RouteType 2", g)
	}
	// MinGroup defaults to 1 for a block that does not set it.
	if g.MinGroup != 1 {
		t.Errorf("MinGroup = %d, want the default 1", g.MinGroup)
	}

	g = gens[1]
	if g.Follower != "Soldado" {
		t.Errorf("Follower = %q, want Soldado", g.Follower)
	}
	if g.FightAction[0] != "Ao ataque!" {
		t.Errorf("FightAction[0] = %q", g.FightAction[0])
	}
	// DieAction2 is the indexed form, landing at [1].
	if g.DieAction[1] != "Argh" {
		t.Errorf("DieAction[1] = %q, want Argh", g.DieAction[1])
	}
}

// A block with no Leader names no template, so it is not a spawn — the original
// skips it and so must the parser, or the block index would shift and break the
// generator_index that npc_definition stores.
func TestLoadSkipsBlocksWithoutLeader(t *testing.T) {
	gens, err := Load(write(t, "#Vazio\nStartX:\t10\n\n#Real\nLeader:\tAki\nStartX:\t20\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(gens) != 1 || gens[0].Leader != "Aki" {
		t.Fatalf("got %+v, want only the block that names a leader", gens)
	}
}

// CNPCGene::SetAct drops action strings that would overflow its [80] buffer.
func TestLoadRejectsOverlongActions(t *testing.T) {
	long := strings.Repeat("x", 79)
	gens, err := Load(write(t, "#B\nLeader:\tAki\nFightAction:\t"+long+"\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if gens[0].FightAction[0] != "" {
		t.Errorf("FightAction[0] kept a %d-char string, want it dropped", len(long))
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "ausente.txt")); err == nil {
		t.Fatal("Load on a missing file returned no error")
	}
}

// TestLoadReal parses the shipped file: the block count is what dbserver's
// import and the moderation panel both index into.
func TestLoadReal(t *testing.T) {
	path := filepath.Join("..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := Load(path)
	if err != nil {
		t.Fatalf("Load(real): %v", err)
	}
	if len(gens) < 6000 {
		t.Errorf("real NPCGener has %d blocks, want ~6099", len(gens))
	}
}
