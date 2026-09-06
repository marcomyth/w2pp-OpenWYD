package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Which items are worth an identity (0033_item_serial).
//
// The rule is the original game's own, from BASE_NeedLog (Basedef.cpp:806) —
// the function the legacy server used to decide whether a transaction was worth
// writing down. Reusing it means the line between "matters" and "does not" was
// drawn by the people who built the economy, not by us.
//
// THE ORIGINAL HAS TWO BUGS, and porting it literally would defeat the whole
// feature. Its second-to-last test reads:
//
//	if((idx >= 551 || idx <= 562) || item->stEffect[0].cEffect != 0 ||
//	   item->stEffect[0].cEffect != 59 || ...)
//	    return TRUE;
//
// Both halves are always true — every integer is either >= 551 or <= 562, and
// no byte is both == 0 and == 59 — so BASE_NeedLog returns TRUE for every valid
// item, and the price check below it is unreachable. The author meant && in
// both places. Ported verbatim, every potion in the world would get a serial.
//
// So what follows is the intent: refine, the damage/magic thresholds, guild
// ownership, and the hand-picked index list. The dead `price >= 100` test is
// left out on purpose — at that threshold nearly every piece of equipment
// qualifies, and it was never once evaluated in the original.

// Serial thresholds, straight from BASE_NeedLog.
const (
	// serialSanc is the refine level from which an item is worth tracking.
	serialSanc = 3

	// nPos 64 and 192 are the equip-slot classes the original held to a higher
	// bar, because those pieces carry bigger numbers to begin with.
	serialPosAlto1 = 64
	serialPosAlto2 = 192

	serialMagicAlto = 12
	serialDamAlto   = 27
	serialMagic     = 8
	serialDam       = 12
)

// serialIndices are the indexes BASE_NeedLog singled out by hand.
//
// Four of them — 412, 413, 419, 420 — are stackable (isSplittable), and
// stackables never get a serial no matter what this list says: merging two
// stacks would have to throw one identity away, and a number that silently
// disappears is worse than no number. They stay listed because the original
// listed them, and because a reader comparing the two should not have to wonder
// whether the omission was deliberate.
var serialIndices = map[int16]bool{
	412: true, 413: true, 419: true, 420: true,
	522: true, 657: true, 753: true,
}

// serialGrowthLo/Hi is the range BASE_GetBonusItemAbility refuses outright
// (Basedef.cpp:2036) — growth items, whose effect bytes are a counter rather
// than a bonus, so reading damage off them means nothing.
const (
	serialGrowthLo = 2330
	serialGrowthHi = 2390
)

// Marcavel reports whether an item deserves a serial.
//
// It is handed to the world at boot (world.Config.Marcavel) because the world
// has no item catalog and should not grow one; the rule needs nPos, which is
// content.
func (d *Dispatcher) Marcavel(it world.Item) bool {
	idx := it.Index
	if idx <= 0 {
		return false
	}
	// Stackables are excluded ahead of everything: they merge and split, and an
	// identity that has to be discarded on a merge is not an identity.
	if isSplittable(idx) {
		return false
	}

	if refine.Level(it) >= serialSanc {
		return true
	}

	magic := serialBonus(it, efMagic)
	dam := serialBonus(it, efDamage)
	if npos := d.itemPos[int(idx)]; npos == serialPosAlto1 || npos == serialPosAlto2 {
		if magic >= serialMagicAlto || dam >= serialDamAlto {
			return true
		}
	} else if magic >= serialMagic || dam >= serialDam {
		return true
	}

	// A guild item belongs to a guild rather than a person, which is exactly the
	// kind of thing worth being able to trace (BASE_GetGuild, Basedef.cpp:4863).
	if serialTemGuilda(it) {
		return true
	}

	return serialIndices[idx]
}

// serialBonus is BASE_GetBonusItemAbility (Basedef.cpp:2034) for the two effects
// this rule reads.
//
// The refine multiplier is reproduced because the original applied it before
// comparing against the thresholds. Only refine 1 and 2 can reach here at all —
// 3 and up already returned true above — so the special cases the original has
// for refine 9 and for the resistances are unreachable and left out.
func serialBonus(it world.Item, ef uint8) int {
	if it.Index >= serialGrowthLo && it.Index < serialGrowthHi {
		return 0
	}
	valor := 0
	for i := 0; i < 3; i++ {
		if it.Effects[i].Effect == ef {
			valor += int(it.Effects[i].Value)
		}
	}
	if sanc := refine.Level(it); sanc > 0 {
		valor = valor * (sanc + 10) / 10
	}
	return valor
}

// serialTemGuilda reports whether the item carries a guild stamp, either half.
func serialTemGuilda(it world.Item) bool {
	for i := 0; i < 3; i++ {
		if e := it.Effects[i].Effect; e == efHWordGuild || e == efLWordGuild {
			return true
		}
	}
	return false
}
