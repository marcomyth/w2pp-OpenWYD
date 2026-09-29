package handler

import (
	"fmt"
	"math"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Kill-streak clamps (GetFunc.cpp SetCurKill/SetTotKill).
const (
	maxCurKill = 200
	maxTotKill = 32767
)

// pkPointMin/pkPointMax are the legacy SetPKPoint clamp bounds (GetFunc.cpp:
// value<1→1, value>150→150); 0 is reserved to mean "never persisted" (see
// completeCharacterLogin).
const (
	pkPointMin uint8 = 1
	pkPointMax uint8 = 150
)

// pvpKilled runs the death penalties for a player slain by another player
// (MobKilled.cpp "#pragma region PvP", issue #210).
//
// The legacy branch has a brace-less `if (killed_x == 1 && killed_y == 31)` right
// before the arena test (MobKilled.cpp:3135) that swallows the whole block below
// it, so the compiled legacy moved nothing on a PvP death outside that one map
// block. It reads as a deleted `ZoneUnk = 1;`, and this port follows what the
// block says, not that accident:
//
//   - inside a city (BASE_GetVillage) or a guild-war arena (BASE_GetArena) no
//     experience is lost (_NN_In_Arena_No_Exp_Loss). Armia's PvP arena (the 0x40
//     tiles at 2124-2147 x 2140-2155) sits inside Armia's city rectangle;
//   - elsewhere the loss goes to Hold, a debt the next kills pay before any
//     experience fills the bar (payHold) — the bar itself is never lowered, so
//     no one drops a level (MobKilled.cpp:3248-3263).
//
// Deliberate divergence, asked by Marco on 29/09/2026: a kill in those zones also
// moves no chaos points on either side. The legacy arena branch still charged the
// killer (MobKilled.cpp:3214-3230). The kill streak counts everywhere, as in the
// legacy arena branch.
//
// Deferred/UNVERIFIED (documented, not ported — see the plan for issue #210):
//   - Castle-siege/RvR AtWar bypasses — those systems are themselves UNVERIFIED;
//     only the guild-war and Tower War bypasses are implemented.
//   - SameClan (legacy: clan 7 vs clan 8, an undocumented kingdom-war NPC-faction
//     pair, NOT "same guild") — unmodeled anywhere in this repo, treated as
//     always false: a normal kill always increments the killer's streak/resets
//     the victim's.
//   - The item 548/549 "triple the PKPoint penalty" rule — unidentified
//     equipment, not chased.
//   - #ifdef PKDrop item-loss-on-death — disabled in the reference build
//     (Basedef.h:89, `//#define PKDrop`); intentionally not ported.
func (d *Dispatcher) pvpKilled(w *world.World, killer, victim *world.Entity) {
	ks := w.Session(killer.ID)
	vs := w.Session(victim.ID)
	semPerda := pvpZonaSemPerda(victim.X, victim.Y)
	// A kill inside the Tower War box while it is open is a war kill, like the
	// legacy's AtWar (MobKilled.cpp:3124-3125): it moves no chaos points.
	atWar := d.guildsAtWar(killer.Guild, victim.Guild) || d.towerPvP(killer, victim) || semPerda

	switch {
	case semPerda:
		if vs != nil {
			d.sendChatText(w, vs, "Na arena nao ha perda de EXP.")
		}
	case pvpSemPerdaPorNivel(victim):
		// _NN_Below_lv20_No_Exp_Loss: the string says 20, the gate is FREEEXP (35).
		if vs != nil {
			d.sendChatText(w, vs, "Abaixo do nivel 35 nao ha perda de EXP.")
		}
	default:
		if loss := pvpExpLoss(victim.Level, victim.ClassMaster, victim.PKPoint); loss > 0 {
			victim.Hold = somaHold(victim.Hold, min(loss, victim.Exp))
			if vs != nil {
				d.sendChatText(w, vs, fmt.Sprintf("Voce perdeu %d pontos de experiencia", loss))
				d.sendEtc(w, vs, victim)
			}
		}
	}

	if lost := pvpLostPk(victim.PKPoint, pkGuilty(victim), atWar); lost != 0 {
		killer.PKPoint = clampPKPoint(int(killer.PKPoint) + lost)
		if ks != nil {
			d.sendChatText(w, ks, notifyPKPointDelta(killer.PKPoint, lost))
		}
	}
	if victim.PKPoint < pkPointNeutral && !atWar {
		victim.PKPoint++
		if vs != nil {
			d.sendChatText(w, vs, notifyPKPointDelta(victim.PKPoint, 1))
		}
	}

	// Kill-streak bookkeeping (SameClan treated as always false — see doc above).
	if killer.CurKill < maxCurKill {
		killer.CurKill++
	}
	if killer.TotKill < maxTotKill {
		killer.TotKill++
	}
	victim.CurKill = 0

	if ks != nil {
		d.broadcastPKState(w, ks, killer)
	}
	if vs != nil {
		d.broadcastPKState(w, vs, victim)
	}
}

// pvpZonaSemPerda is the legacy `arena != 5 || village != 5` (MobKilled.cpp:3137),
// tested on the victim's tile.
func pvpZonaSemPerda(x, y int16) bool {
	return world.Village(x, y) >= 0 || world.Arena(x, y) >= 0
}

// pvpFreeExpLevel is FREEEXP (Server.cpp:50): a Mortal below it loses nothing.
const pvpFreeExpLevel = 35

// pvpSemPerdaPorNivel is the gate around the loss (MobKilled.cpp:3248): only a
// Mortal below FREEEXP is spared; any evolved tier loses at any level.
func pvpSemPerdaPorNivel(victim *world.Entity) bool {
	return victim.Level < pvpFreeExpLevel && victim.ClassMaster == classMasterMortal
}

// pvpExpLossCap is the ceiling of the field branch (MobKilled.cpp:3241-3242),
// applied after the /6. Without it a high level lost 150000*5/6 = 125000.
const pvpExpLossCap = 30000

// somaHold adds to the debt without wrapping: the legacy field is an unsigned
// int, and a debt that wrapped would forgive itself.
func somaHold(hold uint32, add int64) uint32 {
	if add <= 0 {
		return hold
	}
	if t := int64(hold) + add; t < math.MaxUint32 {
		return uint32(t)
	}
	return math.MaxUint32
}

// payHold is the start of every kill payout (MobKilled.cpp:558-573): the gain
// pays the Hold first, and only the rest reaches the experience bar.
func payHold(e *world.Entity, gain int64) int64 {
	if e.Hold == 0 || gain <= 0 {
		return gain
	}
	if gain < int64(e.Hold) {
		e.Hold -= uint32(gain)
		return 0
	}
	gain -= int64(e.Hold)
	e.Hold = 0
	return gain
}

// pvpExpLoss is the victim's EXP loss on a PvP death (MobKilled.cpp "Lose EXP"
// region): a tiered per-level divisor of the exp span between the victim's
// current and next level, scaled by how chaotic the victim already was, then a
// flat /6 and the field ceiling.
func pvpExpLoss(victimLevel int32, victimClassMaster uint8, victimPKPoint uint8) int64 {
	nextExp := level.NextLevelExpTier(victimLevel, victimClassMaster)
	curExp := level.NextLevelExpTier(victimLevel-1, victimClassMaster)
	alpha := nextExp - curExp
	delta := alpha / 20
	switch {
	case victimLevel >= 250:
		delta = alpha / 100
	case victimLevel >= 200:
		delta = alpha / 85
	case victimLevel >= 150:
		delta = alpha / 70
	case victimLevel >= 100:
		delta = alpha / 55
	case victimLevel >= 90:
		delta = alpha / 50
	case victimLevel >= 80:
		delta = alpha / 45
	case victimLevel >= 70:
		delta = alpha / 40
	case victimLevel >= 60:
		delta = alpha / 35
	case victimLevel >= 50:
		delta = alpha / 30
	case victimLevel >= 40:
		delta = alpha / 25
	case victimLevel >= 30:
		delta = alpha / 22
	}
	if delta < 0 {
		delta = 0
	}
	if delta > 150000 {
		delta = 150000
	}
	if victimPKPoint > 10 && victimPKPoint <= 25 {
		delta *= 3
	} else {
		delta *= 5
	}
	delta /= 6
	if delta > pvpExpLossCap {
		delta = pvpExpLossCap
	}
	return delta
}

// pvpLostPk is the killer's PKPoint change for landing a PvP kill
// (MobKilled.cpp:3316-3334, "PK Drop - CP"): scaled by how chaotic the victim
// already was, clamped to [-9,0], and zeroed entirely against an already-guilty
// victim or during a guild war.
func pvpLostPk(victimPKPoint uint8, victimGuilty, atWar bool) int {
	if victimGuilty || atWar {
		return 0
	}
	lost := 3 * int(victimPKPoint) / -25
	if lost < -9 {
		lost = -9
	}
	if lost > 0 {
		lost = 0
	}
	return lost
}

// clampPKPoint enforces the legacy SetPKPoint write bounds (GetFunc.cpp:
// [1,150]) — 0 is reserved to mean "never persisted" elsewhere in this code, so
// arithmetic must never produce it.
func clampPKPoint(v int) uint8 {
	if v < int(pkPointMin) {
		return pkPointMin
	}
	if v > int(pkPointMax) {
		return pkPointMax
	}
	return uint8(v)
}

// guildsAtWar reports mutual guild war (the only AtWar bypass implemented;
// MobKilled.cpp:3113-3125's guild-war check), mirroring the legacy double-check
// against d.guildWars (guild_state.go).
func (d *Dispatcher) guildsAtWar(a, b uint16) bool {
	if a == 0 || b == 0 {
		return false
	}
	return d.guildWars[a] == b && d.guildWars[b] == a
}
