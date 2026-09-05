package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func TestCarrySummary(t *testing.T) {
	items := make([]world.Item, 6)
	items[0] = world.Item{Index: 4117, Effects: [3]world.Effect{{Effect: 10, Value: 5}}}
	items[3] = world.Item{Index: 1234, ExpiresAt: 1750000000}
	// Slot 5 is empty and must not appear: the summary is for spotting the one
	// odd item in a full bag, so 60 empty slots would bury it.
	got := carrySummary(items)
	want := "0:4117(10/5) 3:1234[exp:1750000000]"
	if got != want {
		t.Errorf("carrySummary = %q, want %q", got, want)
	}
}

func TestCarrySummaryEmpty(t *testing.T) {
	if got := carrySummary(make([]world.Item, 4)); got != "" {
		t.Errorf("carrySummary of an empty bag = %q, want empty", got)
	}
}
