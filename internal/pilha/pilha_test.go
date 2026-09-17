package pilha

import (
	"slices"
	"testing"
)

func TestDivide(t *testing.T) {
	const diamante, espada = 2441, 1100
	casos := []struct {
		nome       string
		index      int16
		quantidade int
		quer       []int
	}{
		{"pilha de 1", diamante, 1, []int{1}},
		{"pilha de 120", diamante, 120, []int{120}},
		{"250 vira 120+120+10", diamante, 250, []int{120, 120, 10}},
		{"não empilha, 3 itens", espada, 3, []int{1, 1, 1}},
		{"zero não ocupa espaço", diamante, 0, nil},
	}
	for _, c := range casos {
		if got := Divide(c.index, c.quantidade); !slices.Equal(got, c.quer) {
			t.Errorf("%s: Divide(%d, %d) = %v, quero %v", c.nome, c.index, c.quantidade, got, c.quer)
		}
	}
}

func TestMaxPorEnvioCabeNoBau(t *testing.T) {
	if got := len(Divide(2441, MaxPorEnvio(2441))); got != EspacosDoBau {
		t.Errorf("o máximo do Diamante ocupa %d espaços, quero %d", got, EspacosDoBau)
	}
	if got := len(Divide(2441, MaxPorEnvio(2441)+1)); got <= EspacosDoBau {
		t.Errorf("um a mais que o máximo ainda cabe (%d espaços)", got)
	}
	if MaxPorEnvio(1100) != EspacosDoBau {
		t.Errorf("item avulso: máximo %d, quero %d", MaxPorEnvio(1100), EspacosDoBau)
	}
}
