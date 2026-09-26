package content

import (
	"os"
	"path/filepath"
	"slices"
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

// TestChefesDaLavaBlocos pins the lava room of the Dungeon's 2nd floor (migrations
// 0142 and 0148): ONLY the room in the photos, x 964-985 × y 3984-4005, between
// the HeightMap walls. Its four blocks (2296-2299) use the room's own template
// copies, each born as a full group of 10 with no minute period — the individual
// queue brings each one back in 10 s (handler/dungeon_lava.go) —, the two mini
// bosses (6147, 6148) are inside it, and every other Golem de Pedra and Anf Ninja
// of the floor is back at its legacy count and period.
func TestChefesDaLavaBlocos(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	// 6150 desde 25/09/2026: o Boss Conjurador (migração 0144) entrou depois deles.
	if len(gens) < 6150 {
		t.Fatalf("NPCGener has %d blocks, want 6150 or more", len(gens))
	}
	naSala := func(x, y int16) bool { return x >= 964 && x <= 985 && y >= 3984 && y <= 4005 }
	for idx, nome := range map[int]string{6147: "Boss_Golem", 6148: "Boss_Anf_Ninja"} {
		g := gens[idx]
		if g.Leader != nome || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
			t.Errorf("bloco %d = %+v, want one %s with MinuteGenerate -1", idx, g, nome)
		}
		if !naSala(g.SegX[0], g.SegY[0]) {
			t.Errorf("%s nasce em (%d,%d), fora da sala da lava", nome, g.SegX[0], g.SegY[0])
		}
	}
	sala := map[string]int{}
	for idx := 2296; idx <= 2299; idx++ {
		g := gens[idx]
		if g.Leader != "Golem_Lava" && g.Leader != "Anf_Ninja_Lava" {
			t.Errorf("bloco %d é %s, want a cópia da sala", idx, g.Leader)
		}
		if g.Follower != g.Leader || g.MinuteGenerate != -1 || g.MaxNumMob != 10 || g.MinGroup != 9 || g.MaxGroup != 9 {
			t.Errorf("bloco %d = %+v, want um grupo cheio de 10 sem período", idx, g)
		}
		sala[g.Leader] += g.MaxNumMob
	}
	andar := map[string]int{}
	for idx, g := range gens {
		switch g.Leader {
		case "Golem_Lava", "Anf_Ninja_Lava":
			if idx < 2296 || idx > 2299 || !naSala(g.SegX[0], g.SegY[0]) {
				t.Errorf("%s no bloco %d em (%d,%d): a cópia é só da sala", g.Leader, idx, g.SegX[0], g.SegY[0])
			}
		case "Anf_Ninja", "Golem_de_Pedra":
			if naSala(g.SegX[0], g.SegY[0]) {
				t.Errorf("%s ainda na sala, no bloco %d", g.Leader, idx)
			}
			andar[g.Leader] += g.MaxNumMob
		}
	}
	// A sala tinha 4 Anf e 4 Golens: 5x. O resto do andar é o legado (60 e 56 menos a sala).
	if sala["Anf_Ninja_Lava"] != 20 || sala["Golem_Lava"] != 20 {
		t.Errorf("sala com %v, want 20 de cada", sala)
	}
	if andar["Anf_Ninja"] != 56 || andar["Golem_de_Pedra"] != 52 {
		t.Errorf("resto do andar com %v, want 56 Anf Ninja e 52 Golens, os do legado", andar)
	}
}

// TestDungeonSalasPopulacao pins the grid rooms of the Dungeon's 1st floor that
// the Marco photographed (migration 0143, fixed on 25/09/2026): two lava rooms
// at x 144-195 (y 3736-3764 and 3789-3817) and two grid rooms at x 197-239 (y
// 3743-3759 and 3790-3808). Inside them Urso_Zumbi and Arq_Caveira are 5x and
// Caveira 2x, each block born as one full group of its own template — the
// generator raises ONE group per call. The corridors and halls between them
// keep the legacy count: the first version of 0143 multiplied the whole box
// (x 127-260, y 3700-3860) and filled the corridors too. The Arq_Caveira of the
// Rei Troll Zumbi's west hall and south corridor stay 5x: 0147 counts on them.
func TestDungeonSalasPopulacao(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	salas := [][4]int16{{144, 3736, 195, 3764}, {144, 3789, 195, 3817}, {197, 3743, 239, 3759}, {197, 3790, 239, 3808}}
	naSala := func(x, y int16) bool {
		for _, r := range salas {
			if x >= r[0] && x <= r[2] && y >= r[1] && y <= r[3] {
				return true
			}
		}
		return false
	}
	doRei := func(x, y int16) bool { return x >= 245 && x <= 261 && y >= 3745 && y <= 3815 }
	want := map[string]int{"Caveira": 60, "Urso_Zumbi": 120, "Arq_Caveira": 20}
	got := map[string]int{}
	rei := 0
	for i, g := range gens {
		x, y := g.SegX[0], g.SegY[0]
		if _, ok := want[g.Leader]; !ok || g.Follower != g.Leader || x < 127 || x > 260 || y < 3700 || y > 3860 {
			continue
		}
		switch {
		case naSala(x, y):
			got[g.Leader] += g.MaxNumMob
			if g.MinGroup != g.MaxNumMob-1 || g.MaxGroup != g.MaxNumMob-1 {
				t.Errorf("bloco %d (%s) na sala: max %d, grupo %d-%d; want um grupo cheio",
					i, g.Leader, g.MaxNumMob, g.MinGroup, g.MaxGroup)
			}
		case g.Leader == "Arq_Caveira" && doRei(x, y):
			rei += g.MaxNumMob
		default:
			if g.MinGroup != 0 || g.MaxGroup != 0 || g.MaxNumMob > 2 {
				t.Errorf("bloco %d (%s) em (%d,%d), fora das salas: max %d, grupo %d-%d; want o número do legado",
					i, g.Leader, x, y, g.MaxNumMob, g.MinGroup, g.MaxGroup)
			}
		}
	}
	for nome, n := range want {
		if got[nome] != n {
			t.Errorf("%s nas salas: %d, want %d", nome, got[nome], n)
		}
	}
	if rei != 35 {
		t.Errorf("Arq_Caveira no salão oeste e no corredor sul do Rei: %d, want 35 (0147)", rei)
	}
}

// TestCaveirasDoSpotBlocos pins the fountain room of the Dungeon 1st floor, the
// Caveira Lanc and Conj Caveira spot (migrations 0144 and 0153): x 345-361, y
// 3740-3772, between the HeightMap walls. Only its six blocks (1884 to 1889) are
// 5x, each born as one full group of a room-only template copy, so the Mesa's
// loot stays in the room; the Boss Conjurador is alone in block 6149, inside it,
// with no minute period — the 3 h wait is the individual queue's
// (handler/dungeon_caveiras.go). Every other Caveira Lanc and Conj Caveira block
// is back to its legacy count: 45 and 30 in total, the room included.
func TestCaveirasDoSpotBlocos(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) < 6150 {
		t.Fatalf("NPCGener has %d blocks, want 6150 or more", len(gens))
	}
	sala := func(x, y int16) bool { return x >= 345 && x <= 361 && y >= 3740 && y <= 3772 }
	g := gens[6149]
	if g.Leader != "Boss_Conjurador" || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco 6149 = %+v, want one Boss_Conjurador with MinuteGenerate -1", g)
	}
	if !sala(g.SegX[0], g.SegY[0]) {
		t.Errorf("Boss_Conjurador nasce em (%d,%d), fora da sala da fonte", g.SegX[0], g.SegY[0])
	}
	copia := map[string]string{"Caveira_Lanc_Fonte": "Caveira_Lanc", "Conj_Caveira_Fonte": "Conj_Caveira"}
	legado := map[string]int{}
	var daSala []int
	for i, g := range gens {
		if g.Leader == "Boss_Conjurador" && i != 6149 {
			t.Errorf("Boss_Conjurador também no bloco %d", i)
		}
		if orig, ok := copia[g.Leader]; ok {
			daSala = append(daSala, i)
			if !sala(g.SegX[0], g.SegY[0]) {
				t.Errorf("bloco %d: %s fora da sala em (%d,%d); a Mesa da 0153 vale por template", i, g.Leader, g.SegX[0], g.SegY[0])
			}
			if g.Follower != g.Leader || g.MinuteGenerate != -1 || g.MaxNumMob != 5 || g.MinGroup != 4 || g.MaxGroup != 4 {
				t.Errorf("bloco %d = %+v, want um grupo cheio de 5 %s sem período", i, g, g.Leader)
			}
			legado[orig]++ // era um bicho só antes de ficar 5x
			continue
		}
		if g.Leader != "Caveira_Lanc" && g.Leader != "Conj_Caveira" {
			continue
		}
		if sala(g.SegX[0], g.SegY[0]) {
			t.Errorf("bloco %d: %s original dentro da sala da fonte", i, g.Leader)
		}
		legado[g.Leader] += g.MaxNumMob
	}
	if want := []int{1884, 1885, 1886, 1887, 1888, 1889}; !slices.Equal(daSala, want) {
		t.Errorf("blocos da sala %v, want %v", daSala, want)
	}
	// 45 e 30 no legado, e de novo depois da 0153; só a sala mudou.
	if legado["Caveira_Lanc"] != 45 || legado["Conj_Caveira"] != 30 {
		t.Errorf("Caveira Lanc %d e Conj Caveira %d no tamanho do legado, want 45 e 30", legado["Caveira_Lanc"], legado["Conj_Caveira"])
	}
}

// TestReiTrollZumbiBloco pins the Dungeon's 1st-floor mini boss (migration 0147):
// one Rei_Troll_Zumbi in block 6150, fixed on (284,3753) with range 0 — the
// tiles right next to it are blocked — and no minute period: the 1 h wait after
// its death is the individual queue's (handler/rei_troll_zumbi.go). After it come
// the ten hunting blocks 6151-6160: Troll Zumbi in the three rooms around it, and
// Arq. Caveira only in its own room (the other two got 5x Arq. in b2f1ce68).
func TestReiTrollZumbiBloco(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	const idx, primeiro, ultimo = 6150, 6151, 6160
	// Não é mais o último desde 25/09/2026: o Boss Hidra Dourada (0149) entrou no 6161.
	if ultimo >= len(gens) {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), ultimo)
	}
	g := gens[idx]
	if g.Leader != "Rei_Troll_Zumbi" || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want one Rei_Troll_Zumbi with MinuteGenerate -1", idx, g)
	}
	if g.SegX[0] != 284 || g.SegY[0] != 3753 || g.SegRange[0] != 0 || g.SegRange[4] != 0 {
		t.Errorf("Rei_Troll_Zumbi em (%d,%d) raio %d/%d, want (284,3753) raio 0",
			g.SegX[0], g.SegY[0], g.SegRange[0], g.SegRange[4])
	}
	n := 0
	for _, g := range gens {
		if g.Leader == "Rei_Troll_Zumbi" || g.Follower == "Rei_Troll_Zumbi" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Rei_Troll_Zumbi em %d blocos, want 1", n)
	}
	mobs := map[string]int{}
	for i := primeiro; i <= ultimo; i++ {
		g := gens[i]
		if g.Leader != g.Follower || (g.Leader != "Troll_Zumbi" && g.Leader != "Arq_Caveira") {
			t.Errorf("bloco %d = %s/%s, want Troll_Zumbi ou Arq_Caveira", i, g.Leader, g.Follower)
		}
		if g.MinuteGenerate != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
			t.Errorf("bloco %d: MinuteGenerate %d, grupo %d-%d; want 1 e 0-0, como os vizinhos", i, g.MinuteGenerate, g.MinGroup, g.MaxGroup)
		}
		// Os três salões em volta do Rei: X 241-291, Y 3735-3808.
		if x, y := g.SegX[0], g.SegY[0]; x < 241 || x > 291 || y < 3735 || y > 3808 {
			t.Errorf("bloco %d nasce em (%d,%d), fora dos salões do Rei", i, x, y)
		}
		mobs[g.Leader] += g.MaxNumMob
	}
	if mobs["Troll_Zumbi"] != 19 || mobs["Arq_Caveira"] != 2 {
		t.Errorf("caça nova: %v, want 19 Troll_Zumbi e 2 Arq_Caveira", mobs)
	}
}

// TestBossHidraDouradaBloco pins the Dungeon 2nd-floor boss (migration 0149): one
// Boss_Hidra_Dourada in block 6161 with no minute period — the 4 h wait is the
// individual queue's (handler/dungeon.go) — on the spot of block 2099, which kept
// only the escort: five Guer_Caveira_Escolta raised as one full group.
func TestBossHidraDouradaBloco(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	const chefe, escolta = 6161, 2099
	if chefe >= len(gens) {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), chefe)
	}
	g := gens[chefe]
	if g.Leader != "Boss_Hidra_Dourada" || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want one Boss_Hidra_Dourada with MinuteGenerate -1", chefe, g)
	}
	e := gens[escolta]
	if e.Leader != "Guer_Caveira_Escolta" || e.Follower != "Guer_Caveira_Escolta" || e.MaxNumMob != 5 || e.MinGroup != 4 || e.MaxGroup != 4 {
		t.Errorf("bloco %d = %+v, want five Guer_Caveira_Escolta in one full group", escolta, e)
	}
	if g.SegX[0] != e.SegX[0] || g.SegY[0] != e.SegY[0] || g.SegX[0] != 740 || g.SegY[0] != 3776 {
		t.Errorf("chefe em (%d,%d) e escolta em (%d,%d), want os dois em (740,3776)", g.SegX[0], g.SegY[0], e.SegX[0], e.SegY[0])
	}
	n := map[string]int{}
	for _, g := range gens {
		for _, nome := range []string{"Boss_Hidra_Dourada", "Guer_Caveira_Escolta"} {
			if g.Leader == nome || g.Follower == nome {
				n[nome]++
			}
		}
	}
	if n["Boss_Hidra_Dourada"] != 1 || n["Guer_Caveira_Escolta"] != 1 {
		t.Errorf("chefe em %d blocos e escolta em %d, want 1 e 1", n["Boss_Hidra_Dourada"], n["Guer_Caveira_Escolta"])
	}
}

// TestGargulaSabioBlocos pins the Gárgula Sábio of the photo (migration 0151),
// 2nd floor of the Dungeon: block 2540 is the boss copy alone, with no minute
// period — the 1 h wait is the individual queue's (handler/gargula_sabio.go) —,
// block 6162 is its five Golem guards on the same point, and the
// other Gárgula Sábio of the floor (block 2478) is untouched.
func TestGargulaSabioBlocos(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	const chefe, guardas, outra = 2540, 6162, 2478
	if guardas >= len(gens) {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), guardas)
	}
	g := gens[chefe]
	if g.Leader != "Gargula_Sabio_Chefe" || g.Follower != g.Leader || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want a Gárgula chefe sozinha, sem período", chefe, g)
	}
	gu := gens[guardas]
	if gu.Leader != "Golem_Guarda" || gu.Follower != gu.Leader || gu.MinuteGenerate != -1 || gu.MaxNumMob != 5 || gu.MinGroup != 4 || gu.MaxGroup != 4 {
		t.Errorf("bloco %d = %+v, want um grupo cheio de 5 guardas, sem período", guardas, gu)
	}
	if gu.SegX[0] != g.SegX[0] || gu.SegY[0] != g.SegY[0] || g.SegX[0] != 872 || g.SegY[0] != 3876 {
		t.Errorf("chefe em (%d,%d) e guardas em (%d,%d), want os dois em (872,3876)", g.SegX[0], g.SegY[0], gu.SegX[0], gu.SegY[0])
	}
	if o := gens[outra]; o.Leader != "Gargula_Sabio" || o.Follower != "Golem_de_Fogo" {
		t.Errorf("bloco %d = %s/%s, want a outra Gárgula Sábio intocada", outra, o.Leader, o.Follower)
	}
	for idx, g := range gens {
		for _, n := range []string{g.Leader, g.Follower} {
			if (n == "Gargula_Sabio_Chefe" && idx != chefe) || (n == "Golem_Guarda" && idx != guardas) {
				t.Errorf("%s no bloco %d: a cópia é só do bloco da foto", n, idx)
			}
		}
	}
}

// TestSalaGolemDeFogoBlocos pins the Golem de Fogo room of the Dungeon's 2nd floor
// (migration 0152): ONLY the room in the photo, x 841-868 × y 3913-3941. Its six
// blocks use the room's template copies at 3x, each born as a full group of 6
// with no minute period — the individual queue brings each one back in 10 s
// (handler/dungeon_lava.go) —, and the Boss Golem de Fogo, block 6163, the last,
// stands inside it.
func TestSalaGolemDeFogoBlocos(t *testing.T) {
	path := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "NPCGener.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("NPCGener.txt unavailable: %v", err)
	}
	gens, err := LoadNPCGenerators(path)
	if err != nil {
		t.Fatal(err)
	}
	// Era o último bloco até 26/09/2026; o Ciclope Tirano (6164) entrou depois dele.
	const chefe = 6163
	if len(gens) <= chefe {
		t.Fatalf("NPCGener has %d blocks, want block %d", len(gens), chefe)
	}
	naSala := func(x, y int16) bool { return x >= 841 && x <= 868 && y >= 3913 && y <= 3941 }
	g := gens[chefe]
	if g.Leader != "Boss_Golem_Fogo" || g.MinuteGenerate != -1 || g.MaxNumMob != 1 || g.MinGroup != 0 || g.MaxGroup != 0 {
		t.Errorf("bloco %d = %+v, want one Boss_Golem_Fogo with MinuteGenerate -1", chefe, g)
	}
	if !naSala(g.SegX[0], g.SegY[0]) {
		t.Errorf("Boss_Golem_Fogo nasce em (%d,%d), fora da sala", g.SegX[0], g.SegY[0])
	}
	blocos := map[int]string{2515: "Golem_Fogo_Lava", 2516: "Golem_Fogo_Lava", 2517: "Golem_Fogo_Lava", 2520: "Golem_Fogo_Lava", 2496: "Gargula_Lava", 2497: "Gargula_Lava"}
	mobs := map[string]int{}
	for idx, g := range gens {
		nome, daSala := blocos[idx]
		for _, n := range []string{g.Leader, g.Follower} {
			if (n == "Golem_Fogo_Lava" || n == "Gargula_Lava") && !daSala {
				t.Errorf("%s no bloco %d: a cópia é só da sala", n, idx)
			}
		}
		if !daSala {
			continue
		}
		if g.Leader != nome || g.Follower != nome || g.MinuteGenerate != -1 || g.MaxNumMob != 6 || g.MinGroup != 5 || g.MaxGroup != 5 {
			t.Errorf("bloco %d = %+v, want um grupo cheio de 6 %s sem período", idx, g, nome)
		}
		if !naSala(g.SegX[0], g.SegY[0]) {
			t.Errorf("bloco %d em (%d,%d), fora da sala", idx, g.SegX[0], g.SegY[0])
		}
		mobs[nome] += g.MaxNumMob
	}
	// 8 Golens de Fogo e 4 Gárgulas antes da 0152: 3x.
	if mobs["Golem_Fogo_Lava"] != 24 || mobs["Gargula_Lava"] != 12 {
		t.Errorf("sala com %v, want 24 Golens de Fogo e 12 Gárgulas", mobs)
	}
}
