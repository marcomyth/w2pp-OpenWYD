package level

import "testing"

// The arenas' zone must not move a point of experience when it ships: with no
// Mesa at all it is the field's branch, and with a Mesa that has only the
// field's row it pays that row. Otherwise the deploy alone would change the pay
// of every Quest 256 arena before anyone wrote a row for them.
func TestArenasPagamIgualAoCampoSemMesa(t *testing.T) {
	for _, nivel := range []int32{50, 200, 300, 349, 370, 395} {
		in := ExpRewardInput{
			Zone: ZoneField, MobExp: 175_584, KillerLevel: nivel, MobLevel: 399, Tier: Tier{ClassMaster: classMortal},
		}
		campo := ExpReward(in)
		if campo <= 0 {
			t.Fatalf("nível %d: campo pagou %d, o caso não testa nada", nivel, campo)
		}
		in.Zone = ZoneArenas
		if got := ExpReward(in); got != campo {
			t.Errorf("nível %d nas arenas pagou %d, campo paga %d", nivel, got, campo)
		}
	}
}

func TestArenasLeemALinhaDoCampoAteTeremAPropria(t *testing.T) {
	dezVezes := []Cut{{UpTo: CutOpenEnded, Divisor: 10}}
	cfg := Config{Overrides: map[ConfigKey]Override{
		{Zone: ZoneField, Tier: TierMortal}: {RatePercent: 200, Cuts: dezVezes},
	}}
	in := ExpRewardInput{
		Zone: ZoneField, MobExp: 175_584, KillerLevel: 349, MobLevel: 399,
		Tier: Tier{ClassMaster: classMortal}, Config: cfg,
	}
	campo := ExpReward(in)
	legado := ExpReward(ExpRewardInput{
		Zone: ZoneArenas, MobExp: 175_584, KillerLevel: 349, MobLevel: 399, Tier: Tier{ClassMaster: classMortal},
	})
	if campo == legado {
		t.Fatalf("a linha do Campo não mudou nada (%d); o caso não testa nada", campo)
	}

	in.Zone = ZoneArenas
	if got := ExpReward(in); got != campo {
		t.Errorf("arenas sem linha própria pagaram %d, a linha do Campo paga %d", got, campo)
	}
	if got := cfg.RatePercent(ZoneArenas, TierMortal); got != 200 {
		t.Errorf("taxa das arenas sem linha própria = %d, quero a do Campo (200)", got)
	}
	if got := cfg.Cuts(ZoneArenas, TierMortal); len(got) != 1 || got[0].Divisor != 10 {
		t.Errorf("cortes das arenas sem linha própria = %+v, quero os do Campo", got)
	}

	// Só as arenas herdam: o deserto sem linha fica no legado.
	in.Zone = ZoneDesertoReino
	if got := ExpReward(in); got != legado {
		t.Errorf("deserto sem linha pagou %d, quero o legado %d", got, legado)
	}

	// Com linha própria, as arenas deixam de seguir o Campo.
	cfg.Overrides[ConfigKey{Zone: ZoneArenas, Tier: TierMortal}] = Override{Cuts: []Cut{{UpTo: CutOpenEnded, Divisor: 1}}}
	in.Zone = ZoneArenas
	in.Config = cfg
	propria := ExpReward(in)
	if propria == campo {
		t.Errorf("arenas com linha própria pagaram %d, igual ao Campo", propria)
	}
	if got := cfg.RatePercent(ZoneArenas, TierMortal); got != 100 {
		t.Errorf("taxa das arenas com linha própria sem taxa = %d, quero 100", got)
	}
	// E as outras evoluções, sem linha própria, continuam no Campo.
	if _, ok := cfg.Row(ZoneArenas, TierArch); ok {
		t.Error("Arch das arenas achou linha, e nem o Campo tem linha de Arch")
	}
}

// Um ponto de cada arena cai na zona; o detalhe borda a borda é do teste do
// handler, que lê quest256Steps.
func TestRetangulosDasArenas(t *testing.T) {
	casos := []struct {
		nome string
		x, y int32
		want Zone
	}{
		{"Coveiro", 2398, 2105, ZoneArenas},
		{"Jardim", 2234, 1714, ZoneArenas},
		{"Kaizen", 464, 3902, ZoneArenas},
		{"Hidras", 668, 3756, ZoneArenas},
		{"Elfos", 1322, 4041, ZoneArenas},
		{"Armia", 2113, 2079, ZoneField},
	}
	for _, c := range casos {
		if got := ZoneForTile(c.x, c.y); got != c.want {
			t.Errorf("%s (%d,%d) = %s, want %s", c.nome, c.x, c.y, got.Name(), c.want.Name())
		}
	}
	if ZoneArenas != 12 {
		t.Errorf("ZoneArenas = %d; o número vai para xp_rule.zone e não pode mudar", ZoneArenas)
	}
}
