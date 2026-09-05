package level

import "testing"

// TestCelestialCurveMatchesTheClient anchors g_pNextLevel_2 to the values the
// client actually draws. A Celestial the client labels "Nível 2" shows its next
// threshold as 40,000,000, which is entry [2] — the same relationship the Mortal
// curve has (a "Nível 25" character shows nextLevel[25] = 73,415). The old
// synthetic ramp put [2] at 2,300,000 and the bar was meaningless.
func TestCelestialCurveMatchesTheClient(t *testing.T) {
	tests := []struct {
		index int
		want  int64
	}{
		{0, 0},
		{1, 20_000_000},
		{2, 40_000_000}, // what the client shows a "Nível 2" Celestial
		{3, 60_000_000},
		{100, 2_000_000_000},
		{int(MaxCLevel), int64(MaxCLevel) * 20_000_000},
	}
	for _, tc := range tests {
		if got := nextLevel2[tc.index]; got != tc.want {
			t.Errorf("nextLevel2[%d] = %d, want %d", tc.index, got, tc.want)
		}
	}
	// Strictly increasing, so a level-up can never stall or run away.
	for i := 1; i < len(nextLevel2); i++ {
		if nextLevel2[i] <= nextLevel2[i-1] {
			t.Fatalf("nextLevel2[%d]=%d is not above [%d]=%d", i, nextLevel2[i], i-1, nextLevel2[i-1])
		}
	}
}
