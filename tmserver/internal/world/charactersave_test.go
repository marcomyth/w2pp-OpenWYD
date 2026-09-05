package world

import "testing"

// The progression fields the rewrite keeps in Postgres (rather than in the raw
// MobExtra blob) only survive a relog if CharacterSaveFor copies them off the
// entity. Adding a field to Entity and forgetting this function is a silent
// data-loss bug — the character plays fine and the value is gone next login —
// so the copy is pinned here.
//
// NightmareTickets is the one that costs real money: an Escritura do Pesadelo is
// 10M gold for 13 Arcano entries (handler/pesadelo.go, migration 0023).
func TestCharacterSaveForCopiesProgression(t *testing.T) {
	s := &Session{AccountID: 42, Slot: 1}
	e := &Entity{
		ClassMaster:        3,
		CelLv40:            1,
		CelLv90:            1,
		CelCircle:          1,
		ArchLv355:          1,
		ArchLv370:          1,
		MortalLevel:        399,
		CelestialArchLevel: 5,
		TerraMistica:       2,
		NightmareTickets:   13,
		Soul:               4,
		Fame:               1234,
	}

	w := &World{}
	got := w.CharacterSaveFor(s, e)

	if got.AccountID != 42 || got.Slot != 1 {
		t.Errorf("identity = account %d slot %d, want 42/1", got.AccountID, got.Slot)
	}
	if got.NightmareTickets != 13 {
		t.Errorf("NightmareTickets = %d, want 13", got.NightmareTickets)
	}
	if got.CelestialArchLevel != 5 || got.MortalLevel != 399 || got.TerraMistica != 2 {
		t.Errorf("progression lost: %+v", got)
	}
	if got.Soul != 4 || got.Fame != 1234 {
		t.Errorf("soul/fame lost: soul=%d fame=%d", got.Soul, got.Fame)
	}
}

// A nil entity must not panic: the save path runs on sessions that never got an
// entity (a disconnect during character select).
func TestCharacterSaveForNilEntity(t *testing.T) {
	w := &World{}
	got := w.CharacterSaveFor(&Session{AccountID: 7, Slot: 2}, nil)
	if got.AccountID != 7 || got.Slot != 2 {
		t.Errorf("= %+v, want the identity preserved", got)
	}
	if got.NightmareTickets != 0 {
		t.Errorf("NightmareTickets = %d, want the zero value", got.NightmareTickets)
	}
}
