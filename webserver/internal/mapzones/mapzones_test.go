package mapzones

import "testing"

// The first five ids are frozen: they mirror tmserver/internal/world/city.go's
// cities array (index = city id there) and the ListMapZones contract published
// in docs/integrations/npc-admin-nextjs.md. Derived settlements may be appended
// after them, but renumbering these would silently relabel stored map_id values.
func TestCityIDsAreFrozen(t *testing.T) {
	want := []struct {
		id   int32
		name string
	}{
		{0, "Armia"},
		{1, "Azran"},
		{2, "Erion"},
		{3, "Nippleheim"},
		{4, "Noatum"},
	}
	if len(All) < len(want) {
		t.Fatalf("len(All) = %d, want at least %d", len(All), len(want))
	}
	for i, w := range want {
		if All[i].ID != w.id || All[i].Name != w.name {
			t.Errorf("All[%d] = {%d %q}, want {%d %q}", i, All[i].ID, All[i].Name, w.id, w.name)
		}
		if !All[i].Verified {
			t.Errorf("All[%d] (%s) must be Verified: it comes from Basedef.cpp:54", i, w.name)
		}
	}
}

func TestIDsAreUnique(t *testing.T) {
	seen := map[int32]string{}
	for _, z := range All {
		if prev, dup := seen[z.ID]; dup {
			t.Errorf("id %d used by both %q and %q", z.ID, prev, z.Name)
		}
		seen[z.ID] = z.Name
	}
}

// Settlements must not overlap, otherwise Classify's nearest-centre tie-break
// would be papering over a table mistake rather than a real ambiguity.
func TestSettlementsDoNotOverlap(t *testing.T) {
	for i, a := range All {
		if a.Radius == 0 {
			continue
		}
		for _, b := range All[i+1:] {
			if b.Radius == 0 {
				continue
			}
			dx, dy := int64(a.CX-b.CX), int64(a.CY-b.CY)
			gap := int64(a.Radius) + int64(b.Radius)
			if dx*dx+dy*dy <= gap*gap {
				t.Errorf("%q and %q overlap", a.Name, b.Name)
			}
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		x, y int32
		want int32
	}{
		{"centro de Armia", 2086, 2093, 0},
		{"Azran", 2494, 1707, 1},
		{"Erion", 2453, 2000, 2},
		{"Nippleheim", 3652, 3122, 3},
		{"Noatum", 1050, 1706, 4},

		// The four Armia NPCs the legacy CityLimit rectangle wrongly excludes
		// (it ends at y=2052) — the reason this classifies by radius.
		{"Mestre_Archi ao norte do retangulo de Armia", 2102, 2038, 0},
		{"Foema_Ancian ao norte do retangulo", 2097, 2038, 0},
		{"ForeLearner ao norte do retangulo", 2090, 2047, 0},
		{"Cap.Cavaleiros ao norte do retangulo", 2095, 2047, 0},
		// And the guard 19 tiles east of the rectangle's x limit (2171).
		{"Guarda_Carga a leste do retangulo", 2190, 2148, 0},

		{"cidade do leste (banco Guarda_Carga)", 3241, 1683, 5},
		{"Pesadelo Mistico: loja Smith dentro da masmorra", 1113, 332, 6},
		{"Pesadelo Normal: loja Martin dentro da masmorra", 1317, 346, 7},

		{"Zakum em campo aberto", 2239, 1186, Field},
		{"Prona estacionada no canto do mapa", 4000, 4000, Field},
		{"Merc_Zakum em campo", 866, 999, Field},
		{"origem do grid", 0, 0, Field},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.x, tt.y); got != tt.want {
				t.Errorf("Classify(%d, %d) = %d (%s), want %d (%s)",
					tt.x, tt.y, got, Name(got), tt.want, Name(tt.want))
			}
		})
	}
}

func TestName(t *testing.T) {
	if got := Name(0); got != "Armia" {
		t.Errorf("Name(0) = %q, want %q", got, "Armia")
	}
	if got := Name(Field); got == "" {
		t.Error("Name(Field) = \"\", want a label")
	}
	if got := Name(1234); got != "" {
		t.Errorf("Name(1234) = %q, want \"\" for an unknown id", got)
	}
}

// A Field NPC still needs a useful label, so Nearest must name the settlement
// it stands closest to and how far away it is.
func TestNearest(t *testing.T) {
	tests := []struct {
		name     string
		x, y     int32
		wantZone string
		wantDist int32
	}{
		// Merchant==100 quest NPCs that sit outside Erion's radius: the panel
		// shows them as Campo, qualified by the town they belong near.
		{"Coveiro fora de Erion", 2375, 2104, "Erion", 130},
		{"Ferreiro_Penado fora de Erion", 2515, 2158, "Erion", 169},
		{"Zakum em campo aberto", 2239, 1186, "Azran", 580},
		{"dentro de Armia", 2086, 2093, "Armia", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			z, d := Nearest(tt.x, tt.y)
			if z.Name != tt.wantZone {
				t.Errorf("Nearest(%d,%d) zone = %q, want %q", tt.x, tt.y, z.Name, tt.wantZone)
			}
			// isqrt truncates, so allow the one-unit slack that costs.
			if d < tt.wantDist-1 || d > tt.wantDist+1 {
				t.Errorf("Nearest(%d,%d) dist = %d, want ~%d", tt.x, tt.y, d, tt.wantDist)
			}
		})
	}
}

func TestIsqrt(t *testing.T) {
	for _, n := range []int64{0, 1, 2, 3, 4, 15, 16, 17, 10000, 87530} {
		got := isqrt(n)
		if got*got > n || (got+1)*(got+1) <= n {
			t.Errorf("isqrt(%d) = %d, not the integer square root", n, got)
		}
	}
}

// Zones 6, 7 and 9 are the three Pesadelo instances, and what identifies them is
// that they fall inside the 128-unit segments the entry scrolls teleport into
// (Regions.txt rows 4-6; tmserver/internal/handler/pesadelo.go). Asserting the
// segment rather than the label is what makes the identification checkable —
// before this was noticed they were labelled as two anonymous villages.
func TestPesadeloZonesMatchDungeonSegments(t *testing.T) {
	tests := []struct {
		id         int32
		segX, segY int32
		name       string
	}{
		{6, 8, 2, "Pesadelo Místico (masmorra)"},
		{7, 10, 2, "Pesadelo Normal (masmorra)"},
		{9, 9, 1, "Pesadelo Arcano (masmorra)"},
	}
	for _, tt := range tests {
		var z Zone
		for _, c := range All {
			if c.ID == tt.id {
				z = c
			}
		}
		if z.Name != tt.name {
			t.Errorf("zone %d = %q, want %q", tt.id, z.Name, tt.name)
		}
		if z.CX/128 != tt.segX || z.CY/128 != tt.segY {
			t.Errorf("zone %d centre (%d,%d) is in segment (%d,%d), want (%d,%d)",
				tt.id, z.CX, z.CY, z.CX/128, z.CY/128, tt.segX, tt.segY)
		}
	}
}
