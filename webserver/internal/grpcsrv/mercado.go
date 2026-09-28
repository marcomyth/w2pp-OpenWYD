package grpcsrv

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mercado"
)

// Vitrine é a leitura das barracas, com o cache (satisfeita por *mercado.Mercado).
type Vitrine interface {
	Ofertas(ctx context.Context) ([]mercado.Oferta, time.Duration, error)
}

// ComVitrine liga a vitrine do mercado ao serviço do site.
//
// Construtor separado, como o ComCriadorDePix: sem o link com o jogo a vitrine não
// existe, e o caminho sem ela continua sendo o que os testes antigos cobrem em vez
// de todos passarem a carregar um nulo.
func (s *ServerRmt) ComVitrine(v Vitrine) *ServerRmt {
	s.vitrine = v
	return s
}

// ListMarketListings devolve a vitrine pública.
//
// A MOEDA É OBRIGATÓRIA, e UNSPECIFIED é recusado. Não respondido com tudo por causa
// da ordenação — ordenar por preço misturando moedas poria 300 de ouro ao lado de 300
// centavos e chamaria a lista de ordenada — e não respondido vazio, porque vazio se
// lê como "não há nada à venda", que é uma mentira cara de depurar.
func (s *ServerRmt) ListMarketListings(ctx context.Context, req *webv1.ListMarketListingsRequest,
) (*webv1.ListMarketListingsResponse, error) {
	if s.vitrine == nil {
		return nil, status.Error(codes.Unavailable, "a vitrine precisa do link com o servidor de jogo")
	}
	moeda, ok := moedaDoProto(req.GetCurrency())
	if !ok {
		return nil, status.Error(codes.InvalidArgument,
			"escolha a moeda: ouro, cash ou dinheiro real")
	}

	lista, idade, err := s.vitrine.Ofertas(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "list market: %v", err)
	}
	pagina, total := mercado.Filtrar(lista, mercado.Filtro{
		Moeda:  moeda,
		Item:   req.GetItemIndex(),
		Ordem:  ordemDoProto(req.GetSort()),
		Pagina: req.GetPage(),
	})

	resp := &webv1.ListMarketListingsResponse{
		Listings:         make([]*webv1.MarketListing, 0, len(pagina)),
		Total:            int32(total),
		CachedForSeconds: int32(idade.Seconds()),
	}
	for _, o := range pagina {
		resp.Listings = append(resp.Listings, &webv1.MarketListing{
			ItemIndex:       o.Indice,
			Refine:          o.Refino,
			Amount:          o.Qtd,
			Currency:        moedaParaProto(o.Moeda),
			Price:           o.Preco,
			SellerCharacter: o.Personagem,
			Village:         o.Cidade,
			OpenForSeconds:  int64(o.AbertaHa.Seconds()),
		})
	}
	return resp, nil
}

// moedaDoProto traduz, e diz se a escolha existe. O UNSPECIFIED devolve ok=false,
// que é o que vira a recusa.
func moedaDoProto(c webv1.MarketCurrency) (int32, bool) {
	switch c {
	case webv1.MarketCurrency_MARKET_CURRENCY_GOLD:
		return mercado.MoedaOuro, true
	case webv1.MarketCurrency_MARKET_CURRENCY_CASH:
		return mercado.MoedaCash, true
	case webv1.MarketCurrency_MARKET_CURRENCY_REAL:
		return mercado.MoedaReal, true
	}
	return 0, false
}

func moedaParaProto(m int32) webv1.MarketCurrency {
	switch m {
	case mercado.MoedaOuro:
		return webv1.MarketCurrency_MARKET_CURRENCY_GOLD
	case mercado.MoedaCash:
		return webv1.MarketCurrency_MARKET_CURRENCY_CASH
	case mercado.MoedaReal:
		return webv1.MarketCurrency_MARKET_CURRENCY_REAL
	}
	// Moeda que este código não conhece NÃO vira ouro: viraria um preço em ouro numa
	// tela, e alguém compraria por engano.
	return webv1.MarketCurrency_MARKET_CURRENCY_UNSPECIFIED
}

// ordemDoProto traduz a ordenação. O desconhecido cai na ordem do jogo, que é a
// resposta honesta para "não sei o que você pediu": ela não promete nada.
func ordemDoProto(o webv1.MarketSort) mercado.Ordem {
	switch o {
	case webv1.MarketSort_MARKET_SORT_PRICE_ASC:
		return mercado.OrdemPrecoSobe
	case webv1.MarketSort_MARKET_SORT_PRICE_DESC:
		return mercado.OrdemPrecoDesce
	case webv1.MarketSort_MARKET_SORT_NEWEST:
		return mercado.OrdemMaisNovas
	}
	return mercado.OrdemDoJogo
}
