package grpcsrv

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mercado"
)

type fakeVitrine struct {
	lista []mercado.Oferta
	idade time.Duration
	erro  error
}

func (f *fakeVitrine) Ofertas(context.Context) ([]mercado.Oferta, time.Duration, error) {
	return f.lista, f.idade, f.erro
}

func umaVitrine() *fakeVitrine {
	return &fakeVitrine{
		lista: []mercado.Oferta{
			{Indice: 1415, Refino: 9, Qtd: 1, Moeda: mercado.MoedaOuro, Preco: 500,
				Personagem: "Vendedora", Cidade: "Armia", AbertaHa: 90 * time.Second},
			{Indice: 2000, Moeda: mercado.MoedaReal, Preco: 100,
				Personagem: "Outra", Cidade: "Armia", AbertaHa: time.Minute},
		},
		idade: 3 * time.Second,
	}
}

// A MOEDA É OBRIGATÓRIA, e o vazio é RECUSADO.
//
// Recusar em vez de devolver tudo por causa da ordenação: por preço, misturar moedas
// poria 300 de ouro ao lado de 300 centavos. E em vez de devolver vazio porque vazio
// se lê como "não há nada à venda" — uma mentira que custa uma tarde de depuração de
// quem estiver integrando.
func TestAMoedaEhObrigatoria(t *testing.T) {
	s := NewRmt(&fakePix{}).ComVitrine(umaVitrine())

	_, err := s.ListMarketListings(context.Background(), &webv1.ListMarketListingsRequest{})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("erro = %v, quero InvalidArgument", err)
	}
}

// SEM O LINK COM O JOGO, A RESPOSTA É "INDISPONÍVEL" E NÃO UMA LISTA VAZIA.
//
// A barraca vive na memória do jogo: sem o link não há o que listar, e uma lista
// vazia diria que ninguém está vendendo.
func TestSemVitrineRespondeIndisponivel(t *testing.T) {
	s := NewRmt(&fakePix{})

	_, err := s.ListMarketListings(context.Background(), &webv1.ListMarketListingsRequest{
		Currency: webv1.MarketCurrency_MARKET_CURRENCY_GOLD,
	})

	if status.Code(err) != codes.Unavailable {
		t.Fatalf("erro = %v, quero Unavailable", err)
	}
}

// A RESPOSTA LEVA A IDADE DO CACHE, e o site tem direito de saber que está olhando
// uma foto de alguns segundos atrás.
func TestAIdadeDoCacheVaiNaResposta(t *testing.T) {
	s := NewRmt(&fakePix{}).ComVitrine(umaVitrine())

	resp, err := s.ListMarketListings(context.Background(), &webv1.ListMarketListingsRequest{
		Currency: webv1.MarketCurrency_MARKET_CURRENCY_GOLD,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCachedForSeconds() != 3 {
		t.Errorf("idade = %d, quero 3", resp.GetCachedForSeconds())
	}
	if len(resp.GetListings()) != 1 || resp.GetTotal() != 1 {
		t.Fatalf("listagens = %d, total = %d", len(resp.GetListings()), resp.GetTotal())
	}
	l := resp.GetListings()[0]
	if l.GetSellerCharacter() != "Vendedora" || l.GetVillage() != "Armia" {
		t.Errorf("linha = %+v", l)
	}
	if l.GetOpenForSeconds() != 90 {
		t.Errorf("aberta ha %d segundos, quero 90", l.GetOpenForSeconds())
	}
}

// AS TRÊS MOEDAS ATRAVESSAM NOS DOIS SENTIDOS.
func TestAsMoedasAtravessam(t *testing.T) {
	casos := map[webv1.MarketCurrency]int32{
		webv1.MarketCurrency_MARKET_CURRENCY_GOLD: mercado.MoedaOuro,
		webv1.MarketCurrency_MARKET_CURRENCY_CASH: mercado.MoedaCash,
		webv1.MarketCurrency_MARKET_CURRENCY_REAL: mercado.MoedaReal,
	}
	for proto, interno := range casos {
		if got, ok := moedaDoProto(proto); !ok || got != interno {
			t.Errorf("moedaDoProto(%v) = %d, %v", proto, got, ok)
		}
		if got := moedaParaProto(interno); got != proto {
			t.Errorf("moedaParaProto(%d) = %v, quero %v", interno, got, proto)
		}
	}
	// E a moeda que este código não conhece NÃO vira ouro: viraria um preço em ouro
	// numa tela, e alguém compraria por engano.
	if got := moedaParaProto(99); got != webv1.MarketCurrency_MARKET_CURRENCY_UNSPECIFIED {
		t.Errorf("moeda desconhecida virou %v", got)
	}
}
