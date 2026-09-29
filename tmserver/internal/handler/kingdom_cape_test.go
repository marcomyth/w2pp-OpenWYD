package handler

import (
	"maps"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func TestCapeKingdomMode(t *testing.T) {
	tests := []struct {
		index   int16
		kingdom uint8
		mode    int
	}{
		{543, clanHekalotia, 2}, {545, clanHekalotia, 1}, {734, clanHekalotia, 1}, {736, clanHekalotia, 1},
		{544, clanAkelonia, 2}, {546, clanAkelonia, 1}, {735, clanAkelonia, 1}, {737, clanAkelonia, 1},
		{3191, clanHekalotia, 2}, {3194, clanHekalotia, 2}, {3197, clanHekalotia, 2},
		{3192, clanAkelonia, 2}, {3195, clanAkelonia, 2}, {3198, clanAkelonia, 2},
		{549, 0, 1}, {3193, 0, 1}, {3196, 0, 1}, {3199, 0, 0}, {0, 0, 0},
	}
	for _, tc := range tests {
		kingdom, mode := capeKingdomMode(tc.index)
		if kingdom != tc.kingdom || mode != tc.mode {
			t.Errorf("cape %d = (%d,%d), want (%d,%d)", tc.index, kingdom, mode, tc.kingdom, tc.mode)
		}
	}
}

func TestSapphirePaymentPlan(t *testing.T) {
	const (
		s1  = sapphireUnit1
		s10 = sapphireUnit10
	)
	// pilha é uma Safira com n unidades; s1 sozinha é uma unidade sem EF_AMOUNT,
	// a forma que as bolsas guardavam antes de a Safira empilhar.
	pilha := func(n int) world.Item {
		it := world.Item{Index: s1}
		setItemAmount(&it, n)
		return it
	}
	um := func(index int16) world.Item { return world.Item{Index: index} }
	tests := []struct {
		name  string
		items []world.Item
		cost  int
		want  sapphirePayment
		sobra map[int16]int // unidades de Safira e número de Pacotes na bolsa depois
		mexeu int           // how many slots the client must be told about
		cheia bool          // every other active slot is occupied
	}{
		{"soltas exatas", []world.Item{um(s1), um(s1), um(s1), um(s1)}, 4, sapphirePaid, map[int16]int{}, 4, false},
		{"pacote e soltas", []world.Item{um(s10), um(s1), um(s1)}, 12, sapphirePaid, map[int16]int{}, 3, false},
		{"faltam unidades", []world.Item{um(s1), um(s1)}, 4, sapphireShort, map[int16]int{s1: 2}, 0, false},
		// The report of 17/09/2026: three Pacotes, a King quoting 8. The change
		// is one pile of two, in the slot the Pacote left.
		{"so pacotes, troco de 2", []world.Item{um(s10), um(s10), um(s10)}, 8, sapphirePaid, map[int16]int{s10: 2, s1: 2}, 1, false},
		{"soltas bastam, pacote fica", []world.Item{um(s10), um(s1), um(s1), um(s1)}, 3, sapphirePaid, map[int16]int{s10: 1}, 3, false},
		// O troco cai sobre a Safira que sobrou, e não num espaço novo.
		{"soltas nao bastam, pacote paga", []world.Item{um(s10), um(s10), um(s1)}, 18, sapphirePaid, map[int16]int{s1: 3}, 3, false},
		// Com a bolsa cheia, o troco inteiro cabe numa pilha no espaço do Pacote.
		{"bolsa cheia, troco em pilha", []world.Item{um(s10)}, 8, sapphirePaid, map[int16]int{s1: 2}, 1, true},
		{"um de troco cabe no slot do pacote", []world.Item{um(s10)}, 9, sapphirePaid, map[int16]int{s1: 1}, 1, true},
		// A Safira empilha desde 29/09/2026: a pilha paga por unidade.
		{"pilha de 50 paga 8 e fica com 42", []world.Item{pilha(50)}, 8, sapphirePaid, map[int16]int{s1: 42}, 1, false},
		{"pilha exata some", []world.Item{pilha(8)}, 8, sapphirePaid, map[int16]int{}, 1, false},
		{"pilha curta nao paga", []world.Item{pilha(7)}, 8, sapphireShort, map[int16]int{s1: 7}, 0, false},
		{"duas pilhas somam", []world.Item{pilha(3), pilha(6)}, 8, sapphirePaid, map[int16]int{s1: 1}, 2, false},
		{"troco junta na pilha", []world.Item{pilha(5), um(s10)}, 7, sapphirePaid, map[int16]int{s1: 8}, 2, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e world.Entity
			copy(e.Carry[:], tc.items)
			if tc.cheia {
				for i := len(tc.items); i < activeCarryLimit(&e); i++ {
					e.Carry[i] = world.Item{Index: 1}
				}
			}
			changed, got := sapphirePaymentPlan(&e, tc.cost)
			if got != tc.want {
				t.Fatalf("result = %v, want %v", got, tc.want)
			}
			if len(changed) != tc.mexeu {
				t.Errorf("changed slots = %v, want %d of them", changed, tc.mexeu)
			}
			sobra := map[int16]int{}
			for _, it := range e.Carry {
				switch it.Index {
				case s1:
					sobra[s1] += itemAmount(it)
				case s10:
					sobra[s10]++
				}
			}
			if !maps.Equal(sobra, tc.sobra) {
				t.Errorf("bag afterwards = %v, want %v", sobra, tc.sobra)
			}
		})
	}
}

func TestKingdomCapeTargets(t *testing.T) {
	tests := []struct {
		master  uint8
		level   int32
		current int16
		kingdom uint8
		want    int16
	}{
		{classMasterMortal, 219, 0, clanHekalotia, 545}, {classMasterMortal, 219, 0, clanAkelonia, 546},
		{classMasterMortal, 255, 549, clanHekalotia, 543}, {classMasterArch, 300, 3193, clanAkelonia, 3192},
		{classMasterArch, 300, 3196, clanHekalotia, 3194}, {classMasterCelestial, 0, 3199, clanHekalotia, 3197},
	}
	for _, tc := range tests {
		got, ok := kingdomCapeTarget(tc.master, tc.level, tc.current, tc.kingdom)
		if !ok || got != tc.want {
			t.Errorf("target = %d,%v want %d,true", got, ok, tc.want)
		}
	}
}
