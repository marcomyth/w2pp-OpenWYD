package mountrate

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

func TestBandFor(t *testing.T) {
	cases := []struct {
		level int
		want  int16
	}{
		{0, 0}, {1, 0},
		// A level on a boundary belongs to the band it FINISHES, not the one it
		// starts: reaching 20 is still the 1..20 climb.
		{20, 0}, {21, 1}, {40, 1}, {41, 2},
		{100, 4}, {101, 5}, {120, 5},
		// Past the cap the last band still answers, rather than indexing off the
		// end of the curve.
		{200, 5},
	}
	for _, c := range cases {
		if got := domain.MountGrowthBandFor(c.level); got != c.want {
			t.Errorf("band for level %d = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestRateReadsTheBandForTheLevel(t *testing.T) {
	curve := UnsetCurve()
	curve[0] = 60 // 1..20
	curve[5] = 22 // 101..120
	table := Table{2370: curve}

	if got, ok := table.Rate(2370, 5); !ok || got != 60 {
		t.Errorf("level 5 = %d,%v, want 60", got, ok)
	}
	if got, ok := table.Rate(2370, 110); !ok || got != 22 {
		t.Errorf("level 110 = %d,%v, want 22", got, ok)
	}
	// A band nobody configured reports "no value" rather than zero: zero is a
	// legitimate setting, and collapsing the two would silently make an
	// unconfigured band impossible.
	if _, ok := table.Rate(2370, 50); ok {
		t.Error("an unset band reported a value")
	}
	// A lineage with no curve at all, and a nil table, answer the same way.
	if _, ok := table.Rate(2371, 5); ok {
		t.Error("an unconfigured lineage reported a value")
	}
	var nilTable Table
	if _, ok := nilTable.Rate(2370, 5); ok {
		t.Error("a nil table reported a value")
	}
}

// Zero must survive the round trip: an operator who deliberately sets a band to
// 0 is making that band impossible, and that is a real configuration.
func TestZeroIsAValue(t *testing.T) {
	curve := UnsetCurve()
	curve[2] = 0
	table := Table{2370: curve}
	got, ok := table.Rate(2370, 50)
	if !ok {
		t.Fatal("a band set to 0 reported no value")
	}
	if got != 0 {
		t.Errorf("rate = %d, want 0", got)
	}
}
