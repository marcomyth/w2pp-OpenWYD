package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestTimedItemReportsRemainingLife pins the wire contract for temporary items.
// The client renders both the validity and the days-left counter from the
// EF_WDAY/EF_HOUR/EF_MIN trio, which is the only channel available: the wire
// STRUCT_ITEM holds an index and three effect pairs, so ExpiresAt — a server-side
// Unix stamp — cannot reach the client any other way. A bag or mount sent with
// empty effects is what left players with no expiry and no countdown.
func TestTimedItemReportsRemainingLife(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		left             time.Duration
		day, hour, minim uint8
	}{
		{"fresh 30-day bag", 30 * 24 * time.Hour, 30, 0, 0},
		{"mount most of the way through", 29*24*time.Hour + 5*time.Hour + 42*time.Minute, 29, 5, 42},
		{"last day", 90 * time.Minute, 0, 1, 30},
		{"final minutes", 30 * time.Second, 0, 0, 0},
		{"already expired", -time.Hour, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := remainingTimeEffects(now.Add(tc.left).Unix(), now)
			want := [3]world.Effect{
				{Effect: efWDay, Value: tc.day},
				{Effect: efHour, Value: tc.hour},
				{Effect: efMin, Value: tc.minim},
			}
			if got != want {
				t.Errorf("remainingTimeEffects = %v, want %v", got, want)
			}
		})
	}
}

// TestItemToSelKeepsEffectsOnPermanentItems guards the other half: only a timed
// item gets its effects replaced. A normal item's stored effects (refine, bonuses)
// must travel untouched.
func TestItemToSelKeepsEffectsOnPermanentItems(t *testing.T) {
	it := world.Item{
		Index:   1234,
		Effects: [3]world.Effect{{Effect: efGrid, Value: 0}, {Effect: efClass, Value: 255}, {Effect: 7, Value: 9}},
	}
	sel := itemToSel(it)
	if sel.Index != 1234 {
		t.Fatalf("Index = %d, want 1234", sel.Index)
	}
	want := [3][2]uint8{{efGrid, 0}, {efClass, 255}, {7, 9}}
	if sel.Eff != want {
		t.Errorf("Eff = %v, want %v (a permanent item keeps its own effects)", sel.Eff, want)
	}
}

// TestItemToSelTimedItemCarriesCountdown is the end-to-end shape a client sees
// for a freshly used Bolsa do Andarilho.
func TestItemToSelTimedItemCarriesCountdown(t *testing.T) {
	it := world.Item{
		Index:     itemWandererBag,
		ExpiresAt: time.Now().Add(wandererBagDuration).Unix(),
	}
	sel := itemToSel(it)
	if sel.Eff[0][0] != efWDay || sel.Eff[1][0] != efHour || sel.Eff[2][0] != efMin {
		t.Fatalf("effect ids = %v, want EF_WDAY/EF_HOUR/EF_MIN", sel.Eff)
	}
	// 30 days minus the sliver spent building the item.
	if sel.Eff[0][1] != 29 && sel.Eff[0][1] != 30 {
		t.Errorf("days = %d, want 29 or 30", sel.Eff[0][1])
	}
}
