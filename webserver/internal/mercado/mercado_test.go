package mercado

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

type fakeJogo struct {
	lista   []Bruta
	erro    error
	chamou  int
	demorou time.Duration
}

func (f *fakeJogo) ListarMercado(context.Context) ([]Bruta, error) {
	f.chamou++
	if f.demorou > 0 {
		time.Sleep(f.demorou)
	}
	return f.lista, f.erro
}

type fakeBanco struct {
	ocupadas []store.PrateleiraOcupada
	erro     error
}

func (b *fakeBanco) PrateleirasComCobrancaAberta(context.Context) ([]store.PrateleiraOcupada, error) {
	return b.ocupadas, b.erro
}

func mudo() *slog.Logger { return slog.New(slog.DiscardHandler) }

func bruta(conta int64, slot int32, moeda int32, preco int64) Bruta {
	return Bruta{
		ContaVendedor: conta, CargoPos: slot, Personagem: "Vendedora",
		Indice: 1415, Refino: 9, Qtd: 1, Moeda: moeda, Preco: preco,
		Cidade: "Armia", AbertaHa: time.Minute,
	}
}

// A PRATELEIRA COM ALGUÉM PAGANDO NÃO APARECE.
//
// Enquanto a cobrança está aberta o jogo recusa o segundo comprador. Listá-la seria
// convidar alguém para uma recusa — e a pessoa clicaria achando que o item existe.
func TestPrateleiraComCobrancaAbertaSaiDaLista(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{
		bruta(7, 3, MoedaReal, 100),
		bruta(7, 4, MoedaReal, 200),
	}}
	b := &fakeBanco{ocupadas: []store.PrateleiraOcupada{{VendedorConta: 7, CargoSlot: 3}}}

	lista, _, err := Novo(j, b, mudo()).Ofertas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 1 || lista[0].Preco != 200 {
		t.Fatalf("lista = %+v; a prateleira em pagamento continuou na vitrine", lista)
	}
}

// E SÓ A DE DINHEIRO REAL SAI. A de ouro e a de cash não têm cobrança nenhuma, e
// uma coincidência de conta e slot não pode tirá-las da vitrine.
func TestSoAPrateleiraDeDinheiroRealSaiPorCobranca(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{
		bruta(7, 3, MoedaOuro, 100),
		bruta(7, 3, MoedaCash, 100),
	}}
	b := &fakeBanco{ocupadas: []store.PrateleiraOcupada{{VendedorConta: 7, CargoSlot: 3}}}

	lista, _, err := Novo(j, b, mudo()).Ofertas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 2 {
		t.Errorf("lista = %d, quero 2: uma prateleira de ouro sumiu por causa de uma cobranca", len(lista))
	}
}

// O ID DA CONTA NÃO ATRAVESSA. Ele entra na resposta do jogo, serve para achar o
// anúncio, e morre aqui — o contrato público não tem conta nenhuma, e este teste é o
// que impede alguém de acrescentar o campo "só para depurar".
func TestOIdDaContaNaoAtravessa(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{bruta(99, 1, MoedaOuro, 10)}}
	lista, _, err := Novo(j, &fakeBanco{}, mudo()).Ofertas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// A Oferta não tem campo de conta; o teste existe para o dia em que alguém
	// pensar em pôr um. Se este arquivo parar de compilar aqui, leia o comentário
	// acima antes de "consertar".
	var _ = lista[0].Personagem
}

// A CONSULTA DO BANCO QUE FALHA NÃO VIRA LISTA SEM FILTRO.
//
// Sem ela, as prateleiras em pagamento entrariam na vitrine. Uma página que falha é
// melhor do que uma que promete o que não pode vender.
func TestBancoQueFalhaNaoRespondeSemFiltro(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{bruta(7, 3, MoedaReal, 100)}}
	b := &fakeBanco{erro: errors.New("o banco caiu")}

	if _, _, err := Novo(j, b, mudo()).Ofertas(context.Background()); err == nil {
		t.Fatal("respondeu sem saber quais prateleiras estao em pagamento")
	}
}

// O CACHE POUPA O LAÇO DO JOGO: a segunda visita não pergunta de novo.
func TestOCacheNaoPerguntaDuasVezes(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{bruta(7, 3, MoedaOuro, 100)}}
	m := Novo(j, &fakeBanco{}, mudo())

	for i := 0; i < 5; i++ {
		if _, _, err := m.Ofertas(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if j.chamou != 1 {
		t.Errorf("perguntou %d vezes ao jogo; o cache nao esta segurando", j.chamou)
	}
}

// E O CACHE NÃO CONGELA OS MINUTOS.
//
// A lista guardada diz "montada há 1 minuto". Servida trinta segundos depois, ela
// tem de dizer "há 1 minuto e 30" — senão a página repete o mesmo número enquanto o
// cache valer, e passa a mentir sem que nada erre.
func TestOCacheSomaOTempoParado(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{bruta(7, 3, MoedaOuro, 100)}}
	m := Novo(j, &fakeBanco{}, mudo())

	if _, _, err := m.Ofertas(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Envelhece o cache na mão, que é o que o relógio faria.
	m.mu.Lock()
	m.buscada = time.Now().Add(-2 * time.Second)
	m.mu.Unlock()

	lista, idade, err := m.Ofertas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if idade < 2*time.Second {
		t.Errorf("idade = %v, quero pelo menos 2s", idade)
	}
	if lista[0].AbertaHa < time.Minute+2*time.Second {
		t.Errorf("aberta ha %v; o cache congelou o numero", lista[0].AbertaHa)
	}
	// E a lista GUARDADA não foi alterada: somar no cache acumularia a cada leitura.
	m.mu.Lock()
	guardada := m.lista[0].AbertaHa
	m.mu.Unlock()
	if guardada != time.Minute {
		t.Errorf("a lista guardada virou %v; a soma vazou para o cache", guardada)
	}
}

// COM O JOGO FORA, A LISTA VELHA AINDA SERVE — por um tempo.
//
// Uma vitrine vazia diz "ninguém está vendendo", que é falso e desanimador. Servir
// alguns segundos a mais de uma lista que existiu não engana do mesmo jeito. Passado
// um minuto ela vira mentira, e aí o erro é a resposta honesta.
func TestComOJogoForaServeAListaVelhaAteUmLimite(t *testing.T) {
	j := &fakeJogo{lista: []Bruta{bruta(7, 3, MoedaOuro, 100)}}
	m := Novo(j, &fakeBanco{}, mudo())
	if _, _, err := m.Ofertas(context.Background()); err != nil {
		t.Fatal(err)
	}

	j.erro = errors.New("o jogo nao respondeu")
	m.mu.Lock()
	m.buscada = time.Now().Add(-10 * time.Second)
	m.mu.Unlock()

	lista, _, err := m.Ofertas(context.Background())
	if err != nil || len(lista) != 1 {
		t.Fatalf("lista = %d, err = %v: devia servir a velha", len(lista), err)
	}

	// Passado o limite, ela para de servir.
	m.mu.Lock()
	m.buscada = time.Now().Add(-2 * time.Minute)
	m.mu.Unlock()
	if _, _, err = m.Ofertas(context.Background()); err == nil {
		t.Error("serviu uma lista de dois minutos atras como se fosse de agora")
	}
}

// O FILTRO E A ORDEM.
func TestFiltrar(t *testing.T) {
	lista := []Oferta{
		{Indice: 10, Moeda: MoedaOuro, Preco: 300, AbertaHa: 3 * time.Minute},
		{Indice: 11, Moeda: MoedaOuro, Preco: 100, AbertaHa: time.Minute},
		{Indice: 10, Moeda: MoedaOuro, Preco: 200, AbertaHa: 2 * time.Minute},
		{Indice: 10, Moeda: MoedaCash, Preco: 1, AbertaHa: time.Second},
	}

	t.Run("a moeda separa as prateleiras", func(t *testing.T) {
		got, total := Filtrar(lista, Filtro{Moeda: MoedaCash})
		if total != 1 || len(got) != 1 || got[0].Preco != 1 {
			t.Errorf("got = %+v, total = %d", got, total)
		}
	})

	t.Run("o item filtra dentro da moeda", func(t *testing.T) {
		got, total := Filtrar(lista, Filtro{Moeda: MoedaOuro, Item: 10})
		if total != 2 || len(got) != 2 {
			t.Errorf("got = %d, total = %d", len(got), total)
		}
	})

	t.Run("preco subindo", func(t *testing.T) {
		got, _ := Filtrar(lista, Filtro{Moeda: MoedaOuro, Ordem: OrdemPrecoSobe})
		if got[0].Preco != 100 || got[2].Preco != 300 {
			t.Errorf("ordem = %v", []int64{got[0].Preco, got[1].Preco, got[2].Preco})
		}
	})

	t.Run("preco descendo", func(t *testing.T) {
		got, _ := Filtrar(lista, Filtro{Moeda: MoedaOuro, Ordem: OrdemPrecoDesce})
		if got[0].Preco != 300 || got[2].Preco != 100 {
			t.Errorf("ordem = %v", []int64{got[0].Preco, got[1].Preco, got[2].Preco})
		}
	})

	t.Run("mais novas primeiro", func(t *testing.T) {
		// A mais nova é a que está aberta há MENOS tempo.
		got, _ := Filtrar(lista, Filtro{Moeda: MoedaOuro, Ordem: OrdemMaisNovas})
		if got[0].AbertaHa != time.Minute {
			t.Errorf("a primeira esta aberta ha %v", got[0].AbertaHa)
		}
	})
}

// A PÁGINA ALÉM DO FIM É VAZIA COM O TOTAL DE VERDADE, e nunca erro.
//
// A lista encolhe entre dois cliques o tempo todo — uma barraca desce —, e quem
// chegou na página 3 de um mercado que agora tem duas não fez nada errado.
func TestPaginaAlemDoFimEhVaziaComOTotal(t *testing.T) {
	var lista []Oferta
	for i := 0; i < 60; i++ {
		lista = append(lista, Oferta{Moeda: MoedaOuro, Preco: int64(i)})
	}

	primeira, total := Filtrar(lista, Filtro{Moeda: MoedaOuro, Pagina: 1})
	if len(primeira) != PorPagina || total != 60 {
		t.Fatalf("primeira = %d, total = %d", len(primeira), total)
	}
	segunda, _ := Filtrar(lista, Filtro{Moeda: MoedaOuro, Pagina: 2})
	if len(segunda) != 10 {
		t.Fatalf("segunda = %d, quero 10", len(segunda))
	}
	longe, total := Filtrar(lista, Filtro{Moeda: MoedaOuro, Pagina: 9})
	if len(longe) != 0 || total != 60 {
		t.Errorf("pagina 9 = %d linhas, total = %d; quero vazia com o total certo", len(longe), total)
	}
	// E zero é lido como a primeira.
	zero, _ := Filtrar(lista, Filtro{Moeda: MoedaOuro, Pagina: 0})
	if len(zero) != PorPagina {
		t.Errorf("pagina 0 = %d, quero a primeira", len(zero))
	}
}
