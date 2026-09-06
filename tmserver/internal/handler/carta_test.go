package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	itemCartaN = 3172
	itemCartaM = 3171
	itemCartaA = 1731
)

func cartaFixture(t *testing.T) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{
		Log: log,
		ItemVolatiles: map[int]int{
			itemCartaN: volCartaDuelo, itemCartaM: volCartaDuelo, itemCartaA: volCartaDuelo,
		},
	})
	w := world.New(world.Config{}, log, nil, d.Handle) // default 4096 grid: the dungeon sits near y=3700
	// The run reads generator populations to decide when a sala is clear, so the
	// blocks have to exist as they do in production. They start empty; a test that
	// cares sets CurrentNumMob through w.GeneratorAt.
	gens := make([]*world.Generator, world.SecretRoomGenLast+1)
	for i := world.SecretRoomGenFirst; i <= world.SecretRoomGenLast; i++ {
		gens[i] = &world.Generator{}
	}
	w.RegisterGenerators(gens)
	e := &world.Entity{
		ID: 0, Mode: world.MobUser, Name: "Lider",
		Level: 200, MaxHP: 1000, MaxMP: 1000, HP: 1000, MP: 1000,
		X: cartaAltarBox.x1, Y: cartaAltarBox.y1,
	}
	return d, w, &world.Session{Conn: 0, Mode: world.UserPlay}, e
}

// The three cards select the three block sets, ten apart (Basedef.h:374-420).
// Item 1731 reaches the A set through the legacy's default branch, so it must be
// recognised explicitly rather than fall through as "unknown".
func TestCartaBaseForCard(t *testing.T) {
	cases := []struct {
		item int16
		base int
	}{
		{itemCartaN, 2395},
		{itemCartaM, 2405},
		{itemCartaA, 2415},
	}
	for _, tt := range cases {
		got, ok := cartaBaseForCard(tt.item)
		if !ok || got != tt.base {
			t.Errorf("cartaBaseForCard(%d) = %d,%v; want %d,true", tt.item, got, ok, tt.base)
		}
	}
	if _, ok := cartaBaseForCard(9999); ok {
		t.Error("cartaBaseForCard(9999) reported a card set")
	}
}

// Every block maps to its sala, and to the sibling blocks that must all be down
// before the run advances. Sala 4 owns four blocks — the two boss templates
// included, which is how the legacy counts it even though the ticket only spawns
// one of them.
func TestCartaBlockSala(t *testing.T) {
	cases := []struct {
		block  int
		sala   uint8
		blocks []int
	}{
		{2395, 1, []int{2395, 2396}},
		{2396, 1, []int{2395, 2396}},
		{2398, 2, []int{2397, 2398}},
		{2400, 3, []int{2399, 2400}},
		{2403, 4, []int{2401, 2402, 2403, 2404}},
		{2414, 4, []int{2411, 2412, 2413, 2414}}, // M set
		{2415, 1, []int{2415, 2416}},             // A set
	}
	for _, tt := range cases {
		sala, blocks, ok := cartaBlockSala(tt.block)
		if !ok || sala != tt.sala {
			t.Errorf("cartaBlockSala(%d) = sala %d,%v; want %d,true", tt.block, sala, ok, tt.sala)
			continue
		}
		if len(blocks) != len(tt.blocks) {
			t.Errorf("cartaBlockSala(%d) blocks = %v, want %v", tt.block, blocks, tt.blocks)
			continue
		}
		for i := range blocks {
			if blocks[i] != tt.blocks[i] {
				t.Errorf("cartaBlockSala(%d) blocks = %v, want %v", tt.block, blocks, tt.blocks)
				break
			}
		}
	}
	for _, block := range []int{2394, 2425, 0, 1078} {
		if _, _, ok := cartaBlockSala(block); ok {
			t.Errorf("cartaBlockSala(%d) claimed a sala", block)
		}
	}
}

// The clock walks the run 1→2→3→4 and then ends it. Reaching 1 is what advances:
// the legacy never lets the counter reach 0 except by finishing.
func TestCartaClockAdvancesThroughSalas(t *testing.T) {
	d, w, _, _ := cartaFixture(t)
	d.events.cartaTime, d.events.cartaSala = cartaSalaSeconds, 1

	for sala := uint8(1); sala <= 3; sala++ {
		if d.events.cartaSala != sala {
			t.Fatalf("sala = %d, want %d", d.events.cartaSala, sala)
		}
		d.events.cartaTime = 1 // the tick that trips the advance
		d.tickCarta(w)
		if d.events.cartaSala != sala+1 {
			t.Fatalf("after the clock ran out on sala %d: sala = %d, want %d",
				sala, d.events.cartaSala, sala+1)
		}
		if d.events.cartaTime != cartaSalaSeconds {
			t.Errorf("clock = %d entering sala %d, want %d",
				d.events.cartaTime, d.events.cartaSala, cartaSalaSeconds)
		}
	}

	// Sala 4 does not advance — it ends the run.
	d.events.cartaTime = 1
	d.tickCarta(w)
	if d.events.cartaSala != 0 || d.events.cartaTime != 0 {
		t.Errorf("after sala 4: sala=%d time=%d, want 0/0", d.events.cartaSala, d.events.cartaTime)
	}
	if d.cartaRunning() {
		t.Error("cartaRunning() still true after the run ended")
	}
}

// A plain tick only counts down.
func TestCartaClockCountsDown(t *testing.T) {
	d, w, _, _ := cartaFixture(t)
	d.events.cartaTime, d.events.cartaSala = 10, 2
	d.tickCarta(w)
	if d.events.cartaTime != 9 || d.events.cartaSala != 2 {
		t.Errorf("time=%d sala=%d, want 9/2", d.events.cartaTime, d.events.cartaSala)
	}
	// With no run in progress the clock must stay put.
	d.events.cartaTime, d.events.cartaSala = 0, 0
	d.tickCarta(w)
	if d.events.cartaTime != 0 || d.events.cartaSala != 0 {
		t.Errorf("idle tick moved the run to time=%d sala=%d", d.events.cartaTime, d.events.cartaSala)
	}
}

// The card is refused off the altar, and NOT consumed — the player must keep it.
func TestCartaRefusedOffTheAltar(t *testing.T) {
	d, w, s, e := cartaFixture(t)
	e.X, e.Y = 2100, 2100 // anywhere else
	e.Carry[0] = world.Item{Index: itemCartaN}

	d.useCartaDuelo(w, s, e, 0)

	if e.Carry[0].Index != itemCartaN {
		t.Error("the card was consumed by a refused attempt")
	}
	if d.cartaRunning() {
		t.Error("a run started from off the altar")
	}
}

// Only a party leader opens a run; members carry their leader's conn in Leader.
func TestCartaRefusedForPartyMember(t *testing.T) {
	d, w, s, e := cartaFixture(t)
	e.Leader = 7 // a member, not the leader
	e.Carry[0] = world.Item{Index: itemCartaM}

	d.useCartaDuelo(w, s, e, 0)

	if e.Carry[0].Index != itemCartaM {
		t.Error("the card was consumed by a refused attempt")
	}
	if d.cartaRunning() {
		t.Error("a party member started a run")
	}
}

// Clearing sala 4 sets the short fuse instead of ending the run outright, so the
// boss's drop and death land before the room is emptied (MobKilled.cpp:1975).
func TestCartaBossSetsShortFuse(t *testing.T) {
	d, w, _, _ := cartaFixture(t)
	d.events.cartaTime, d.events.cartaSala = cartaSalaSeconds, cartaLastSala

	d.advanceCarta(w) // the boss path goes through the end branch
	if d.events.cartaSala != 0 {
		t.Fatalf("sala = %d after the boss, want the run ended", d.events.cartaSala)
	}

	// And the hook itself only arms the fuse. The dying mob is still counted here
	// — mobKilled runs before DespawnMob — so "last one down" is a sum of 1.
	d.events.cartaTime, d.events.cartaSala = cartaSalaSeconds, cartaLastSala
	w.GeneratorAt(2403).CurrentNumMob = 1
	mob := &world.Entity{GenIndex: 2403} // an N-set sala-4 block
	d.cartaRoomCleared(w, mob)
	if d.events.cartaSala != cartaLastSala {
		t.Errorf("sala = %d, want the run still on the boss", d.events.cartaSala)
	}
	if d.events.cartaTime != cartaBossSeconds {
		t.Errorf("clock = %d, want the %d-second fuse", d.events.cartaTime, cartaBossSeconds)
	}
}

// A kill in a sala the run is not in must not advance anything: leftovers from an
// earlier run would otherwise push a party forward out of nowhere.
func TestCartaIgnoresKillsFromAnotherSala(t *testing.T) {
	d, w, _, _ := cartaFixture(t)
	d.events.cartaTime, d.events.cartaSala = cartaSalaSeconds, 2

	// Sala 1 is down to its last mob, which WOULD advance the run if the sala
	// guard were missing — the population alone is what the legacy reads.
	w.GeneratorAt(2395).CurrentNumMob = 1
	d.cartaRoomCleared(w, &world.Entity{GenIndex: 2395}) // sala 1 block
	if d.events.cartaSala != 2 {
		t.Errorf("sala = %d, want it untouched at 2", d.events.cartaSala)
	}
	// And a mob that belongs to no carta block at all.
	d.cartaRoomCleared(w, &world.Entity{GenIndex: 1078})
	if d.events.cartaSala != 2 {
		t.Errorf("sala = %d after an unrelated kill, want 2", d.events.cartaSala)
	}
	// Nothing happens while no run is in progress either.
	d.events.cartaSala = 0
	d.cartaRoomCleared(w, &world.Entity{GenIndex: 2395})
	if d.cartaRunning() {
		t.Error("a kill started a run")
	}
}

// Clearing the sala the run is in advances it early, without waiting out the
// clock. Both blocks of the sala count: the run advances only when the pair is
// down to its last mob, not when one block empties.
func TestCartaSalaClearedAdvancesEarly(t *testing.T) {
	d, w, _, _ := cartaFixture(t)
	d.events.cartaTime, d.events.cartaSala = 42, 1

	// One block empty, the other still holding two: not clear yet.
	w.GeneratorAt(2395).CurrentNumMob = 0
	w.GeneratorAt(2396).CurrentNumMob = 2
	d.cartaRoomCleared(w, &world.Entity{GenIndex: 2396})
	if d.events.cartaSala != 1 {
		t.Fatalf("sala = %d with mobs still up, want 1", d.events.cartaSala)
	}

	// Last one down (still counted, so the sum is 1) → advance.
	w.GeneratorAt(2396).CurrentNumMob = 1
	d.cartaRoomCleared(w, &world.Entity{GenIndex: 2396})
	if d.events.cartaSala != 2 {
		t.Errorf("sala = %d after the sala was cleared, want 2", d.events.cartaSala)
	}
	if d.events.cartaTime != cartaSalaSeconds {
		t.Errorf("clock = %d, want it reset to %d", d.events.cartaTime, cartaSalaSeconds)
	}
}
