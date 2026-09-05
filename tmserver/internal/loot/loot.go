// Package loot holds the mob-death drop formulas as pure functions
// (game-rules.md §2, MobKilled.cpp). Like package combat, the rand() calls go
// through an injected Rand in the original order, so the MSVC LCG reproduces the
// drops exactly (parity-tests.md §4.0).
package loot

import "github.com/jeanluca/w2pp-openwyd/internal/droprate"

// Rand is the minimal RNG; *rng.MSVC satisfies it. Intn(n) == C rand()%n.
type Rand interface {
	Intn(n int) int
}

// The drop tables and the effective-rate formula moved to internal/droprate so
// the webServer can read them: it shows a moderator the real odds, and Go
// internal rule keeps it out of tmserver/internal. These wrappers keep the
// callers here unchanged.

// DropRate is g_pDropRate[64] — the base drop odds PER Carry slot of the mob
// (NOT per item): the larger the value, the rarer the drop (it is a divisor).
var DropRate = &droprate.DropRate

// DropBonus is g_pDropBonus[64] — all 100 (neutral) by default.
var DropBonus = &droprate.DropBonus

// EffectiveDropRate computes the per-slot drop odds after the killer drop bonus
// and the target-level adjustment. A larger result is rarer.
func EffectiveDropRate(slot, killerBonus, mobLevel int) int {
	return droprate.EffectiveDropRate(slot, killerBonus, mobLevel)
}

// Drops rolls a single drop against an effective rate: drop iff rand()%rate == 0
// (the odds-as-divisor pattern, like the gold gate).
//
// UNVERIFIED: the exact final comparison is truncated in the source
// (MobKilled.cpp after :2800); this is the conventional pattern and should be
// confirmed by capture.
func Drops(r Rand, rate int) bool {
	if rate <= 0 {
		return true // rate 0 ⇒ always (slot 56 = 1 still rolls 0)
	}
	return r.Intn(rate) == 0
}

// GoldDrop returns the gold dropped by a dying mob (game-rules.md §2.1,
// MobKilled.cpp:2693). It consumes two rand() values: a drop gate, then (if it
// drops) the amount. Capped at 2000 per kill. 0 means no gold.
func GoldDrop(r Rand, mobLevel, mobCoin int) int {
	unkGold := 18
	switch {
	case mobLevel < 10:
		unkGold = 2
	case mobLevel < 20:
		unkGold = 4
	case mobLevel < 30:
		unkGold = 6
	case mobLevel < 50:
		unkGold = 9
	}
	if mobCoin == 0 || r.Intn(unkGold+1) != 0 {
		return 0
	}
	q := (mobCoin + 1) / 4
	coin := 4 * (r.Intn(q+1) + q + mobCoin)
	if coin > 2000 {
		coin = 2000
	}
	return coin
}
