package world

import "testing"

func TestVillage(t *testing.T) {
	cases := []struct {
		x, y int16
		want int
	}{
		{2096, 2096, 0}, // Armia (player default spawn area)
		{2086, 2093, 0}, // Armia spawn point
		{2494, 1707, 1}, // Azran
		{2453, 2000, 2}, // Erion
		{3652, 3122, 3}, // Nippleheim
		{1050, 1706, 4}, // Noatum
		{0, 0, -1},      // wilderness
		{3000, 3000, -1},
	}
	for _, c := range cases {
		if got := Village(c.x, c.y); got != c.want {
			t.Errorf("Village(%d,%d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}

func TestCitySpawn(t *testing.T) {
	// Spawn is CitySpawn base + rand%15 → must land in the city and on Armia for
	// out-of-range ids (the saved last-city only holds 2 bits; Noatum=4 → Armia).
	for city := 0; city < 4; city++ {
		for i := 0; i < 50; i++ {
			x, y := CitySpawn(city)
			base := cities[city]
			if x < base.spawnX || x >= base.spawnX+15 || y < base.spawnY || y >= base.spawnY+15 {
				t.Fatalf("CitySpawn(%d) = %d,%d out of [%d,%d)+15", city, x, y, base.spawnX, base.spawnY)
			}
			if Village(x, y) != city {
				t.Fatalf("CitySpawn(%d) landed in village %d", city, Village(x, y))
			}
		}
	}
	x, y := CitySpawn(4) // Noatum not savable → Armia
	if Village(x, y) != 0 {
		t.Errorf("CitySpawn(4) should fall back to Armia, got village %d", Village(x, y))
	}
}

func TestSpawnMobClassifiesTownNPCsAsNonCombat(t *testing.T) {
	w := New(Config{GridDim: 4096}, slogDiscard(), nil, nil)

	neutral := w.SpawnMob(genMobTemplate(3), 2086, 2093) // friendly city actor
	if neutral < 0 {
		t.Fatal("neutral town NPC did not spawn")
	}
	if e := w.Entity(neutral); e == nil || !e.NonCombatNPC {
		t.Fatalf("neutral town NPC NonCombatNPC = %v, want true", e)
	}

	hostile := w.SpawnMob(genMobTemplate(1), 2087, 2093) // hostile mob inside a city rectangle
	if hostile < 0 {
		t.Fatal("hostile city mob did not spawn")
	}
	if e := w.Entity(hostile); e == nil || e.NonCombatNPC {
		t.Fatalf("hostile city mob NonCombatNPC = %v, want false", e)
	}

	merchant := w.SpawnMob(genMerchantTemplate(1), 0, 0) // merchants are non-combat anywhere
	if merchant < 0 {
		t.Fatal("merchant NPC did not spawn")
	}
	if e := w.Entity(merchant); e == nil || !e.NonCombatNPC {
		t.Fatalf("merchant NonCombatNPC = %v, want true", e)
	}
}

func TestNearestCitySpawnEscolheACidadeMaisPerto(t *testing.T) {
	casos := []struct {
		nome string
		x, y int16
		want string
	}{
		{"do lado de Armia", 2100, 2100, "Armia"},
		{"do lado de Azran", 2500, 1710, "Azran"},
		{"do lado de Erion", 2455, 2005, "Erion"},
		{"do lado de Nippleheim", 3650, 3120, "Nippleheim"},
		{"do lado de Noatum", 1055, 1710, "Noatum"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, _, _, nome := NearestCitySpawn(c.x, c.y)
			if nome != c.want {
				t.Errorf("NearestCitySpawn(%d,%d) = %q, want %q", c.x, c.y, nome, c.want)
			}
		})
	}
}

// CitySpawn clamps to 0..3 because the saved "last city" only holds two bits, so
// Noatum falls back to Armia. That quirk belongs to the save format: reproducing
// it here would send a player stuck outside Noatum across the whole map.
func TestNoatumEAlcancavelAoContrarioDeCitySpawn(t *testing.T) {
	_, _, idx, nome := NearestCitySpawn(1050, 1706)
	if idx != 4 || nome != "Noatum" {
		t.Fatalf("cidade = %d (%q), want 4 (Noatum)", idx, nome)
	}
	// And the clamped one really does behave differently, so this test is
	// guarding a live difference rather than a hypothetical.
	x, y := CitySpawn(4)
	if Village(x, y) == 4 {
		t.Error("premissa mudou: CitySpawn(4) parou de cair em Armia")
	}
}

func TestNearestCitySpawnCaiDentroDaCidade(t *testing.T) {
	// Several people rescued at once must not land stacked on one tile, so the
	// point is spread — but the spread has to stay inside the city.
	for i := 0; i < 200; i++ {
		x, y, idx, _ := NearestCitySpawn(2800, 2600)
		if got := Village(x, y); got != idx {
			t.Fatalf("desatolo caiu em %d, fora da cidade %d (%d,%d)", got, idx, x, y)
		}
	}
}

func TestCityName(t *testing.T) {
	if got := CityName(0); got != "Armia" {
		t.Errorf("CityName(0) = %q", got)
	}
	for _, i := range []int{-1, 5, 99} {
		if got := CityName(i); got != "" {
			t.Errorf("CityName(%d) = %q, want vazio", i, got)
		}
	}
}
