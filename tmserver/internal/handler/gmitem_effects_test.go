package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestAbsoluteDateFromEffects: a calendar date is an explicit deadline, so an
// authoring path starts the clock on it at once. The month and the year are what
// identify the form — the legacy's BASE_SetItemDate spelling.
func TestAbsoluteDateFromEffects(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	eff := [3]world.Effect{
		{Effect: efWDay, Value: 5},
		{Effect: efWMonth, Value: 10},
		{Effect: efYear, Value: 26},
	}
	got, ok := absoluteDateFromEffects(eff, now)
	if !ok {
		t.Fatal("absoluteDateFromEffects returned false for a complete date")
	}
	want := time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC)
	if got != want.Unix() {
		t.Errorf("expiry = %v, want %v", time.Unix(got, 0).UTC(), want)
	}
	// Once running, the player still reads days left rather than the date.
	if back := expiryEffects(got, now); back[0].Effect != efWDay || back[0].Value != 30 {
		t.Errorf("display = %v, want 30 days left", back)
	}
}

// TestAbsoluteDateFromEffectsRejectsEverythingElse is the guard that matters
// most. A bare duration is NOT a deadline — "106 30" is thirty days not yet
// begun, which startTimedItem converts when the item is first worn — and a refine
// or a damage bonus is a plain effect that must be left alone.
func TestAbsoluteDateFromEffectsRejectsEverythingElse(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		eff  [3]world.Effect
	}{
		{"bare duration in days", [3]world.Effect{{Effect: efWDay, Value: 30}}},
		{"duration with a clock", [3]world.Effect{
			{Effect: efWDay, Value: 29}, {Effect: efHour, Value: 23}, {Effect: efMin, Value: 59},
		}},
		{"refine and damage", [3]world.Effect{{Effect: 7, Value: 45}, {Effect: efGrid, Value: 0}, {}}},
		{"no effects at all", [3]world.Effect{}},
		{"date missing the year", [3]world.Effect{{Effect: efWDay, Value: 5}, {Effect: efWMonth, Value: 10}, {}}},
		{"impossible month", [3]world.Effect{
			{Effect: efWDay, Value: 5}, {Effect: efWMonth, Value: 23}, {Effect: efYear, Value: 59},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := absoluteDateFromEffects(tc.eff, now); ok {
				t.Errorf("absoluteDateFromEffects called %v a deadline, want false", tc.eff)
			}
		})
	}
}
