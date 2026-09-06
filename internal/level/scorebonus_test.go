package level

import "testing"

// base returns an input at the class base attributes, so nothing is spent and
// the result is exactly what the tier grants.
func base(cm uint8, lvl int32) ScoreBonusInput {
	return ScoreBonusInput{Class: 0, ClassMaster: cm, Level: lvl, Str: 8, Int: 4, Dex: 7, Con: 6}
}

// The regression: a freshly reborn Celestial arrived with ZERO points to
// distribute because every tier used the Mortal formula. The legacy grants a
// flat 1001 before the level is even counted (Basedef.cpp:995).
func TestScoreBonusCelestialStartsRich(t *testing.T) {
	in := base(classCelestial, 0)
	if got := ScoreBonus(in); got != 1001 {
		t.Errorf("fresh Celestial = %d points, want 1001", got)
	}

	// The Arch band it came from is worth 100/300/600/900/1200.
	for band, want := range map[uint8]int32{1: 1101, 2: 1301, 3: 1601, 4: 1901, 5: 2201} {
		in := base(classCelestial, 0)
		in.CelestialArchLevel = band
		if got := ScoreBonus(in); got != want {
			t.Errorf("Celestial from Arch band %d = %d, want %d", band, got, want)
		}
	}

	// Each completed Arch crystal is worth 100 — the crystals pre-pay the next life.
	in = base(classCelestial, 0)
	in.ArchCristal = 4
	if got := ScoreBonus(in); got != 1401 {
		t.Errorf("Celestial with 4 crystals = %d, want 1401", got)
	}

	// The character that hit this bug: Arch band 5, all four crystals.
	in = base(classCelestial, 0)
	in.CelestialArchLevel, in.ArchCristal = 5, 4
	if got := ScoreBonus(in); got != 2601 {
		t.Errorf("Celestial (band 5, 4 crystals) = %d, want 2601", got)
	}
}

// An Arch is paid for the Mortal levels it climbed before the rebirth, at 6 per
// level for both lives (Basedef.cpp:947-957).
func TestScoreBonusArch(t *testing.T) {
	in := base(classArch, 0)
	in.MortalLevel = 100
	if got := ScoreBonus(in); got != 600 {
		t.Errorf("Arch at level 0 with MortalLevel 100 = %d, want 600", got)
	}

	in = base(classArch, 10)
	in.MortalLevel = 100
	if got := ScoreBonus(in); got != 660 {
		t.Errorf("Arch level 10 = %d, want 660", got)
	}

	// Past 354 an Arch earns a second 6 per level — unlike the Mortal, which is
	// docked 8 there.
	in = base(classArch, 360)
	if got, want := ScoreBonus(in), int32(360*6+6*6); got != want {
		t.Errorf("Arch level 360 = %d, want %d", got, want)
	}
}

// The celestial ladder past 120 stacks on top of lvl*6.
func TestScoreBonusCelestialLevelSteps(t *testing.T) {
	in := base(classCelestial, 120)
	want := int32(120*6 + 1001 + (120-119)*6)
	if got := ScoreBonus(in); got != want {
		t.Errorf("Celestial level 120 = %d, want %d", got, want)
	}

	in = base(classCelestial, 190)
	want = int32(190*6 + 1001 + (190-119)*6 + (190-149)*2 + (190-169)*2 + (190-179)*2 + (190-189)*2)
	if got := ScoreBonus(in); got != want {
		t.Errorf("Celestial level 190 = %d, want %d", got, want)
	}
}

// The Mortal path must be untouched by the split.
func TestScoreBonusMortalUnchanged(t *testing.T) {
	tests := []struct {
		lvl  int32
		want int32
	}{
		{10, 50},
		{254, 254 * 5},
		{300, 300*5 + (300-254)*5 + (300-299)*10},
		{360, 360*5 + (360-254)*5 + (360-299)*10 + (360-354)*-8},
	}
	for _, tt := range tests {
		if got := ScoreBonus(base(classMortal, tt.lvl)); got != tt.want {
			t.Errorf("Mortal level %d = %d, want %d", tt.lvl, got, tt.want)
		}
	}
	// ClassMaster 0 is the never-persisted value and must behave as Mortal.
	if ScoreBonus(base(0, 10)) != ScoreBonus(base(classMortal, 10)) {
		t.Error("unset ClassMaster diverged from Mortal")
	}
}

// Spending attributes reduces the pool, and it never goes negative.
func TestScoreBonusSpendingAndFloor(t *testing.T) {
	in := base(classCelestial, 0)
	in.Str += 200
	if got, want := ScoreBonus(in), int32(1001-200); got != want {
		t.Errorf("after spending 200 = %d, want %d", got, want)
	}

	in = base(classMortal, 1)
	in.Str += 500 // far more than a level-1 Mortal could have
	if got := ScoreBonus(in); got != 0 {
		t.Errorf("over-spent = %d, want 0 (floored, never negative)", got)
	}
}
