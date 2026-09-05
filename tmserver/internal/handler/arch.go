package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The Arch unlock quests (Lindy's combine, _MSG_CombineItemLindy.cpp) are handed
// out at exactly level 355 and 370 — the legacy checks the level for EQUALITY.
// What keeps a character inside that one-level window is the pair of walls at
// GetFunc.cpp:1032 (no experience) and CMob.cpp:1110 (no level-up); both now
// exist here (level.ExpApply and applyLevelUps).
//
// Characters stranded above a wall before those gates existed still need a way
// out, so this server accepts the recipe at or ABOVE the quest level and walks
// the character back down to it on success. That is a deliberate divergence from
// the legacy equality check, and it is one-directional: it never lets an Arch
// skip a quest — the recipe, the items and the flags are unchanged — it only
// lets a stranded one finally run the quest it should have run, paying for it
// with the levels it gained while past the wall.

// archUnlockLevel reports which unlock the character is due next and whether its
// level has reached it. The 355 quest always comes first: an Arch that is past
// both walls with neither flag set runs 355, drops to 354, and climbs to 369
// again for the second one — the same order it would have followed.
func archUnlockLevel(e *world.Entity) (questLevel int32, eligible bool) {
	if e.ClassMaster != classMasterArch {
		return 0, false
	}
	if e.ArchLv355 == 0 {
		return level.ArchGateLv355, e.Level >= level.ArchGateLv355
	}
	if e.ArchLv370 == 0 {
		return level.ArchGateLv370, e.Level >= level.ArchGateLv370
	}
	return 0, false // both unlocks already done
}

// archSkillBonusPerLevel is the SkillBonus granted per level at the Arch quest
// levels. Both walls sit far above 200, so the +4 branch (CMob.cpp:1121) is the
// only one that can apply to the levels being taken back.
const archSkillBonusPerLevel = 4

// archSpecialBonusPerLevel is the SpecialBonus granted per level (CMob.cpp:1126).
const archSpecialBonusPerLevel = 2

// downlevelArch walks a stranded Arch back to its quest level, undoing the
// per-level grants applyLevelUps handed out above it. It is only ever called for
// a character that levelled past a wall it should not have crossed.
//
// Not everything can be undone exactly: SkillBonus and SpecialBonus may already
// be spent, so they are clawed back out of the unspent pool and floored at zero
// rather than forcing skills to be unlearned. The claw-back is therefore a lower
// bound on what was granted — never more than the character actually has.
func (d *Dispatcher) downlevelArch(e *world.Entity, questLevel int32) {
	lost := e.Level - questLevel
	if lost <= 0 {
		return
	}
	e.Level = questLevel
	// Experience has to come down with the level. Left where it was, the very
	// next applyLevelUps — now unblocked by the flag this quest just set — would
	// replay every level in a single tick, undoing the whole point.
	e.Exp = level.NextLevelExp(questLevel - 1)
	// The grants that ACCUMULATE on the entity. BaseAC is not among them:
	// playerBaseAC derives it from the level, so re-deriving is both correct and
	// safe against drift.
	e.BaseMaxHP = addClamp(e.BaseMaxHP, -level.IncHP(e.Class)*lost, level.MaxHPCap)
	e.BaseMaxMP = addClamp(e.BaseMaxMP, -level.IncMP(e.Class)*lost, level.MaxMPCap)
	e.BaseAC = playerBaseAC(e)
	e.SkillBonus = subBonus(e.SkillBonus, archSkillBonusPerLevel*lost)
	e.SpecialBonus = subBonus(e.SpecialBonus, archSpecialBonusPerLevel*lost)
	// ScoreBonus is a pure function of level and allocated attributes, so the
	// lower level rebuilds it. Note it goes UP: BASE_GetBonusScorePoint subtracts
	// 8 points per level above 354 (Basedef.cpp:924), which is the legacy's own
	// way of saying those levels were never meant to be reached unpaid.
	e.ScoreBonus = uint16(level.ScoreBonus(e.Class, e.Level, e.BaseStr, e.BaseInt, e.BaseDex, e.BaseCon))
	d.refreshScore(e)
	e.HP, e.MP = min(e.HP, e.MaxHP), min(e.MP, e.MaxMP)
}

// subBonus subtracts n from an unsigned point pool, flooring at zero.
func subBonus(pool uint16, n int32) uint16 {
	if int32(pool) <= n {
		return 0
	}
	return pool - uint16(n)
}

// sendClientMessage is SendClientMessage (SendFunc.cpp:27): one line of server
// text in the client's message panel. It is the channel the legacy uses for
// every "here is what just happened" line — including the Lindy unlock's
// _NN_Processing_Complete — and this port had the opcode but never a payload
// encoder, so nothing the server said ever reached the player.
func sendClientMessage(w *world.World, s *world.Session, text string) {
	if s == nil || text == "" {
		return
	}
	w.Send(s, protocol.MsgMessagePanel, protocol.EncodeMessagePanelBody(text))
}

// msgProcessingComplete is _NN_Processing_Complete (Language.txt:158), the line
// the legacy shows when a combine succeeds. Kept as a literal rather than read
// from the string table because nothing else in this port loads Language.txt
// yet; the text is copied verbatim from the shipped file.
const msgProcessingComplete = "Processo de combinação foi concluído."

// The reward for the level-370 unlock. SERVER RULE, NOT PARITY: the original
// grants nothing here — QuestInfo.Arch.Level370 is read in exactly three places
// (CMob.cpp:1110, GetFunc.cpp:1035 and the Lindy handler), all of them gates. A
// quest that costs a point of Fame and holds the character's progression
// hostage until it is done deserves to leave something behind, so this server
// attaches a cape bonus to it.
const (
	archCape370HP     int32 = 120
	archCape370Resist int16 = 8
)

// archCapeResist is the resistance the level-370 cape carries, added to all four
// elements. It is DERIVED from the persisted quest flag rather than stored,
// because Resist has no base term at all — refreshScore builds it purely from
// equipment (the legacy has no base term either). Deriving keeps that true while
// still surviving a relog, the same trick playerBaseAC uses for the crystals.
//
// The HP half needs none of this: it lands on BaseMaxHP at hand-in time and
// rides the persisted MaxHp.
func archCapeResist(e *world.Entity) int16 {
	if e.ClassMaster == classMasterArch && e.ArchLv370 != 0 {
		return archCape370Resist
	}
	return 0
}
