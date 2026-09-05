package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The serialization boundary is the guarantee: whatever the in-memory item looks
// like, a stackable leaves with an amount.
func TestItemToSelAlwaysCarriesAmount(t *testing.T) {
	sel := itemToSel(world.Item{Index: 4117})
	found := false
	for _, e := range sel.Eff {
		if e[0] == efAmount && e[1] == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("stackable serialized without EF_AMOUNT: %v", sel.Eff)
	}
	// A non-stackable is untouched.
	sel = itemToSel(world.Item{Index: 31})
	for _, e := range sel.Eff {
		if e[0] == efAmount {
			t.Errorf("non-stackable gained EF_AMOUNT: %v", sel.Eff)
		}
	}
}

// The stored item is left exactly as it was: repairing it in place would move
// the combine recipes' effect positions.
func TestCountStacksMissingAmountDoesNotRepair(t *testing.T) {
	items := []world.Item{
		{Index: 401, Effects: [3]world.Effect{{Effect: efAmount, Value: 117}}},
		{Index: 4117},
		{},
		{Index: 31, Effects: [3]world.Effect{{Effect: 43, Value: 0}}}, // equipment
	}
	if n := countStacksMissingAmount(items); n != 1 {
		t.Fatalf("counted %d stackables missing an amount, want 1", n)
	}
	if hasAmountEffect(items[1]) {
		t.Error("the stored item was modified; it must be left alone")
	}
	if got := itemAmount(items[0]); got != 117 {
		t.Errorf("existing stack = %d, want 117 untouched", got)
	}
}
