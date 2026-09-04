package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestArchLevelGate is the Arch counterpart of TestCelestialLevelGate
// (CMob.cpp:1110). It matters more than pacing: combineItemLindy demands the
// character be at exactly level 354 or 369, so an Arch that levels past a wall
// loses access to its unlock quest permanently. Before this gate existed,
// characters reached the 360s with the 355 unlock still undone and no way back.
func TestArchLevelGate(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	e.ClassMaster = classMasterArch
	e.Level = 353
	e.Exp = level.MaxExp

	d.applyLevelUps(w, nil, e)
	if e.Level != level.ArchGateLv355 {
		t.Fatalf("gated level = %d, want %d (blocked without ArchLv355)", e.Level, level.ArchGateLv355)
	}

	e.ArchLv355 = 1
	d.applyLevelUps(w, nil, e)
	if e.Level != level.ArchGateLv370 {
		t.Fatalf("after 355 unlock level = %d, want %d (blocked without ArchLv370)", e.Level, level.ArchGateLv370)
	}

	e.ArchLv370 = 1
	d.applyLevelUps(w, nil, e)
	if e.Level != level.MaxLevel {
		t.Fatalf("after 370 unlock level = %d, want MaxLevel %d", e.Level, level.MaxLevel)
	}
	// The Arch rides the Mortal curve and keeps the per-level point grants.
	if e.SkillBonus == 0 || e.SpecialBonus == 0 {
		t.Errorf("arch level-up granted no skill/special (%d/%d)", e.SkillBonus, e.SpecialBonus)
	}
}

// A Mortal shares the Arch curve but none of its walls: it must run straight
// through 354/369 with the quest flags unset.
func TestMortalIgnoresArchGate(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	e.ClassMaster = classMasterMortal
	e.Level = 353
	e.Exp = level.MaxExp

	d.applyLevelUps(w, nil, e)
	if e.Level != level.MaxLevel {
		t.Fatalf("mortal stalled at %d, want MaxLevel %d — arch gate leaked into the mortal path", e.Level, level.MaxLevel)
	}
}

// archUnlockLevel picks the pending unlock and accepts the character from that
// level upward — the divergence that lets an Arch stranded above a wall (by the
// missing gate above) still run its quest.
func TestArchUnlockLevel(t *testing.T) {
	tests := []struct {
		name        string
		classMaster uint8
		lvl         int32
		l355, l370  uint8
		wantLevel   int32
		wantOK      bool
	}{
		{"mortal is never eligible", classMasterMortal, 364, 0, 0, 0, false},
		{"arch below 355", classMasterArch, 353, 0, 0, level.ArchGateLv355, false},
		{"arch exactly at 355", classMasterArch, level.ArchGateLv355, 0, 0, level.ArchGateLv355, true},
		{"arch stranded above 355", classMasterArch, 364, 0, 0, level.ArchGateLv355, true},
		{"355 done, below 370", classMasterArch, 360, 1, 0, level.ArchGateLv370, false},
		{"355 done, at 370", classMasterArch, level.ArchGateLv370, 1, 0, level.ArchGateLv370, true},
		{"355 done, stranded above 370", classMasterArch, 390, 1, 0, level.ArchGateLv370, true},
		// Past both walls with neither flag: 355 comes first, never 370.
		{"neither done, past both walls", classMasterArch, 390, 0, 0, level.ArchGateLv355, true},
		{"both done", classMasterArch, 390, 1, 1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &world.Entity{ClassMaster: tt.classMaster, Level: tt.lvl, ArchLv355: tt.l355, ArchLv370: tt.l370}
			gotLevel, gotOK := archUnlockLevel(e)
			if gotLevel != tt.wantLevel || gotOK != tt.wantOK {
				t.Errorf("archUnlockLevel() = (%d, %v), want (%d, %v)", gotLevel, gotOK, tt.wantLevel, tt.wantOK)
			}
		})
	}
}

// downlevelArch must take back exactly what the climb granted, and must leave
// the character unable to instantly re-climb — Exp comes down with the level.
func TestDownlevelArch(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	e.ClassMaster = classMasterArch
	e.Level = 353
	e.Exp = level.MaxExp
	e.ArchLv355 = 1 // let it climb past 354 for the test setup
	d.applyLevelUps(w, nil, e)
	if e.Level != level.ArchGateLv370 {
		t.Fatalf("setup: level = %d, want %d", e.Level, level.ArchGateLv370)
	}
	hpAt369, mpAt369 := e.BaseMaxHP, e.BaseMaxMP
	skillAt369, specialAt369 := e.SkillBonus, e.SpecialBonus

	lost := e.Level - level.ArchGateLv355 // 369 − 354 = 15 levels
	d.downlevelArch(e, level.ArchGateLv355)

	if e.Level != level.ArchGateLv355 {
		t.Errorf("level = %d, want %d", e.Level, level.ArchGateLv355)
	}
	// Exp must sit exactly on the floor of the quest level, so the next
	// applyLevelUps does not immediately replay the levels just taken back.
	if want := level.NextLevelExp(level.ArchGateLv355 - 1); e.Exp != want {
		t.Errorf("exp = %d, want the level floor %d", e.Exp, want)
	}
	if d.applyLevelUps(w, nil, e) {
		t.Error("applyLevelUps levelled again right after the downlevel — exp was not brought down")
	}
	if want := hpAt369 - level.IncHP(e.Class)*lost; e.BaseMaxHP != want {
		t.Errorf("BaseMaxHP = %d, want %d", e.BaseMaxHP, want)
	}
	if want := mpAt369 - level.IncMP(e.Class)*lost; e.BaseMaxMP != want {
		t.Errorf("BaseMaxMP = %d, want %d", e.BaseMaxMP, want)
	}
	if want := playerBaseAC(e); e.BaseAC != want {
		t.Errorf("BaseAC = %d, want %d (re-derived from the level)", e.BaseAC, want)
	}
	if want := skillAt369 - archSkillBonusPerLevel*uint16(lost); e.SkillBonus != want {
		t.Errorf("SkillBonus = %d, want %d", e.SkillBonus, want)
	}
	if want := specialAt369 - archSpecialBonusPerLevel*uint16(lost); e.SpecialBonus != want {
		t.Errorf("SpecialBonus = %d, want %d", e.SpecialBonus, want)
	}
}

// Points already spent cannot be clawed back: the pool floors at zero instead of
// wrapping around uint16.
func TestDownlevelArchFloorsSpentPoints(t *testing.T) {
	d, _, e := mobKilledWorld(t)
	e.ClassMaster = classMasterArch
	e.Level = 364
	e.SkillBonus, e.SpecialBonus = 3, 0 // the player spent almost everything
	d.downlevelArch(e, level.ArchGateLv355)
	if e.SkillBonus != 0 || e.SpecialBonus != 0 {
		t.Errorf("bonuses = (%d, %d), want (0, 0) — spent points must floor, not wrap",
			e.SkillBonus, e.SpecialBonus)
	}
}

// A character already at its quest level loses nothing.
func TestDownlevelArchNoopAtQuestLevel(t *testing.T) {
	d, _, e := mobKilledWorld(t)
	e.ClassMaster = classMasterArch
	e.Level = level.ArchGateLv355
	e.Exp = 12345
	before := *e
	d.downlevelArch(e, level.ArchGateLv355)
	if e.Level != before.Level || e.Exp != before.Exp || e.BaseMaxHP != before.BaseMaxHP {
		t.Errorf("downlevelArch mutated a character already at the quest level")
	}
}
