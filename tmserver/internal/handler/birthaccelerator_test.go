package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	itemBirthAccelerator = 3438
	itemOvoAndaluzN      = 2310
)

// accelDB puts the accelerator in slot 0 and the target in slot 1.
func accelDB(target world.Item, targetLevel int) *fakeDB {
	db := newDB()
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 100}
	st.Carry[0] = world.Item{Index: itemBirthAccelerator}
	refine.Bootstrap(&target)
	if targetLevel > 0 {
		refine.Set(&target, targetLevel, 0)
	}
	st.Carry[1] = target
	db.loadResult = st
	return db
}

func accelFrame(t *testing.T, c net.Conn) {
	t.Helper()
	body := protocol.MsgUseItemBody{
		SourType: world.ItemPlaceCarry, SourPos: 0,
		DestType: world.ItemPlaceCarry, DestPos: 1,
	}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

// slotItem drains until it sees the SendItem for slot and returns its raw item.
func slotItem(t *testing.T, c net.Conn, slot int) []byte {
	t.Helper()
	for i := 0; i < 12; i++ {
		p := expect(t, c, protocol.MsgSendItem)
		if le16(p[2:4]) == uint16(slot) {
			return p[4:]
		}
	}
	t.Fatalf("no SendItem for slot %d", slot)
	return nil
}

// decodeItem rebuilds an item from a SendItem payload.
func decodeItem(b []byte) world.Item {
	var it world.Item
	it.Index = int16(le16(b[0:2]))
	it.Effects[0] = world.Effect{Effect: b[2], Value: b[3]}
	it.Effects[1] = world.Effect{Effect: b[4], Value: b[5]}
	it.Effects[2] = world.Effect{Effect: b[6], Value: b[7]}
	return it
}

// eggWithIncubate is the shape a GM hands out for testing: "/gm item 2310 78 3",
// an egg whose INSTANCE carries the hatch threshold.
func eggWithIncubate(threshold uint8) world.Item {
	return world.Item{
		Index:   itemOvoAndaluzN,
		Effects: [3]world.Effect{{}, {Effect: efIncubate, Value: threshold}},
	}
}

// The whole point of the item: one guaranteed refine level, no roll.
func TestBirthAcceleratorRaisesEggOneLevel(t *testing.T) {
	vols := map[int]int{itemBirthAccelerator: volBirthAccelerator}
	addr, stop := startServerClockVol(t, accelDB(eggWithIncubate(3), 1), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	accelFrame(t, c)
	it := decodeItem(slotItem(t, c, 1))
	if got := refine.Level(it); got != 2 {
		t.Errorf("egg refine = +%d, want +2", got)
	}
	if it.Index != itemOvoAndaluzN {
		t.Errorf("egg index = %d, want %d (below its threshold it must not hatch)", it.Index, itemOvoAndaluzN)
	}
}

// Reaching the threshold hatches it, on the same rule a dust refine uses: the
// accelerator is a guaranteed step toward the mount, not a way past the last one.
func TestBirthAcceleratorHatchesAtThreshold(t *testing.T) {
	vols := map[int]int{itemBirthAccelerator: volBirthAccelerator}
	addr, stop := startServerClockVol(t, accelDB(eggWithIncubate(3), 2), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	accelFrame(t, c)
	it := decodeItem(slotItem(t, c, 1))
	if it.Index != itemOvoAndaluzN+eggHatchAdd {
		t.Errorf("egg index = %d, want %d (hatched)", it.Index, itemOvoAndaluzN+eggHatchAdd)
	}
}

// "Aplicável a todos ovos de montaria": anything else is refused, and — the part
// that matters to a player — NOT consumed. A premium item that vanishes without
// a word reads as theft.
func TestBirthAcceleratorRefusesNonEggWithoutConsuming(t *testing.T) {
	vols := map[int]int{itemBirthAccelerator: volBirthAccelerator}
	addr, stop := startServerClockVol(t, accelDB(world.Item{Index: 1042}, 0), vols) // a sword
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	accelFrame(t, c)
	if code := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); code != NoticeCantUseHere {
		t.Errorf("notice = %d, want NoticeCantUseHere", code)
	}
	acc := slotItem(t, c, 0)
	if le16(acc[0:2]) != itemBirthAccelerator {
		t.Errorf("accelerator was consumed on a refused use: slot 0 = %d", le16(acc[0:2]))
	}
}
