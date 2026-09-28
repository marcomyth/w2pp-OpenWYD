package world

import "testing"

// O NOME DA VILA SAI DA MESMA TABELA DOS LIMITES, e fora de cidade é VAZIO.
//
// Vazio e não "cidade 7": é melhor não dizer nada do que mostrar um número que
// ninguém sabe ler. E o -1, que é o "fora de cidade" do Village, tem de cair aqui.
func TestNomeDaVila(t *testing.T) {
	casos := map[int]string{
		0: "Armia", 1: "Azran", 2: "Erion", 3: "Nippleheim", 4: "Noatum",
		-1: "", 5: "", 99: "",
	}
	for i, quer := range casos {
		if got := NomeDaVila(i); got != quer {
			t.Errorf("NomeDaVila(%d) = %q, quero %q", i, got, quer)
		}
	}
	// E a tabela dos nomes acompanha a dos limites: uma cidade a mais numa e não na
	// outra faria a vitrine mostrar o nome errado.
	if len(nomesDasCidades) != len(cities) {
		t.Errorf("%d nomes para %d cidades", len(nomesDasCidades), len(cities))
	}
}

// A QUANTIDADE NA VITRINE: item sem EF_AMOUNT é UM, e não zero.
//
// Um zero faria a vitrine anunciar "0 unidades" de um item que existe.
func TestQuantidadeNaVitrine(t *testing.T) {
	var vazio Item
	if got := vazio.QuantidadeNaVitrine(); got != 1 {
		t.Errorf("item sem EF_AMOUNT = %d, quero 1", got)
	}

	var pilha Item
	pilha.Effects[0] = Effect{Effect: efAmountNaVitrine, Value: 20}
	if got := pilha.QuantidadeNaVitrine(); got != 20 {
		t.Errorf("pilha de 20 = %d", got)
	}

	var zero Item
	zero.Effects[0] = Effect{Effect: efAmountNaVitrine, Value: 0}
	if got := zero.QuantidadeNaVitrine(); got != 1 {
		t.Errorf("EF_AMOUNT zero = %d, quero 1", got)
	}
}

// O REFINO SAI DE EF_SANC, e item sem ele é +0.
func TestRefino(t *testing.T) {
	var liso Item
	if got := liso.Refino(); got != 0 {
		t.Errorf("item liso = +%d", got)
	}
	var anc Item
	anc.Effects[1] = Effect{Effect: efSancNaVitrine, Value: 9}
	if got := anc.Refino(); got != 9 {
		t.Errorf("refino = +%d, quero +9", got)
	}
}
