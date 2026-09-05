// Package droprate holds the per-slot mob drop odds and the formula that turns
// them into the number the game actually rolls against (game-rules.md 2.2,
// MobKilled.cpp).
//
// It lives at the root because two services need it. The tmServer rolls the
// drop; the webServer shows a moderator what the odds are, and Go internal rule
// keeps it out of tmserver/internal. webserver/internal/droptool used to carry
// a hand-copied version of the table for exactly that reason, with a comment
// saying so — this replaces that copy.
//
// A copy was not harmless. The base table alone is not the drop chance: the
// level adjustment and the four hard overrides below change it by more than an
// order of magnitude, and slot 11 is guaranteed rather than rare. A screen built
// on the base numbers would have told a moderator that a certain drop happens
// once in nine hundred kills when it happens every time.
//
// tmserver/internal/loot wraps these, so nothing that already called
// loot.EffectiveDropRate had to change.
package droprate

// DropRate is g_pDropRate[64] — the base drop odds PER Carry slot of the mob
// (NOT per item): the larger the value, the rarer the drop (it is a divisor).
// Real values from Basedef.cpp:222 (game-rules.md §2.2).
var DropRate [64]int

// DropBonus is g_pDropBonus[64] — all 100 (neutral) by default (Basedef.cpp).
var DropBonus [64]int

func init() {
	fill := func(from, to, v int) {
		for i := from; i <= to; i++ {
			DropRate[i] = v
		}
	}
	fill(0, 7, 900)     // common equip
	fill(8, 11, 4)      // very common (gold/potion)
	fill(12, 15, 900)   //
	fill(16, 23, 20000) // ultra rare
	fill(24, 47, 2000)  //
	fill(48, 55, 3000)  //
	DropRate[56] = 1    // always drops
	for i, v := range []int{35, 500, 2500, 5000, 5000, 10000, 20000} {
		DropRate[57+i] = v
	}
	for i := range DropBonus {
		DropBonus[i] = 100
	}
}

// EffectiveDropRate computes the per-slot drop odds after the killer's drop
// bonus and the target-level adjustment (game-rules.md §2.2). A larger result is
// rarer. killerBonus is the killer's DropBonus (0 = none).
func EffectiveDropRate(slot, killerBonus, mobLevel int) int {
	droprate := DropRate[slot]
	dropbonus := DropBonus[slot] + killerBonus
	if dropbonus != 100 {
		dropbonus = 10000 / (dropbonus + 1)
		droprate = dropbonus * droprate / 100
	}
	if slot < 60 {
		switch slot / 8 {
		case 0, 1, 2:
			switch {
			case mobLevel < 10:
				droprate = 4 * droprate / 100
			case mobLevel < 20:
				droprate = 5 * droprate / 100
			case mobLevel < 30:
				droprate = 6 * droprate / 100
			case mobLevel < 40:
				droprate = 7 * droprate / 100
			case mobLevel < 60:
				droprate = 8 * droprate / 100
			default:
				droprate = 99 * droprate / 100
			}
		}
	} else {
		switch {
		case mobLevel < 170:
			droprate = 90 * droprate / 100
		case mobLevel < 200:
			droprate = 60 * droprate / 100
		case mobLevel < 230:
			droprate = 50 * droprate / 100
		case mobLevel < 255:
			droprate = 43 * droprate / 100
		case mobLevel < 320:
			droprate = 38 * droprate / 100
		default:
			droprate = 50 * droprate / 100
		}
	}

	// These four Carry positions are hard overrides applied after every level
	// adjustment in MobKilled.cpp. Slot 11 is therefore guaranteed.
	switch slot {
	case 8, 9, 10:
		droprate = 4
	case 11:
		droprate = 1
	}
	if droprate > 32000 {
		droprate = 32000
	}
	return droprate
}
