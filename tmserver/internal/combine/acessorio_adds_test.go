package combine

import (
	"reflect"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func comAdds(adds ...world.Effect) world.Item {
	it := world.Item{Index: 551, Effects: [3]world.Effect{{Effect: efAddSanc, Value: 9}}}
	copy(it.Effects[1:], adds)
	return it
}

func add(ef uint8, v uint8) world.Effect { return world.Effect{Effect: ef, Value: v} }

// sorteios devolve os valores na ordem pedida e falha o teste se a regra sortear
// mais vezes do que o caso previu.
func sorteios(t *testing.T, valores ...int) func(int) int {
	t.Helper()
	return func(n int) int {
		if len(valores) == 0 {
			t.Fatalf("sorteio inesperado (rand %% %d)", n)
		}
		v := valores[0]
		valores = valores[1:]
		if v < 0 || v >= n {
			t.Fatalf("sorteio %d fora de rand %% %d", v, n)
		}
		return v
	}
}

func TestMesclarAdds(t *testing.T) {
	cases := []struct {
		nome     string
		a, b     world.Item
		sorteios []int
		quer     []world.Effect
	}{
		{"nenhum add", comAdds(), comAdds(), nil, []world.Effect{}},
		{"só o primeiro tem add: fica, sem sorteio", comAdds(add(efAddMagia, 8)), comAdds(), nil,
			[]world.Effect{add(efAddMagia, 8)}},
		{"só o segundo tem add: fica", comAdds(), comAdds(add(efAddHP, 60)), nil,
			[]world.Effect{add(efAddHP, 60)}},
		{"junção de magia", comAdds(add(efAddMagia, 8)), comAdds(add(efAddMagia, 6)), nil,
			[]world.Effect{add(efAddMagia, 14)}},
		{"junção de dano bate no teto", comAdds(add(efAddDano, 18)), comAdds(add(efAddDano, 15)), nil,
			[]world.Effect{add(efAddDano, 30)}},
		{"junção de crítico", comAdds(add(efAddCritico, 20)), comAdds(add(efAddCritico, 20)), nil,
			[]world.Effect{add(efAddCritico, 30)}},
		{"junção de HP bate no teto", comAdds(add(efAddHP, 60)), comAdds(add(efAddHP, 55)), nil,
			[]world.Effect{add(efAddHP, 105)}},
		{"magia contra dano: fica o dano", comAdds(add(efAddMagia, 8)), comAdds(add(efAddDano, 15)), []int{0},
			[]world.Effect{add(efAddMagia, 8)}},
		{"magia contra dano: fica a magia", comAdds(add(efAddMagia, 8)), comAdds(add(efAddDano, 15)), []int{1},
			[]world.Effect{add(efAddDano, 15)}},
		{"mescla magia e HP: 40% e 80%", comAdds(add(efAddMagia, 10)), comAdds(add(efAddHP, 60)), []int{0, 40},
			[]world.Effect{add(efAddMagia, 4), add(efAddHP, 48)}},
		{"mescla nunca zera", comAdds(add(efAddMagia, 1)), comAdds(add(efAddHP, 2)), []int{0, 0},
			[]world.Effect{add(efAddMagia, 1), add(efAddHP, 1)}},
		{"mescla de crítico anda de 1% em 1%, mínimo 1%", comAdds(add(efAddCritico, 20)), comAdds(add(efAddDano, 15)), []int{15, 0},
			[]world.Effect{add(efAddCritico, 10), add(efAddDano, 6)}},
		{"item já mesclado com um add do mesmo tipo: junta sem reduzir",
			comAdds(add(efAddMagia, 5), add(efAddHP, 40)), comAdds(add(efAddMagia, 6)), nil,
			[]world.Effect{add(efAddMagia, 11), add(efAddHP, 40)}},
		{"item mesclado (MG+HP) com dano: o dano perde e nada é reduzido",
			comAdds(add(efAddMagia, 5), add(efAddHP, 40)), comAdds(add(efAddDano, 15)), []int{0},
			[]world.Effect{add(efAddMagia, 5), add(efAddHP, 40)}},
		{"item mesclado (MG+HP) com dano: a magia perde e HP + dano mesclam",
			comAdds(add(efAddMagia, 5), add(efAddHP, 40)), comAdds(add(efAddDano, 20)), []int{1, 40, 0},
			[]world.Effect{add(efAddHP, 32), add(efAddDano, 8)}},
		{"três tipos que convivem: sorteia qual sai, e os que ficam mesclam",
			comAdds(add(efAddMagia, 10), add(efAddHP, 60)), comAdds(add(efAddCritico, 20)), []int{1, 40, 40},
			[]world.Effect{add(efAddMagia, 8), add(efAddCritico, 10)}},
		{"três tipos, sai o que já vinha com outro: sem redução",
			comAdds(add(efAddMagia, 10), add(efAddHP, 60)), comAdds(add(efAddCritico, 20)), []int{2},
			[]world.Effect{add(efAddMagia, 10), add(efAddHP, 60)}},
		{"o espaço vazio do drop não é add", comAdds(world.Effect{Effect: efAddVazio, Value: 77}), comAdds(add(efAddHP, 60)), nil,
			[]world.Effect{add(efAddHP, 60)}},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			got := MesclarAdds(tc.a, tc.b, sorteios(t, tc.sorteios...))
			if !reflect.DeepEqual(got, tc.quer) {
				t.Errorf("MesclarAdds = %+v, quero %+v", got, tc.quer)
			}
		})
	}
}

// TestMesclarAddsNuncaPassaDeDois: qualquer combinação cabe nos dois espaços.
func TestMesclarAddsNuncaPassaDeDois(t *testing.T) {
	tipos := []uint8{efAddMagia, efAddDano, efAddCritico, efAddHP, efAddMP}
	for _, a1 := range tipos {
		for _, a2 := range tipos {
			for _, b1 := range tipos {
				for _, b2 := range tipos {
					got := MesclarAdds(comAdds(add(a1, 9), add(a2, 9)), comAdds(add(b1, 9), add(b2, 9)), func(n int) int { return n - 1 })
					if len(got) > MaxAdds {
						t.Fatalf("%d %d + %d %d deu %d adds", a1, a2, b1, b2, len(got))
					}
					if len(got) == 2 && got[0].Effect == got[1].Effect {
						t.Fatalf("%d %d + %d %d repetiu o tipo: %+v", a1, a2, b1, b2, got)
					}
					mg, ad := false, false
					for _, g := range got {
						mg = mg || g.Effect == efAddMagia
						ad = ad || g.Effect == efAddDano
					}
					if mg && ad {
						t.Fatalf("%d %d + %d %d deixou magia e dano juntos: %+v", a1, a2, b1, b2, got)
					}
				}
			}
		}
	}
}
