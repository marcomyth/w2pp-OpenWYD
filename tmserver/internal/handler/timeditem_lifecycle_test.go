package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func durationDispatcher() *Dispatcher {
	d := New(Config{ItemDurations: map[int]int{
		4150: 30, // Conjunto_Yin-Yang(30dias)
		3900: 5,  // Fada_Verde(5dias)
		4160: 7,  // a future 7-day variant needs no code, only a catalog row
	}})
	return d
}

// TestCostumeStartsOnFirstEquip is the behaviour a player expects and the legacy
// did not give them: BASE_SetItemDate stamped the deadline at grant, so thirty
// days ran down in the bag. Ours begins when the item is first worn.
func TestCostumeStartsOnFirstEquip(t *testing.T) {
	d := durationDispatcher()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	it := world.Item{Index: 4150}
	if !d.startTimedItem(&it, now) {
		t.Fatal("startTimedItem = false, want the costume to start")
	}
	want := now.Add(30 * 24 * time.Hour).Unix()
	if it.ExpiresAt != want {
		t.Errorf("ExpiresAt = %v, want %v", time.Unix(it.ExpiresAt, 0).UTC(), time.Unix(want, 0).UTC())
	}
	if it.Effects != ([3]world.Effect{}) {
		t.Errorf("Effects = %v, want cleared once running", it.Effects)
	}

	// Equipping it again must NOT hand back another thirty days.
	later := now.Add(10 * 24 * time.Hour)
	if d.startTimedItem(&it, later) {
		t.Error("startTimedItem restarted a running item")
	}
	if it.ExpiresAt != want {
		t.Errorf("ExpiresAt moved to %v, want it pinned at %v", it.ExpiresAt, want)
	}
}

// TestUnstartedItemDoesNotAge: the whole point of the un-started state. An item
// sitting in the bag reports its full life however long it waits.
func TestUnstartedItemDoesNotAge(t *testing.T) {
	it := world.Item{Index: 4150, Effects: durationEffects(30 * 24 * time.Hour)}
	sel := itemToSel(it)
	if sel.Eff[0] != [2]uint8{efWDay, 30} {
		t.Fatalf("Eff = %v, want 30 days", sel.Eff)
	}
	// Same item, a year later: itemToSel reads no ExpiresAt, so nothing decays.
	if again := itemToSel(it); again.Eff != sel.Eff {
		t.Errorf("an un-started item aged: %v then %v", sel.Eff, again.Eff)
	}
}

// TestFairyDoesNotConvertOnEquip: a fairy is "consumed only while equipped"
// (its own tooltip; ProcessSecMinTimer.cpp:612 ticks Equip[13] alone), so it must
// keep a duration rather than gaining a wall-clock deadline.
func TestFairyDoesNotConvertOnEquip(t *testing.T) {
	d := durationDispatcher()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	it := world.Item{Index: 3900}
	if !d.startTimedItem(&it, now) {
		t.Fatal("startTimedItem = false, want the fairy seeded")
	}
	if it.ExpiresAt != 0 {
		t.Errorf("ExpiresAt = %d, want 0 — a fairy must never run on the wall clock", it.ExpiresAt)
	}
	if got := effectsDuration(it.Effects); got != 5*24*time.Hour {
		t.Errorf("seeded life = %v, want 5 days", got)
	}
	// Seeding is once: re-equipping must not refill it.
	it.Effects = durationEffects(2 * time.Hour)
	if d.startTimedItem(&it, now) {
		t.Error("startTimedItem refilled a fairy that was already counting")
	}
	if got := effectsDuration(it.Effects); got != 2*time.Hour {
		t.Errorf("life = %v, want the 2h it had left", got)
	}
}

// TestPermanentItemNeverStarts guards everything else in the game.
func TestPermanentItemNeverStarts(t *testing.T) {
	d := durationDispatcher()
	now := time.Now()
	for _, it := range []world.Item{
		{Index: 177}, // Traje_Coreano, permanent
		{Index: 2861, Effects: [3]world.Effect{{Effect: 7, Value: 45}}}, // a weapon bonus
		{},
	} {
		got := it
		if d.startTimedItem(&got, now) {
			t.Errorf("startTimedItem started %v, want it left alone", it)
		}
		if got.ExpiresAt != 0 || got.Effects != it.Effects {
			t.Errorf("startTimedItem modified %v into %v", it, got)
		}
	}
}

// TestDurationEffectsRoundTrip pins the pair the un-started state is stored in.
func TestDurationEffectsRoundTrip(t *testing.T) {
	for _, want := range []time.Duration{
		30 * 24 * time.Hour,
		5*24*time.Hour + 23*time.Hour + 59*time.Minute,
		time.Minute,
		0,
	} {
		if got := effectsDuration(durationEffects(want)); got != want {
			t.Errorf("round trip %v = %v", want, got)
		}
	}
}
