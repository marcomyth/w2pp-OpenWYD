package level

// BASE_GetBonusScorePoint (Basedef.cpp:898) computes the free attribute points a
// character should have: what its tier grants minus what is already spent above
// the class base. It is a different formula PER TIER, and the higher ones are not
// small adjustments of the Mortal one — a fresh Celestial is granted over a
// thousand points before its level is even counted.
//
// Porting only the Mortal branch is why Arch and Celestial characters arrived
// with zero points to distribute.

// ScoreBonusInput is what the formula reads. Beyond the attributes it needs the
// tier and the quest counters that buy points in the higher tiers — a Celestial's
// grant depends on how many Arch crystals it completed and how far it got as an
// Arch, not only on its current level.
type ScoreBonusInput struct {
	Class              uint8
	ClassMaster        uint8
	Level              int32
	Str, Int, Dex, Con int16

	// MortalLevel is QuestInfo.Arch.MortalLevel: the level held as a Mortal. An
	// Arch is paid for the levels it climbed before the rebirth.
	MortalLevel uint16
	// ArchCristal is QuestInfo.Arch.Cristal (0..4). Worth 100 points each to a
	// Celestial — the crystals are how an Arch pre-pays for its next life.
	ArchCristal uint8
	// CelestialArchLevel is QuestInfo.Celestial.ArchLevel (1..5): the band the
	// character reached as an Arch, worth 100/300/600/900/1200.
	CelestialArchLevel uint8
	// CelestialReset is QuestInfo.Celestial.Reset, 200 points each. NOT MODELED
	// yet — the resets are part of the Sub-Celestial flow, so this is always zero
	// and a reset character would be short 200 points per reset.
	CelestialReset uint8
	// SubCelestialLevel is QuestInfo.Celestial.SubCelestialLevel, which only the
	// CELESTIALCS branch adds. NOT MODELED yet; zero here.
	SubCelestialLevel int32
}

// ScoreBonus returns the free attribute points for the input's tier.
func ScoreBonus(in ScoreBonusInput) int32 {
	c := validClass(in.Class)
	spent := (int32(in.Str) - baseSIDCHM[c][0]) +
		(int32(in.Int) - baseSIDCHM[c][1]) +
		(int32(in.Dex) - baseSIDCHM[c][2]) +
		(int32(in.Con) - baseSIDCHM[c][3])

	granted := grantedPoints(in)
	if bonus := granted - spent; bonus > 0 {
		return bonus
	}
	return 0
}

// grantedPoints is the `leveluse` the legacy builds per tier.
//
// The over-spend correction the original performs (docking an attribute when
// totaluse exceeds leveluse, Basedef.cpp:928-947) is deliberately NOT ported: it
// silently mutates the character's stats, and it exists to repair data from an
// era when the tier formulas changed under live characters. Here the same
// situation just yields zero free points, which is visible instead of destructive.
func grantedPoints(in ScoreBonusInput) int32 {
	lvl := in.Level
	switch in.ClassMaster {
	case classArch:
		// lvl*6 plus the Mortal levels it climbed before the rebirth (:947-957).
		granted := lvl*6 + int32(in.MortalLevel)*6
		if lvl >= 354 {
			granted += (lvl - 354) * 6
		}
		return granted

	case classCelestial, classCelestialCS, classSCelestial:
		granted := lvl*6 + int32(in.ArchCristal)*100 + int32(in.CelestialReset)*200
		// The flat grant every Celestial starts with (:995).
		granted += 1001
		granted += archBandPoints(in.CelestialArchLevel)
		granted += celestialLevelSteps(lvl)
		if in.ClassMaster == classCelestialCS {
			// The Sub-Celestial life is counted at half rate, then its own steps
			// at full (:1042-1060).
			sub := in.SubCelestialLevel
			granted += sub * 6 / 2
			granted += celestialLevelSteps(sub)
		}
		return granted

	default:
		// Mortal, and the unset ClassMaster 0 that completeCharacterLogin treats
		// as Mortal (:910-925).
		granted := lvl * 5
		if lvl >= 254 {
			granted += (lvl - 254) * 5
		}
		if lvl >= 299 {
			granted += (lvl - 299) * 10
		}
		if lvl >= 354 {
			granted += (lvl - 354) * -8
		}
		return granted
	}
}

// archBandPoints is what the Arch band a Celestial came from is worth
// (QuestInfo.Celestial.ArchLevel, :997-1010).
func archBandPoints(band uint8) int32 {
	switch band {
	case 1:
		return 100
	case 2:
		return 300
	case 3:
		return 600
	case 4:
		return 900
	case 5:
		return 1200
	default:
		return 0
	}
}

// celestialLevelSteps is the per-level ladder the celestial tiers add on top of
// lvl*6 (:1013-1027). Each threshold counts from one below itself, exactly as
// the original writes it.
func celestialLevelSteps(lvl int32) int32 {
	var extra int32
	if lvl >= 120 {
		extra += (lvl - 119) * 6
	}
	if lvl >= 150 {
		extra += (lvl - 149) * 2
	}
	if lvl >= 170 {
		extra += (lvl - 169) * 2
	}
	if lvl >= 180 {
		extra += (lvl - 179) * 2
	}
	if lvl >= 190 {
		extra += (lvl - 189) * 2
	}
	return extra
}
