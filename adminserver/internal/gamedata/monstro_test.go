package gamedata

import "testing"

// Equip is positional: the caller draws a sixteen-cell grid, so a template with
// gear only in slots 1 and 6 still has to yield sixteen entries in order.
func TestMobStatEquipPositional(t *testing.T) {
	// A stat with no override at all still has to produce the empty grid rather
	// than nil, or the editor renders no slots for an untouched template.
	vazio := MobStat{}.Equip()
	if len(vazio) != maxMobEquip {
		t.Fatalf("slots = %d, want %d", len(vazio), maxMobEquip)
	}
	for i, it := range vazio {
		if it.Slot != i || !it.Vazio() {
			t.Errorf("slot %d = %+v, want vazio na posição certa", i, it)
		}
	}
}

func TestMobEquipItemVazio(t *testing.T) {
	if !(MobEquipItem{Slot: 3}).Vazio() {
		t.Error("índice zero deveria contar como vazio")
	}
	// An item with no effects is still an item: only the index decides.
	if (MobEquipItem{Slot: 3, ItemIndex: 144}).Vazio() {
		t.Error("índice preenchido não deveria contar como vazio")
	}
}
