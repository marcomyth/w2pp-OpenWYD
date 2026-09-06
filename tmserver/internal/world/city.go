package world

import "math/rand"

// city is a spawn zone (STRUCT_GUILDZONE subset). The table is hardcoded in the
// original (Basedef.cpp:54): CityLimit rectangle + CitySpawn default area.
type city struct {
	spawnX, spawnY   int16
	limitX1, limitY1 int16
	limitX2, limitY2 int16
}

// cities is the fixed 5-city table (Armia, Azran, Erion, Nippleheim, Noatum).
var cities = [5]city{
	{2086, 2093, 2052, 2052, 2171, 2163}, // 0 Armia
	{2494, 1707, 2432, 1672, 2675, 1767}, // 1 Azran
	{2453, 2000, 2448, 1966, 2476, 2024}, // 2 Erion
	{3652, 3122, 3605, 3090, 3690, 3260}, // 3 Nippleheim
	{1050, 1706, 1036, 1700, 1072, 1760}, // 4 Noatum
}

// Village returns the city index (0..4) whose rectangle contains (x,y), or -1
// (BASE_GetVillage).
func Village(x, y int16) int {
	for i := range cities {
		c := cities[i]
		if x >= c.limitX1 && x <= c.limitX2 && y >= c.limitY1 && y <= c.limitY2 {
			return i
		}
	}
	return -1
}

// cityTax is g_pGuildZone[village].CityTax — the percent tax a village charges on
// a personal-shop sale (issue #115). The static init is 5 for every village
// (Basedef.cpp:54-61); at runtime a siege can override it (0..20, default 10,
// CReadFiles.cpp:809), which the rewrite does not model yet. Indexed by Village().
var cityTax = [5]int16{5, 5, 5, 5, 5}

// CityTax returns the sale tax percent for a village index (0..4), or 0 if out of
// range (BASE_GetVillage returned -1 → no tax). Loop-only read.
func CityTax(village int) int16 {
	if village < 0 || village >= len(cityTax) {
		return 0
	}
	return cityTax[village]
}

// CitySpawn returns a default spawn position for the given city (CitySpawn +
// rand%15). city is clamped to 0..3 (the saved "last city" only holds 2 bits;
// Noatum=4 falls back to Armia, mirroring the original Merchant<<6 overflow).
func CitySpawn(city int) (int16, int16) {
	if city < 0 || city > 3 {
		city = 0
	}
	c := cities[city]
	return c.spawnX + int16(rand.Intn(15)), c.spawnY + int16(rand.Intn(15))
}

// CityName is the label for a city index (0..4), or "" when out of range.
var cityNames = [5]string{"Armia", "Azran", "Erion", "Nippleheim", "Noatum"}

func CityName(city int) string {
	if city < 0 || city >= len(cityNames) {
		return ""
	}
	return cityNames[city]
}

// NearestCitySpawn returns the spawn point of the city closest to (x, y), its
// index and its name.
//
// It is what an unstuck should use, and it is deliberately NOT CitySpawn: that
// one clamps to 0..3 because the saved "last city" only holds two bits, so
// Noatum falls back to Armia. Reproducing that quirk here would send a player
// stuck by Noatum across the entire map for no reason — the quirk belongs to
// the save format, not to picking somewhere safe to stand.
//
// The five cities are the whole candidate set on purpose: the wider zone table
// in internal/mapzones also holds the three Pesadelo interiors, and dropping a
// stuck player inside a dungeon is not a rescue.
func NearestCitySpawn(x, y int16) (int16, int16, int, string) {
	melhor, melhorDist := 0, int64(-1)
	for i := range cities {
		dx := int64(x) - int64(cities[i].spawnX)
		dy := int64(y) - int64(cities[i].spawnY)
		if d := dx*dx + dy*dy; melhorDist < 0 || d < melhorDist {
			melhor, melhorDist = i, d
		}
	}
	c := cities[melhor]
	// The same rand%15 spread CitySpawn uses: several people rescued at once
	// must not land stacked on one tile.
	return c.spawnX + int16(rand.Intn(15)), c.spawnY + int16(rand.Intn(15)),
		melhor, cityNames[melhor]
}

// nonCombatNPC classifies service/town NPCs independently of the client-visible
// Merchant byte. Some real city templates ship Merchant==0 (for example combine
// statues and decorative town actors), so relying on Merchant alone makes them
// combat targets. Hostile clan mobs inside/near city rectangles stay combat-capable.
func nonCombatNPC(merchant, clan uint8, x, y int16) bool {
	if merchant != 0 {
		return true
	}
	return Village(x, y) >= 0 && !ClanHostile(0, clan)
}
