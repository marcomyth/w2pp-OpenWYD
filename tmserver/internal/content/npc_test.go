package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNPCGenerators(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "NPCGener.txt")
	// gen[0] mirrors a real Start/Dest patrol block (Coliseu): waypoints land in
	// SegX[0] (Start) and SegX[4] (Dest) exactly as ParseString maps them.
	const sample = `// header comment
#	[   0]
	MinuteGenerate:	-1
	MaxNumMob:	100
	MinGroup:	4
	MaxGroup:	7
	Leader:		Ciclope_Forte
	Follower:	Arq_Ciclope
	RouteType:	2
	Formation:	0
	FightAction:	Lute
	FightAction:	Venha
	FightAction4:	Ultima
	DieAction2:	Morri
	StartX:		2635
	StartY:		1726
	StartRange:	5
	StartWait:	10
	DestX:		2625
	DestY:		1726
	DestRange:	10
	DestWait:	10

#	[   1]
	Leader:		Ciclop_Selvagem
	Follower:	0
	MinGroup:	3
	StartX:		2700
	StartY:		1800
	StartRange:	8
	Segment1X:	2710
	Segment1Y:	1810
	Segment1Wait:	4
`
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatalf("LoadNPCGenerators: %v", err)
	}
	if len(gens) != 2 {
		t.Fatalf("got %d generators, want 2", len(gens))
	}
	g := gens[0]
	if g.Leader != "Ciclope_Forte" || g.SegX[0] != 2635 || g.SegY[0] != 1726 ||
		g.SegRange[0] != 5 || g.MinGroup != 4 {
		t.Errorf("gen[0] = %+v", g)
	}
	if g.Follower != "Arq_Ciclope" || g.MinuteGenerate != -1 || g.MaxGroup != 7 || g.RouteType != 2 {
		t.Errorf("gen[0] extras = %+v", g)
	}
	if g.FightAction != [4]string{"Lute", "Venha", "", "Ultima"} {
		t.Errorf("gen[0] FightAction = %#v", g.FightAction)
	}
	if g.DieAction != [4]string{"", "Morri", "", ""} {
		t.Errorf("gen[0] DieAction = %#v", g.DieAction)
	}
	if g.SegX[4] != 2625 || g.SegY[4] != 1726 || g.SegRange[4] != 10 || g.SegWait[4] != 10 || g.SegWait[0] != 10 {
		t.Errorf("gen[0] Dest→Seg[4] mapping = %+v", g)
	}
	if g.SegX[1] != 0 || g.SegX[2] != 0 || g.SegX[3] != 0 {
		t.Errorf("gen[0] middle waypoints should stay 0, got %+v", g.SegX)
	}
	g1 := gens[1]
	if g1.Leader != "Ciclop_Selvagem" || g1.MinGroup != 3 || g1.SegX[0] != 2700 {
		t.Errorf("gen[1] = %+v", g1)
	}
	if g1.Follower != "" { // "Follower: 0" means none
		t.Errorf("gen[1] Follower = %q, want empty", g1.Follower)
	}
	if g1.SegX[1] != 2710 || g1.SegY[1] != 1810 || g1.SegWait[1] != 4 {
		t.Errorf("gen[1] Segment1 = %+v", g1)
	}
}

// TestLoadNPCGeneratorsReal parses the shipped NPCGener.txt if present.
func TestLoadNPCGeneratorsReal(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatalf("LoadNPCGenerators(real): %v", err)
	}
	if len(gens) < 1000 {
		t.Errorf("real NPCGener has %d generators, want many", len(gens))
	}
	for i, g := range gens {
		if g.Leader == "Premium_Neil" || g.Follower == "Premium_Neil" {
			t.Fatalf("real NPCGener contains Premium_Neil at generator %d: %+v", i, g)
		}
	}
}

// TestBlocosDesligadosPorIndice pins the blocks that are switched off BY INDEX —
// world.eventOwnedGenerators (23-26) and migration 0047 — to the NPCs they were
// meant for. An index is a position in NPCGener.txt: remove a block above them
// and the same numbers silently switch off someone else.
func TestBlocosDesligadosPorIndice(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]string{
		23: "Torre_de_Thor", 24: "Torre_de_Thor", 25: "Torre_de_Thor", 26: "Torre_de_Thor",
		3903: "Camponesa_", 4511: "Merc_Fantasma", 4849: "Mercador", 4852: "Mercador",
		5995: "Torcedor", 5996: "Torcedor_", 5997: "Torcedor__", 5998: "Torcedor___",
		5999: "Pescador", 6058: "Cap_Rowena", 6077: "Curandeiro",
		// Migration 0055: the Erion shop row. The "#[n]" labels in the file say
		// 6098-6102; they are stale and never the index.
		6084: "AcessoriosErion", 6085: "Set_TK_Erion", 6086: "Set_BM_Erion",
		6087: "Set_HT_Erion", 6088: "Set_FM_Erion",
		// Migration 0064: the world's Troll Enigma, whose cage the Acampamento
		// Troll quest took over. Switched off in npc_generator_off.
		3804: "Troll_Enigma",
		// Migration 0109: the two Agmo of the Deserto are event-only now (every
		// kill drops a guaranteed mount core, handler/deserto.go). Switched off in npc_generator_off.
		3451: "Verme_Agmo", 3452: "Tauron_Agmo",
		// Migration 0129: the Terras Místicas NPC, whose spot is kept for another
		// quest. Switched off in npc_generator_off.
		985: "Cap.Mercenario",
		// Migration 0139: the FrenzyDemonLord's second block, on the same spot as
		// 3134, which came back every 15 s. Switched off in npc_generator_off.
		3135: "FrenzyDemonLord",
		// The two the square keeps.
		3442: "Perzen", 3809: "GodGovernment",
	}
	for idx, leader := range want {
		if idx >= len(gens) || gens[idx].Leader != leader {
			got := ""
			if idx < len(gens) {
				got = gens[idx].Leader
			}
			t.Errorf("bloco %d = %q, want %q — o NPCGener mudou; revise eventOwnedGenerators e a migração 0047", idx, got, leader)
		}
	}
}

// TestCavLugeferSaoVinte pins the Deserto boss population the team asked for on
// 23/09/2026 ("2x a quantidade, para deixar pior"): the ten Cav._Lugefer blocks
// hold two each, and all ten regenerate on the generator clock (MinuteGenerate 4
// is four 12 s passes, 48 s — internal/spawnrate). The boot raises
// one group per block, so a MinuteGenerate -1 block would never reach its second
// one — that is why 3235 left -1 in the same change.
func TestCavLugeferSaoVinte(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	blocos, total := 0, 0
	for i, g := range gens {
		if g.Leader != "Cav._Lugefer" {
			continue
		}
		blocos++
		total += g.MaxNumMob
		if g.MaxNumMob != 2 || g.MinuteGenerate != 4 || g.MinGroup != 0 || g.MaxGroup != 0 {
			t.Errorf("bloco %d: MaxNumMob %d, MinuteGenerate %d, grupo %d-%d; want 2, 4, 0-0",
				i, g.MaxNumMob, g.MinuteGenerate, g.MinGroup, g.MaxGroup)
		}
	}
	if blocos != 10 || total != 20 {
		t.Errorf("%d blocos com %d Cav._Lugefer, want 10 com 20", blocos, total)
	}
}

// TestBossManticoraBloco pins the Deserto_Manticora boss (migration 0109): one
// Boss_Manticora in block 6145, the last one, with no minute period — the 5 h
// wait after its death is the individual queue's (handler/deserto.go), and a
// positive MinuteGenerate would refill it on the generator clock instead.
func TestBossManticoraBloco(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	const idx = 6145
	if idx >= len(gens) {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), idx)
	}
	g := gens[idx]
	if g.Leader != "Boss_Manticora" || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want one Boss_Manticora with MinuteGenerate -1", idx, g)
	}
	// Deserto_Manticora in Regions.txt: 1282,1664 - 1396,1785.
	if x, y := g.SegX[0], g.SegY[0]; x < 1282 || x > 1396 || y < 1664 || y > 1785 {
		t.Errorf("Boss_Manticora nasce em (%d,%d), fora do Deserto_Manticora", x, y)
	}
	n := 0
	for _, g := range gens {
		if g.Leader == "Boss_Manticora" || g.Follower == "Boss_Manticora" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Boss_Manticora em %d blocos, want 1", n)
	}
}

// TestBossDragaoLichBloco pins the Dungeon's 3rd-floor boss (migration 0140): one
// Boss_Dragao_Lich in block 6146, the last one, with no minute period — the 4 h
// wait after its death is the individual queue's (handler/dungeon.go).
func TestBossDragaoLichBloco(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	const idx = 6146
	if idx >= len(gens) {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), idx)
	}
	g := gens[idx]
	if g.Leader != "Boss_Dragao_Lich" || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want one Boss_Dragao_Lich with MinuteGenerate -1", idx, g)
	}
	// Dungeon_3_Andar in Regions.txt: 898,3712 - 1143,3830.
	if x, y := g.SegX[0], g.SegY[0]; x < 898 || x > 1143 || y < 3712 || y > 3830 {
		t.Errorf("Boss_Dragao_Lich nasce em (%d,%d), fora do Dungeon_3_Andar", x, y)
	}
	n := 0
	for _, g := range gens {
		if g.Leader == "Boss_Dragao_Lich" || g.Follower == "Boss_Dragao_Lich" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Boss_Dragao_Lich em %d blocos, want 1", n)
	}
}

// TestChefesDaLavaBlocos pins the Dungeon 2nd floor lava hall (migration 0142): the
// two mini bosses in blocks 6147 and 6148, the last ones, with no minute period —
// the 2 h wait is the individual queue's (handler/dungeon_lava.go) — and the Golem
// de Pedra and Anf Ninja blocks of the hall at five times the legacy count.
func TestChefesDaLavaBlocos(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 6149 {
		t.Fatalf("NPCGener has %d blocks, want 6149 with the lava bosses last", len(gens))
	}
	// Dungeon_2_Andar in Regions.txt: 632,3847 - 1022,4091.
	dentro := func(x, y int16) bool { return x >= 632 && x <= 1022 && y >= 3847 && y <= 4091 }
	for idx, nome := range map[int]string{6147: "Boss_Golem", 6148: "Boss_Anf_Ninja"} {
		g := gens[idx]
		if g.Leader != nome || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
			t.Errorf("bloco %d = %+v, want one %s with MinuteGenerate -1", idx, g, nome)
		}
		if !dentro(g.SegX[0], g.SegY[0]) {
			t.Errorf("%s nasce em (%d,%d), fora do Dungeon_2_Andar", nome, g.SegX[0], g.SegY[0])
		}
	}
	mobs := map[string]int{}
	for _, g := range gens {
		if g.Leader != "Anf_Ninja" && g.Leader != "Golem_de_Pedra" {
			continue
		}
		if !dentro(g.SegX[0], g.SegY[0]) {
			t.Errorf("%s fora do 2º andar em (%d,%d): a Mesa da 0142 vale por template", g.Leader, g.SegX[0], g.SegY[0])
		}
		if g.MinuteGenerate <= 0 {
			t.Errorf("%s em (%d,%d) com MinuteGenerate %d: só o relógio de minuto enche o bloco até o teto", g.Leader, g.SegX[0], g.SegY[0], g.MinuteGenerate)
		}
		mobs[g.Leader] += g.MaxNumMob
	}
	// 60 e 56 antes da 0142.
	if mobs["Anf_Ninja"] != 300 || mobs["Golem_de_Pedra"] != 280 {
		t.Errorf("Anf Ninja %d e Golem de Pedra %d, want 300 e 280 (5x)", mobs["Anf_Ninja"], mobs["Golem_de_Pedra"])
	}
}
