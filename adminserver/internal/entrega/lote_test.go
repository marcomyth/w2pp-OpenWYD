package entrega

import (
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/pilha"
)

const (
	diamante = 2441 // empilha
	espada   = 1100 // não empilha
)

// quantidades lê de volta quantas unidades cada linha do lote leva.
func quantidades(itens []Item) []int {
	out := make([]int, len(itens))
	for i, it := range itens {
		out[i] = 1
		for _, ef := range it.Eff {
			if ef[0] == pilha.EfAmount {
				out[i] = int(ef[1])
			}
		}
	}
	return out
}

func TestLoteDivideAQuantidade(t *testing.T) {
	casos := []struct {
		nome       string
		index      int32
		quantidade int
		quer       []int
	}{
		{"pilha de 1", diamante, 1, []int{1}},
		{"pilha de 120", diamante, 120, []int{120}},
		{"250 vira 120+120+10", diamante, 250, []int{120, 120, 10}},
		{"não empilha, 3 itens", espada, 3, []int{1, 1, 1}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			itens, err := Lote(Item{Index: c.index, Dias: 7}, c.quantidade)
			if err != nil {
				t.Fatalf("Lote: %v", err)
			}
			got := quantidades(itens)
			if len(got) != len(c.quer) {
				t.Fatalf("linhas %v, quero %v", got, c.quer)
			}
			for i := range got {
				if got[i] != c.quer[i] {
					t.Fatalf("linhas %v, quero %v", got, c.quer)
				}
			}
			for _, it := range itens {
				if it.Index != c.index || it.Dias != 7 {
					t.Errorf("linha mudou o item ou o prazo: %+v", it)
				}
			}
		})
	}
}

// Uma pilha de 1 sai exatamente como antes da quantidade existir: sem EF_AMOUNT.
func TestLoteDeUmNaoEscreveQuantidade(t *testing.T) {
	itens, err := Lote(Item{Index: diamante}, 1)
	if err != nil || len(itens) != 1 || itens[0].Eff != ([3][2]uint8{}) {
		t.Fatalf("Lote(1) = %+v, %v; quero um item sem efeito", itens, err)
	}
}

func TestLoteRespeitaOLimiteDoBau(t *testing.T) {
	for _, c := range []struct {
		index int32
		max   int
	}{{diamante, pilha.EspacosDoBau * pilha.MaxPorPilha}, {espada, pilha.EspacosDoBau}} {
		if itens, err := Lote(Item{Index: c.index}, c.max); err != nil || len(itens) != pilha.EspacosDoBau {
			t.Errorf("item %d no limite (%d): %d linhas, %v", c.index, c.max, len(itens), err)
		}
		if _, err := Lote(Item{Index: c.index}, c.max+1); !errors.Is(err, ErrQuantidade) {
			t.Errorf("item %d acima do limite: err = %v, quero ErrQuantidade", c.index, err)
		}
	}
	if _, err := Lote(Item{Index: diamante}, 0); !errors.Is(err, ErrQuantidade) {
		t.Errorf("quantidade 0: err = %v, quero ErrQuantidade", err)
	}
}

func TestLotePilhaPrecisaDeEfeitoLivre(t *testing.T) {
	cheio := Item{Index: diamante, Eff: [3][2]uint8{{1, 1}, {2, 2}, {3, 3}}}
	if _, err := Lote(cheio, 2); !errors.Is(err, ErrEfeitos) {
		t.Errorf("três efeitos ocupados: err = %v, quero ErrEfeitos", err)
	}
	if _, err := Lote(cheio, 1); err != nil {
		t.Errorf("pilha de 1 não precisa de efeito livre: %v", err)
	}
	aMao := Item{Index: diamante, Eff: [3][2]uint8{{pilha.EfAmount, 50}}}
	if _, err := Lote(aMao, 10); !errors.Is(err, ErrEfeitos) {
		t.Errorf("EF_AMOUNT escrito à mão com quantidade: err = %v, quero ErrEfeitos", err)
	}
	comEfeito := Item{Index: diamante, Eff: [3][2]uint8{{43, 5}}}
	itens, err := Lote(comEfeito, 30)
	if err != nil || itens[0].Eff[0] != [2]uint8{43, 5} || itens[0].Eff[1] != [2]uint8{pilha.EfAmount, 30} {
		t.Errorf("efeito do moderador + quantidade = %+v, %v", itens, err)
	}
}
