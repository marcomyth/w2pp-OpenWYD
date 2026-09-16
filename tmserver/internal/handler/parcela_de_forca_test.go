package handler

import "testing"

func TestParcelaDeForca(t *testing.T) {
	cases := []struct {
		name     string
		str, dex int
		want     int
	}{
		{"1600 / 500 é força pura", 1600, 500, 1000},
		{"3× a destreza é o começo da força pura", 1500, 500, 1000},
		{"um pouco abaixo de 3× ainda não é", 1400, 500, 973},
		{"2000 / 1000", 2000, 1000, 833},
		{"1500 / 1000", 1500, 1000, 700},
		{"meio a meio", 1000, 1000, 500},
		{"500 / 1600 é destreza pura", 500, 1600, 0},
		{"sem atributo conta meio a meio", 0, 0, 500},
		{"negativo conta como zero", 1000, -300, 1000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parcelaDeForca(c.str, c.dex); got != c.want {
				t.Errorf("parcelaDeForca(%d, %d) = %d, want %d", c.str, c.dex, got, c.want)
			}
		})
	}
}
