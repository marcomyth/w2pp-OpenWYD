package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
)

const (
	escudoDeTeste   = 1705
	armaduraDeTeste = 1331
)

func dispatcherDoEscudo() *Dispatcher {
	return New(Config{
		ItemEffects: map[int][]content.BaseEffect{
			escudoDeTeste:   {{Eff: efAc, Val: 200}},
			armaduraDeTeste: {{Eff: efAc, Val: 200}},
		},
		ItemPos: map[int]int{escudoDeTeste: nPosDef3, armaduraDeTeste: nPosDef1},
	})
}

// O escudo +10 ou mais ganha 10% da própria defesa (já escalada pelo refino);
// abaixo de +10, e em qualquer peça que não seja escudo, não ganha nada.
func TestEscudoMais10GanhaDezPorCentoDaPropriaDefesa(t *testing.T) {
	d := dispatcherDoEscudo()
	cases := []struct {
		nome  string
		index int16
		level int
		bonus bool
	}{
		{"escudo +0", escudoDeTeste, 0, false},
		{"escudo +9", escudoDeTeste, 9, false},
		{"escudo +10", escudoDeTeste, 10, true},
		{"escudo +15", escudoDeTeste, 15, true},
		{"armadura +10 não é escudo", armaduraDeTeste, 10, false},
	}
	for _, c := range cases {
		it := itemRefinado(t, c.index, c.level)
		want := int32(0)
		if c.bonus {
			want = d.itemAbilityRefined(it, efAc) / 10
			if want <= 0 {
				t.Fatalf("%s: defesa refinada zerada, o teste não prova nada", c.nome)
			}
		}
		if got := d.defesaDoEscudoRefinado(it); got != want {
			t.Errorf("%s: bônus = %d, esperado %d", c.nome, got, want)
		}
	}
}

// Pela conta inteira: o escudo +10 na mão esquerda soma a defesa refinada, os
// +25 do legado a partir do +9 e os 10% da regra nova.
func TestEscudoMais10NaDefesaDoPersonagem(t *testing.T) {
	d := dispatcherDoEscudo()
	sem := testPlayerEntity()
	d.refreshScore(sem)

	it := itemRefinado(t, escudoDeTeste, 10)
	com := testPlayerEntity()
	com.Equip[weaponSlotL] = it
	d.refreshScore(com)

	refinada := d.itemAbilityRefined(it, efAc)
	if got, want := com.AC-sem.AC, refinada+25+refinada/10; got != want {
		t.Errorf("defesa do escudo +10 = %d, esperado %d (refinada %d + 25 + 10%%)", got, want, refinada)
	}
}
