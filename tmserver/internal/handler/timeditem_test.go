package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestExpiryEffectsAlwaysCountsDown pins what the player reads. The client
// renders these three slots with labels, so the legacy's absolute date came out
// as "Mês 10 4 Dia(s)" — a date the player has to subtract from today. A
// countdown says it outright. We are free to choose: dropExpired kills the item
// off ExpiresAt, so BASE_CheckItemDate never sees these values.
func TestExpiryEffectsAlwaysCountsDown(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		left             time.Duration
		day, hour, minim uint8
	}{
		{"fresh 30-day bag", 30 * 24 * time.Hour, 30, 0, 0},
		{"a costume most of the way through", 29*24*time.Hour + 23*time.Hour + 59*time.Minute, 29, 23, 59},
		{"partway", 2*24*time.Hour + 5*time.Hour + 42*time.Minute, 2, 5, 42},
		{"last hours", 90 * time.Minute, 0, 1, 30},
		{"final seconds", 30 * time.Second, 0, 0, 0},
		{"already expired", -time.Hour, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expiryEffects(now.Add(tc.left).Unix(), now)
			want := [3]world.Effect{
				{Effect: efWDay, Value: tc.day},
				{Effect: efHour, Value: tc.hour},
				{Effect: efMin, Value: tc.minim},
			}
			if got != want {
				t.Errorf("expiryEffects = %v, want %v", got, want)
			}
		})
	}
}

// TestItemToSelKeepsEffectsOnPermanentItems: only a timed item gets its effects
// replaced. A refine or a damage bonus must travel untouched.
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

// TestItemToSelBagCountsDown is the end-to-end shape a client sees for a freshly
// used Bolsa do Andarilho: days left, not a calendar date.
func TestItemToSelBagCountsDown(t *testing.T) {
	sel := itemToSel(world.Item{
		Index:     itemWandererBag,
		ExpiresAt: time.Now().Add(wandererBagDuration).Unix(),
	})
	if sel.Eff[0][0] != efWDay || sel.Eff[1][0] != efHour || sel.Eff[2][0] != efMin {
		t.Fatalf("effect ids = %v, want EF_WDAY/EF_HOUR/EF_MIN", sel.Eff)
	}
	if sel.Eff[0][1] != 29 && sel.Eff[0][1] != 30 {
		t.Errorf("days = %d, want 29 or 30", sel.Eff[0][1])
	}
}
