package npcpanel

import "testing"

func TestIsNPC(t *testing.T) {
	tests := []struct {
		name     string
		merchant int
		clan     uint8
		x, y     int32
		want     bool
	}{
		// A role byte is enough on its own, wherever the actor stands: a Zakum
		// raid spawn out in the field is still something a moderator places.
		{"lojista em Armia", 1, 0, 2100, 2100, true},
		{"Zakum em campo com Merchant=64", 64, 1, 2239, 1186, true},

		// Merchant==0 needs the built-up town. Real city content ships combine
		// statues and decorative actors with no role byte.
		{"ator decorativo dentro do retangulo de Armia", 0, 0, 2100, 2100, true},
		{"mesmo ator, clã hostil", 0, 1, 2100, 2100, false},
		{"clã 5 também é hostil", 0, 5, 2100, 2100, false},

		// The reason InTown uses the rectangle and not Classify's radius: these
		// Ghoul/Troll spawns sit within 150 of Azran's centre but outside its
		// streets, and counting them as town NPCs put 148 monsters in the city.
		{"monstro perto de Azran mas fora do retangulo", 0, 2, 2494, 1600, false},
		{"dentro do retangulo de Azran", 0, 2, 2494, 1700, true},

		{"campo aberto sem papel", 0, 0, 500, 500, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNPC(tt.merchant, tt.clan, tt.x, tt.y); got != tt.want {
				t.Errorf("isNPC(%d, %d, %d, %d) = %v, want %v",
					tt.merchant, tt.clan, tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// hostileToPlayers reproduces one row of the legacy clan matrix by hand, so it
// is worth pinning against the row as documented in world/clan.go.
func TestHostileToPlayers(t *testing.T) {
	row := [9]uint8{1, 0, 1, 1, 1, 0, 1, 1, 1} // g_pClanTable[0], 0 = hostile
	for clan := uint8(0); clan < 9; clan++ {
		want := row[clan] == 0
		if got := hostileToPlayers(clan); got != want {
			t.Errorf("hostileToPlayers(%d) = %v, want %v per g_pClanTable[0]", clan, got, want)
		}
	}
}

// applyZone must keep the city label consistent with the position, because the
// panel lets a moderator move an NPC and the city has to follow.
func TestApplyZoneFollowsPosition(t *testing.T) {
	n := NPC{X: 2086, Y: 2093}
	applyZone(&n)
	if n.Zone != "Armia" || n.NearZone != "" {
		t.Fatalf("in Armia: zone=%q nearZone=%q, want Armia with no nearest", n.Zone, n.NearZone)
	}

	// Move it out to open field: the label must flip and gain a bearing.
	n.X, n.Y = 500, 500
	applyZone(&n)
	if n.ZoneID != 8 {
		t.Errorf("moved to open field: zoneId = %d, want 8 (Campo)", n.ZoneID)
	}
	if n.NearZone == "" || n.NearDist == 0 {
		t.Errorf("Campo NPC must carry a nearest settlement, got %q at %d", n.NearZone, n.NearDist)
	}

	// And back again: the stale bearing must be cleared, not left behind.
	n.X, n.Y = 2086, 2093
	applyZone(&n)
	if n.Zone != "Armia" || n.NearZone != "" || n.NearDist != 0 {
		t.Errorf("moved back: zone=%q nearZone=%q nearDist=%d, want Armia cleared",
			n.Zone, n.NearZone, n.NearDist)
	}
}

// IsShop is the panel default scope, so it has to follow the tmServer handler
// rather than the label table: reqShopList (handler/shop.go) opens ShopType 1
// for Merchant 1 and ShopType 3 for Merchant 19, and nothing else.
func TestIsShop(t *testing.T) {
	tests := []struct {
		merchant int
		want     bool
		why      string
	}{
		{1, true, "loja comum, ShopType 1"},
		{19, true, "loja especial, ShopType 3"},
		// Merchant 2 arrives on the same _MSG_REQShopList message, which makes it
		// look like a shop, but the handler branches early into the warehouse.
		{2, false, "banco: abre o cofre, não uma lista de preços"},
		{0, false, "monstro"},
		{16, false, "montaria"},
		{64, false, "quest / evento"},
		{100, false, "quest"},
		{111, false, "rei"},
	}
	for _, tt := range tests {
		if got := IsShop(tt.merchant); got != tt.want {
			t.Errorf("IsShop(%d) = %v, want %v (%s)", tt.merchant, got, tt.want, tt.why)
		}
	}
}
