package npcpanel

import "github.com/jeanluca/w2pp-openwyd/internal/mapzones"

// applyZone recomputes the settlement labels for an NPC from its coordinates.
// Position is the source of truth for which city an NPC is in — move it and the
// city follows, which is why the panel never lets the two drift apart.
func applyZone(n *NPC) {
	n.ZoneID = mapzones.Classify(n.X, n.Y)
	n.Zone = mapzones.Name(n.ZoneID)
	n.NearZone, n.NearDist = "", 0
	if n.ZoneID == mapzones.Field {
		near, dist := mapzones.Nearest(n.X, n.Y)
		n.NearZone, n.NearDist = near.Name, dist
	}
}

// deriveZone is the map_id written back when a moderator moves an NPC without
// picking a zone by hand.
func deriveZone(x, y int32) int32 { return mapzones.Classify(x, y) }

// Merchant codes that open a buy/sell window. The authority is the tmServer
// handler, not the label table: reqShopList (tmserver/internal/handler/shop.go)
// maps Merchant 1 to ShopType 1 and Merchant 19 to ShopType 3, and those are the
// only two that produce a shop.
//
// Merchant 2 (Guarda_Carga) is deliberately NOT here. It arrives on the same
// _MSG_REQShopList message, which makes it look like a shop, but the handler
// branches early and opens the account warehouse instead of a price list — it is
// a bank, and it has no stock to edit. Every other code routes to _MSG_Quest or
// the combine family (npc-map.md).
const (
	merchantShop      = 1
	merchantShopType3 = 19
)

// IsShop reports whether a Merchant byte belongs to a shopkeeper.
func IsShop(merchant int) bool {
	return merchant == merchantShop || merchant == merchantShopType3
}

// hostileToPlayers reports whether a template clan attacks players on sight.
//
// This is row 0 of g_pClanTable (Basedef.cpp:207-220, mirrored in
// tmserver/internal/world/clan.go): clan 0 is what players carry, and the row
// reads {1,0,1,1,1,0,1,1,1} where 0 means hostile — so clans 1 and 5 are the
// aggressive ones. Only that row is reproduced here, rather than the whole 9x9
// matrix, because the panel asks a single question the world package cannot be
// imported to answer: is this template a town NPC or a monster?
func hostileToPlayers(clan uint8) bool { return clan == 1 || clan == 5 }

// isNPC decides whether a spawn block belongs in the panel at all.
//
// It mirrors world.nonCombatNPC: a non-zero Merchant byte always marks an
// actor with a role (shop, bank, quest, mount, event), and a template with
// Merchant == 0 still counts as a town NPC when it stands inside a settlement
// and is not hostile — real city content ships combine statues and decorative
// actors with Merchant == 0, and a merchant-only filter would lose them.
//
// Note the asymmetry with the city label: membership uses mapzones.InTown (the
// legacy CityLimit rectangle), not Classify's wider radius. Monsters spawn a
// hundred units outside a city gate but not in its streets, so using the radius
// here swept 148 Ghouls, Trolls and Orcs into Azran.
//
// Everything else is a field monster. Those belong to the drop tool and the mob
// template editor, not here: including all 5527 of them would bury the 572 real
// NPCs and inflate the page from under 1 MB to over 4 MB.
func isNPC(merchant int, clan uint8, x, y int32) bool {
	if merchant != 0 {
		return true
	}
	return mapzones.InTown(x, y) && !hostileToPlayers(clan)
}
