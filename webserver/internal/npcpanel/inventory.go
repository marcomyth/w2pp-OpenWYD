// Package npcpanel builds the moderator-facing inventory of every NPC in the
// world — who they are, which city they stand in, what they wear and what they
// sell — and serves it as a local panel.
//
// The inventory is read from the read-only content tree (NPCGener.txt plus the
// 816-byte STRUCT_MOB templates), not from the database, for two reasons. The
// content tree is the only complete source: npc_definition is seeded from it and
// holds a curated subset (80 rows) until `dbserver import-npcs` runs. And it
// lets the panel answer "what NPCs do we have, and where" with no database at
// all, which is the question that comes first.
//
// Where a database IS configured the panel overlays it, so a moderator sees the
// edited position/visibility rather than the shipped one. That overlay is the
// same one-way flow the rest of the stack uses: Postgres owns the definition and
// the tmServer materializes it (npc-editing-plan.md §2).
package npcpanel

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mapzones"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npctemplates"
)

// OriginContent marks an NPC that ships with the game content; OriginCustom
// marks one a moderator created. The split mirrors npc_definition.origin
// (migration 0018), which is what the tmServer overlay keys off.
const (
	OriginContent = "content"
	OriginCustom  = "custom"
)

// Item is one occupied slot of a template's Equip[16] (the look) or Carry[64]
// (the shop window), resolved against ItemList.csv.
type Item struct {
	Slot  int    `json:"slot"`
	Index int32  `json:"index"`
	Name  string `json:"name"`
	// Mesh is IndexMesh — the model that actually decides what the NPC looks
	// like, so two items with different names but the same mesh render as the
	// same "set". Grade is the item tier (1=Normal, 2=Místico, 3=Arcano…).
	Mesh    int32     `json:"mesh,omitempty"`
	Grade   int32     `json:"grade,omitempty"`
	IconKey string    `json:"iconKey,omitempty"`
	Effects [3][2]int `json:"effects,omitempty"`
}

// NPC is one spawn block: a template placed somewhere in the world.
type NPC struct {
	// GeneratorIndex is the block position in NPCGener.txt — the stable id that
	// npc_definition.generator_index stores.
	GeneratorIndex int `json:"generatorIndex"`
	// Slug matches the one dbserver import-npcs derives, so a row edited in the
	// database lines up with the block it came from.
	Slug     string `json:"slug"`
	Template string `json:"template"`
	Name     string `json:"name"`
	Origin   string `json:"origin"`
	Enabled  bool   `json:"enabled"`

	Merchant int `json:"merchant"`
	Class    int `json:"class"`
	Clan     int `json:"clan"`
	Level    int `json:"level"`

	X int32 `json:"x"`
	Y int32 `json:"y"`
	// ZoneID/Zone is the settlement derived from X/Y. NearZone/NearDist qualify
	// a Campo NPC with the town it stands closest to.
	ZoneID   int32  `json:"zoneId"`
	Zone     string `json:"zone"`
	NearZone string `json:"nearZone,omitempty"`
	NearDist int32  `json:"nearDist,omitempty"`

	// Equip is what the NPC wears; Shop is its Carry[] window (merchants only).
	Equip []Item `json:"equip,omitempty"`
	Shop  []Item `json:"shop,omitempty"`

	Follower       string `json:"follower,omitempty"`
	RouteType      int    `json:"routeType"`
	MinuteGenerate int    `json:"minuteGenerate"`
	MaxNumMob      int    `json:"maxNumMob"`

	// DBID is the npc_definition row id when the panel runs against a database,
	// and 0 when the block has never been imported. Editing needs a row, so the
	// UI uses this to tell "editable" from "content only".
	DBID int64 `json:"dbId,omitempty"`
}

// Zone is a settlement plus how many NPCs the inventory placed in it.
type Zone struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Verified bool   `json:"verified"`
	X        int32  `json:"x"`
	Y        int32  `json:"y"`
	Radius   int32  `json:"radius"`
	Count    int    `json:"count"`
}

// Data is the whole payload the panel UI consumes.
type Data struct {
	Zones []Zone `json:"zones"`
	NPCs  []NPC  `json:"npcs"`
	// Editable reports whether a database is wired up. When false the panel is
	// a read-only map of the content tree.
	Editable bool  `json:"editable"`
	Stats    Stats `json:"stats"`
}

// Stats records what the content scan saw, so the UI can explain why a number
// differs from the raw file instead of silently dropping blocks.
type Stats struct {
	Blocks          int `json:"blocks"`
	MissingTemplate int `json:"missingTemplate"`
	UndecodableMob  int `json:"undecodableMob"`
	Merchants       int `json:"merchants"`
	InSettlement    int `json:"inSettlement"`
	// FieldMobs counts the blocks isNPC rejected — hostile monsters out in the
	// world. Reported so the panel can say what it left out instead of quietly
	// showing a smaller number than NPCGener holds.
	FieldMobs int `json:"fieldMobs"`
	// Shops counts the shopkeepers among the NPCs. It is what the panel lists by
	// default; the rest (quest givers, banks, mounts, event actors) need -all.
	Shops int `json:"shops"`
	// EmptyShops counts shopkeepers whose Carry[] is entirely zero in the content
	// — they stand in the world selling nothing. Eight ship that way (both Prona
	// blocks and the six northwest village shops). Surfaced rather than hidden:
	// an empty window is usually the reason someone opens this panel.
	EmptyShops int `json:"emptyShops"`
	// ShopsOnly records that the payload was narrowed by OnlyShops, so the UI
	// states its scope instead of implying it shows every NPC.
	ShopsOnly bool `json:"shopsOnly"`
}

// Load reads the content tree and returns every spawn block, classified by
// settlement and resolved against the item catalog.
//
// A block whose template is missing or undecodable is counted in Stats rather
// than failing the load: the shipped npc/ directory mixes STRUCT_MOB layouts and
// holds files that are not templates at all (npctemplates.Scan makes the same
// trade-off), and one bad file must not hide the other six thousand blocks.
func Load(contentDir string) (Data, error) {
	blocks, err := npcgener.Load(filepath.Join(contentDir, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		return Data{}, err
	}

	catalog, err := itemcatalog.Scan(contentDir)
	if err != nil {
		return Data{}, fmt.Errorf("npcpanel: item catalog: %w", err)
	}
	byIndex := make(map[int32]itemcatalog.Entry, len(catalog.Items))
	for _, it := range catalog.Items {
		byIndex[it.Index] = it
	}

	// Many blocks share a leader, so decode each template once.
	type decoded struct {
		mob savefmt.Mob
		ok  bool
	}
	cache := make(map[string]decoded)

	data := Data{Stats: Stats{Blocks: len(blocks)}}
	counts := make(map[int32]int)

	for i, b := range blocks {
		d, seen := cache[b.Leader]
		if !seen {
			raw, _, err := npctemplate.Load(contentDir, b.Leader)
			switch {
			case err != nil:
				data.Stats.MissingTemplate++
			default:
				mob, _, err := savefmt.DecodeMobAny(raw)
				if err != nil {
					data.Stats.UndecodableMob++
				} else {
					d = decoded{mob: mob, ok: true}
				}
			}
			cache[b.Leader] = d
		}
		if !d.ok {
			continue
		}

		x, y := int32(b.SegX[0]), int32(b.SegY[0])
		zoneID := mapzones.Classify(x, y)
		if !isNPC(int(d.mob.CurrentScore.Merchant), d.mob.Clan, x, y) {
			data.Stats.FieldMobs++
			continue
		}
		npc := NPC{
			GeneratorIndex: i,
			Slug:           fmt.Sprintf("%s-%d", b.Leader, i),
			Template:       b.Leader,
			Name:           npctemplates.CString(d.mob.Name[:]),
			Origin:         OriginContent,
			Enabled:        true,
			Merchant:       int(d.mob.CurrentScore.Merchant),
			Class:          int(d.mob.Class),
			Clan:           int(d.mob.Clan),
			Level:          int(d.mob.CurrentScore.Level),
			X:              x,
			Y:              y,
			ZoneID:         zoneID,
			Zone:           mapzones.Name(zoneID),
			Equip:          resolveItems(d.mob.Equip[:], byIndex),
			Follower:       b.Follower,
			RouteType:      b.RouteType,
			MinuteGenerate: b.MinuteGenerate,
			MaxNumMob:      b.MaxNumMob,
		}
		if zoneID == mapzones.Field {
			near, dist := mapzones.Nearest(x, y)
			npc.NearZone, npc.NearDist = near.Name, dist
		} else {
			data.Stats.InSettlement++
		}
		// Carry[] is the shop window only for a merchant; on a plain mob the
		// same array is the drop table (world/api.go, protocol.MobCarry), which
		// belongs to the drop tool, not here.
		if IsShop(npc.Merchant) {
			data.Stats.Shops++
		}
		if npc.Merchant != 0 {
			data.Stats.Merchants++
			npc.Shop = resolveItems(d.mob.Carry[:], byIndex)
		}
		counts[zoneID]++
		data.NPCs = append(data.NPCs, npc)
	}

	for _, z := range mapzones.All {
		data.Zones = append(data.Zones, Zone{
			ID: z.ID, Name: z.Name, Verified: z.Verified,
			X: z.CX, Y: z.CY, Radius: z.Radius, Count: counts[z.ID],
		})
	}
	sort.SliceStable(data.NPCs, func(i, j int) bool {
		if data.NPCs[i].ZoneID != data.NPCs[j].ZoneID {
			return data.NPCs[i].ZoneID < data.NPCs[j].ZoneID
		}
		return data.NPCs[i].Name < data.NPCs[j].Name
	})
	return data, nil
}

// OnlyShops narrows the inventory to shopkeepers — the NPCs with stock to edit.
//
// Load deliberately returns every NPC, because the by-city map is worth having
// whole and the tests pin it. Narrowing is a separate step so the wider map stays
// one flag away (-all) instead of being lost in the scan.
//
// Zone counts and Stats.InSettlement are recomputed rather than carried over: a
// count that still described the wider set would quietly contradict the rows on
// screen.
func (d Data) OnlyShops() Data {
	out := d
	out.NPCs = make([]NPC, 0, len(d.NPCs))
	counts := make(map[int32]int)
	out.Stats.InSettlement = 0
	out.Stats.EmptyShops = 0
	for _, n := range d.NPCs {
		if !IsShop(n.Merchant) {
			continue
		}
		out.NPCs = append(out.NPCs, n)
		if len(n.Shop) == 0 {
			out.Stats.EmptyShops++
		}
		counts[n.ZoneID]++
		if n.ZoneID != mapzones.Field {
			out.Stats.InSettlement++
		}
	}
	out.Zones = make([]Zone, 0, len(d.Zones))
	for _, z := range d.Zones {
		z.Count = counts[z.ID]
		out.Zones = append(out.Zones, z)
	}
	out.Stats.Shops = len(out.NPCs)
	out.Stats.ShopsOnly = true
	return out
}

// resolveItems turns an Equip[]/Carry[] array into its occupied slots only,
// named from the catalog. An index with no catalog row keeps its number and an
// empty name — the content tree ships items the CSV does not describe, and
// hiding them would misreport what the NPC is actually carrying.
func resolveItems(slots []savefmt.Item, byIndex map[int32]itemcatalog.Entry) []Item {
	var out []Item
	for i, it := range slots {
		if it.Empty() {
			continue
		}
		item := Item{Slot: i, Index: int32(it.Index)}
		if e, ok := byIndex[item.Index]; ok {
			item.Name = e.DisplayName
			item.Mesh = e.Mesh
			item.Grade = e.Grade
			item.IconKey = e.IconKey
		}
		for k, ef := range it.Effects {
			item.Effects[k] = [2]int{int(ef.Effect), int(ef.Value)}
		}
		out = append(out, item)
	}
	return out
}
