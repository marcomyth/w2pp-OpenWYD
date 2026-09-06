package panel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// --- ordenar por link ---

func TestOrdemLinkInverteAColunaAtual(t *testing.T) {
	// Clicking the column you are already sorted by flips the direction;
	// clicking another starts it ascending. Anything else surprises everybody
	// who has ever used a table.
	o := ordem{Por: "nome"}
	if got := o.Link("/itens", "nome", nil); !strings.Contains(got, "desc=1") {
		t.Errorf("link = %q, want ao contrário na coluna atual", got)
	}
	if got := o.Link("/itens", "preco", nil); strings.Contains(got, "desc=1") {
		t.Errorf("link = %q, want ascendente numa coluna nova", got)
	}
	descendo := ordem{Por: "nome", Desc: true}
	if got := descendo.Link("/itens", "nome", nil); strings.Contains(got, "desc=1") {
		t.Errorf("link = %q, want voltando para ascendente", got)
	}
}

func TestOrdemLinkPreservaABusca(t *testing.T) {
	// Sorting must not throw away the search term: the whole point is to sort
	// what you already narrowed down.
	o := ordem{}
	got := o.Link("/itens", "preco", url.Values{"q": {"espada"}, "aviso": {""}})
	if !strings.Contains(got, "q=espada") {
		t.Errorf("link = %q, want mantendo a busca", got)
	}
	// And it must not carry the old order forward, or the link would mean two
	// things at once.
	if strings.Count(got, "ordem=") != 1 {
		t.Errorf("link = %q, want um ordem= só", got)
	}
	// Empty values are dropped rather than carried as noise.
	if strings.Contains(got, "aviso=") {
		t.Errorf("link = %q, want sem os vazios", got)
	}
}

func TestOrdemDesconhecidaCaiNoPadrao(t *testing.T) {
	// It arrives from an old bookmark. Refusing the page over a column that no
	// longer exists is a dead end where there was still something to show.
	r := httptest.NewRequest(http.MethodGet, "/itens?ordem=coluna_que_sumiu", nil)
	if o := ordemDe(r, "nome", "preco"); o.Por != "" {
		t.Errorf("ordem = %q, want o padrão da página", o.Por)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/itens?ordem=preco&desc=1", nil)
	o := ordemDe(r2, "nome", "preco")
	if o.Por != "preco" || !o.Desc {
		t.Errorf("ordem = %+v, want preco descendente", o)
	}
}

func TestMenorAplicaADirecao(t *testing.T) {
	xs := []int{3, 1, 2}
	asc := func(i, j int) bool { return xs[i] < xs[j] }

	sort.SliceStable(xs, ordem{}.Menor(asc))
	if xs[0] != 1 {
		t.Errorf("crescente = %v", xs)
	}
	sort.SliceStable(xs, ordem{Desc: true}.Menor(asc))
	if xs[0] != 3 {
		t.Errorf("decrescente = %v", xs)
	}
}

func TestAListaDeItensOrdenaPorPreco(t *testing.T) {
	// The fake catalog returns 1415 (no price) before 2000 (price 5000).
	h := painelDeEditores(t, &fakeJogo{estado: estadoDeTeste()})
	get := signedIn(t, h)

	corpo := get("/itens?ordem=preco&desc=1").Body.String()
	i2000 := strings.Index(corpo, "Espada Longa")
	i1415 := strings.Index(corpo, "Sapatos Pele de Animal")
	if i2000 < 0 || i1415 < 0 {
		t.Fatal("a lista não trouxe os dois itens")
	}
	if i2000 > i1415 {
		t.Error("o mais caro não veio primeiro com a ordem decrescente por preço")
	}
	// The header marks which column is doing it, or the reader has to guess.
	if !strings.Contains(corpo, `class="ord desc"`) {
		t.Error("a coluna ordenada não está marcada")
	}
}

func TestOsCabecalhosDeOrdemSaoLinks(t *testing.T) {
	// No JavaScript: a sortable column has to be a link, and the order has to
	// survive being shared or bookmarked.
	h := painelDeEditores(t, &fakeJogo{estado: estadoDeTeste()})
	corpo := signedIn(t, h)("/itens?q=espada").Body.String()
	if !strings.Contains(corpo, `href="/itens?ordem=preco`) {
		t.Error("o cabeçalho de preço não é um link de ordenação")
	}
	// And it keeps the search: sorting a filtered list must not un-filter it.
	if !strings.Contains(corpo, "q=espada") {
		t.Error("ordenar jogaria fora a busca")
	}
	if strings.Contains(strings.ToLower(corpo), "<script") {
		t.Error("a ordenação passou a depender de JavaScript")
	}
}
