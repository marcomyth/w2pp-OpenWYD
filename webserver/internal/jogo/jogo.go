// Package jogo is the web-api's link to the RUNNING game server, for the two
// calls the RMT path needs: hand the item to the buyer now, and clear the sold
// slot of the seller now.
//
// WHY THIS IS NOT AN IMPORT OF adminserver/internal/jogo, which already speaks
// this API: that package belongs to the staff panel, and the panel is
// deliberately standalone and deletable — nothing imports it (CLAUDE.md). An
// import here would quietly make the panel undeletable, and the web-api would
// start depending on a service it has nothing to do with.
//
// What is NOT copied is the authentication, which is the part that must not
// drift: the header name is a constant of the generated proto, the secret is the
// same W2PP_CONTROL_TOKEN, and the address is the same W2PP_TMSERVER_CONTROL.
// There is no new, weaker channel here — only fewer methods.
//
// EVERY CALL IN THIS PACKAGE IS A COURTESY, and that shapes how failures read.
// By the time the web-api calls, the payment is already written in the database:
// the buyer owns the item and the seller's slot is already marked. The game
// drains both at the next login on its own. Calling merely spares the person the
// wait. So a failure here is a WARNING, never an error, and never rolls anything
// back.
package jogo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	gamev1 "github.com/jeanluca/w2pp-openwyd/api/game/v1"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/mercado"
)

// tempoDeChamada bounds a call. The control API answers from inside the game
// loop, which drains its queue first, so this guards against a server that is
// gone rather than one that is busy.
const tempoDeChamada = 8 * time.Second

var (
	// ErrRecusado means the game server rejected the token: the two sides hold
	// different secrets.
	//
	// It is told apart from the others ON PURPOSE even though the caller only
	// logs. Every call failing this way looks exactly like a server that is
	// merely offline, and the difference is that the offline one fixes itself
	// while this one never does — somebody has to set the variable. The log line
	// is the only place that can say which it is.
	ErrRecusado = errors.New("jogo: o servidor de jogo recusou o nosso token")
	// ErrForaDoAr means the game server could not be reached.
	ErrForaDoAr = errors.New("jogo: o servidor de jogo nao respondeu")
)

// Cliente calls the game server's control API.
type Cliente struct {
	api   gamev1.GameControlServiceClient
	token string
}

// New wraps a connection. The token must match the game server's.
func New(conn grpc.ClientConnInterface, token string) *Cliente {
	return &Cliente{api: gamev1.NewGameControlServiceClient(conn), token: token}
}

// ctx attaches the token and the deadline.
func (c *Cliente) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	md := metadata.Pairs(gamev1.TokenHeader, c.token)
	return context.WithTimeout(metadata.NewOutgoingContext(parent, md), tempoDeChamada)
}

func traduz(err error, oque string) error {
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.Unauthenticated:
		return fmt.Errorf("%s: %w", oque, ErrRecusado)
	case codes.Unavailable, codes.DeadlineExceeded:
		return fmt.Errorf("%s: %w", oque, ErrForaDoAr)
	default:
		return fmt.Errorf("jogo: %s: %w", oque, err)
	}
}

// Entrega asks the game to drain the account's pending grants now.
//
// Devolve entregou=false quando a conta NAO esta conectada, e isso nao e erro:
// os itens ficam guardados e o proximo login os entrega. Quem chama trata os
// dois casos igual, e o valor existe so para o log poder dizer o que houve.
func (c *Cliente) Entrega(parent context.Context, conta string) (entregou bool, err error) {
	ctx, cancel := c.ctx(parent)
	defer cancel()
	resp, err := c.api.DeliverNow(ctx, &gamev1.DeliverNowRequest{AccountName: conta})
	if err != nil {
		return false, traduz(err, "entrega imediata")
	}
	return resp.GetFound(), nil
}

// LiberaVenda asks the game to remove the sold slot of a seller who is online.
//
// Mesma leitura do Entrega: encontrou=false significa vendedor desconectado, o
// item continua inerte, e o proximo login dele limpa a marca.
func (c *Cliente) LiberaVenda(parent context.Context, conta string) (encontrou bool, err error) {
	ctx, cancel := c.ctx(parent)
	defer cancel()
	resp, err := c.api.SettleRmtSaleNow(ctx, &gamev1.SettleRmtSaleNowRequest{AccountName: conta})
	if err != nil {
		return false, traduz(err, "liberar a venda")
	}
	return resp.GetFound(), nil
}

// ListarMercado pede ao jogo as prateleiras das barracas abertas.
//
// É a única leitura possível delas: a barraca vive na memória do laço, na sessão do
// vendedor, e nunca é persistida. Quem quiser a vitrine pergunta ao jogo ou não tem.
func (c *Cliente) ListarMercado(parent context.Context) ([]mercado.Bruta, error) {
	ctx, cancel := c.ctx(parent)
	defer cancel()
	resp, err := c.api.ListMarket(ctx, &gamev1.ListMarketRequest{})
	if err != nil {
		return nil, traduz(err, "listar o mercado")
	}
	fora := make([]mercado.Bruta, 0, len(resp.GetOffers()))
	for _, o := range resp.GetOffers() {
		fora = append(fora, mercado.Bruta{
			ContaVendedor: o.GetSellerAccountId(),
			CargoPos:      o.GetCargoPos(),
			Personagem:    o.GetSellerCharacter(),
			Indice:        o.GetItemIndex(),
			Refino:        o.GetRefine(),
			Qtd:           o.GetAmount(),
			Moeda:         o.GetCurrency(),
			Preco:         o.GetPrice(),
			Cidade:        o.GetVillage(),
			AbertaHa:      time.Duration(o.GetOpenForSeconds()) * time.Second,
		})
	}
	return fora, nil
}
