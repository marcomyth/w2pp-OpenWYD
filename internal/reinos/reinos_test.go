package reinos

import "testing"

// As capas são as de Basedef.cpp:3220-3244, uma a uma.
func TestClanDaCapa(t *testing.T) {
	for _, c := range []int16{543, 545, 734, 736, 3191, 3194, 3197} {
		if got := ClanDaCapa(c); got != ClanHekalotia {
			t.Errorf("capa %d = clã %d, want Hekalotia", c, got)
		}
	}
	for _, c := range []int16{544, 546, 735, 737, 3192, 3195, 3198} {
		if got := ClanDaCapa(c); got != ClanAkelonia {
			t.Errorf("capa %d = clã %d, want Akelonia", c, got)
		}
	}
	for _, c := range []int16{0, 3199, 548, 3193, 3196} {
		if got := ClanDaCapa(c); got != 0 {
			t.Errorf("capa %d = clã %d, want 0", c, got)
		}
	}
}

func TestMonstroDoReino(t *testing.T) {
	casos := []struct {
		nome        string
		mobMerchant uint8
		clan        uint8
		x, y        int
		want        bool
	}{
		{"guarda azul na cidade", 0, 7, 1706, 1686, true},
		{"Rei Glantuar", 15, 8, 1750, 1880, true},
		{"Guarda_Real protegido", 100, 7, 1720, 1600, false},
		{"oráculo de clã 6", 20, 6, 1720, 1610, false},
		{"quartel de RvR longe da cidade", 0, 7, 2614, 1736, false},
		{"canto de dentro", 0, 8, minX, maxY, true},
		{"um tile fora", 0, 8, minX - 1, maxY, false},
	}
	for _, c := range casos {
		if got := MonstroDoReino(c.mobMerchant, c.clan, c.x, c.y); got != c.want {
			t.Errorf("%s: MonstroDoReino = %v, want %v", c.nome, got, c.want)
		}
	}
}
