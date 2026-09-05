package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The four Arch crystal quests (Cristal Elime / Sylphed / Thelion / Noas, items
// 4106..4109, all EF_VOLATILE 187 — _MSG_UseItem.cpp:3366-3468). Each one is a
// one-shot hand-in that costs 100 million experience and permanently raises a
// base attribute. They must be done in order.
const (
	volArchCrystal = 187
	// archCrystalBase is the item index of stage 1; the stage number is the
	// item's offset from it plus one (the legacy's `sIndex - 4106 + 1`).
	archCrystalBase = 4106
	// archCrystalStages is how many stages the quest has.
	archCrystalStages = 4
	// The two stages that grant AC, named because playerBaseAC rebuilds their
	// grant from the persisted counter.
	archCrystalStageAC30 = 2
	archCrystalStageAll  = 4
	// archCrystalExpCost is the 100 million experience each stage burns (:3424).
	archCrystalExpCost int64 = 100_000_000
	// archCrystalExpCap is the ceiling the legacy clamps to right after the
	// subtraction (:3425). It sits above level.MaxExp, so it never actually
	// binds — carried over so the arithmetic reads the same as the original.
	archCrystalExpCap int64 = 40_000_000_000
)

// useArchCrystal is the crystal hand-in. The stage must be exactly the next one
// due: handing in Thelion before Sylphed is refused rather than skipped, which
// is what makes the four a chain instead of four independent items.
//
// The legacy has its "Archs only" and "level 355+" checks COMMENTED OUT
// (:3368-3381), so any character can run the quest. That is reproduced rather
// than restored: the items are Arch quest rewards in practice, and inventing a
// gate the original does not enforce would refuse hand-ins that work on a real
// server.
func (d *Dispatcher) useArchCrystal(w *world.World, s *world.Session, e *world.Entity, src int) {
	stage := int(e.Carry[src].Index) - archCrystalBase + 1
	if stage < 1 || stage > archCrystalStages {
		return
	}
	current := int(e.ArchCristal)

	// Both refusals echo the item back, undoing the removal the client already
	// performed when the player dragged it (:3395, :3402).
	//
	// The line the player reads is _NN_Youve_Done_It_Already, carried by notify
	// through the message panel (notice.go); the local literal that used to be
	// sent beside it would now arrive twice.
	if current >= stage {
		d.notify(w, s, NoticeAlreadyDone)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}
	if current != stage-1 {
		sendClientMessage(w, s, msgNeedBeforeQuest)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	e.ArchCristal = uint8(stage)
	switch stage {
	case 1:
		e.BaseMaxMP = addClamp(e.BaseMaxMP, 80, level.MaxMPCap)
	case 2:
		e.BaseAC += 30
	case 3:
		e.BaseMaxHP = addClamp(e.BaseMaxHP, 80, level.MaxHPCap)
	default:
		e.BaseMaxHP = addClamp(e.BaseMaxHP, 60, level.MaxHPCap)
		e.BaseMaxMP = addClamp(e.BaseMaxMP, 60, level.MaxMPCap)
		e.BaseAC += 20
	}

	// The experience price. The legacy subtracts without checking the balance
	// first, which on a character below 100M would wrap the counter negative;
	// clamping at zero is a deliberate divergence, because a negative Exp feeds
	// straight into the level curve and there is no legitimate state it encodes.
	e.Exp -= archCrystalExpCost
	if e.Exp < 0 {
		e.Exp = 0
	}
	if e.Exp > archCrystalExpCap {
		e.Exp = archCrystalExpCap
	}

	// The grants land on BaseScore, so the live score has to be rebuilt before it
	// is sent — otherwise the player sees the old numbers until something else
	// happens to refresh them.
	e.ScoreBonus = uint16(level.ScoreBonus(e.Class, e.Level, e.BaseStr, e.BaseInt, e.BaseDex, e.BaseCon))
	d.refreshScore(e)
	d.sendScore(w, s, e)
	d.sendEtc(w, s, e)

	sendEmotion(w, s, e, motionLevelUp, motionLevelUpParm)
	sendClientMessage(w, s, "Você concluiu a Quest "+archCrystalName(stage)+".")

	d.log.Info("arch crystal quest", "conn", s.Conn, "account", s.AccountName,
		"stage", stage, "exp_left", e.Exp)

	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	w.SaveCharacterAsync(s)
}

// The player-facing lines, copied verbatim from the shipped Language.txt so the
// wording matches what the original prints (382, 383 and 314). They are Go
// literals, so they reach the client through protocol.ClientText's Windows-1252
// conversion — see sendClientMessage.
const (
	msgNeedBeforeQuest = "Antes você deve concluir a quest anterior."
)

// archCrystalName is the crystal's in-game name, for the completion line's %s
// (_DN_Play_Quest). Read from the stage rather than the item catalog: the four
// names are fixed content and this keeps the message independent of whether the
// catalog is mounted.
func archCrystalName(stage int) string {
	switch stage {
	case 1:
		return "Cristal Elime"
	case 2:
		return "Cristal Sylphed"
	case 3:
		return "Cristal Thelion"
	default:
		return "Cristal Noas"
	}
}
