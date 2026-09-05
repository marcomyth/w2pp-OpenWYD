package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestExpiryFromEffectsAbsoluteDate: a costume authored as "106 5 110 10 109 26"
// means 5 October 2026, and must become an ExpiresAt — only that expires an item
// here (dropExpired); the legacy's BASE_CheckItemDate is not in this path.
func TestExpiryFromEffectsAbsoluteDate(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	eff := [3]world.Effect{
		{Effect: efWDay, Value: 5},
		{Effect: efWMonth, Value: 10},
		{Effect: efYear, Value: 26},
	}
	got, ok := expiryFromEffects(4150, eff, now)
	if !ok {
		t.Fatal("expiryFromEffects returned false for a complete date")
	}
	want := time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC)
	if got != want.Unix() {
		t.Errorf("expiry = %v, want %v", time.Unix(got, 0).UTC(), want)
	}
	// And it must survive the round trip back onto the wire. Both directions read
	// the same wall clock in production; the test passes one explicitly so the
	// assertion cannot depend on the machine's zone.
	back := expiryEffects(4150, got, now)
	wantBack := [3]world.Effect{
		{Effect: efWDay, Value: 5},
		{Effect: efWMonth, Value: 10},
		{Effect: efYear, Value: 26},
	}
	if back != wantBack {
		t.Errorf("round trip = %v, want %v", back, wantBack)
	}
}

// TestExpiryFromEffectsFairyCountdown: fairies author time REMAINING.
func TestExpiryFromEffectsFairyCountdown(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	eff := [3]world.Effect{
		{Effect: efWDay, Value: 3},
		{Effect: efHour, Value: 2},
		{Effect: efMin, Value: 30},
	}
	got, ok := expiryFromEffects(3900, eff, now)
	if !ok {
		t.Fatal("expiryFromEffects returned false for a fairy countdown")
	}
	want := now.Add(3*24*time.Hour + 2*time.Hour + 30*time.Minute)
	if got != want.Unix() {
		t.Errorf("expiry = %v, want %v", time.Unix(got, 0).UTC(), want)
	}
}

// TestExpiryFromEffectsIgnoresOrdinaryEffects is the guard that matters most: a
// refine or a damage bonus is NOT an expiry and must stay on the item.
func TestExpiryFromEffectsIgnoresOrdinaryEffects(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		index int16
		eff   [3]world.Effect
	}{
		{"refine and damage", 2861, [3]world.Effect{{Effect: 7, Value: 45}, {Effect: efGrid, Value: 0}, {}}},
		{"no effects at all", 4150, [3]world.Effect{}},
		{"date missing the year", 4150, [3]world.Effect{{Effect: efWDay, Value: 5}, {Effect: efWMonth, Value: 10}, {}}},
		{"impossible month", 4150, [3]world.Effect{{Effect: efWDay, Value: 5}, {Effect: efWMonth, Value: 23}, {Effect: efYear, Value: 59}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := expiryFromEffects(tc.index, tc.eff, now); ok {
				t.Errorf("expiryFromEffects said %v is an expiry, want false", tc.eff)
			}
		})
	}
}
