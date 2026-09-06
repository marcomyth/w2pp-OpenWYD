package level

import "testing"

// For a celestial tier the zone changes nothing, and that is not an accident of
// these particular numbers: celestialBands is the same table (10/20/40/80/160/
// 320) in all seven branches, and the +MaxLevel+1 offset puts myLevel past 400,
// beyond every rung but the last. So a celestial always lands on ÷320 wherever
// it kills.
//
// This matters because it is the opposite of what the map suggests. Água Arcano
// is gated to celestial tiers alone (handler.waterClassAllowed), so somebody
// asking "why does the Arcano version of this mob pay differently" is asking
// about a difference that, for everyone who can actually stand there, does not
// exist. Making the Arcano pay more for a celestial takes an override in the XP
// table screen; editing the mob's Exp raises it in every zone at once.
func TestCelestialNaoVeDiferencaDeZona(t *testing.T) {
	// The three Pesadelo branches are excluded, and not because they are
	// awkward: they use identityBase, whose 32-bit product overflows for a
	// celestial (myLevel carries the +400 offset, so the multiplier is ~2x the
	// mortal one) and lands on a value the (0,10M] gate throws away. A celestial
	// killing a mob worth more than ~1M Exp gets ZERO there. That is the legacy's
	// own arithmetic, reproduced deliberately — see TestPesadeloZeraCelestial.
	tier := Tier{ClassMaster: classCelestial, CelLv40: true, CelLv90: true}
	zonas := []Zone{ZoneField, ZoneAguaNormal, ZoneAguaMistico, ZoneAguaArcano}
	for _, nivel := range []int32{3, 40, 200, 395} {
		var base int64
		for i, z := range zonas {
			got := ExpReward(ExpRewardInput{
				Zone: z, MobExp: 2990849, KillerLevel: nivel, MobLevel: 399, Tier: tier,
			})
			if i == 0 {
				base = got
				if base <= 0 {
					t.Fatalf("nível %d: recompensa base %d — o caso não testa nada", nivel, base)
				}
				continue
			}
			if got != base {
				t.Errorf("nível %d: %s deu %d, campo deu %d — a tabela celestial "+
					"deveria ser a mesma nas sete zonas", nivel, z.Name(), got, base)
			}
		}
	}
}

// The corollary: mortal and arch DO see the zone. If this ever stops being true
// the test above stops meaning anything — it would be passing because nothing
// differs anywhere, not because the celestial table is shared.
func TestMortalEArchVeemDiferencaDeZona(t *testing.T) {
	for _, c := range []struct {
		nome        string
		classMaster uint8
	}{{"mortal", classMortal}, {"arch", classArch}} {
		in := func(z Zone) ExpRewardInput {
			return ExpRewardInput{
				Zone: z, MobExp: 2990849, KillerLevel: 395, MobLevel: 399,
				Tier: Tier{ClassMaster: c.classMaster, ArchLv355: true, ArchLv370: true},
			}
		}
		a := ExpReward(in(ZoneAguaArcano))
		m := ExpReward(in(ZoneAguaMistico))
		if a == m {
			t.Errorf("%s: Arcano e Místico pagaram %d — as tabelas por tier diferem, "+
				"o cálculo deveria refletir isso", c.nome, a)
		}
		if a <= m {
			t.Errorf("%s: Arcano %d não é maior que Místico %d — os divisores do "+
				"Arcano são menores em toda a faixa", c.nome, a, m)
		}
	}
}

// A celestial earns NOTHING in Pesadelo from a mob worth much over ~1M Exp.
//
// The three Pesadelo branches scale with `(30+myLevel) * isExp / (30+myLevel)`
// — algebraically the identity, but computed on a 32-bit int in the legacy, so
// the product wraps. A celestial carries the +MaxLevel+1 offset, which roughly
// doubles the multiplier and pulls the overflow down into the range real boss
// templates already occupy: @@Gargula ships with Exp 2990849.
//
// This is the legacy's own arithmetic and is reproduced on purpose, but it is a
// live trap for the mob editor: raising a Pesadelo mob's Exp past the threshold
// silently takes its celestial reward to zero, which reads in game as "the
// dungeon stopped giving XP".
func TestPesadeloZeraCelestialComExpAlta(t *testing.T) {
	reward := func(exp int64) int64 {
		return ExpReward(ExpRewardInput{
			Zone: ZonePesadeloArcano, MobExp: exp, KillerLevel: 395, MobLevel: 399,
			Tier: Tier{ClassMaster: classCelestial, CelLv40: true, CelLv90: true},
		})
	}
	if got := reward(1_000_000); got <= 0 {
		t.Errorf("1M de Exp deu %d — abaixo do limiar ainda tem que pagar", got)
	}
	if got := reward(2_990_849); got != 0 {
		t.Errorf("2.99M de Exp deu %d, want 0 — se o overflow foi corrigido, "+
			"este teste e o aviso no painel de monstros precisam sair juntos", got)
	}
	// Mortal is unaffected: its myLevel has no offset, so the product still fits.
	mortal := ExpReward(ExpRewardInput{
		Zone: ZonePesadeloArcano, MobExp: 2_990_849, KillerLevel: 395, MobLevel: 399,
		Tier: Tier{ClassMaster: classMortal},
	})
	if mortal <= 0 {
		t.Errorf("mortal deu %d na mesma Exp — o estouro só atinge as tiers celestiais", mortal)
	}
}

// ExpOverflow must agree with what ExpReward actually does: the limit it
// reports has to pay, and just above it must not. A limit derived from a
// restated formula would drift from the real one silently, which is exactly the
// failure it exists to prevent.
func TestExpOverflowConcordaComOCalculo(t *testing.T) {
	in := ExpRewardInput{
		Zone: ZonePesadeloArcano, MobExp: 2_990_849, KillerLevel: 395, MobLevel: 399,
		Tier: Tier{ClassMaster: classCelestial, CelLv40: true, CelLv90: true},
	}
	estoura, limite := ExpOverflow(in)
	if !estoura {
		t.Fatal("2.99M num Pesadelo para celestial estoura — ExpOverflow disse que não")
	}
	if limite <= 0 || limite >= in.MobExp {
		t.Fatalf("limite = %d, tinha que ficar entre 0 e %d", limite, in.MobExp)
	}

	noLimite := in
	noLimite.MobExp = limite
	if got := ExpReward(noLimite); got <= 0 {
		t.Errorf("no limite (%d) o cálculo pagou %d — o limite devia ser o último valor que paga", limite, got)
	}
	// A margin above the edge, since ExpApply's integer scaling means the very
	// next unit of MobExp does not always change isExp.
	acima := in
	acima.MobExp = limite + limite/100 + 2
	if got := ExpReward(acima); got != 0 {
		t.Errorf("acima do limite (%d) o cálculo pagou %d, want 0", acima.MobExp, got)
	}
}

// Zones without identityBase never overflow, and a mortal in Pesadelo does not
// either — reporting a limit there would send somebody lowering a reward for no
// reason.
func TestExpOverflowSoOndeExiste(t *testing.T) {
	base := ExpRewardInput{MobExp: 9_000_000, KillerLevel: 395, MobLevel: 399}

	celestial := base
	celestial.Tier = Tier{ClassMaster: classCelestial, CelLv40: true, CelLv90: true}
	for _, z := range []Zone{ZoneField, ZoneAguaNormal, ZoneAguaMistico, ZoneAguaArcano} {
		celestial.Zone = z
		if estoura, _ := ExpOverflow(celestial); estoura {
			t.Errorf("%s não usa identityBase e mesmo assim reportou estouro", z.Name())
		}
	}

	// A mortal overflows too — the offset only moves the ceiling, it does not
	// create it. What matters is that the ceiling sits far higher, so an Exp
	// that already zeroes the celestial still pays the mortal. That gap is the
	// whole reason the dungeon looks broken for one tier and fine for the other.
	const expQueZeraCelestial = 2_990_849
	mortal := ExpRewardInput{
		Zone: ZonePesadeloArcano, MobExp: expQueZeraCelestial, KillerLevel: 395,
		MobLevel: 399, Tier: Tier{ClassMaster: classMortal},
	}
	if estoura, _ := ExpOverflow(mortal); estoura {
		t.Error("mortal estourou numa Exp que só devia zerar o celestial")
	}
	if got := ExpReward(mortal); got <= 0 {
		t.Errorf("mortal recebeu %d na Exp que zera o celestial, want > 0", got)
	}

	celestialMesmaExp := mortal
	celestialMesmaExp.Tier = Tier{ClassMaster: classCelestial, CelLv40: true, CelLv90: true}
	estoura, limiteCel := ExpOverflow(celestialMesmaExp)
	if !estoura {
		t.Fatal("celestial devia estourar nessa mesma Exp")
	}
	_, limiteMortal := ExpOverflow(ExpRewardInput{
		Zone: ZonePesadeloArcano, MobExp: 9_000_000, KillerLevel: 395, MobLevel: 399,
		Tier: Tier{ClassMaster: classMortal},
	})
	if limiteMortal <= limiteCel {
		t.Errorf("teto do mortal (%d) devia ser bem maior que o do celestial (%d)",
			limiteMortal, limiteCel)
	}
}
