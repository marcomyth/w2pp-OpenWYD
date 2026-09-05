package droprate

import "testing"

// TestEffectiveRateIsNotTheBaseTable is the reason this package exists.
//
// A screen that divided by the raw table would be wrong by more than an order
// of magnitude in both directions, and wrong in the one direction that matters
// most: it would report a guaranteed drop as a one-in-nine-hundred rarity.
func TestEffectiveRateIsNotTheBaseTable(t *testing.T) {
	for _, c := range []struct {
		nome     string
		slot     int
		nivel    int
		querBase int
		querReal int
	}{
		{"slot 11 sempre cai, a tabela diz 4", 11, 100, 4, 1},
		{"slots 8 a 10 sao fixos em 4", 8, 300, 4, 4},
		{"monstro fraco derruba muito mais", 0, 5, 900, 36},
		{"monstro forte fica perto da tabela", 0, 100, 900, 891},
		{"raro de nivel alto", 16, 100, 20000, 19800},
		{"faixa sem ajuste de nivel", 24, 100, 2000, 2000},
		{"acima do slot 60 usa a outra escala", 60, 100, 5000, 4500},
	} {
		if got := DropRate[c.slot]; got != c.querBase {
			t.Errorf("%s: tabela base do slot %d = %d, want %d", c.nome, c.slot, got, c.querBase)
		}
		if got := EffectiveDropRate(c.slot, 0, c.nivel); got != c.querReal {
			t.Errorf("%s: taxa real (slot %d, nível %d) = %d, want %d",
				c.nome, c.slot, c.nivel, got, c.querReal)
		}
	}
}

// TestSlot11IsGuaranteedAtEveryLevel pins the case a moderator is most likely to
// be misled about: the override runs after the level adjustment, so no level can
// make it rare.
func TestSlot11IsGuaranteedAtEveryLevel(t *testing.T) {
	for _, nivel := range []int{1, 9, 19, 29, 39, 59, 100, 169, 199, 229, 254, 319, 400} {
		if got := EffectiveDropRate(11, 0, nivel); got != 1 {
			t.Errorf("slot 11 no nível %d = %d, want 1 (sempre cai)", nivel, got)
		}
	}
}

// TestLevelBandsOnlyTouchTheFirstThreeGroups guards the boundary the formula
// draws at slot 24: below it the mob level scales the odds, above it it does
// not. Getting that wrong would misreport every mid-table drop.
func TestLevelBandsOnlyTouchTheFirstThreeGroups(t *testing.T) {
	// Slot 23 is the last one the level bands reach.
	baixo := EffectiveDropRate(23, 0, 5)
	alto := EffectiveDropRate(23, 0, 100)
	if baixo >= alto {
		t.Errorf("slot 23: nível 5 deu %d e nível 100 deu %d; o fraco tinha que ser mais generoso", baixo, alto)
	}
	// Slot 24 is the first one they do not.
	if a, b := EffectiveDropRate(24, 0, 5), EffectiveDropRate(24, 0, 300); a != b {
		t.Errorf("slot 24 mudou com o nível: %d contra %d", a, b)
	}
}

// TestKillerBonusMakesDropsMoreLikely checks the direction of the bonus, which
// is easy to invert: the result is a divisor, so a bonus has to lower it.
func TestKillerBonusMakesDropsMoreLikely(t *testing.T) {
	sem := EffectiveDropRate(0, 0, 100)
	com := EffectiveDropRate(0, 50, 100)
	if com >= sem {
		t.Errorf("com bônus deu %d e sem bônus %d; o bônus tinha que baixar o divisor", com, sem)
	}
}
