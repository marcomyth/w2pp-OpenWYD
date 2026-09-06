package npcpanel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/mapzones"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
)

func TestResolveItems(t *testing.T) {
	catalog := map[int32]itemcatalog.Entry{
		7: {Index: 7, DisplayName: "Espada Longa", Mesh: 42, Grade: 2, IconKey: "i0007"},
	}
	slots := make([]savefmt.Item, 4)
	slots[1] = savefmt.Item{Index: 7, Effects: [3]savefmt.Effect{{Effect: 61, Value: 5}}}
	// An index the catalog does not describe must survive with its number, not
	// be dropped: the NPC really is carrying it.
	slots[3] = savefmt.Item{Index: 9999}

	got := resolveItems(slots, catalog)
	if len(got) != 2 {
		t.Fatalf("resolveItems returned %d items, want 2 (empty slots skipped)", len(got))
	}
	if got[0].Slot != 1 || got[0].Name != "Espada Longa" || got[0].Mesh != 42 || got[0].Grade != 2 {
		t.Errorf("got[0] = %+v, want slot 1 resolved from the catalog", got[0])
	}
	if got[0].Effects[0] != [2]int{61, 5} {
		t.Errorf("got[0].Effects[0] = %v, want [61 5]", got[0].Effects[0])
	}
	if got[1].Slot != 3 || got[1].Index != 9999 || got[1].Name != "" {
		t.Errorf("got[1] = %+v, want the uncatalogued index kept with an empty name", got[1])
	}
}

func contentDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "Release")
	if _, err := os.Stat(filepath.Join(dir, "TMsrv", "run", "NPCGener.txt")); err != nil {
		t.Skipf("content tree unavailable: %v", err)
	}
	return dir
}

// TestLoadReal exercises the whole inventory against the shipped content tree —
// the parity-critical path, since every number the panel shows comes from it.
func TestLoadReal(t *testing.T) {
	data, err := Load(contentDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if data.Stats.Blocks < 6000 {
		t.Errorf("parsed %d blocks, want the full NPCGener (~6099)", data.Stats.Blocks)
	}
	// The merchant count is the figure npc-generator-inventory.md documents; a
	// drift here means the content tree or the decoder changed under us.
	if data.Stats.Merchants != 548 {
		t.Errorf("merchant blocks = %d, want 548 (npc-generator-inventory.md)", data.Stats.Merchants)
	}
	if len(data.NPCs) == 0 {
		t.Fatal("no NPCs loaded")
	}

	for _, n := range data.NPCs {
		if n.Zone == "" {
			t.Fatalf("NPC %s has no zone label", n.Slug)
		}
		// Every NPC must be classified: Field is a real answer, "unlabelled" is
		// not. A Field NPC must also carry the nearest settlement, which is what
		// makes the row useful to a moderator.
		if n.ZoneID == mapzones.Field && n.NearZone == "" {
			t.Fatalf("Field NPC %s has no nearest settlement", n.Slug)
		}
		if n.ZoneID != mapzones.Field && n.NearZone != "" {
			t.Fatalf("settled NPC %s should not carry a nearest settlement", n.Slug)
		}
		if n.Merchant == 0 && len(n.Shop) > 0 {
			t.Fatalf("non-merchant %s has a shop window; Carry[] there is a drop table", n.Slug)
		}
	}
}

// The four Armia NPCs that the legacy CityLimit rectangle excludes are the
// reason the panel classifies by radius. If they ever fall out of Armia again
// the city grouping has silently regressed.
func TestLoadPlacesArmiaStragglers(t *testing.T) {
	data, err := Load(contentDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]bool{
		"Mestre_Archi": false, "Foema_Ancian": false,
		"ForeLearner": false, "Cap.Cavaleiros": false,
	}
	for _, n := range data.NPCs {
		if _, tracked := want[n.Template]; !tracked {
			continue
		}
		if n.Zone != "Armia" {
			t.Errorf("%s at (%d,%d) classified as %q, want Armia", n.Template, n.X, n.Y, n.Zone)
		}
		want[n.Template] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("%s not found in the inventory", name)
		}
	}
}

// OnlyShops is what the panel serves by default, so its counts must describe the
// narrowed set — a zone tally left over from the wider map would contradict the
// rows on screen.
func TestOnlyShops(t *testing.T) {
	full := Data{
		Zones: []Zone{{ID: 0, Name: "Armia", Count: 3}, {ID: 8, Name: "Campo", Count: 1}},
		NPCs: []NPC{
			{Slug: "loja-1", Merchant: 1, ZoneID: 0},
			{Slug: "loja-19", Merchant: 19, ZoneID: 0},
			{Slug: "banco", Merchant: 2, ZoneID: 0},
			{Slug: "quest", Merchant: 100, ZoneID: 0},
			{Slug: "loja-campo", Merchant: 1, ZoneID: 8},
		},
		Stats: Stats{Blocks: 6099, InSettlement: 4},
	}

	got := full.OnlyShops()
	if len(got.NPCs) != 3 {
		t.Fatalf("kept %d NPCs, want the 3 shopkeepers", len(got.NPCs))
	}
	for _, n := range got.NPCs {
		if !IsShop(n.Merchant) {
			t.Errorf("%s (merchant %d) survived the shop filter", n.Slug, n.Merchant)
		}
	}
	if got.Zones[0].Count != 2 || got.Zones[1].Count != 1 {
		t.Errorf("zone counts = %d/%d, want 2 in Armia and 1 in Campo", got.Zones[0].Count, got.Zones[1].Count)
	}
	// Only the two Armia shops are in a settlement; the field one is not.
	if got.Stats.InSettlement != 2 {
		t.Errorf("InSettlement = %d, want 2", got.Stats.InSettlement)
	}
	if got.Stats.EmptyShops != 3 {
		t.Errorf("EmptyShops = %d, want 3 (nenhuma das lojas de teste tem estoque)", got.Stats.EmptyShops)
	}
	if got.Stats.Shops != 3 || !got.Stats.ShopsOnly {
		t.Errorf("Stats = %+v, want Shops 3 and ShopsOnly true", got.Stats)
	}
	// The scan-level figures describe the content tree, not the filter, so they
	// must survive: the UI reports them to explain what it left out.
	if got.Stats.Blocks != 6099 {
		t.Errorf("Blocks = %d, want the scan figure preserved", got.Stats.Blocks)
	}
	// Narrowing must not mutate the caller's data.
	if len(full.NPCs) != 5 || full.Zones[0].Count != 3 {
		t.Error("OnlyShops mutated the receiver")
	}
}

// Against the real content tree the shop scope must resolve to the documented
// figures: 88 blocks with Merchant 1 plus 8 with Merchant 19
// (npc-generator-inventory.md).
func TestOnlyShopsReal(t *testing.T) {
	data, err := Load(contentDir(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	shops := data.OnlyShops()
	if len(shops.NPCs) != 96 {
		t.Errorf("shop scope has %d NPCs, want 96 (88 Merchant 1 + 8 Merchant 19)", len(shops.NPCs))
	}
	var m1, m19 int
	for _, n := range shops.NPCs {
		switch n.Merchant {
		case 1:
			m1++
		case 19:
			m19++
		default:
			t.Fatalf("%s has merchant %d in shop scope", n.Slug, n.Merchant)
		}
	}
	if m1 != 88 || m19 != 8 {
		t.Errorf("merchant split = %d/%d, want 88/8", m1, m19)
	}

	// Eight shopkeepers ship with an entirely zero Carry[] — verified against the
	// raw 816-byte templates, so this is content, not a decoding failure. Pinning
	// the exact set means a real decoding regression (which would empty many more)
	// still fails here.
	wantEmpty := map[string]bool{
		"Prona-22": true, "Prona-4232": true, "Irena_-289": true, "Lainy-286": true,
		"RoPerion-288": true, "Balmers-271": true, "Naomi-273": true, "Rubyen-272": true,
	}
	gotEmpty := map[string]bool{}
	for _, n := range shops.NPCs {
		if len(n.Shop) == 0 {
			gotEmpty[n.Slug] = true
		}
	}
	for slug := range wantEmpty {
		if !gotEmpty[slug] {
			t.Errorf("%s was expected to have an empty window but has stock", slug)
		}
	}
	for slug := range gotEmpty {
		if !wantEmpty[slug] {
			t.Errorf("%s has an empty window unexpectedly — check the STRUCT_MOB decode", slug)
		}
	}
	if shops.Stats.EmptyShops != len(wantEmpty) {
		t.Errorf("Stats.EmptyShops = %d, want %d", shops.Stats.EmptyShops, len(wantEmpty))
	}
}
