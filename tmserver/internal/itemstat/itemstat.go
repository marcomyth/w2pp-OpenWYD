// Package itemstat applies moderator-edited item base stats over the catalog
// loaded from Release/Common/ItemList.csv (0023_item_stats).
//
// It is the item-side sibling of package mobstat, and it applies at the same
// moment: once, at boot, before the dispatcher is built. There is no
// hot-reload. These numbers feed the equip score model, which is recomputed per
// character on equip, level and so on — swapping the map under a running server
// would leave two players wearing the same item with different stats until each
// happened to recompute, which is worse than making the operator restart.
//
// Item price is the deliberate contrast: it lives in the ~15s NPC config poll
// and hot-reloads safely, because a price is only ever read at the moment of a
// shop transaction.
package itemstat

import "github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"

// Override is one item's replacement numbers.
//
// Effects is the item's WHOLE effect list, not a patch over the catalog's. The
// database row cannot express "leave this one alone": every column is a plain
// number and 0 is a legitimate value for all of them, so the moderator UI reads
// the catalog first and sends back a complete list.
type Override struct {
	Req     content.ItemReq
	Effects []content.BaseEffect
}

// Apply overlays overrides onto the catalog maps, in place.
//
// Both maps come straight from the ItemList.csv loader and are owned by the
// caller at boot, before anything else can read them, so mutating them is safe
// here and nowhere later.
func Apply(effects map[int][]content.BaseEffect, reqs map[int]content.ItemReq, overrides map[int]Override) {
	for idx, ov := range overrides {
		// An empty list is meaningful: it is how a moderator strips an item of
		// everything it granted. Assigning nil says exactly that, and matches
		// what the loader produces for an item with no effects at all.
		if len(ov.Effects) == 0 {
			delete(effects, idx)
		} else {
			effects[idx] = ov.Effects
		}

		// Requirements() omits an all-zero requirement rather than storing one,
		// so an override that clears every requirement has to delete the entry
		// instead of writing a zero value — otherwise "no requirement" and
		// "requires nothing" would be two different states of the same map.
		if ov.Req == (content.ItemReq{}) {
			delete(reqs, idx)
		} else {
			reqs[idx] = ov.Req
		}
	}
}
