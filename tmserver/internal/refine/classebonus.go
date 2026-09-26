package refine

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// bonusValue3/5 are g_pBonusValue3/5 (Basedef.cpp:432/353): SetItemBonus2's
// reroll pools for helm and boot. Each row is {effect1, value1, effect2,
// value2}. Chest, legs and glove no longer use the legacy g_pBonusValue2/4 —
// see classeAdds below.
var (
	bonusValue3 = [25][4]int{ // Elmo (nPos 2)
		{4, 60, 26, 18}, {4, 60, 26, 15}, {4, 60, 26, 12},
		{4, 50, 26, 18}, {4, 50, 26, 15}, {4, 50, 26, 12},
		{4, 40, 26, 18}, {4, 40, 26, 15}, {4, 40, 26, 12},
		{4, 30, 26, 18}, {4, 30, 26, 15}, {4, 30, 26, 12},
		{4, 60, 60, 12}, {4, 60, 60, 10}, {4, 60, 60, 8}, {4, 60, 60, 6},
		{4, 50, 60, 12}, {4, 50, 60, 10}, {4, 50, 60, 8}, {4, 50, 60, 6},
		{4, 40, 60, 12}, {4, 40, 60, 10}, {4, 40, 60, 8}, {4, 40, 60, 6}, {4, 40, 60, 4},
	}

	bonusValue5 = [30][4]int{ // Bota (nPos 32)
		{2, 30, 74, 18}, {2, 30, 74, 15}, {2, 30, 74, 12},
		{2, 24, 74, 18}, {2, 24, 74, 15}, {2, 24, 74, 12},
		{2, 18, 74, 18}, {2, 18, 74, 15}, {2, 18, 74, 12},
		{2, 12, 74, 18}, {2, 12, 74, 15}, {2, 12, 74, 12},
		{2, 6, 74, 18}, {2, 6, 74, 15}, {2, 6, 74, 12},
		{2, 30, 60, 10}, {2, 30, 60, 8}, {2, 30, 60, 6},
		{2, 24, 60, 10}, {2, 24, 60, 8}, {2, 24, 60, 6},
		{2, 18, 60, 10}, {2, 18, 60, 8}, {2, 18, 60, 6},
		{2, 12, 60, 10}, {2, 12, 60, 8}, {2, 12, 60, 6},
		{2, 6, 60, 10}, {2, 6, 60, 8}, {2, 6, 60, 6},
	}
)

// SERVER RULE, NOT PARITY (Marco, 26/09/2026): the Classe adds of chest, legs
// and glove. The legacy pools (g_pBonusValue2/4) topped defense at 30, gave
// chest crit as EF_CRITICAL2 50-70, and put the glove's defense on EF_ACADD2,
// which this port does not read — the glove add counted for nothing.
//
// Each add is drawn on its own, with a weight per value, instead of one row out
// of a flat table: the team's odds (5% for the top defense) would need a table
// of thousands of rows, and a single rand() draw over it would pass the 32767
// ceiling of the MSVC rand() this port reproduces.
type classeValor struct {
	effect, value, weight int
}

// classeDanoPeito and classeDanoLuva keep the legacy first add of chest/legs and of glove: damage or
// magic, with the weight each value had as rows of g_pBonusValue2/4.
var (
	classeDanoPeito = []classeValor{
		{efDamageBonus, 30, 7}, {efDamageBonus, 24, 7}, {efDamageBonus, 18, 7},
		{efMagic, 10, 8}, {efMagic, 8, 8}, {efMagic, 6, 8}, {efMagic, 4, 3},
	}
	classeDanoLuva = []classeValor{
		{efDamageBonus, 30, 5}, {efDamageBonus, 24, 5}, {efDamageBonus, 18, 5},
		{efMagic, 10, 4}, {efMagic, 8, 4}, {efMagic, 6, 4},
	}

	// The second add of chest/legs: defense or crit, half and half. Defense is
	// 35/40/45/50 at the team's 50/30/20/5 (they sum 105, taken as weights).
	// Crit is 1% or 2% — 10 or 20 on the byte, which the tooltip divides by ten.
	classeDefesaPeito = []classeValor{
		{efAC, 35, 50}, {efAC, 40, 30}, {efAC, 45, 20}, {efAC, 50, 5},
	}
	classeCriticoPeito = []classeValor{
		{efCritical2, 10, 1}, {efCritical2, 20, 1},
	}

	// The second add of the glove: skill or defense, half and half. Skill is
	// the boot's 12/15/18 with 18 very rare; defense is the chest's odds from
	// 40 up, on EF_AC so the server counts it.
	classeSkillLuva = []classeValor{
		{efSpecialAll, 12, 55}, {efSpecialAll, 15, 40}, {efSpecialAll, 18, 5},
	}
	classeDefesaLuva = []classeValor{
		{efAC, 40, 30}, {efAC, 45, 20}, {efAC, 50, 5},
	}
)

// Effect ids the Classe adds write (ItemEffect.h).
const (
	efAC         = 3
	efMagic      = 60
	efCritical2  = 71
	efSpecialAll = 74
)

// classeSorteia draws one value out of pool by weight.
func classeSorteia(pool []classeValor, roll func(int) int) world.Effect {
	total := 0
	for _, v := range pool {
		total += v.weight
	}
	n := roll(total)
	for _, v := range pool {
		if n < v.weight {
			return world.Effect{Effect: uint8(v.effect), Value: uint8(v.value)}
		}
		n -= v.weight
	}
	last := pool[len(pool)-1]
	return world.Effect{Effect: uint8(last.effect), Value: uint8(last.value)}
}

// classeAdds draws the two adds of a chest, legs or glove piece: the first,
// then which kind the second is, then its value.
func classeAdds(nPos int, roll func(int) int) (world.Effect, world.Effect) {
	if nPos == nPosGlove {
		primeiro := classeSorteia(classeDanoLuva, roll)
		if roll(2) == 0 {
			return primeiro, classeSorteia(classeSkillLuva, roll)
		}
		return primeiro, classeSorteia(classeDefesaLuva, roll)
	}
	primeiro := classeSorteia(classeDanoPeito, roll)
	if roll(2) == 0 {
		return primeiro, classeSorteia(classeDefesaPeito, roll)
	}
	return primeiro, classeSorteia(classeCriticoPeito, roll)
}

// classeSancCap is the +6 ceiling SetItemBonus2 puts on the sanc it bumps
// (Server.cpp:2727-2739 and its three siblings) — distinct from the dust
// path's own caps, and never lowered, only raised toward it.
const classeSancCap = 6

// efDamageBonus is EF_DAMAGE (ItemEffect.h:4), reused here for the boot-only
// clamp below.
const efDamageBonus = 2

// classeSancEffect is EF_SANC (ItemEffect.h:100). Kept local (rather than
// exported from this package) because SetItemBonus2 checks slot 0
// specifically, not the general readSlot/writeSlot scan the dust path uses.
const classeSancEffect = efSanc

// nPos equip-slot classes SetItemBonus2 rerolls (Basedef.h:1162+). Any other
// nPos (weapons, accessories, …) is left completely untouched — the legacy
// function simply has no `if` branch for them.
const (
	nPosHelm      = 2
	nPosChest     = 4
	nPosLegs      = 8
	nPosGlove     = 16
	nPosBoot      = 32
	bootDamageCap = 30
)

// ClasseBonus rerolls dest's sanc (capped at +6) and its two bonus effects,
// selected by dest's equip slot nPos (SetItemBonus2, Server.cpp:2719-2861).
// roll(n) must behave like rand()%n; itemAbility must sum an item's catalog +
// instance effects for a given id (BASE_GetItemAbility), needed only for the
// boot damage clamp. Reports whether nPos was one of the four covered slots —
// the caller does not need to branch on it (an uncovered item is left
// unchanged, matching the legacy exactly), but tests find it useful.
func ClasseBonus(dest *world.Item, nPos int, roll func(int) int, itemAbility func(world.Item, uint8) int) bool {
	// The adds are drawn BEFORE any sanc roll: every branch of SetItemBonus2
	// opens with `int _rand = rand()%N;` and only then touches stEffect[0]
	// (Server.cpp:2723-2726 and its three siblings). Helm and boot keep the
	// legacy single draw, so a captured rand() sequence still reproduces there.
	var add1, add2 world.Effect
	switch nPos {
	case nPosHelm, nPosBoot:
		table := bonusValue3[:]
		if nPos == nPosBoot {
			table = bonusValue5[:]
		}
		row := table[roll(len(table))]
		add1 = world.Effect{Effect: uint8(row[0]), Value: uint8(row[1])}
		add2 = world.Effect{Effect: uint8(row[2]), Value: uint8(row[3])}
	case nPosChest, nPosLegs, nPosGlove:
		add1, add2 = classeAdds(nPos, roll)
	default:
		return false
	}

	if dest.Effects[0].Effect == classeSancEffect {
		lvl := Level(*dest)
		if lvl < classeSancCap {
			lvl += roll(2)
			if lvl >= classeSancCap {
				lvl = classeSancCap
			}
			Set(dest, lvl, 0)
		}
	} else {
		dest.Effects[0] = world.Effect{Effect: classeSancEffect, Value: uint8(roll(2))}
	}

	dest.Effects[1] = add1
	dest.Effects[2] = add2

	if nPos == nPosBoot {
		clampBootDamage(dest, itemAbility)
	}
	return true
}

// clampBootDamage ports the boot-only EF_DAMAGE ceiling (Server.cpp:2845-2861):
// a freshly-rolled EF_DAMAGE bonus is trimmed so the item's total damage
// ability (catalog + instance, read AFTER the roll above) never exceeds 30.
func clampBootDamage(dest *world.Item, itemAbility func(world.Item, uint8) int) {
	for i := 1; i <= 2; i++ {
		if dest.Effects[i].Effect != efDamageBonus {
			continue
		}
		ability := itemAbility(*dest, efDamageBonus)
		if ability <= bootDamageCap {
			continue
		}
		over := ability - bootDamageCap
		v := int(dest.Effects[i].Value)
		if v < over {
			over = v
		}
		v -= over
		if v < 0 {
			v = 0
		}
		dest.Effects[i].Value = uint8(v)
	}
}
