package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestExpiryFromEffectsDuration: "106 30" means thirty days from now — the way a
// GM naturally says it, and the same spelling the display uses.
func TestExpiryFromEffectsDuration(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		eff  [3]world.Effect
		want time.Duration
	}{
		{"days only", [3]world.Effect{{Effect: efWDay, Value: 30}}, 30 * 24 * time.Hour},
		{"days hours minutes", [3]world.Effect{
			{Effect: efWDay, Value: 29}, {Effect: efHour, Value: 23}, {Effect: efMin, Value: 59},
		}, 29*24*time.Hour + 23*time.Hour + 59*time.Minute},
		{"hours only", [3]world.Effect{{Effect: efHour, Value: 12}}, 12 * time.Hour},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := expiryFromEffects(tc.eff, now)
			if !ok {
				t.Fatalf("expiryFromEffects(%v) = false, want a duration", tc.eff)
			}
			if got != now.Add(tc.want).Unix() {
				t.Errorf("expiry = %v, want %v", time.Unix(got, 0).UTC(), now.Add(tc.want))
			}
		})
	}
}

// TestExpiryFromEffectsAbsoluteDate: a month and a year alongside the day still
// mean the legacy's calendar date, so one copied off an old item keeps working.
func TestExpiryFromEffectsAbsoluteDate(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	eff := [3]world.Effect{
		{Effect: efWDay, Value: 5},
		{Effect: efWMonth, Value: 10},
		{Effect: efYear, Value: 26},
	}
	got, ok := expiryFromEffects(eff, now)
	if !ok {
		t.Fatal("expiryFromEffects returned false for a complete date")
	}
	want := time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC)
	if got != want.Unix() {
		t.Errorf("expiry = %v, want %v", time.Unix(got, 0).UTC(), want)
	}
	// However it was authored, the player sees days left.
	if back := expiryEffects(got, now); back[0].Effect != efWDay || back[0].Value != 30 {
		t.Errorf("display = %v, want 30 days left", back)
	}
}

// TestExpiryFromEffectsIgnoresOrdinaryEffects is the guard that matters most: a
// refine or a damage bonus is NOT an expiry and must stay on the item.
func TestExpiryFromEffectsIgnoresOrdinaryEffects(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		eff  [3]world.Effect
	}{
		{"refine and damage", [3]world.Effect{{Effect: 7, Value: 45}, {Effect: efGrid, Value: 0}, {}}},
		{"no effects at all", [3]world.Effect{}},
		{"zero duration", [3]world.Effect{{Effect: efWDay, Value: 0}, {Effect: efHour, Value: 0}, {Effect: efMin, Value: 0}}},
		{"impossible month", [3]world.Effect{{Effect: efWDay, Value: 5}, {Effect: efWMonth, Value: 23}, {Effect: efYear, Value: 59}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := expiryFromEffects(tc.eff, now); ok {
				t.Errorf("expiryFromEffects said %v is an expiry, want false", tc.eff)
			}
		})
	}
}
