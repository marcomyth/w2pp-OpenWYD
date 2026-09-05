package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The King's Ideal Stone craft (_MSG_Quest.cpp:769-841). Wearing the Pedra da
// Imortalidade and a Sephirot, carrying all four Secret Stones, and confirming
// with the King fuses them into the Pedra Ideal.
//
// NOTE ON A MISLEADING NAME: idealStoneItem (1742) is the Pedra da
// IMORTALIDADE, not the Pedra Ideal. The Ideal Stone is 5338 and is what this
// craft produces. The existing constant is left alone because renaming it would
// touch the Mortal→Arch path for no behavioural gain, but the two are different
// items and the difference matters here.
const (
	// secretStoneLo..Hi are the four Pedras Secretas: Água, Terra, Sol, Vento.
	secretStoneLo = 5334
	secretStoneHi = 5337
	// idealStoneResult is the Pedra Ideal the fusion yields.
	idealStoneResult = 5338
	// msgKingBless is _NN_My_King_Bless1 (Language.txt:178).
	msgKingBless = "Que a luz de Sephira o ilumine."
)

// findSecretStones returns the carry slot holding each of the four Secret
// Stones, and whether all four were found. Index 0 is Água (5334), which is the
// slot the Ideal Stone replaces — the legacy converts that one in place and
// clears the other three.
func findSecretStones(e *world.Entity) (slots [4]int, complete bool) {
	for i := range slots {
		slots[i] = -1
	}
	for slot := range e.Carry {
		idx := int(e.Carry[slot].Index)
		if idx < secretStoneLo || idx > secretStoneHi {
			continue
		}
		if pos := idx - secretStoneLo; slots[pos] < 0 {
			slots[pos] = slot
		}
	}
	for _, slot := range slots {
		if slot < 0 {
			return slots, false
		}
	}
	return slots, true
}

// kingIdealStone fuses the four Secret Stones into the Pedra Ideal. It runs
// BEFORE the Mortal→Arch branch, mirroring the legacy: the two share the same
// entry condition (Immortality + Sephirot equipped) and the stones win when the
// player is carrying them.
//
// Everything is consumed: the four stones, and both equipped items. The Água
// slot becomes the Ideal Stone in place, which is where the player will look for
// it. Reports whether it ran, so the caller can fall through to the Arch path.
func (d *Dispatcher) kingIdealStone(w *world.World, s *world.Session, e *world.Entity) bool {
	slots, complete := findSecretStones(e)
	if !complete {
		return false
	}

	e.Carry[slots[0]] = world.Item{Index: idealStoneResult}
	d.sendSlot(w, s, world.ItemPlaceCarry, slots[0], e.Carry[slots[0]])
	for _, slot := range slots[1:] {
		e.Carry[slot] = world.Item{}
		d.sendSlot(w, s, world.ItemPlaceCarry, slot, e.Carry[slot])
	}

	// The Immortality Stone and the Sephirot are spent by the fusion (:828-832).
	for _, slot := range [2]int{idealStoneEquipSlot, sephirotEquipSlot} {
		e.Equip[slot] = world.Item{}
		d.sendSlot(w, s, world.ItemPlaceEquip, slot, e.Equip[slot])
	}
	d.refreshScore(e)
	d.sendScore(w, s, e)

	sendClientMessage(w, s, msgKingBless)
	d.log.Info("king ideal stone", "conn", s.Conn, "account", s.AccountName,
		"name", e.Name, "level", e.Level, "slot", slots[0])
	w.SaveCharacterAsync(s)
	return true
}
