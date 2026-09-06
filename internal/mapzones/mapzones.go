// Package mapzones labels and derives the settlement a world position belongs
// to, for the moderator UI's map_id picker, the NPC-by-city inventory and the
// admin panel's player map.
//
// It sits in the repo-root internal/ rather than under webserver/ because the
// admin panel needs it too and Go's internal rule forbids reaching across:
// coordinates arrive from the game control API as bare numbers, and a page that
// prints "2091, 2101" instead of "Armia" makes a moderator go look it up.
//
// NPCDefinition.MapID is stored but not consumed by the world (npc-editing-plan.md
// §9.2: the world runs a single grid, so spawn position comes from pos_x/pos_y
// alone), and every seeded row currently carries the column default 0 — which
// would read as "Armia" for NPCs that are nowhere near it. So the panel derives
// the zone from the position instead of trusting the stored value, and writes
// the derived id back only when a moderator confirms or overrides it.
package mapzones

// Zone is one labeled map_id value: a settlement center plus the radius within
// which a position is considered part of it.
//
// Radius (not a rectangle) is deliberate. The legacy CityLimit rectangles in
// tmserver/internal/world/city.go are the *tax and respawn* zones, and they are
// tight enough to exclude NPCs that plainly belong to the town: Armia's
// rectangle ends at y=2052, leaving Mestre_Archi (2102,2038), Foema_Ancian
// (2097,2038), ForeLearner (2090,2047) and Cap.Cavaleiros (2095,2047) outside a
// city they clearly stand in. Classifying by distance to the center fixes that
// without redefining the gameplay rectangles, which stay untouched.
type Zone struct {
	ID   int32
	Name string
	// CX, CY is the settlement center on the single global grid.
	CX, CY int32
	// Radius bounds membership; 0 marks the catch-all Field entry, which is
	// never matched by distance.
	Radius int32
	// LimitX1..LimitY2 is the legacy CityLimit rectangle (Basedef.cpp:54, the
	// same numbers as tmserver/internal/world/city.go). It answers a different
	// question from Radius: the rectangle is the built-up town where monsters
	// do not spawn, so it decides whether a Merchant==0 template is a town
	// actor or a monster, while Radius decides which town an NPC belongs to.
	// Zero for derived zones, which have no legacy rectangle.
	LimitX1, LimitY1, LimitX2, LimitY2 int32
	// Verified reports whether the center comes from the legacy source
	// (Basedef.cpp:54, mirrored in tmserver/internal/world/city.go) or was
	// derived from the content tree. Derived zones are real settlements found
	// in NPCGener.txt but with no name anywhere in the legacy source or this
	// repo's docs, so their labels are placeholders a moderator can rename.
	Verified bool
}

// Field is the map_id for a position that belongs to no settlement. Most
// NPCGener blocks with Merchant != 0 land here: the legacy byte is overloaded
// for raid actors, mounts and field guards, not just shopkeepers
// (npc-generator-inventory.md).
const Field int32 = 8

// All is the zone table. Ids 0..4 are frozen: they match the city order in
// tmserver/internal/world/city.go and the ListMapZones contract already
// published in docs/integrations/npc-admin-nextjs.md, so they must not be
// renumbered. Derived settlements are appended from 5 up.
//
// Radii are calibrated against the content tree, not guessed. For each
// settlement the distances of nearby NPCGener blocks are dense up to a point
// and then jump: Armia runs 2..144 then nothing until 289; Azran 2..142 then
// 272; Erion 17..23 then 130; the east city 0..52 then 219; both villages stop
// below 40 with the next block 150+ away. Each radius sits inside its own gap,
// so nudging it either way reclassifies nothing.
var All = []Zone{
	{ID: 0, Name: "Armia", CX: 2086, CY: 2093, Radius: 150, Verified: true,
		LimitX1: 2052, LimitY1: 2052, LimitX2: 2171, LimitY2: 2163},
	{ID: 1, Name: "Azran", CX: 2494, CY: 1707, Radius: 150, Verified: true,
		LimitX1: 2432, LimitY1: 1672, LimitX2: 2675, LimitY2: 1767},
	{ID: 2, Name: "Erion", CX: 2453, CY: 2000, Radius: 60, Verified: true,
		LimitX1: 2448, LimitY1: 1966, LimitX2: 2476, LimitY2: 2024},
	{ID: 3, Name: "Nippleheim", CX: 3652, CY: 3122, Radius: 60, Verified: true,
		LimitX1: 3605, LimitY1: 3090, LimitX2: 3690, LimitY2: 3260},
	{ID: 4, Name: "Noatum", CX: 1050, CY: 1706, Radius: 60, Verified: true,
		LimitX1: 1036, LimitY1: 1700, LimitX2: 1072, LimitY2: 1760},

	// Derived from the content tree, UNVERIFIED names. A Guarda_Carga (the
	// bank, Merchant==2) only ever stands in a real city, which is what marks
	// this one as a settlement rather than a camp: it holds Creta (shop),
	// Guarda_Carga, Alquimista_Odin, Urnammu and Uxmal. No name for it exists
	// in the legacy source or in this repo's docs.
	{ID: 5, Name: "Cidade do Leste (não identificada)", CX: 3241, CY: 1683, Radius: 80},
	// These two read as anonymous shop clusters until you compare them with the
	// Pesadelo instances: they sit exactly inside the dungeon segments the entry
	// scrolls teleport into — (8,2) is Pesadelo_M and (10,2) is Pesadelo_N
	// (Regions.txt rows 4-5, handler/pesadelo.go). So they are dungeon interiors,
	// not settlements, and their shops are dungeon NPCs. That also explains why
	// six of them ship with an entirely empty Carry[].
	{ID: 6, Name: "Pesadelo Místico (masmorra)", CX: 1090, CY: 315, Radius: 60, Verified: true},
	{ID: 7, Name: "Pesadelo Normal (masmorra)", CX: 1310, CY: 330, Radius: 60, Verified: true},
	// Pesadelo Arcano, the third instance (segment (9,1)). It holds no shops, so
	// nothing in the NPC panel lands here — it is listed to keep the three
	// instances together rather than leaving one of them unnamed.
	{ID: 9, Name: "Pesadelo Arcano (masmorra)", CX: 1216, CY: 192, Radius: 64, Verified: true},

	{ID: Field, Name: "Campo (fora de cidade)", CX: 0, CY: 0, Radius: 0},
}

// Classify returns the id of the settlement containing (x, y), or Field when
// the position is in open world. When radii overlap the nearest center wins.
func Classify(x, y int32) int32 {
	best := Field
	var bestDist int64 = -1
	for _, z := range All {
		if z.Radius == 0 {
			continue
		}
		dx, dy := int64(x-z.CX), int64(y-z.CY)
		d := dx*dx + dy*dy
		if r := int64(z.Radius) * int64(z.Radius); d > r {
			continue
		}
		if bestDist < 0 || d < bestDist {
			best, bestDist = z.ID, d
		}
	}
	return best
}

// Nearest returns the closest settlement to (x, y) and the distance to its
// center, ignoring radii. The panel uses it to say where a Field NPC actually
// stands ("Campo — mais próximo: Erion, 130"), which is more useful to a
// moderator than an unqualified "outside every city".
func Nearest(x, y int32) (Zone, int32) {
	var best Zone
	var bestDist int64 = -1
	for _, z := range All {
		if z.Radius == 0 {
			continue
		}
		dx, dy := int64(x-z.CX), int64(y-z.CY)
		if d := dx*dx + dy*dy; bestDist < 0 || d < bestDist {
			best, bestDist = z, d
		}
	}
	if bestDist < 0 {
		return Zone{}, 0
	}
	return best, int32(isqrt(bestDist))
}

// isqrt is an integer square root; the panel only ever shows this distance
// rounded, so pulling in math.Sqrt's float conversion buys nothing.
func isqrt(n int64) int64 {
	if n < 2 {
		return n
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x, y = y, (y+n/y)/2
	}
	return x
}

// InTown reports whether (x, y) is inside the built-up area of a settlement:
// the legacy CityLimit rectangle for the five canonical cities, and the (tight)
// radius for the derived ones, which have no legacy rectangle.
//
// This is world.Village's question, not Classify's. Monsters spawn freely a
// hundred units outside a city gate but not in the streets, so the rectangle is
// what tells a decorative town actor apart from a Ghoul — while Classify's wider
// radius is what groups an NPC standing just outside the gate with its own city.
func InTown(x, y int32) bool {
	for _, z := range All {
		if z.Radius == 0 {
			continue
		}
		if z.LimitX2 == 0 { // derived zone: no legacy rectangle, use its radius
			dx, dy := int64(x-z.CX), int64(y-z.CY)
			if dx*dx+dy*dy <= int64(z.Radius)*int64(z.Radius) {
				return true
			}
			continue
		}
		if x >= z.LimitX1 && x <= z.LimitX2 && y >= z.LimitY1 && y <= z.LimitY2 {
			return true
		}
	}
	return false
}

// Name returns the label for a map_id, or "" when the id is not in the table.
func Name(id int32) string {
	for _, z := range All {
		if z.ID == id {
			return z.Name
		}
	}
	return ""
}
