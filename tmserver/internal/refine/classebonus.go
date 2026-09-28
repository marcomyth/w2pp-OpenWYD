package refine

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"

// bonusValue3/2/4/5 are g_pBonusValue3/2/4/5 (Basedef.cpp:432/467/396/353):
// SetItemBonus2's reroll pools for helm, chest/legs, glove and boot. Each row is
// {effect1, value1, effect2, value2}.
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

	// Peito e calça (nPos 4 e 8): dano ou magia, e defesa 10-30 ou crítico
	// 5-7% (EF_CRITICAL2 50/60/70, que o tooltip divide por dez).
	bonusValue2 = [48][4]int{
		{2, 30, 3, 30}, {2, 30, 3, 25}, {2, 30, 3, 20}, {2, 30, 3, 10},
		{2, 24, 3, 30}, {2, 24, 3, 25}, {2, 24, 3, 20}, {2, 24, 3, 15},
		{2, 18, 3, 30}, {2, 18, 3, 25}, {2, 18, 3, 20}, {2, 18, 3, 15},
		{2, 30, 71, 50}, {2, 30, 71, 60}, {2, 30, 71, 70},
		{2, 24, 71, 50}, {2, 24, 71, 60}, {2, 24, 71, 70},
		{2, 18, 71, 50}, {2, 18, 71, 60}, {2, 18, 71, 70},
		{60, 10, 3, 30}, {60, 10, 3, 25}, {60, 10, 3, 20}, {60, 10, 3, 15}, {60, 10, 3, 10},
		{60, 8, 3, 30}, {60, 8, 3, 25}, {60, 8, 3, 20}, {60, 8, 3, 15}, {60, 8, 3, 10},
		{60, 6, 3, 30}, {60, 6, 3, 25}, {60, 6, 3, 20}, {60, 6, 3, 15}, {60, 6, 3, 10},
		{60, 10, 71, 50}, {60, 10, 71, 60}, {60, 10, 71, 70},
		{60, 8, 71, 50}, {60, 8, 71, 60}, {60, 8, 71, 70},
		{60, 6, 71, 50}, {60, 6, 71, 60}, {60, 6, 71, 70},
		{60, 4, 71, 50}, {60, 4, 71, 60}, {60, 4, 71, 70},
	}

	// Luva (nPos 16): dano ou magia, e defesa extra 10-30 em EF_ACADD2, que
	// substitui a EF_ACADD do catálogo em vez de somar (Basedef.cpp:1717; o
	// handler faz a troca em temAcAdd2). O legado sorteia rand()%30 sobre estas
	// 27 linhas, e as três que faltam deixavam 10% das luvas sem add: aqui o
	// sorteio fica nas 27 (Marco, 28/09/2026).
	bonusValue4 = [27][4]int{
		{2, 30, 72, 30}, {2, 30, 72, 25}, {2, 30, 72, 20}, {2, 30, 72, 15}, {2, 30, 72, 10},
		{2, 24, 72, 30}, {2, 24, 72, 25}, {2, 24, 72, 20}, {2, 24, 72, 15}, {2, 24, 72, 10},
		{2, 18, 72, 30}, {2, 18, 72, 25}, {2, 18, 72, 20}, {2, 18, 72, 15}, {2, 18, 72, 10},
		{60, 10, 72, 30}, {60, 10, 72, 25}, {60, 10, 72, 20}, {60, 10, 72, 15},
		{60, 8, 72, 30}, {60, 8, 72, 25}, {60, 8, 72, 20}, {60, 8, 72, 15},
		{60, 6, 72, 30}, {60, 6, 72, 25}, {60, 6, 72, 20}, {60, 6, 72, 15},
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

// A weighted value of one Classe add. The team's odds (5% for the top defense)
// would need a table of hundreds of rows, so the adds that are not legacy rows
// are drawn one at a time, each with a weight per value.
type classeValor struct {
	effect, value, weight int
}

var (
	// The high defense of chest and legs: 35/40/45/50 at the team's 50/30/20/5
	// (they sum 105, taken as weights). It comes alone (classeDefesaAltaPct).
	classeDefesaPeito = []classeValor{
		{efAC, 35, 50}, {efAC, 40, 30}, {efAC, 45, 20}, {efAC, 50, 5},
	}

	// The team's glove skill: the boot's 12/15/18 with 18 very rare. It takes
	// the place of the damage or magic (classeSkillLuvaPct).
	classeSkillLuva = []classeValor{
		{efSpecialAll, 12, 55}, {efSpecialAll, 15, 40}, {efSpecialAll, 18, 5},
	}
)

// Effect ids the Classe adds write (ItemEffect.h).
const (
	efAC         = 3
	efMagic      = 60
	efCritical2  = 71
	efAcAdd2     = 72
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

// classeDefesaAltaPct is the chance, in percent, that a chest or legs piece
// comes out with the team's high defense alone instead of its legacy row.
const classeDefesaAltaPct = 10

// classeSkillLuvaPct is the chance, in percent, that a glove comes out with the
// team's skill in place of its legacy damage or magic.
const classeSkillLuvaPct = 10

// classeAdds draws the two adds of a chest, legs or glove piece.
//
// Every piece keeps the legacy formula (Marco, 28/09/2026): a legacy row, drawn
// first like the rand()%N SetItemBonus2 opens with (Server.cpp:2755/2787). The
// 26/09 pools had replaced it, and with it every combination players farm the
// Classe for — Magia 10 + Defesa 30, Dano 24 + Crítico 7% — could no longer
// come out. The team's adds are a layer on top of the legacy row:
//
//   - chest and legs: classeDefesaAltaPct of the pieces get defense 35-50
//     instead, alone — combined adds top out at defense 30 (limite.go), so the
//     first slot gets EF_UNIQUE, the game's "nothing here" marker, the way an
//     empty drop slot does (dropbonus.go);
//   - glove: classeSkillLuvaPct get skill in place of the damage or magic, and
//     keep the row's defense. A glove is damage-or-magic + defense, or skill +
//     defense; never skill with damage or magic.
func classeAdds(nPos int, roll func(int) int) (world.Effect, world.Effect) {
	if nPos == nPosGlove {
		row := bonusValue4[roll(len(bonusValue4))]
		defesa := world.Effect{Effect: uint8(row[2]), Value: uint8(row[3])}
		if roll(100) < classeSkillLuvaPct {
			return classeSorteia(classeSkillLuva, roll), defesa
		}
		return world.Effect{Effect: uint8(row[0]), Value: uint8(row[1])}, defesa
	}
	row := bonusValue2[roll(len(bonusValue2))]
	if roll(100) < classeDefesaAltaPct {
		defesa := classeSorteia(classeDefesaPeito, roll)
		return world.Effect{Effect: efUnique, Value: uint8(roll(128))}, defesa
	}
	return world.Effect{Effect: uint8(row[0]), Value: uint8(row[1])},
		world.Effect{Effect: uint8(row[2]), Value: uint8(row[3])}
}

// classeDefesaSozinhaAcima is the defense add above which the Repletion piece
// carries no other add.
const classeDefesaSozinhaAcima = 30

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
	// legacy single draw, so a captured rand() sequence still reproduces there;
	// chest and legs open with the same legacy draw and then spend one more on
	// the high-defense chance.
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
	// O teto dos adds de armadura (limite.go). As faixas de hoje já cabem nele;
	// ele segura o dia em que alguém mexer nelas.
	LimitaAddsDeArmadura(dest, nPos)
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

// repletionDefesaAlta are the defense values only the Repletion rolls (the
// chest/legs and glove pools above). A drop tops out below them, and the shop
// pieces that carry more defense (70, 99) are not in this list — which is what
// lets CorrigeDefesaCombinada touch Repletion output and nothing else.
var repletionDefesaAlta = map[uint8]bool{35: true, 40: true, 45: true, 50: true}

// CorrigeDefesaCombinada brings an item the old Repletion rolled back inside the
// limit (Marco, 27/09/2026): a chest, legs or glove whose adds are a Repletion
// defense (35-50) AND damage or magic gets the defense cut to 30 and keeps the
// other add. Those pieces came out between the 26/09 pools and the fix above,
// which now keeps a high defense alone. Reports whether it changed the item.
//
// Narrow on purpose: only slots 1 and 2 (where SetItemBonus2 writes the adds),
// only the Repletion defense values, only when the other add is damage or
// magic, and only chest and legs. The glove has its own rule (corrigeLuva).
func CorrigeDefesaCombinada(it *world.Item, nPos int) bool {
	if it == nil || it.Empty() {
		return false
	}
	if nPos == nPosGlove {
		return corrigeLuva(it)
	}
	if nPos != nPosChest && nPos != nPosLegs {
		return false
	}
	for i, j := 1, 2; i <= 2; i, j = i+1, j-1 {
		def, outro := it.Effects[i], it.Effects[j]
		if def.Effect != efAC || !repletionDefesaAlta[def.Value] {
			continue
		}
		if outro.Effect != efDamageBonus && outro.Effect != efMagic {
			continue
		}
		it.Effects[i].Value = classeDefesaSozinhaAcima
		return true
	}
	return false
}

// luvaSkillNaCorrecao is what a glove's Repletion defense add turns into: skill
// 12, the glove's own second add at its most common value.
var luvaSkillNaCorrecao = world.Effect{Effect: efSpecialAll, Value: 12}

// corrigeLuva takes the Repletion defense off a glove (Marco, 27/09/2026: the
// high defense add is chest and legs only). The glove rolled defense 40-50 until
// then — alone, or cut to 30 when it came with damage or magic — and it stacked
// on the base EF_ACADD of gloves like the Manoplas Elementais(M). The add can't
// be re-rolled on load, so it becomes skill 12 and the player keeps a glove add.
//
// Only defense of 30 or more in slots 1 and 2: that is every value the Repletion
// left on a glove (40-50, and the 30 of the first correction). The level-item
// glove's defense 20 (LevelItem.txt, Marco's call of 26/09) stays.
func corrigeLuva(it *world.Item) bool {
	mudou := false
	for i := 1; i <= 2; i++ {
		if it.Effects[i].Effect == efAC && int(it.Effects[i].Value) >= classeDefesaSozinhaAcima {
			it.Effects[i] = luvaSkillNaCorrecao
			mudou = true
		}
	}
	return mudou
}
