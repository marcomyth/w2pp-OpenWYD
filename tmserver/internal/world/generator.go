package world

// Per-generator mob (re)generation: the runtime port of GenerateMob
// (Server.cpp:3442-3810) and the CurrentNumMob accounting of DeleteMob
// (Server.cpp:7809-7843). Each NPCGener.txt block becomes a Generator; the AI
// tick fires GenerateMob on the block's MinuteGenerate cadence and death
// decrements its population, so farmed areas repopulate in groups the way the
// original world does. All of this is loop-only state.

import (
	"github.com/jeanluca/w2pp-openwyd/internal/mapaevento"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
)

// Generator is the runtime state of one NPCGener.txt block (NPCGENLIST,
// CNPCGene.h:29-51): the spawn recipe plus the live population counter.
type Generator struct {
	// Name is the block's Leader template name as NPCGener.txt writes it — what
	// /gm criar looks a template up by. Informational; spawning never reads it.
	Name           string
	DBManaged      bool // content merchant recipe must be supplied by npc_definition
	MinuteGenerate int  // respawn period in minutes; <=0 = the timer never regenerates
	MinGroup       int  // follower count rolled as MinGroup + rand()%(MaxGroup-MinGroup+1)
	MaxGroup       int
	MaxNumMob      int // population cap (leader and followers both count toward it)
	RouteType      uint8
	Formation      int
	SegX, SegY     [5]int16
	SegRange       [5]int16
	SegWait        [5]int16
	LeaderTmpl     []byte // raw 816-byte STRUCT_MOB; nil = generator unusable
	FollowerTmpl   []byte // nil = leader-only groups ("Follower: 0")
	CurrentNumMob  int    // live mobs from this block (SpawnMobAt ++ / DespawnMob --)
	FightAction    [4]string
	DieAction      [4]string
	// Off is a staff switch (/gm npc off, table npc_generator_off): the block
	// generates nothing — boot, minute timer, NPC overlay, GM command — until it
	// is switched back on. The recipe stays, so switching on needs nothing else.
	Off bool
	// ArenaRefill marks a Quest 256 arena block (handler/populacao_arenas.go): the
	// handler's own 12 s pass refills it, to a cap that grows with the players
	// inside, so the plain minute timer and the individual 15 s respawn queue both
	// leave it alone. Set once at boot; the content fields above stay as loaded.
	ArenaRefill bool

	// LeaderName and FollowerName are the template FILE names behind the two
	// byte blobs — the file Name resolved to, not Name as NPCGener.txt spells it —
	// carried onto every mob spawned from them (MobSpawn.TemplateName), which is
	// what the Mesa de Drops matches on.
	LeaderName   string
	FollowerName string
}

// generateWorldCap stops the generator timer from filling every entity slot:
// the head-room above it keeps the plain respawn queue (MinuteGenerate<=0
// blocks) from being starved by drop-on-full. Deliberate divergence — the
// original's only cap is MAX_MOB itself.
const generateWorldCap = 20000

// formationOffsets is g_pFormation[5][12][2] (Basedef.cpp:193), interpreted as
// [formation][follower slot][x/y]. The Server.cpp use site indexes the array in
// a decompiled-looking order, but NPCGener contains Formation=4 blocks, so the
// table's first dimension is the only shape that can represent the content.
var formationOffsets = [5][MaxParty]struct{ x, y int16 }{
	{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {2, 0}, {-2, 0}, {0, 2}, {0, -2}},
	{{1, 0}, {-1, 0}, {2, 0}, {-2, 0}, {3, 0}, {-3, 0}, {4, 0}, {-4, 0}, {5, 0}, {-5, 0}, {0, 6}, {0, -6}},
	{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {2, 0}, {-2, 0}, {0, 2}, {0, -2}},
	{{1, 0}, {-1, 0}, {2, 0}, {-2, 0}, {3, 0}, {-3, 0}, {4, 0}, {-4, 0}, {5, 0}, {-5, 0}, {0, 6}, {0, -6}},
	{{2, 0}, {0, 2}, {1, 1}, {0, 1}, {1, 0}, {-1, 3}, {3, -1}, {-1, 2}, {2, -1}, {-1, 1}, {1, -1}, {1, 2}},
}

// RegisterGenerators installs the generator table (index = NPCGener block =
// Entity.GenIndex). Wiring-time only, before Run.
func (w *World) RegisterGenerators(gens []*Generator) { w.generators = gens }

// GeneratorCount returns the number of registered generator slots (some may be
// nil — blocks whose templates failed to load).
func (w *World) GeneratorCount() int { return len(w.generators) }

// GeneratorAt returns the generator at idx, or nil. Loop-only (the caller may
// read live CurrentNumMob).
func (w *World) GeneratorAt(idx int) *Generator {
	if idx < 0 || idx >= len(w.generators) {
		return nil
	}
	return w.generators[idx]
}

// DBManagedGeneratorCount reports how many content slots require DB recipes.
func (w *World) DBManagedGeneratorCount() int {
	n := 0
	for _, g := range w.generators {
		if g != nil && g.DBManaged {
			n++
		}
	}
	return n
}

// A CLASSIFICAÇÃO DOS BLOCOS MUDOU DE CASA (21/09/2026), sem mudar de regra.
//
// As perguntas "isto é da Água?", "isto é do Kefra?", "isto é de evento?" viraram
// internal/npcgener: o painel precisa da MESMA resposta para dizer quais monstros
// nascem de verdade, e a regra de pacote interno do Go impede o webServer de
// importar daqui. Duas cópias da mesma pergunta é como a Sala Secreta entrou no
// modelo de XP como campo aberto em 16/09.
//
// O que fica aqui são apelidos: mesmo nome, mesma resposta, um lugar só. Quem
// chama não mudou. Para ver as faixas e o porquê de cada uma, npcgener/classe.go.
const (
	WaterGenBaseN = npcgener.WaterGenBaseN
	WaterGenBaseM = npcgener.WaterGenBaseM
	WaterGenBaseA = npcgener.WaterGenBaseA

	SecretRoomGenFirst = npcgener.SecretRoomGenFirst
	SecretRoomGenLast  = npcgener.SecretRoomGenLast

	CasteloOrcGenFirst = npcgener.CasteloOrcGenFirst
	CasteloOrcGenLast  = npcgener.CasteloOrcGenLast

	AcampamentoTrollGenFirst = npcgener.AcampamentoTrollGenFirst
	AcampamentoTrollGenLast  = npcgener.AcampamentoTrollGenLast

	SecretRoomStrayGenFirst = npcgener.SecretRoomStrayGenFirst
	SecretRoomStrayGenLast  = npcgener.SecretRoomStrayGenLast
)

// IsKefraGenerator reports whether an NPCGener block is the Kefra (396) or one
// of its guards (397-400). They come back weekly (handler/kefra.go), never
// through the 15 s queue: a boss that returns fifteen seconds after dying is not
// a weekly boss.
func IsKefraGenerator(idx int) bool { return npcgener.IsKefraGenerator(idx) }

// IsWaterDungeonGenerator reports whether an NPCGener block belongs to a
// Pergaminho da Água room.
//
// These blocks are spawned on demand, when a party opens the room with a scroll
// (handler/waterscroll.go), so the boot populate must skip them. Populating them
// up front — which is what this fork does for every other MinuteGenerate<=0
// block — would leave the rooms permanently occupied: entry refuses a non-empty
// room, and the clear reward fires on the last mob dying, which would never
// happen for monsters nobody was sent in to fight.
func IsWaterDungeonGenerator(idx int) bool { return npcgener.IsWaterDungeonGenerator(idx) }

// IsSecretRoomGenerator reports whether a block belongs to the Sala Secreta.
func IsSecretRoomGenerator(idx int) bool { return npcgener.IsSecretRoomGenerator(idx) }

// IsCasteloOrcGenerator reports whether a block belongs to the Castelo Orc quest.
func IsCasteloOrcGenerator(idx int) bool { return npcgener.IsCasteloOrcGenerator(idx) }

// IsAcampamentoTrollGenerator diz se um bloco é da quest do Acampamento Troll.
func IsAcampamentoTrollGenerator(idx int) bool { return npcgener.IsAcampamentoTrollGenerator(idx) }

// IsEventOwnedGenerator reports whether a block belongs to a scripted event
// rather than to the world population. O boot pula estes blocos e a morte não
// enfileira o respawn de 15 s — senão o cenário do evento fica de pé para sempre.
func IsEventOwnedGenerator(idx int) bool { return npcgener.IsEventOwnedGenerator(idx) }

// ClearGenerator removes every live entity and queued respawn owned by one
// generator slot before a DB snapshot replaces its recipe. Loop-only.
func (w *World) ClearGenerator(idx int) {
	if idx < 0 || idx >= len(w.generators) {
		return
	}
	for id := MaxUser; id < MaxMob; id++ {
		if e := w.entities[id]; e != nil && int(e.GenIndex) == idx {
			w.DespawnMob(id, 0)
		}
	}
	kept := w.respawnQueue[:0]
	for _, entry := range w.respawnQueue {
		if int(entry.spawn.GenIndex) != idx {
			kept = append(kept, entry)
		}
	}
	w.respawnQueue = kept
	if g := w.generators[idx]; g != nil {
		g.CurrentNumMob = 0
	}
}

// SpawnGeneratorLeader spawns exactly one generator leader at its first
// waypoint without consuming RNG. Scripted world events use this instead of
// GenerateMob so they cannot perturb the combat/drop parity stream.
func (w *World) SpawnGeneratorLeader(idx int) int {
	g := w.GeneratorAt(idx)
	if g == nil || g.LeaderTmpl == nil {
		return -1
	}
	x, y := g.SegX[0], g.SegY[0]
	if x == 0 {
		for i := range g.SegX {
			if g.SegX[i] != 0 {
				x, y = g.SegX[i], g.SegY[i]
				break
			}
		}
	}
	if x == 0 {
		return -1
	}
	x, y, ok := w.emptyCellNear(x, y)
	if !ok {
		return -1
	}
	sp := MobSpawn{Template: g.LeaderTmpl, TemplateName: g.LeaderName, X: x, Y: y, RouteType: g.RouteType, GenIndex: int16(idx)}
	sp.SegX, sp.SegY, sp.SegWait = g.SegX, g.SegY, g.SegWait
	return w.SpawnMobAt(sp)
}

// GenerateMob spawns one group (leader + rolled followers) from generator idx —
// the port of GenerateMob (Server.cpp:3442-3810). Returns the spawned ids (the
// leader first) so the caller can reveal them to in-view players. Loop-only.
//
// Rand-call order is parity-relevant (the original burns the same global
// rand()): (1) the group-size roll happens BEFORE the population check
// (Server.cpp:3489-3491); (2) two rolls per set waypoint with a range, X then Y
// (Server.cpp:3536-3546, offset biased toward −Range as in the original);
// (3) one roll per spawned mob whose template Clan is 1 (`rand()%10==1` demotes
// it to clan 2, Server.cpp:3624 — short-circuit: no roll for other clans).
//
// Kept quirk: the follower clamp ignores the leader, so a MaxNumMob=1 block
// still spawns leader+1 follower and sits at CurrentNumMob=2 until both die.
// Not ported: the MinuteGenerate>=500 relocation hack, the Coliseum-rectangle
// disable and event hooks (BrState/GTORRE) — event systems, out of scope.
func (w *World) GenerateMob(idx int) []int {
	return w.generateMob(idx, false, 0, 0, 0)
}

// GenerateMobUpTo is GenerateMob against a cap the caller gives instead of the
// block's MaxNumMob, and the cap is exact: the group is cut so that leader plus
// followers never pass it. The arena population (handler/populacao_arenas.go)
// raises an arena block above its content cap while players are inside; the
// plain GenerateMob keeps the legacy count, which lets a group overshoot the cap
// by its leader.
func (w *World) GenerateMobUpTo(idx, limit int) []int {
	if limit <= 0 {
		return nil
	}
	return w.generateMob(idx, false, 0, 0, limit)
}

// GenerateMobNear is GenerateMob with the group raised around (x, y) instead of
// the block's own start: every waypoint moves by the same offset, so the mob
// keeps its route shape and leash, only anchored where the caller stands. It is
// the legacy GM "generate", which passed the GM's position to GenerateMob
// (imple.cpp:663-673) — how staff calls a boss to where the event is.
func (w *World) GenerateMobNear(idx int, x, y int16) []int {
	return w.generateMob(idx, true, x, y, 0)
}

// geradorEmMapaDeEvento diz se o ponto de partida do bloco — o primeiro waypoint
// preenchido, a mesma âncora que generateMob usa — fica num mapa de evento.
func geradorEmMapaDeEvento(g *Generator) bool {
	for i := range g.SegX {
		if g.SegX[i] != 0 {
			return mapaevento.Contem(int(g.SegX[i]), int(g.SegY[i]))
		}
	}
	return false
}

// generateMob spawns one group. limit > 0 replaces MaxNumMob with an exact cap
// (GenerateMobUpTo); 0 is the legacy count.
func (w *World) generateMob(idx int, near bool, nearX, nearY int16, limit int) []int {
	g := w.GeneratorAt(idx)
	if g == nil || g.LeaderTmpl == nil || g.Off {
		return nil
	}
	// Mapa guardado para evento (internal/mapaevento): nenhum bloco põe mob lá —
	// nem o boot, nem o relógio de minuto, nem a tabela de NPCs, nem o "gerar
	// <bloco>". Só o "gerar <bloco> aqui" passa, porque é o GM trazendo o grupo
	// para onde ele está, que é justamente como se monta um evento. Testado antes
	// de qualquer rand(), para não mexer na sequência dos outros blocos.
	if !near && geradorEmMapaDeEvento(g) {
		return nil
	}
	qmob := g.MaxGroup - g.MinGroup + 1
	if qmob <= 0 {
		qmob = 1 // "err,zero divide" guard (Server.cpp:3486)
	}
	n := g.MinGroup + w.rng.Intn(qmob)
	maxNumMob := g.MaxNumMob
	if maxNumMob < 0 {
		// NPCGener uses -1 on at least Mestre_Grifo. The old bootstrap policy
		// populates static merchant blocks up front, so keep one live instance
		// instead of treating the negative cap as already saturated.
		maxNumMob = 1
	}
	if limit > 0 {
		maxNumMob = limit
	}
	if g.CurrentNumMob >= maxNumMob {
		return nil
	}
	if limit > 0 {
		// The leader counts too: cut the followers so the group fits exactly.
		n = min(n, maxNumMob-g.CurrentNumMob-1)
	} else if g.CurrentNumMob+n > maxNumMob {
		n = maxNumMob - g.CurrentNumMob
	}
	if w.mobCount >= generateWorldCap {
		return nil
	}

	sp := MobSpawn{Template: g.LeaderTmpl, TemplateName: g.LeaderName, RouteType: g.RouteType, GenIndex: int16(idx)}
	for i := 0; i < 5; i++ {
		if g.SegX[i] == 0 {
			continue
		}
		sp.SegX[i], sp.SegY[i] = g.SegX[i], g.SegY[i]
		sp.SegWait[i] = g.SegWait[i]
		if r := int(g.SegRange[i]); r > 0 {
			sp.SegX[i] = g.SegX[i] - int16(r) + int16(w.rng.Intn(r+1))
			sp.SegY[i] = g.SegY[i] - int16(r) + int16(w.rng.Intn(r+1))
		}
	}
	baseX, baseY := sp.SegX[0], sp.SegY[0]
	if baseX == 0 { // no Start waypoint: anchor on any set one
		for i := range sp.SegX {
			if sp.SegX[i] != 0 {
				baseX, baseY = sp.SegX[i], sp.SegY[i]
				break
			}
		}
	}
	if baseX == 0 {
		return nil // generator without a position
	}
	if near {
		dx, dy := nearX-baseX, nearY-baseY
		for i := range sp.SegX {
			if sp.SegX[i] != 0 {
				sp.SegX[i] += dx
				sp.SegY[i] += dy
			}
		}
		baseX, baseY = nearX, nearY
	}

	x, y, ok := w.emptyCellNear(baseX, baseY)
	if !ok {
		return nil // "err,No empty mobgrid" (Server.cpp:3631)
	}
	sp.X, sp.Y = x, y
	leaderID := w.SpawnMobAt(sp)
	if leaderID < 0 {
		return nil
	}
	le := w.entities[leaderID]
	if le.Clan == 1 && w.rng.Intn(10) == 1 {
		le.Clan = 2
	}
	ids := []int{leaderID}

	if g.FollowerTmpl == nil {
		return ids
	}
	for i := 0; i < n && i < MaxParty; i++ {
		fsp := followerSpawn(sp, g, i)
		fsp.Template = g.FollowerTmpl
		fsp.TemplateName = g.FollowerName
		fbaseX, fbaseY := spawnAnchor(fsp, baseX, baseY)
		fx, fy, fok := w.emptyCellNear(fbaseX, fbaseY)
		if !fok {
			break
		}
		fsp.X, fsp.Y = fx, fy
		fid := w.SpawnMobAt(fsp)
		if fid < 0 {
			break
		}
		fe := w.entities[fid]
		fe.Leader = leaderID // group link: followers never self-aggro (CMob.cpp:158)
		le.PartyList[i] = fid
		if fe.Clan == 1 && w.rng.Intn(10) == 1 {
			fe.Clan = 2
		}
		ids = append(ids, fid)
	}
	return ids
}

func spawnAnchor(sp MobSpawn, fallbackX, fallbackY int16) (int16, int16) {
	if sp.SegX[0] != 0 {
		return sp.SegX[0], sp.SegY[0]
	}
	for i := range sp.SegX {
		if sp.SegX[i] != 0 {
			return sp.SegX[i], sp.SegY[i]
		}
	}
	return fallbackX, fallbackY
}

func followerSpawn(leader MobSpawn, g *Generator, slot int) MobSpawn {
	sp := leader
	if g == nil || g.Formation < 0 || g.Formation >= len(formationOffsets) || slot < 0 || slot >= MaxParty {
		return sp
	}
	off := formationOffsets[g.Formation][slot]
	for i := 0; i < len(sp.SegX); i++ {
		if sp.SegX[i] == 0 {
			continue
		}
		if g.SegRange[i] != 0 {
			sp.SegX[i] = leader.SegX[i] + off.x
			sp.SegY[i] = leader.SegY[i] + off.y
		} else {
			sp.SegX[i] = g.SegX[i]
			sp.SegY[i] = g.SegY[i]
		}
	}
	return sp
}

// emptyCellNear finds a mob-free grid cell at or around (x,y), scanning the
// expanding boxes GetEmptyMobGrid uses (GetFunc.cpp:2027, rings 1..3). The
// original also rejects height-127 cells; the walkability grid lives on the
// handler side, so that check is skipped here — spawn anchors come from the
// curated NPCGener data (UNVERIFIED that no generator anchors on blocked
// ground).
func (w *World) emptyCellNear(x, y int16) (int16, int16, bool) {
	if _, ok := w.grid.MobAt(int(x), int(y)); !ok && w.grid.inBounds(int(x), int(y)) {
		return x, y, true
	}
	for ring := 1; ring <= 3; ring++ {
		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				cx, cy := int(x)+dx, int(y)+dy
				if !w.grid.inBounds(cx, cy) {
					continue
				}
				if _, ok := w.grid.MobAt(cx, cy); !ok {
					return int16(cx), int16(cy), true
				}
			}
		}
	}
	return 0, 0, false
}
