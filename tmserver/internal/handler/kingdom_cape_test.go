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
	tests := []struct {
		name  string
		items []int16
		cost  int
		want  sapphirePayment
		sobra map[int16]int // what the bag holds afterwards, by item
		mexeu int           // how many slots the client must be told about
		cheia bool          // every other active slot is occupied
	}{
		{"soltas exatas", []int16{s1, s1, s1, s1}, 4, sapphirePaid, map[int16]int{}, 4, false},
		{"pacote e soltas", []int16{s10, s1, s1}, 12, sapphirePaid, map[int16]int{}, 3, false},
		{"faltam unidades", []int16{s1, s1}, 4, sapphireShort, map[int16]int{s1: 2}, 0, false},
		// The report of 17/09/2026: three Pacotes, a King quoting 8.
		{"so pacotes, troco de 2", []int16{s10, s10, s10}, 8, sapphirePaid, map[int16]int{s10: 2, s1: 2}, 2, false},
		{"soltas bastam, pacote fica", []int16{s10, s1, s1, s1}, 3, sapphirePaid, map[int16]int{s10: 1}, 3, false},
		{"soltas nao bastam, pacote paga", []int16{s10, s10, s1}, 18, sapphirePaid, map[int16]int{s1: 3}, 2, false},
		// The pack's own slot takes the first unit of change; the second needs
		// a slot that a full bag does not have.
		{"sem espaco para o troco", []int16{s10}, 8, sapphireNoRoom, map[int16]int{s10: 1}, 0, true},
		{"um de troco cabe no slot do pacote", []int16{s10}, 9, sapphirePaid, map[int16]int{s1: 1}, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var e world.Entity
			for i, index := range tc.items {
				e.Carry[i] = world.Item{Index: index}
			}
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
				if it.Index == s1 || it.Index == s10 {
					sobra[it.Index]++
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
