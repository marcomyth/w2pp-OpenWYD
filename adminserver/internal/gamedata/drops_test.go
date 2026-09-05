package gamedata

import "testing"

func TestDropOddsReadWellAcrossTheWholeRange(t *testing.T) {
	// The divisors in this game span 1 to 32000. Two decimals everywhere would
	// print every rare slot as "0.01%" or "0.00%", which is the range a
	// moderator most needs to compare.
	for _, c := range []struct {
		divisor int32
		pct     string
		mortes  int32
	}{
		{1, "100%", 1},
		{4, "25.0%", 4},
		{36, "2.78%", 36},
		{900, "0.111%", 900},
		{2000, "0.0500%", 2000},
		{19800, "0.0051%", 19800},
		{32000, "0.0031%", 32000},
	} {
		m := DropMob{Divisor: c.divisor}
		if got := m.Porcentagem(); got != c.pct {
			t.Errorf("divisor %d: porcentagem = %q, want %q", c.divisor, got, c.pct)
		}
		if got := m.Mortes(); got != c.mortes {
			t.Errorf("divisor %d: mortes = %d, want %d", c.divisor, got, c.mortes)
		}
	}
}

func TestGuaranteedDropIsMarkedAndNeverPrintedAsRare(t *testing.T) {
	// Slot 11 is the case the raw table gets most wrong. If this ever renders as
	// a percentage instead of a mark, the screen is back to misleading people.
	for _, d := range []int32{0, 1} {
		m := DropMob{Divisor: d}
		if !m.Garantido() {
			t.Errorf("divisor %d não foi marcado como garantido", d)
		}
		if m.Chance() != 100 {
			t.Errorf("divisor %d: chance = %v, want 100", d, m.Chance())
		}
		if m.Mortes() > 1 {
			t.Errorf("divisor %d: mortes = %d, want no máximo 1", d, m.Mortes())
		}
	}
	if (DropMob{Divisor: 2}).Garantido() {
		t.Error("um drop de 50% foi marcado como garantido")
	}
}

func TestExpectedKillsIsTheDivisorItself(t *testing.T) {
	// For a roll that succeeds with probability 1/d, the expected number of
	// attempts is exactly d. Rounding or scaling it would only add error.
	for _, d := range []int32{4, 36, 900, 20000} {
		if got := (DropMob{Divisor: d}).Mortes(); got != d {
			t.Errorf("divisor %d: mortes = %d, want %d", d, got, d)
		}
	}
}
