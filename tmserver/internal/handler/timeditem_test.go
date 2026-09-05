package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestExpiryEffectsAbsoluteDate covers the scheme BASE_SetItemDate writes and
// BASE_CheckItemDate reads (Basedef.cpp:7408-7434): day of month, month 1-12, and
// year-100. It is what the Bolsa do Andarilho, the premium equips and the mount
// costumes use, and the client renders it as a validity DATE.
func TestExpiryEffectsAbsoluteDate(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		index            int16
		expires          time.Time
		day, month, year uint8
	}{
		{"bag 30 days out", itemWandererBag, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), 5, 10, 26},
		{"crossing into a new year", itemWandererBag, time.Date(2027, 1, 3, 8, 0, 0, 0, time.UTC), 3, 1, 27},
		{"mount costume range", 4160, time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC), 31, 12, 26},
		{"premium equip range", 3985, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), 30, 9, 26},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expiryEffects(tc.index, tc.expires.Unix(), now)
			want := [3]world.Effect{
				{Effect: efWDay, Value: tc.day},
				{Effect: efWMonth, Value: tc.month},
				{Effect: efYear, Value: tc.year},
			}
			if got != want {
				t.Errorf("expiryEffects = %v, want %v", got, want)
			}
		})
	}
}

// TestExpiryEffectsFairyCountdown covers the OTHER scheme: fairies alone hold a
// countdown, which BASE_CheckFairyDate decrements every minute. Sending a date
// here — or a countdown to the items above — shows the player the wrong expiry.
func TestExpiryEffectsFairyCountdown(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name             string
		index            int16
		left             time.Duration
		day, hour, minim uint8
	}{
		{"fresh 3-day fairy", fairyFirstIndex, 3 * 24 * time.Hour, 3, 0, 0},
		{"partway through", 3907, 2*24*time.Hour + 5*time.Hour + 42*time.Minute, 2, 5, 42},
		{"last hours", fairyLastIndex, 90 * time.Minute, 0, 1, 30},
		{"already expired", 3903, -time.Hour, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expiryEffects(tc.index, now.Add(tc.left).Unix(), now)
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

// TestItemToSelBagCarriesExpiryDate is the end-to-end shape a client sees for a
// freshly used Bolsa do Andarilho: the date it dies, not a countdown.
func TestItemToSelBagCarriesExpiryDate(t *testing.T) {
	expires := time.Now().Add(wandererBagDuration)
	sel := itemToSel(world.Item{Index: itemWandererBag, ExpiresAt: expires.Unix()})
	if sel.Eff[0][0] != efWDay || sel.Eff[1][0] != efWMonth || sel.Eff[2][0] != efYear {
		t.Fatalf("effect ids = %v, want EF_WDAY/EF_WMONTH/EF_YEAR", sel.Eff)
	}
	if got, want := sel.Eff[0][1], uint8(expires.Day()); got != want {
		t.Errorf("day = %d, want %d", got, want)
	}
	if got, want := sel.Eff[1][1], uint8(int(expires.Month())); got != want {
		t.Errorf("month = %d, want %d", got, want)
	}
}
