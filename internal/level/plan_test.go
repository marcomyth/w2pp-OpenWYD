package level

import "testing"

func mortalPlanInput() ExpRewardInput {
	return ExpRewardInput{
		Zone: ZoneField, MobExp: 20_000, MobLevel: 300,
		Tier:   Tier{ClassMaster: classMortal},
		Events: ExpEvents{KefraLive: true},
	}
}

func TestPlanKillsReachesTheCap(t *testing.T) {
	t.Parallel()
	p := PlanKills(mortalPlanInput(), 1)
	if p.Wall != 0 {
		t.Fatalf("bateu num muro no nível %d, esperava chegar ao topo", p.Wall)
	}
	if p.Capped != MaxLevel {
		t.Fatalf("parou em %d, esperava %d", p.Capped, MaxLevel)
	}
	if p.TotalKills <= 0 {
		t.Fatalf("total de mortes = %d", p.TotalKills)
	}
	for _, s := range p.Steps {
		if s.Kills <= 0 || s.ExpPerKill <= 0 {
			t.Fatalf("nível %d: %d mortes a %d de XP", s.Level, s.Kills, s.ExpPerKill)
		}
	}
}

// TestPlanKillsStopsAtTheArchWall is the reason Wall exists: an Arch that never
// did the level-355 quest stops earning there, and a plan that ignored it would
// report a grind the character cannot finish.
func TestPlanKillsStopsAtTheArchWall(t *testing.T) {
	t.Parallel()
	in := mortalPlanInput()
	in.Tier = Tier{ClassMaster: classArch} // sem as flags de quest
	p := PlanKills(in, 300)
	if p.Wall != ArchGateLv355 {
		t.Fatalf("muro em %d, esperava o portão Arch em %d", p.Wall, ArchGateLv355)
	}

	in.Tier.ArchLv355, in.Tier.ArchLv370 = true, true
	if p := PlanKills(in, 300); p.Wall != 0 {
		t.Fatalf("com as quests feitas ainda parou em %d", p.Wall)
	}
}

// TestPlanKillsCelestialStopsAtItsOwnCap: the celestial curve tops out at 199,
// not 399, and a plan that walked to 399 would invent 200 levels.
func TestPlanKillsCelestialStopsAtItsOwnCap(t *testing.T) {
	t.Parallel()
	in := mortalPlanInput()
	in.Tier = Tier{ClassMaster: classCelestial, CelLv40: true, CelLv90: true}
	in.MobExp = 5_000_000 // celestial divide por até 320; sem isso o muro é a XP zerada
	p := PlanKills(in, 1)
	if p.Capped > MaxCLevel {
		t.Fatalf("chegou a %d, acima do teto celestial %d", p.Capped, MaxCLevel)
	}
}

// TestLowLevelMobStopsPayingHighLevelCharacter pins the behaviour the whole cut
// system exists for: a Gremlin keeps a level-1 Mortal moving and pays a
// level-300 one almost nothing.
func TestLowLevelMobStopsPayingHighLevelCharacter(t *testing.T) {
	t.Parallel()
	in := ExpRewardInput{
		Zone: ZoneField, MobExp: 60, MobLevel: 10,
		Tier: Tier{ClassMaster: classMortal}, Events: ExpEvents{KefraLive: true},
	}
	in.KillerLevel = 5
	novato := ExpReward(in)
	in.KillerLevel = 300
	veterano := ExpReward(in)
	if novato <= veterano {
		t.Fatalf("o Gremlin pagou %d ao novato e %d ao veterano", novato, veterano)
	}
}

func TestBandsFoldTheWalk(t *testing.T) {
	t.Parallel()
	p := PlanKills(mortalPlanInput(), 1)
	bands := p.Bands(50)
	if len(bands) == 0 {
		t.Fatal("nenhuma faixa")
	}
	var total int64
	for _, b := range bands {
		if b.To < b.From {
			t.Fatalf("faixa invertida %d-%d", b.From, b.To)
		}
		total += b.Kills
	}
	if total != p.TotalKills {
		t.Fatalf("as faixas somam %d mortes, o plano tem %d", total, p.TotalKills)
	}
	if got := p.Bands(0); got != nil {
		t.Fatalf("Bands(0) = %v, quero nil", got)
	}
}

// TestConfigRateScalesTheReward pins the one lever that is ours and not the
// legacy's: it multiplies the finished number, so it is exactly predictable.
func TestConfigRateScalesTheReward(t *testing.T) {
	t.Parallel()
	in := mortalPlanInput()
	in.KillerLevel = 300
	base := ExpReward(in)

	in.Config = Config{Overrides: map[ConfigKey]Override{
		{Zone: ZoneField, Tier: TierMortal}: {RatePercent: 200},
	}}
	if got, want := ExpReward(in), base*2; got != want {
		t.Fatalf("a 200%% pagou %d, esperava %d", got, want)
	}
}

// TestConfigCutsReplaceTheLegacyTable checks that an edited table is read as
// written — including removing every cut, which is a real configuration.
func TestConfigCutsReplaceTheLegacyTable(t *testing.T) {
	t.Parallel()
	in := mortalPlanInput()
	in.KillerLevel = 300
	legacy := ExpReward(in)

	key := ConfigKey{Zone: ZoneField, Tier: TierMortal}
	in.Config = Config{Overrides: map[ConfigKey]Override{key: {Cuts: []Cut{}}}}
	semCortes := ExpReward(in)
	if semCortes <= legacy {
		t.Fatalf("sem cortes pagou %d, não mais que o legado (%d)", semCortes, legacy)
	}

	in.Config = Config{Overrides: map[ConfigKey]Override{
		key: {Cuts: []Cut{{UpTo: CutOpenEnded, Divisor: 2}}},
	}}
	if got, want := ExpReward(in), semCortes/2; got < want-2 || got > want+2 {
		t.Fatalf("com um corte de ÷2 pagou %d, esperava perto de %d", got, want)
	}
}

// TestCutsFallBackToTheLegacyTable: a nil Cuts slice means "não mexi nessa
// tabela", and must not be confused with an empty one.
func TestCutsFallBackToTheLegacyTable(t *testing.T) {
	t.Parallel()
	cfg := Config{Overrides: map[ConfigKey]Override{
		{Zone: ZoneField, Tier: TierMortal}: {RatePercent: 150},
	}}
	got := cfg.Cuts(ZoneField, TierMortal)
	want := LegacyCuts(ZoneField, TierMortal)
	if len(got) != len(want) {
		t.Fatalf("caiu para %d cortes, o legado tem %d", len(got), len(want))
	}
	if cfg.RatePercent(ZoneField, TierArch) != 100 {
		t.Fatal("uma aba não editada deveria valer 100%")
	}
}
