package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The Esmeralda gem — the last unread branch of the equipment walk at
// CMob.cpp:820-874.
//
// That one legacy loop fills four accumulators from the same sixteen slots, and
// three of them were already here: exp_bonus.go reads the Coral (gem 2) and
// Grade 7, drop_bonus.go the Diamante (gem 0) and Grade 5, pvp.go the Grade-8
// reflect. The Esmeralda was read by nothing at all, so every one ever socketed
// on this server has been decoration.
//
//	if(itemGem == 1) ForceDamage += (Grade == 6 ? 80 : 40) * isanc;   // :866
//
// NOT ported, deliberately: `if (Grade == 6) ForceDamage += i == 20;` (:837).
// The loop counter only reaches 15, so the comparison is false on every
// iteration and the line adds zero — dead code in the original, and porting it
// would mean inventing the slot its author meant.
//
// The Garnet (gem 3) is read by equipGarnet below, but it is NOT the legacy
// subtraction: garnet.go applies it under the rule decided on 2026-09-17.

// gemRefineSteps is the legacy's isanc: how many refine steps above +9 a piece
// carries, 1 at +10 through 6 at +15, and 0 below that.
//
// The original reaches the same number through a ladder of REF_* codes
// (BASE_GetItemSanc returns 10/12/15/18/22/27, NOT 10..15, and CMob.cpp compares
// against those) — a packed encoding, not a level. refine.Level already decodes
// it back to the true 10..15, so the ladder collapses to one subtraction.
func gemRefineSteps(it world.Item) int32 {
	lvl := refine.Level(it)
	if lvl < 10 {
		return 0
	}
	return int32(lvl - 9)
}

// equipForceDamage totals the character's perfuração: flat damage added to a
// blow after the target's defence has already been subtracted from it.
//
// Nothing downstream scales it, which is why a number that looks modest next to
// a damage stat is not: against an armoured target, where the ordinary damage
// has been ground down to single digits, this is most of the blow.
func (d *Dispatcher) equipForceDamage(e *world.Entity) int32 {
	var force int32
	for slot := range e.Equip {
		it := e.Equip[slot]
		if it.Empty() || itemGem(it) != 1 {
			continue
		}
		per := int32(40)
		if d.itemGrades[int(it.Index)] == 6 {
			per = 80
		}
		force += per * gemRefineSteps(it)
	}
	return force
}

// equipGarnet totals the character's Garnet: the legacy's gem-3 share of
// ReflectDamage (CMob.cpp:872-873), 40 per refine step above +9, or 80 on a
// Grade 8 piece. How much of it a blow actually loses is garnet.go's business.
func (d *Dispatcher) equipGarnet(e *world.Entity) int32 {
	var garnet int32
	for slot := range e.Equip {
		it := e.Equip[slot]
		if it.Empty() || itemGem(it) != 3 {
			continue
		}
		per := int32(40)
		if d.itemGrades[int(it.Index)] == 8 {
			per = 80
		}
		garnet += per * gemRefineSteps(it)
	}
	return garnet
}
