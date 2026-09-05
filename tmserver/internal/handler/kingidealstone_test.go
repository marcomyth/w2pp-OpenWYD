package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// equipForKing puts the Immortality Stone and a Sephirot on, which is the entry
// condition both King transformations share.
func equipForKing(e *world.Entity) {
	e.Equip[idealStoneEquipSlot] = world.Item{Index: idealStoneItem}
	e.Equip[sephirotEquipSlot] = world.Item{Index: archSephirotMin}
}

func TestFindSecretStones(t *testing.T) {
	t.Run("all four found", func(t *testing.T) {
		e := &world.Entity{}
		e.Carry[3] = world.Item{Index: 5336} // Sol
		e.Carry[7] = world.Item{Index: 5334} // Água
		e.Carry[1] = world.Item{Index: 5337} // Vento
		e.Carry[9] = world.Item{Index: 5335} // Terra
		slots, ok := findSecretStones(e)
		if !ok {
			t.Fatal("complete = false with all four stones carried")
		}
		want := [4]int{7, 9, 3, 1} // Água, Terra, Sol, Vento — by stone, not by slot
		if slots != want {
			t.Errorf("slots = %v, want %v", slots, want)
		}
	})

	t.Run("one missing is not complete", func(t *testing.T) {
		e := &world.Entity{}
		e.Carry[0] = world.Item{Index: 5334}
		e.Carry[1] = world.Item{Index: 5335}
		e.Carry[2] = world.Item{Index: 5336}
		if _, ok := findSecretStones(e); ok {
			t.Error("complete = true with only three stones")
		}
	})

	t.Run("duplicates of one stone do not stand in for another", func(t *testing.T) {
		e := &world.Entity{}
		for i := range 4 {
			e.Carry[i] = world.Item{Index: 5334}
		}
		if _, ok := findSecretStones(e); ok {
			t.Error("complete = true with four copies of the same stone")
		}
	})
}

// The fusion consumes everything and leaves the Ideal Stone where the Água stone
// was, which is where the player will look for it.
func TestKingIdealStone(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	equipForKing(e)
	e.Carry[5] = world.Item{Index: 5334}
	e.Carry[6] = world.Item{Index: 5335}
	e.Carry[7] = world.Item{Index: 5336}
	e.Carry[8] = world.Item{Index: 5337}

	if !d.kingIdealStone(w, s, e) {
		t.Fatal("kingIdealStone() = false with all four stones")
	}
	if got := int(e.Carry[5].Index); got != idealStoneResult {
		t.Errorf("carry[5] = %d, want the Ideal Stone %d", got, idealStoneResult)
	}
	for _, slot := range []int{6, 7, 8} {
		if e.Carry[slot].Index != 0 {
			t.Errorf("carry[%d] = %d, want empty — the stone was not consumed", slot, e.Carry[slot].Index)
		}
	}
	if e.Equip[idealStoneEquipSlot].Index != 0 {
		t.Error("the Immortality Stone survived the fusion")
	}
	if e.Equip[sephirotEquipSlot].Index != 0 {
		t.Error("the Sephirot survived the fusion")
	}
}

// Without the stones the fusion must decline, so the caller falls through to the
// Mortal→Arch path instead of swallowing the click.
func TestKingIdealStoneDeclinesWithoutStones(t *testing.T) {
	d, w, e := mobKilledWorld(t)
	s := &world.Session{Conn: 0, Mode: world.UserPlay}
	equipForKing(e)
	e.Carry[5] = world.Item{Index: 5334}
	e.Carry[6] = world.Item{Index: 5335}

	if d.kingIdealStone(w, s, e) {
		t.Fatal("kingIdealStone() = true with only two stones")
	}
	if e.Carry[5].Index != 5334 || e.Carry[6].Index != 5335 {
		t.Error("a declined fusion consumed stones anyway")
	}
	if e.Equip[idealStoneEquipSlot].Index != idealStoneItem {
		t.Error("a declined fusion consumed the Immortality Stone")
	}
}
