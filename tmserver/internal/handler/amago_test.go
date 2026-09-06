package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	itemAmagoAndaluzN = 2400 // Âmago_de_Andaluz_N
	itemCriaAndaluzN  = 2340 // Cria_de_Andaluz_N — the hatched egg
)

// amagoDB wears mount in Equip[14] and puts the âmago in carry slot 0.
func amagoDB(mount int16, level uint8, amago int16) *fakeDB {
	db := newDB()
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 100}
	m := world.Item{Index: mount}
	putShort(&m.Effects[0], 20000) // fed: a starved mount refuses the âmago
	m.Effects[1].Effect = level
	st.Equip[mountEquipSlot] = m
	st.Carry[0] = world.Item{Index: amago}
	db.loadResult = st
	return db
}

func amagoFrame(t *testing.T, c net.Conn) {
	t.Helper()
	body := protocol.MsgUseItemBody{
		SourType: world.ItemPlaceCarry, SourPos: 0,
		DestType: 0, DestPos: mountEquipSlot,
	}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

// equipItemFrame drains until the SendItem for the mount slot.
func equipItem(t *testing.T, c net.Conn) world.Item {
	t.Helper()
	for i := 0; i < 12; i++ {
		p := expect(t, c, protocol.MsgSendItem)
		if le16(p[0:2]) == uint16(world.ItemPlaceEquip) && le16(p[2:4]) == uint16(mountEquipSlot) {
			return decodeItem(p[4:])
		}
	}
	t.Fatal("no SendItem for the mount slot")
	return world.Item{}
}

// A cria grows on every feed — no roll — which is what makes the early mount
// levels deterministic.
func TestAmagoFeedsCria(t *testing.T) {
	vols := map[int]int{itemAmagoAndaluzN: volAmago}
	addr, stop := startServerClockVol(t, amagoDB(itemCriaAndaluzN, 5, itemAmagoAndaluzN), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	amagoFrame(t, c)
	got := equipItem(t, c)
	if got.Effects[1].Effect != 6 {
		t.Errorf("mount level = %d, want 6", got.Effects[1].Effect)
	}
	if got.Index != itemCriaAndaluzN {
		t.Errorf("mount index = %d, want %d (below its growth threshold)", got.Index, itemCriaAndaluzN)
	}
}

// Reaching the threshold turns the cria into the adult, sIndex+30 — the same
// step the egg took to become the cria.
func TestAmagoGrowsCriaIntoAdult(t *testing.T) {
	vols := map[int]int{itemAmagoAndaluzN: volAmago}
	// 2340 is past 2331, so its threshold is 100.
	addr, stop := startServerClockVol(t, amagoDB(itemCriaAndaluzN, 99, itemAmagoAndaluzN), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	amagoFrame(t, c)
	got := equipItem(t, c)
	if got.Index != itemCriaAndaluzN+mountRowSize {
		t.Errorf("mount index = %d, want %d (grown)", got.Index, itemCriaAndaluzN+mountRowSize)
	}
}

// The âmago row has to match the mount row: feeding an Andaluz with someone
// else's âmago is refused, and the item is NOT consumed.
func TestAmagoRefusesMismatchedMount(t *testing.T) {
	const otherAmago = 2401 // Âmago_de_Ca_s/Sela_B, a different row
	vols := map[int]int{otherAmago: volAmago}
	addr, stop := startServerClockVol(t, amagoDB(itemCriaAndaluzN, 5, otherAmago), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	amagoFrame(t, c)
	if code := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); code != NoticeMountNotMatch {
		t.Errorf("notice = %d, want NoticeMountNotMatch", code)
	}
}

func TestMountAmagoSlotSharedRows(t *testing.T) {
	// Sleipnir and Svadilfari are fed by another row's âmago (:1583-1587).
	if got := mountAmagoSlot(mountLo + mountSlotSleipnir); got != amagoSlotSleipnir {
		t.Errorf("Sleipnir slot = %d, want %d", got, amagoSlotSleipnir)
	}
	if got := mountAmagoSlot(mountLo + mountSlotSvadilfari); got != amagoSlotSvadilfari {
		t.Errorf("Svadilfari slot = %d, want %d", got, amagoSlotSvadilfari)
	}
	// An adult sits one row up and maps to the same âmago slot as its cria.
	if mountAmagoSlot(itemCriaAndaluzN) != mountAmagoSlot(itemCriaAndaluzN+mountRowSize) {
		t.Error("cria and adult must share an âmago slot")
	}
}

func TestCriaGrowsAt(t *testing.T) {
	cases := []struct {
		index int16
		want  int
	}{
		{2330, criaGrowth2330},
		{2331, criaGrowth2331},
		{itemCriaAndaluzN, criaGrowthRest},
		{2359, criaGrowthRest},
		{2360, 0}, // already an adult
	}
	for _, c := range cases {
		if got := criaGrowsAt(c.index); got != c.want {
			t.Errorf("criaGrowsAt(%d) = %d, want %d", c.index, got, c.want)
		}
	}
}
