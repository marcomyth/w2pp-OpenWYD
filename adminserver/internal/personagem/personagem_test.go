package personagem

import "testing"

// The Bolsa do Andarilho geometry is the one piece of game rule this package
// carries, and getting it wrong is not cosmetic: a slot the editor believes is
// open but the game does not is an item the player can never reach.
func TestLimiteCarry(t *testing.T) {
	bolsa := func(slots ...int) []Item {
		carry := make([]Item, MaxCarry)
		for i := range carry {
			carry[i].Slot = i
		}
		for _, s := range slots {
			carry[s] = Item{Slot: s, Index: ItemBolsaAndarilho}
		}
		return carry
	}

	cases := []struct {
		nome  string
		carry []Item
		want  int
	}{
		{"sem bolsa", bolsa(), 30},
		{"primeira bolsa", bolsa(SlotMarcadorBolsa1), 45},
		{"segunda bolsa", bolsa(SlotMarcadorBolsa2), 45},
		{"duas bolsas", bolsa(SlotMarcadorBolsa1, SlotMarcadorBolsa2), 60},
		// An item that is not a bag sitting in a marker slot must not unlock
		// anything: the game checks the index, not that the slot is occupied.
		{"item errado no marcador", func() []Item {
			c := bolsa()
			c[SlotMarcadorBolsa1] = Item{Slot: SlotMarcadorBolsa1, Index: 400}
			return c
		}(), 30},
		{"carry vazio", nil, 30},
	}

	for _, c := range cases {
		t.Run(c.nome, func(t *testing.T) {
			f := Ficha{Carry: c.carry}
			if got := f.LimiteCarry(); got != c.want {
				t.Errorf("LimiteCarry() = %d, want %d", got, c.want)
			}
		})
	}
}

// The unlocked band never exceeds 60: slots 62 and 63 exist in the array and are
// unreachable in game no matter what is held.
func TestLimiteCarryNuncaPassaDe60(t *testing.T) {
	carry := make([]Item, MaxCarry)
	for i := range carry {
		carry[i] = Item{Slot: i, Index: ItemBolsaAndarilho}
	}
	if got := (Ficha{Carry: carry}).LimiteCarry(); got != MaxCarryLiberado {
		t.Errorf("LimiteCarry() = %d, want %d", got, MaxCarryLiberado)
	}
}

func TestEmJogo(t *testing.T) {
	if (Ficha{}).EmJogo() {
		t.Error("ficha sem marca de presença não deveria estar em jogo")
	}
}

func TestDestinoTamanho(t *testing.T) {
	cases := []struct {
		dest Destino
		want int
		ok   bool
	}{
		{DestinoEquip, MaxEquip, true},
		{DestinoCarry, MaxCarry, true},
		{DestinoCargo, MaxCargo, true},
		// An unknown destination must not fall through to a default container:
		// it is how a typo in a form would write into the wrong place.
		{Destino("char_qualquer"), 0, false},
		{Destino(""), 0, false},
	}
	for _, c := range cases {
		n, ok := c.dest.tamanho()
		if n != c.want || ok != c.ok {
			t.Errorf("Destino(%q).tamanho() = %d,%v; want %d,%v", c.dest, n, ok, c.want, c.ok)
		}
	}
}

func TestDonos(t *testing.T) {
	// The warehouse hangs off the account and the other two off the character;
	// the item table has a CHECK constraint that refuses anything else.
	if char, acc := donos(7, 9, DestinoCargo); char != nil || acc != int64(9) {
		t.Errorf("donos(cargo) = %v,%v; want nil,9", char, acc)
	}
	for _, d := range []Destino{DestinoEquip, DestinoCarry} {
		char, acc := donos(7, 9, d)
		if char != int64(7) || acc != nil {
			t.Errorf("donos(%q) = %v,%v; want 7,nil", d, char, acc)
		}
	}
}
