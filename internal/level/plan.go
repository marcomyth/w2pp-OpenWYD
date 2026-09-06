package level

// PlanStep is one level of a grind: what a kill pays there, how much
// experience the level spans, and how many kills that works out to.
type PlanStep struct {
	Level      int32
	ExpPerKill int64
	ExpSpan    int64
	Kills      int64
}

// Plan is the answer to "how long does this mob take me from here to the cap".
// It is deliberately a count of kills, not a duration: how fast a kill happens
// is a question about the character and the room, and the caller multiplies.
type Plan struct {
	Steps      []PlanStep
	TotalKills int64

	// Wall is the first level where the mob pays nothing, or 0 when the grind
	// completes. It is almost always a tier quest gate (ArchGateLv355 and
	// friends) or a mob so far below the character that GetExpApply zeroes it —
	// which is exactly the "Gremlin stops paying a level-300 character" case
	// the cut tables exist to shape.
	Wall int32

	// Capped is the level the plan stopped at.
	Capped int32
}

// PlanKills walks a character from level `from` to the tier's cap killing the
// same mob over and over, and reports the kills each level costs. The input's
// KillerLevel is ignored — the walk sets it — and every other field (zone, mob,
// bonuses, events, configuration) is held fixed, which is what makes two plans
// comparable.
//
// The walk is bounded by the tier's own cap, so a celestial plan ends at
// MaxCLevel and not at 399.
func PlanKills(in ExpRewardInput, from int32) Plan {
	levelCap := MaxLevelForTier(in.Tier.ClassMaster)
	if from < 0 {
		from = 0
	}
	p := Plan{Capped: from}
	for lv := from; lv < levelCap; lv++ {
		step := in
		step.KillerLevel = lv
		perKill := ExpReward(step)
		span := NextLevelExpTier(lv, in.Tier.ClassMaster) - ExpTier(lv, in.Tier.ClassMaster)
		if span < 0 {
			span = 0
		}
		if perKill <= 0 {
			p.Wall = lv
			return p
		}
		kills := span / perKill
		if span%perKill != 0 {
			kills++
		}
		p.Steps = append(p.Steps, PlanStep{Level: lv, ExpPerKill: perKill, ExpSpan: span, Kills: kills})
		p.TotalKills += kills
		p.Capped = lv + 1
	}
	return p
}

// PlanBand is a plan folded into a readable band of levels — a table of 400
// rows helps nobody, and the interesting shape is where the pace changes.
type PlanBand struct {
	From, To   int32
	ExpPerKill int64 // at the band's first level
	Kills      int64
}

// Bands folds a plan into one row per `size` levels, which is the form the
// panel shows. The last band is short when the range does not divide evenly.
func (p Plan) Bands(size int32) []PlanBand {
	if size < 1 || len(p.Steps) == 0 {
		return nil
	}
	var out []PlanBand
	for i := 0; i < len(p.Steps); {
		start := p.Steps[i]
		b := PlanBand{From: start.Level, To: start.Level, ExpPerKill: start.ExpPerKill}
		for ; i < len(p.Steps) && p.Steps[i].Level < start.Level+size; i++ {
			b.To = p.Steps[i].Level
			b.Kills += p.Steps[i].Kills
		}
		out = append(out, b)
	}
	return out
}
