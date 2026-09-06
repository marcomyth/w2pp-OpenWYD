package level

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestZoneForTile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		x, y int32
		want Zone
	}{
		{"pesadelo arcano", 9*128 + 10, 1*128 + 10, ZonePesadeloArcano},
		{"pesadelo místico", 8*128 + 3, 2*128 + 3, ZonePesadeloMistico},
		{"pesadelo normal", 10*128 + 64, 2*128 + 64, ZonePesadeloNormal},
		{"água arcano", 10*128 + 1, 27*128 + 1, ZoneAguaArcano},
		{"água místico", 9*128 + 127, 28*128 + 127, ZoneAguaMistico},
		{"água normal", 8 * 128, 27 * 128, ZoneAguaNormal},
		{"armia", 2100, 2100, ZoneField},
		{"origem", 0, 0, ZoneField},
		// The blocks are two-dimensional: a matching column with the wrong row
		// is the open field, not the dungeon.
		{"coluna do pesadelo, linha da água", 9 * 128, 28 * 128, ZoneAguaMistico},
		{"coluna 9, linha 0", 9 * 128, 0, ZoneField},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ZoneForTile(tc.x, tc.y); got != tc.want {
				t.Fatalf("ZoneForTile(%d, %d) = %v (%s), quero %v (%s)",
					tc.x, tc.y, got, got.Name(), tc.want, tc.want.Name())
			}
		})
	}
}

// TestZoneGoldenRewards pins one kill through every branch with the arithmetic
// worked out by hand from MobKilled.cpp, so the whole pipeline — and not only
// the divisor tables — is anchored.
//
// The kill: a level-371 Mortal killing a level-371 mob worth 20.000, with no
// item bonus and every event off (KefraLive false is the legacy's ÷2).
// GetExpApply returns 20.000 (same level ⇒ ratio 100%).
//
//	Campo            450×20000/401 = 22443 → /2.10f = 10687 → ×0,6 = 6412
//	                 → ÷2 = 3206 → −15% = 2726
//	Pesadelo Arcano  identidade 20000 → /1.95f = 10256 → ×0,6 = 6153
//	                 → ÷2 = 3076 → −15% = 2615
//	Pesadelo Místico identidade 20000 → /2.78f = 7194 → ×0,6 = 4316
//	                 → ÷2 = 2158 → −15% = 1835
//	Pesadelo Normal  identidade 20000, sem tabela → ×0,6 = 12000
//	                 → ÷2 = 6000 → −15% = 5100
//
// The lesson the numbers carry: ×450/(30+level) is a *multiplier* below level
// 420, so the field's base scaling is worth ~1,12× at 371 while Pesadelo's
// identity is worth exactly 1×. Pesadelo is richer only where it drops the
// divisor table (Normal), not because it skips the 450 scaling.
func TestZoneGoldenRewards(t *testing.T) {
	t.Parallel()
	want := map[Zone]int64{
		ZoneField:           2726,
		ZonePesadeloArcano:  2615,
		ZonePesadeloMistico: 1835,
		ZonePesadeloNormal:  5100,
	}
	for zone, exp := range want {
		in := ExpRewardInput{
			Zone: zone, MobExp: 20_000, KillerLevel: 371, MobLevel: 371,
			Tier: Tier{ClassMaster: classMortal},
		}
		if got := ExpReward(in); got != exp {
			t.Errorf("%s pagou %d, a conta do legado dá %d", zone.Name(), got, exp)
		}
	}
}

// TestPesadeloNormalDoesNotDivideMortalOrArch pins the branch that has no
// Mortal and no Arch table at all (MobKilled.cpp:747-790) — the level bands
// that shape every other branch simply do not exist there.
func TestPesadeloNormalDoesNotDivideMortalOrArch(t *testing.T) {
	t.Parallel()
	for _, cm := range []uint8{classMortal, classArch} {
		var prev int64
		for _, lv := range []int32{200, 300, 356, 370, 380, 390, 399} {
			in := ExpRewardInput{
				Zone: ZonePesadeloNormal, MobExp: 100_000,
				KillerLevel: lv, MobLevel: lv, Tier: Tier{
					ClassMaster: cm, ArchLv355: true, ArchLv370: true,
				},
			}
			got := ExpReward(in)
			if prev != 0 && got != prev {
				t.Errorf("classe %d: nível %d pagou %d, mas %d no nível anterior — há uma banda dividindo",
					cm, lv, got, prev)
			}
			prev = got
		}
	}
}

// TestPesadeloIgnoresTheFadaSuprema pins the one place g_pFairyContent[0]
// changes the answer: the Água and field branches add it to ExpBonus, Pesadelo
// does not (MobKilled.cpp:535 vs :944).
func TestPesadeloIgnoresTheFadaSuprema(t *testing.T) {
	t.Parallel()
	base := ExpRewardInput{
		MobExp: 50_000, KillerLevel: 380, MobLevel: 380,
		Tier: Tier{ClassMaster: classMortal}, ExpBonus: 16,
	}
	withFairy := base
	withFairy.FairyContent = 30

	for _, z := range []Zone{ZoneField, ZoneAguaArcano, ZoneAguaMistico, ZoneAguaNormal} {
		base.Zone, withFairy.Zone = z, z
		if ExpReward(withFairy) <= ExpReward(base) {
			t.Errorf("%s: o conteúdo da fada não somou", z.Name())
		}
	}
	for _, z := range []Zone{ZonePesadeloArcano, ZonePesadeloMistico, ZonePesadeloNormal} {
		base.Zone, withFairy.Zone = z, z
		if got, want := ExpReward(withFairy), ExpReward(base); got != want {
			t.Errorf("%s: o conteúdo da fada somou %d → %d, e não deveria", z.Name(), want, got)
		}
	}
}

// TestSoloExpRewardIsTheFieldZone keeps the short form and the zone form in
// step, since 24 files still call the short one.
func TestSoloExpRewardIsTheFieldZone(t *testing.T) {
	t.Parallel()
	tier := Tier{ClassMaster: classMortal}
	ev := ExpEvents{KefraLive: true}
	want := ExpReward(ExpRewardInput{
		Zone: ZoneField, MobExp: 8_000, KillerLevel: 250, MobLevel: 240,
		Tier: tier, ExpBonus: 34, Events: ev,
	})
	if got := SoloExpReward(8_000, 250, 240, tier, 34, ev); got != want {
		t.Fatalf("SoloExpReward = %d, ExpReward(ZoneField) = %d", got, want)
	}
}

// --- parity against the legacy source ------------------------------------

// legacyBand is a divisor rung as it is written in MobKilled.cpp.
type legacyBand struct {
	line int
	cond string // "<=" or "<"
	lv   int64
	lit  string // the divisor literal verbatim, "" for a bare `else`
}

var (
	reTier  = regexp.MustCompile(`ClassMaster (==|!=) (MORTAL|ARCH)`)
	reBand  = regexp.MustCompile(`myLevel (<=?) (\d+)`)
	reDiv   = regexp.MustCompile(`exp /= ([0-9.]+f?)`)
	reElse  = regexp.MustCompile(`^\s*else\s*$`)
	reSixth = regexp.MustCompile(`exp = 6 \* exp / 10`)
)

// legacySource reads MobKilled.cpp as lines, 1-indexed at index 1.
func legacySource(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "Source", "Code", "TMSrv", "MobKilled.cpp")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fonte legada ausente (%v); a paridade só é verificável com Source/ presente", err)
	}
	return append([]string{""}, strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")...)
}

// divisorRegion is the span of a branch that holds its tier tables: from the
// branch's first line to the `exp = 6 * exp / 10` that closes them.
func divisorRegion(t *testing.T, src []string, start int) (int, int) {
	t.Helper()
	for i := start; i < len(src) && i < start+400; i++ {
		if reSixth.MatchString(src[i]) {
			return start, i
		}
	}
	t.Fatalf("não achei o `exp = 6 * exp / 10` que fecha o bloco iniciado em :%d", start)
	return 0, 0
}

// readLegacyBands walks a branch and returns its rungs per tier, keyed
// "MORTAL"/"ARCH"/"CELEST", in source order. A tier appearing twice (Pesadelo
// Normal) yields its rungs twice.
func readLegacyBands(src []string, from, to int) map[string][]legacyBand {
	out := map[string][]legacyBand{}
	tier := ""
	for i := from; i <= to; i++ {
		l := src[i]
		if m := reTier.FindStringSubmatch(l); m != nil && strings.Contains(l, "if (") {
			switch {
			case m[1] == "==" && m[2] == "MORTAL":
				tier = "MORTAL"
			case m[1] == "==" && m[2] == "ARCH":
				tier = "ARCH"
			case m[1] == "!=":
				tier = "CELEST"
			}
			continue
		}
		if tier == "" {
			continue
		}
		if m := reBand.FindStringSubmatch(l); m != nil && strings.Contains(l, "if") {
			lv, _ := strconv.ParseInt(m[2], 10, 64)
			b := legacyBand{line: i, cond: m[1], lv: lv}
			for j := i + 1; j <= i+3 && j <= to; j++ {
				if d := reDiv.FindStringSubmatch(src[j]); d != nil {
					b.lit, b.line = d[1], j
					break
				}
			}
			if b.lit != "" {
				out[tier] = append(out[tier], b)
			}
			continue
		}
		if reElse.MatchString(l) && i < to {
			if d := reDiv.FindStringSubmatch(src[i+1]); d != nil {
				out[tier] = append(out[tier], legacyBand{line: i + 1, lit: d[1]})
			}
		}
	}
	return out
}

// bandString renders a ported rung the way the legacy writes it, so a mismatch
// prints two comparable strings.
func bandString(b expBand, celestial bool) string {
	cond, lv := "<=", strconv.FormatInt(b.upTo, 10)
	if celestial {
		cond, lv = "<", strconv.FormatInt(b.upTo+1, 10)
	}
	switch b.kind {
	case divNone:
		return fmt.Sprintf("myLevel %s %s /= 1", cond, lv)
	case divInt:
		return fmt.Sprintf("myLevel %s %s /= %d", cond, lv, int64(b.div))
	case divF32:
		return fmt.Sprintf("myLevel %s %s /= %sf", cond, lv, trimZeros(b.div))
	default:
		return fmt.Sprintf("myLevel %s %s /= %s", cond, lv, trimZeros(b.div))
	}
}

func trimZeros(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// legacyBandString renders a legacy rung the same way, normalising the last
// celestial `else` into the `< maxint+1` rung the port writes.
func legacyBandString(b legacyBand) string {
	if b.cond == "" {
		return fmt.Sprintf("else /= %s", b.lit)
	}
	lit := b.lit
	if strings.HasSuffix(lit, "f") {
		lit = trimZeros(mustFloat(strings.TrimSuffix(lit, "f"))) + "f"
	} else if strings.Contains(lit, ".") {
		lit = trimZeros(mustFloat(lit))
	}
	return fmt.Sprintf("myLevel %s %d /= %s", b.cond, b.lv, lit)
}

func mustFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return f
}

// TestZoneTablesMatchLegacy re-reads every divisor out of MobKilled.cpp and
// compares it with the ported table. It is the anchor that keeps a hand edit in
// expzone.go from silently drifting from the server it is a port of.
func TestZoneTablesMatchLegacy(t *testing.T) {
	t.Parallel()
	src := legacySource(t)

	for z := range zoneRules {
		zone := Zone(z)
		r := zoneRules[z]
		t.Run(r.name, func(t *testing.T) {
			from, to := divisorRegion(t, src, r.line)
			legacy := readLegacyBands(src, from, to)

			for _, tc := range []struct {
				tier  string
				bands []expBand
			}{
				{"MORTAL", r.mortal},
				{"ARCH", r.arch},
				{"CELEST", r.celestial},
			} {
				got := legacy[tc.tier]
				if tc.tier == "CELEST" && r.celestialTwice {
					// The branch repeats the block verbatim; compare one copy.
					if len(got) != 2*len(tc.bands) {
						t.Fatalf("%s/%s: o legado tem %d degraus, esperava a tabela repetida (%d)",
							r.name, tc.tier, len(got), 2*len(tc.bands))
					}
					if !sameBands(got[:len(tc.bands)], got[len(tc.bands):]) {
						t.Fatalf("%s: as duas cópias do bloco celestial divergem", r.name)
					}
					got = got[:len(tc.bands)]
				}
				if len(got) != len(tc.bands) {
					t.Fatalf("%s/%s: o legado tem %d degraus (a partir de :%d), a porta tem %d",
						r.name, tc.tier, len(got), r.line, len(tc.bands))
				}
				for i := range tc.bands {
					want := legacyBandString(got[i])
					mine := bandString(tc.bands[i], tc.tier == "CELEST")
					// The legacy's trailing celestial rung is a bare `else`.
					if got[i].cond == "" {
						mine = fmt.Sprintf("else /= %d", int64(tc.bands[i].div))
					}
					if mine != want {
						t.Errorf("%s/%s degrau %d: portei %q, MobKilled.cpp:%d diz %q",
							r.name, tc.tier, i, mine, got[i].line, want)
					}
				}
			}
			if zone != ZoneField && zone.Name() != r.name {
				t.Errorf("Name() = %q, quero %q", zone.Name(), r.name)
			}
		})
	}
}

func sameBands(a, b []legacyBand) bool {
	for i := range a {
		if a[i].cond != b[i].cond || a[i].lv != b[i].lv || a[i].lit != b[i].lit {
			return false
		}
	}
	return true
}

// TestZoneFlagsMatchLegacy checks the three per-branch switches that are not
// divisors: the base scaling, the eMob cap and the fairy content.
func TestZoneFlagsMatchLegacy(t *testing.T) {
	t.Parallel()
	src := legacySource(t)

	for z := range zoneRules {
		r := zoneRules[z]
		t.Run(r.name, func(t *testing.T) {
			from, to := divisorRegion(t, src, r.line)
			head := strings.Join(src[from:to], "\n")
			// The tail runs from the ×0.6 line to the bonus line, which holds
			// the cap and the fairy content.
			tailTo := to
			for i := to; i < len(src) && i < to+20; i++ {
				if strings.Contains(src[i], "ExpBonus > 0") {
					tailTo = i + 2
					break
				}
			}
			tail := strings.Join(src[to:tailTo], "\n")

			identity := strings.Contains(head, "(UNK_1 + myLevel) * isExp / (UNK_1 + myLevel)")
			if identity != r.identityBase {
				t.Errorf("identityBase = %v, mas o legado a partir de :%d %s a escala identidade",
					r.identityBase, r.line, pick(identity, "usa", "não usa"))
			}
			if !identity && !strings.Contains(head, "450 * isExp / (UNK_1 + myLevel)") {
				t.Errorf("nem identidade nem 450/(30+lv) em :%d — a base mudou", r.line)
			}

			teto := false
			for _, l := range strings.Split(tail, "\n") {
				trimmed := strings.TrimSpace(l)
				if strings.HasPrefix(trimmed, "//") {
					continue // Pesadelo Arcano tem o cap comentado (:531)
				}
				if strings.Contains(trimmed, "exp > eMob") {
					teto = true
				}
			}
			if teto != r.capToEMob {
				t.Errorf("capToEMob = %v, mas o legado %s o teto eMob", r.capToEMob, pick(teto, "aplica", "não aplica"))
			}

			fairy := strings.Contains(tail, "g_pFairyContent[0]")
			if fairy != r.fairyContent {
				t.Errorf("fairyContent = %v, mas o legado %s g_pFairyContent[0]",
					r.fairyContent, pick(fairy, "soma", "não soma"))
			}
		})
	}
}

func pick(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

// TestZoneForKillNeedsBothOnTheSameBlock pins the guard as the legacy writes it:
// every dungeon's else-if demands the corpse AND the killer be on the block, so
// a kill reaching across a boundary pays field rates.
//
// It also pins the argument order. The corpse comes first, and a swap would be
// invisible in the common case — both are in the room — while quietly changing
// what an edge kill pays.
func TestZoneForKillNeedsBothOnTheSameBlock(t *testing.T) {
	t.Parallel()
	const (
		dentroX = 10*128 + 40 // Pesadelo Normal, bloco (10,2)
		dentroY = 2*128 + 40
		foraX   = 2100 // Armia
		foraY   = 2100
	)
	cases := []struct {
		name                         string
		mobX, mobY, killerX, killerY int32
		want                         Zone
	}{
		{"os dois dentro", dentroX, dentroY, dentroX, dentroY, ZonePesadeloNormal},
		{"o corpo dentro, quem matou fora", dentroX, dentroY, foraX, foraY, ZoneField},
		{"quem matou dentro, o corpo fora", foraX, foraY, dentroX, dentroY, ZoneField},
		{"os dois fora", foraX, foraY, foraX, foraY, ZoneField},
		// x and y transposed lands on block (2,10), which is nobody's dungeon.
		{"coordenadas trocadas", dentroY, dentroX, dentroY, dentroX, ZoneField},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ZoneForKill(tc.mobX, tc.mobY, tc.killerX, tc.killerY); got != tc.want {
				t.Fatalf("ZoneForKill = %s, quero %s", got.Name(), tc.want.Name())
			}
		})
	}
}

// TestZoneRuleSurvivesAnImpossibleZone: the zone reaches the reward as a number,
// and a bad one must fall back to the field rather than panic inside the game
// loop, where a panic is the whole server.
func TestZoneRuleSurvivesAnImpossibleZone(t *testing.T) {
	t.Parallel()
	fora := Zone(200)
	if got := fora.rule().name; got != "Campo" {
		t.Fatalf("uma zona fora da faixa caiu em %q, quero o campo", got)
	}
	if fora.Name() != "Campo" {
		t.Fatalf("Name() = %q", fora.Name())
	}
	in := ExpRewardInput{Zone: fora, MobExp: 1000, KillerLevel: 100, MobLevel: 100,
		Tier: Tier{ClassMaster: classMortal}, Events: ExpEvents{KefraLive: true}}
	in.Zone = ZoneField
	want := ExpReward(in)
	in.Zone = fora
	if got := ExpReward(in); got != want {
		t.Fatalf("a zona inválida pagou %d, o campo paga %d", got, want)
	}
}
