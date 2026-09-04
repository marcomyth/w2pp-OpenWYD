package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The Pergaminho da Água dungeon (_MSG_UseItem.cpp:1726/1920/2025,
// MobKilled.cpp:1560-1740, ProcessSecMinTimer.cpp:1576-1595).
//
// Three parallel dungeons — N, M and A — of eight rooms plus a boss room. The
// scroll's EF_VOLATILE selects the room; using it teleports the party leader and
// the whole party in, starts a countdown and spawns the room's monsters. Clearing
// the room BEFORE the countdown expires hands the leader the next scroll, which
// is the only way to obtain it: only the LV1 scroll of each chain drops from
// mobs, LV2..LV8 and the boss summon come exclusively from clearing the previous
// room. Letting the timer run out teleports everyone back out empty-handed — the
// scroll was already consumed on entry.
//
// The legacy numbers rooms 0..9 ("Sala"); rooms 0..7 are the LV1..LV8 scrolls and
// room 9 is the boss. Room 8 exists in the tables but is unreachable: no catalog
// item carries the EF_VOLATILE that selects it, and the legacy spawns nothing for
// it. It is kept in the position table only so the indices line up.

const (
	waterN = 0
	waterM = 1
	waterA = 2

	// waterBossRoom is the legacy "Sala 9": the shared centre room, entered with
	// the Evocação Neses summon rather than a numbered scroll.
	waterBossRoom = 9

	// waterDeadRoom is the unreachable room 8 (see the package note above).
	waterDeadRoom = 8
)

// waterScrollPosition is WaterScrollPosition[3][10][2] (Server.cpp:342): the
// centre of every room, by variant then room. Entries 8 and 9 repeat the same
// centre in the original.
var waterScrollPosition = [3][10][2]int16{
	waterN: {
		{1121, 3554}, {1085, 3554}, {1049, 3554}, {1049, 3518}, {1049, 3482},
		{1085, 3482}, {1121, 3482}, {1121, 3518}, {1085, 3518}, {1085, 3518},
	},
	waterM: {
		{1250, 3682}, {1214, 3682}, {1178, 3682}, {1178, 3646}, {1178, 3610},
		{1214, 3610}, {1250, 3610}, {1250, 3646}, {1214, 3646}, {1214, 3646},
	},
	waterA: {
		{1379, 3554}, {1340, 3554}, {1305, 3554}, {1305, 3518}, {1305, 3482},
		{1341, 3482}, {1377, 3482}, {1377, 3518}, {1343, 3518}, {1343, 3518},
	},
}

// waterVariant binds one dungeon chain to its catalog and content ids.
type waterVariant struct {
	volLo, volHi int   // EF_VOLATILE of the LV1..LV8 scrolls
	volBoss      int   // EF_VOLATILE of the Evocação Neses summon
	genBase      int   // NPCGener block of room 0; +8..+11 are the boss blocks
	rewardBase   int16 // item handed out for clearing room 0 (i.e. the LV2 scroll)
}

// waterVariants indexes the three chains. rewardBase+room lands on the boss
// summon for room 7, which is what makes LV8 the last numbered room
// (MobKilled.cpp:1587/1660/1733).
var waterVariants = [3]waterVariant{
	waterN: {volLo: 131, volHi: 138, volBoss: 140, genBase: world.WaterGenBaseN, rewardBase: 3174},
	waterM: {volLo: 21, volHi: 28, volBoss: 30, genBase: world.WaterGenBaseM, rewardBase: 778},
	waterA: {volLo: 161, volHi: 168, volBoss: 170, genBase: world.WaterGenBaseA, rewardBase: 3183},
}

const (
	// waterTickPeriod is the legacy cadence: ProcessSecMinTimer runs on a 500ms
	// timer and the water countdown fires on SecCounter%4 — every 2 seconds. Our
	// tick is 1s, so the period is 2 and one unit of the counter is 2 seconds.
	waterTickPeriod = 2

	// Countdown units, in 2-second steps (_MSG_UseItem.cpp:1778).
	waterRoomTime = 30 // 60s to clear a numbered room
	waterBossTime = 15 // 30s in the boss room

	// Once the room is cleared the remaining time is CUT, not extended: the
	// party gets a short window to move on (MobKilled.cpp:1573/1614).
	waterRoomClearTime = 15 // 30s
	waterBossClearTime = 5  // 10s

	// waterRoomRadius / waterBossRadius are the half-sides of a room box
	// (_MSG_UseItem.cpp:1734-1747).
	waterRoomRadius = 8
	waterBossRadius = 12

	// waterStagingTileX / Y are the staging square outside the rooms, where the
	// chain is started: the legacy accepts the scroll there even though the
	// player is not inside any room (`TargetX/4 == 491 && TargetY/4 == 443`,
	// _MSG_UseItem.cpp:1750). Integer division, so this is a 4x4 tile.
	waterStagingTileX = 491
	waterStagingTileY = 443
)

// waterExit is the ClearAreaTeleport destination for an expired room
// (ProcessSecMinTimer.cpp:1586).
var waterExit = [2]int16{1965, 1769}

// waterMCelestialMaxLevel caps the Celestial's access to the M chain. Arch has
// no cap of its own — MaxLevel (399) is already the ceiling for that tier.
const waterMCelestialMaxLevel = 40

// waterClassAllowed gates each chain to a progression tier.
//
// DELIBERATE DIVERGENCE: the legacy has no class gate at all — any character can
// open any water room. This is a server rule, scoping each chain so a maxed
// character cannot farm the entry-tier dungeon:
//
//	N  Mortal only.
//	M  Arch at any level, plus Celestial up to level 40.
//	A  every celestial tier (Celestial, CelestialCS, SCelestial), no cap.
func waterClassAllowed(variant int, classMaster uint8, level int32) bool {
	switch variant {
	case waterN:
		return classMaster == classMasterMortal
	case waterM:
		if classMaster == classMasterArch {
			return true
		}
		return classMaster == classMasterCelestial && level <= waterMCelestialMaxLevel
	case waterA:
		return isCelestialTier(classMaster)
	}
	return false
}

// waterRoomForVolatile maps an EF_VOLATILE to its dungeon and room. The dead
// room 8 is deliberately unmapped: no item selects it.
func waterRoomForVolatile(vol int) (variant, room int, ok bool) {
	for v := range waterVariants {
		d := waterVariants[v]
		switch {
		case vol >= d.volLo && vol <= d.volHi:
			return v, vol - d.volLo, true
		case vol == d.volBoss:
			return v, waterBossRoom, true
		}
	}
	return 0, 0, false
}

// isWaterScrollVolatile reports whether an EF_VOLATILE belongs to a water scroll,
// so the use-item dispatch can route it here.
func isWaterScrollVolatile(vol int) bool {
	_, _, ok := waterRoomForVolatile(vol)
	return ok
}

// waterRoomRadiusFor returns the box half-side of one room.
func waterRoomRadiusFor(room int) int16 {
	if room >= waterDeadRoom {
		return waterBossRadius
	}
	return waterRoomRadius
}

// insideAnyWaterRoom mirrors the entry gate (_MSG_UseItem.cpp:1732-1748): the
// caller must stand inside ANY room of that dungeon, not specifically the one
// being opened — that is what lets a cleared party open the next room from where
// they are.
func insideAnyWaterRoom(variant int, x, y int16) bool {
	for room := 0; room < len(waterScrollPosition[variant]); room++ {
		if inWaterRoomBox(variant, room, x, y) {
			return true
		}
	}
	return false
}

// inWaterRoomBox reports whether (x,y) is inside one room's box.
func inWaterRoomBox(variant, room int, x, y int16) bool {
	c := waterScrollPosition[variant][room]
	r := waterRoomRadiusFor(room)
	return abs16(x-c[0]) <= int(r) && abs16(y-c[1]) <= int(r)
}

// onWaterStagingTile reports whether the caller stands on the staging square
// that starts a chain from outside the rooms.
func onWaterStagingTile(x, y int16) bool {
	return x/4 == waterStagingTileX && y/4 == waterStagingTileY
}

// waterRoomBusy reports whether anyone is standing in the target room, and the
// name of one of them. Like the legacy GetUserInArea (Server.cpp:4612) it does
// NOT exclude the caller: opening the room you are already standing in is
// refused, which is why a chain starts from the staging tile.
func (d *Dispatcher) waterRoomBusy(w *world.World, variant, room int) (string, bool) {
	name, busy := "", false
	w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
		if busy || !inWaterRoomBox(variant, room, e.X, e.Y) {
			return
		}
		name, busy = e.Name, true
	})
	return name, busy
}

// refuseWaterScroll answers a rejected use: notify, then resend the slot so the
// client never shows the scroll as consumed (the legacy SendItem on every
// refusal path, _MSG_UseItem.cpp:1753/1762/1775).
func (d *Dispatcher) refuseWaterScroll(w *world.World, s *world.Session, e *world.Entity, src int, n Notice) {
	d.notify(w, s, n)
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
}

// useWaterScroll ports the Pergaminho da Água entry (_MSG_UseItem.cpp:1726 and
// its N/A twins). The three gates run in the legacy order — area, party leader,
// room occupancy — and each refusal returns the scroll uneaten.
func (d *Dispatcher) useWaterScroll(w *world.World, s *world.Session, e *world.Entity, src, vol int) {
	variant, room, ok := waterRoomForVolatile(vol)
	if !ok {
		d.rejectUnimplementedConsumable(w, s, e, src)
		return
	}
	// Logged on EVERY attempt, before the gates. The notify() wire format is
	// still a placeholder (notice.go), so a refused scroll looks exactly like a
	// dead handler on the client: this line is the only way to tell "the gate
	// said no" from "this code never ran".
	d.log.Info("water scroll attempt",
		"account", s.AccountName, "vol", vol, "variant", variant, "room", room,
		"x", e.X, "y", e.Y, "leader", e.Leader)

	// Gate 1: inside the dungeon, or on the staging tile that starts a chain.
	if !insideAnyWaterRoom(variant, e.X, e.Y) && !onWaterStagingTile(e.X, e.Y) {
		d.log.Info("water scroll refused: outside the dungeon",
			"account", s.AccountName, "x", e.X, "y", e.Y, "variant", variant)
		d.refuseWaterScroll(w, s, e, src, NoticeCantUseHere)
		return
	}
	// Gate 1b: the chain is scoped to a progression tier (server rule, see
	// waterClassAllowed). It sits AFTER the area check so someone nowhere near
	// the dungeon is told where to stand rather than which class to be.
	if !waterClassAllowed(variant, e.ClassMaster, e.Level) {
		d.log.Info("water scroll refused: class not allowed in this chain",
			"account", s.AccountName, "variant", variant, "classMaster", e.ClassMaster, "level", e.Level)
		d.refuseWaterScroll(w, s, e, src, NoticeWaterClassNotAllowed)
		return
	}
	// Gate 2: party members cannot open a room; only the leader (Leader == 0)
	// may, and the whole party rides along.
	if e.Leader != 0 {
		d.log.Info("water scroll refused: not the party leader",
			"account", s.AccountName, "leader", e.Leader)
		d.refuseWaterScroll(w, s, e, src, NoticePartyLeaderOnly)
		return
	}
	// Gate 3: the room must be empty.
	if occupant, busy := d.waterRoomBusy(w, variant, room); busy {
		d.log.Info("water scroll refused: room busy",
			"account", s.AccountName, "variant", variant, "room", room, "occupant", occupant)
		d.refuseWaterScroll(w, s, e, src, NoticeSomeoneOnQuest)
		return
	}

	// Arm the countdown before moving anyone, so the timer the party sees and the
	// one the tick decrements are the same value.
	countdown := uint8(waterRoomTime)
	if room >= waterDeadRoom {
		countdown = waterBossTime
	}
	d.events.water[variant][room] = countdown
	// A fresh run: this room owes exactly one reward again.
	d.events.waterPaid[variant][room] = false

	dest := waterScrollPosition[variant][room]
	d.enterWaterRoom(w, s, e, dest, countdown)

	// The room's monsters. Numbered rooms get their own block twice (the legacy
	// tops the block up to MaxNumMob with two calls); the boss room draws one of
	// four blocks with the legacy's weights: 40% +8, 10% +9, 10% +10, 40% +11.
	base := waterVariants[variant].genBase
	if room < waterDeadRoom {
		d.revealSpawned(w, w.GenerateMob(base+room))
		d.revealSpawned(w, w.GenerateMob(base+room))
	} else {
		d.revealSpawned(w, w.GenerateMob(base+waterBossBlock(w.Rand().Intn(10))))
	}

	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	d.log.Info("water scroll used",
		"account", s.AccountName, "variant", variant, "room", room, "countdown", countdown)
}

// waterBossBlock maps a 0..9 roll to a boss block offset
// (_MSG_UseItem.cpp:1802-1815).
func waterBossBlock(roll int) int {
	switch {
	case roll < 4:
		return 8
	case roll < 5:
		return 9
	case roll < 6:
		return 10
	default:
		return 11
	}
}

// enterWaterRoom teleports the leader and every online party member to the room
// and pushes the countdown to each of them. The parm is the countdown in
// SECONDS: one unit is 2s, which is exactly the legacy's `WaterClear1 * 2`.
func (d *Dispatcher) enterWaterRoom(w *world.World, s *world.Session, e *world.Entity, dest [2]int16, countdown uint8) {
	d.doTeleport(w, s, dest[0], dest[1])
	d.sendWaterCountdown(w, s, countdown)
	for i := 0; i < world.MaxParty; i++ {
		member := e.PartyList[i]
		if member <= 0 || member == e.ID {
			continue
		}
		ms := w.Session(member)
		if ms == nil || ms.Mode != world.UserPlay {
			continue
		}
		d.doTeleport(w, ms, dest[0], dest[1])
		d.sendWaterCountdown(w, ms, countdown)
	}
}

// sendWaterCountdown pushes the room timer to one client (MSG_StartTime, the
// same signal the castle quest uses).
func (d *Dispatcher) sendWaterCountdown(w *world.World, s *world.Session, countdown uint8) {
	body := protocol.EncodeStandardParm(int32(countdown) * 2)
	w.SendTo(s, protocol.Header{Type: protocol.MsgStartTime, ID: protocol.IDScene}, body)
}

// waterRoomCleared is the mob-death hook (MobKilled.cpp:1565-1600). It fires as
// the LAST mob of a water block dies — CurrentNumMob is still 1 here, because
// mobKilled runs before DespawnMob decrements it.
//
// Clearing a numbered room hands the party LEADER the next scroll of the chain;
// the boss room grants no item (its reward is the boss's own loot). Either way
// the countdown is cut short.
func (d *Dispatcher) waterRoomCleared(w *world.World, reward, mob *world.Entity) {
	variant, room, ok := waterBlockRoom(int(mob.GenIndex))
	if !ok {
		return
	}
	gen := w.GeneratorAt(int(mob.GenIndex))
	if gen == nil || gen.CurrentNumMob != 1 {
		return // not the last one down yet
	}
	if !d.claimWaterReward(variant, room) {
		d.log.Info("water room already rewarded this run; skipping payout",
			"variant", variant, "room", room)
		return
	}

	leader := reward
	if reward.Leader != 0 {
		if le := w.Entity(reward.Leader); le != nil {
			leader = le
		}
	}

	cut := uint8(waterRoomClearTime)
	if room >= waterDeadRoom {
		cut = waterBossClearTime
	}
	if d.events.water[variant][room] > cut {
		d.events.water[variant][room] = cut
	}

	if room < waterDeadRoom {
		d.grantNextWaterScroll(w, leader, variant, room)
	}
	d.broadcastWaterCountdown(w, leader, d.events.water[variant][room])
	d.log.Info("water room cleared",
		"variant", variant, "room", room, "leader", leader.Name, "remaining", d.events.water[variant][room])
}

// claimWaterReward takes the single payout a room run owes, returning false if
// the run already paid.
//
// The clear is detected from the block's population reaching its last mob, and
// that condition can be met more than once per run: a respawn, the second
// GenerateMob of entry, or mobs left over from an expired run all bring the
// count back down. Charging the payout to the RUN rather than to the count is
// what keeps one scroll from minting many.
func (d *Dispatcher) claimWaterReward(variant, room int) bool {
	if d.events.waterPaid[variant][room] {
		return false
	}
	d.events.waterPaid[variant][room] = true
	return true
}

// grantNextWaterScroll puts the next scroll of the chain in the leader's bag.
// A full inventory silently drops it, exactly like the legacy PutItem.
func (d *Dispatcher) grantNextWaterScroll(w *world.World, leader *world.Entity, variant, room int) {
	next := world.Item{Index: waterVariants[variant].rewardBase + int16(room)}
	slot := firstEmptyAccessibleCarry(leader)
	if slot < 0 {
		d.log.Warn("water reward lost: leader inventory full", "leader", leader.Name, "item", next.Index)
		return
	}
	leader.Carry[slot] = next
	if ls := w.Session(leader.ID); ls != nil {
		d.sendSlot(w, ls, world.ItemPlaceCarry, slot, next)
	}
}

// broadcastWaterCountdown re-pushes the timer to the leader and its party.
func (d *Dispatcher) broadcastWaterCountdown(w *world.World, leader *world.Entity, countdown uint8) {
	if ls := w.Session(leader.ID); ls != nil {
		d.sendWaterCountdown(w, ls, countdown)
	}
	for i := 0; i < world.MaxParty; i++ {
		member := leader.PartyList[i]
		if member <= 0 || member == leader.ID {
			continue
		}
		if ms := w.Session(member); ms != nil && ms.Mode == world.UserPlay {
			d.sendWaterCountdown(w, ms, countdown)
		}
	}
}

// waterBlockRoom maps an NPCGener block index back to its dungeon and room. The
// four boss blocks (+8..+11) all resolve to the boss room.
func waterBlockRoom(gen int) (variant, room int, ok bool) {
	if gen < 0 {
		return 0, 0, false
	}
	for v := range waterVariants {
		off := gen - waterVariants[v].genBase
		switch {
		case off >= 0 && off < waterDeadRoom:
			return v, off, true
		case off >= waterDeadRoom && off <= 11:
			return v, waterBossRoom, true
		}
	}
	return 0, 0, false
}

// tickWaterRooms is the countdown (ProcessSecMinTimer.cpp:1576-1595): every two
// seconds each armed room ticks down, and the one that reaches the end throws
// everyone still inside back to the dungeon entrance.
func (d *Dispatcher) tickWaterRooms(w *world.World) {
	if d.tickCount%waterTickPeriod != 0 {
		return
	}
	for variant := range d.events.water {
		for room := range d.events.water[variant] {
			switch t := d.events.water[variant][room]; {
			case t > 1:
				d.events.water[variant][room]--
			case t == 1:
				d.clearWaterRoom(w, variant, room)
				d.events.water[variant][room] = 0
			}
		}
	}
}

// clearWaterRoom is ClearAreaTeleport (Server.cpp:6388) for one expired room:
// everyone standing in it is moved out to the dungeon entrance.
func (d *Dispatcher) clearWaterRoom(w *world.World, variant, room int) {
	var evicted []*world.Session
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		if inWaterRoomBox(variant, room, e.X, e.Y) {
			evicted = append(evicted, s)
		}
	})
	// Teleport outside the iteration: doTeleport mutates position and view, which
	// ForEachPlaying is walking.
	for _, s := range evicted {
		d.doTeleport(w, s, waterExit[0], waterExit[1])
	}
	if len(evicted) > 0 {
		d.log.Info("water room expired", "variant", variant, "room", room, "evicted", len(evicted))
	}
}
