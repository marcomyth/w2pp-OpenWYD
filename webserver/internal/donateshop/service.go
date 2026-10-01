// Package donateshop holds the web platform's donate web-shop logic (issue #34):
// the moderator CRUD over the cold donate_shop_item catalog + the manual donate
// credit path, and the player-facing vitrine + purchase. Admin operations are
// gated on account.role (mirroring npcadmin); the player surface trusts the
// account_id the BFF supplies from its httpOnly session (like CharacterWebService).
// It never touches live game state — a purchase only enqueues a delivery_queue
// grant that the tmServer drains inside its single-owner loop.
package donateshop

import (
	"context"
	"errors"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/painelator"
)

// Store is the persistence surface the service needs (satisfied by *store.Store).
type Store interface {
	AccountRole(ctx context.Context, id int64) (string, error)
	ListDonateShopItems(ctx context.Context) ([]domain.DonateShopItem, error)
	ListEnabledDonateShopItems(ctx context.Context) ([]domain.DonateShopItem, error)
	UpsertDonateShopItem(ctx context.Context, d domain.DonateShopItem, ator domain.Ator) (int64, error)
	SetDonateShopItemEnabled(ctx context.Context, id int64, enabled bool, ator domain.Ator) error
	DeleteDonateShopItem(ctx context.Context, id int64, ator domain.Ator) error
	CreditDonateBalance(ctx context.Context, accountID int64, amount int32, ator domain.Ator, reason string) (int32, error)
	DonateBalance(ctx context.Context, accountID int64) (int32, error)
	BuyDonateItem(ctx context.Context, accountID, shopItemID int64) (int32, error)
}

// Result is the business outcome of an admin operation, mirroring npcadmin.Result.
// Only infra failures are returned as errors; these ride in the response body.
type Result int

const (
	// OK means the operation succeeded.
	OK Result = iota
	// Forbidden means the caller is not a moderator/admin.
	Forbidden
	// Invalid means the request failed validation.
	Invalid
	// NotFound means the target offer/account does not exist.
	NotFound
)

// BuyOutcome is the result of a player purchase.
type BuyOutcome int

const (
	// BuyOK means the balance was debited and the item queued for delivery.
	BuyOK BuyOutcome = iota
	// BuyInsufficient means the account's balance is below the price.
	BuyInsufficient
	// BuyNotFound means the offer or account does not exist.
	BuyNotFound
	// BuyDisabled means the offer exists but is not on sale.
	BuyDisabled
)

// Service implements the donate-shop operations.
type Service struct {
	store Store
}

// New builds the service over the given store.
func New(s Store) *Service { return &Service{store: s} }

// --- moderator surface ---

// List returns every offer (enabled or not), after authorizing the caller.
func (s *Service) List(ctx context.Context, moderatorID int64) (Result, []domain.DonateShopItem, error) {
	if r, err := s.authorize(ctx, moderatorID); r != OK || err != nil {
		return r, nil, err
	}
	items, err := s.store.ListDonateShopItems(ctx)
	if err != nil {
		return Invalid, nil, fmt.Errorf("donateshop: list: %w", err)
	}
	return OK, items, nil
}

// Upsert creates (d.ID == 0) or updates an offer after validating it.
func (s *Service) Upsert(ctx context.Context, moderatorID int64, d domain.DonateShopItem) (Result, int64, error) {
	ator, r, err := s.autorizaEscrita(ctx, moderatorID)
	if r != OK || err != nil {
		return r, 0, err
	}
	if d.ItemIndex <= 0 || d.Price <= 0 || d.ExpiresDays < 0 {
		return Invalid, 0, nil
	}
	id, err := s.store.UpsertDonateShopItem(ctx, d, ator)
	if errors.Is(err, store.ErrNotFound) {
		return NotFound, 0, nil
	}
	if err != nil {
		return Invalid, 0, fmt.Errorf("donateshop: upsert %d: %w", d.ID, err)
	}
	return OK, id, nil
}

// SetEnabled toggles whether an offer is on sale.
func (s *Service) SetEnabled(ctx context.Context, moderatorID, itemID int64, enabled bool) (Result, error) {
	ator, r, err := s.autorizaEscrita(ctx, moderatorID)
	if r != OK || err != nil {
		return r, err
	}
	return classifyWrite(s.store.SetDonateShopItemEnabled(ctx, itemID, enabled, ator), "set enabled")
}

// Delete removes an offer.
func (s *Service) Delete(ctx context.Context, moderatorID, itemID int64) (Result, error) {
	ator, r, err := s.autorizaEscrita(ctx, moderatorID)
	if r != OK || err != nil {
		return r, err
	}
	return classifyWrite(s.store.DeleteDonateShopItem(ctx, itemID, ator), "delete")
}

// CreditBalance adds donate currency to an account's wallet (the manual/admin
// credit path). Returns the new balance on success.
func (s *Service) CreditBalance(ctx context.Context, moderatorID, accountID int64, amount int32, reason string) (Result, int32, error) {
	// ESTA É A ÚNICA ESCRITA DO PAINEL QUE EXIGE ADMIN, e a razão é o que ela faz:
	// pôr saldo de doação numa conta é criar dinheiro. Todas as outras vinte e quatro
	// mudam regra do jogo — preço, drop, recompensa, montaria —, e um moderador erra
	// nelas de um jeito que se desfaz editando de volta. Esta move valor para uma conta
	// e não volta sozinha.
	//
	// A CONFERÊNCIA VEM DEPOIS DO autorizaEscrita e não em vez dele: o autorizaEscrita
	// é quem decide QUEM é o ator (e recusa os dois juntos), e é dele que sai o autor
	// da auditoria. Aqui só se estreita o cargo.
	ator, r, err := s.autorizaEscrita(ctx, moderatorID)
	if r != OK || err != nil {
		return r, 0, err
	}
	if !s.ehAdmin(ctx, moderatorID) {
		return Forbidden, 0, nil
	}
	if accountID <= 0 || amount <= 0 {
		return Invalid, 0, nil
	}
	newBal, err := s.store.CreditDonateBalance(ctx, accountID, amount, ator, reason)
	if errors.Is(err, store.ErrNotFound) {
		return NotFound, 0, nil
	}
	if err != nil {
		return Invalid, 0, fmt.Errorf("donateshop: credit a=%d: %w", accountID, err)
	}
	return OK, newBal, nil
}

// --- player surface ---

// Vitrine returns the enabled offers for the shop front-end.
func (s *Service) Vitrine(ctx context.Context) ([]domain.DonateShopItem, error) {
	items, err := s.store.ListEnabledDonateShopItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("donateshop: vitrine: %w", err)
	}
	return items, nil
}

// Balance returns the account's donate balance. A missing account reports 0
// (the caller decides how to surface it); infra failures surface as errors.
func (s *Service) Balance(ctx context.Context, accountID int64) (int32, error) {
	bal, err := s.store.DonateBalance(ctx, accountID)
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("donateshop: balance a=%d: %w", accountID, err)
	}
	return bal, nil
}

// Buy purchases an offer for the account, returning the outcome and (on success)
// the new balance.
func (s *Service) Buy(ctx context.Context, accountID, shopItemID int64) (BuyOutcome, int32, error) {
	if accountID <= 0 || shopItemID <= 0 {
		return BuyNotFound, 0, nil
	}
	newBal, err := s.store.BuyDonateItem(ctx, accountID, shopItemID)
	switch {
	case err == nil:
		return BuyOK, newBal, nil
	case errors.Is(err, store.ErrInsufficientDonate):
		return BuyInsufficient, 0, nil
	case errors.Is(err, store.ErrShopItemDisabled):
		return BuyDisabled, 0, nil
	case errors.Is(err, store.ErrNotFound):
		return BuyNotFound, 0, nil
	default:
		return BuyNotFound, 0, fmt.Errorf("donateshop: buy a=%d item=%d: %w", accountID, shopItemID, err)
	}
}

// --- helpers ---

// authorize checks the caller has a moderator/admin role. A missing account or a
// plain-player role yields Forbidden (never leaks whether the account exists).
func (s *Service) authorize(ctx context.Context, moderatorID int64) (Result, error) {
	if moderatorID <= 0 {
		// SEM CONTA DE JOGO, PODE SER O PAINEL. Desde o #130 a staff entra como
		// usuário do painel, sem conta de jogo, e o painel manda zero aqui — era esta
		// linha que recusava todas as páginas de administração para ela.
		//
		// O ator já veio conferido contra o banco pelo interceptador, nesta mesma
		// chamada: existe e está ativo, ou nem chegou até aqui.
		if pode, doPainel := painelator.AutorizaPeloPainel(ctx); doPainel {
			if !pode {
				return Forbidden, nil
			}
			return OK, nil
		}
		return Forbidden, nil
	}
	role, err := s.store.AccountRole(ctx, moderatorID)
	if errors.Is(err, store.ErrNotFound) {
		return Forbidden, nil
	}
	if err != nil {
		return Invalid, fmt.Errorf("donateshop: role lookup %d: %w", moderatorID, err)
	}
	if role != "moderator" && role != "admin" {
		return Forbidden, nil
	}
	return OK, nil
}

// classifyWrite maps a store write error to a Result: ErrNotFound → NotFound,
// nil → OK, anything else → Invalid (wrapped).
func classifyWrite(err error, op string) (Result, error) {
	switch {
	case err == nil:
		return OK, nil
	case errors.Is(err, store.ErrNotFound):
		return NotFound, nil
	default:
		return Invalid, fmt.Errorf("donateshop: %s: %w", op, err)
	}
}

// autorizaEscrita e o authorize das ESCRITAS: alem de dizer se PODE, diz QUEM E.
//
// EXISTE SEPARADO DO authorize porque as leituras nao precisam de ator, e misturar as
// duas coisas faria cada pagina de consulta carregar um ator que ela nao usa.
//
// AS DUAS JUNTAS SAO RECUSA, e nenhuma delas ganha da outra. Se nao houver nenhuma, o
// ator sai vazio e o internal/store recusa antes de escrever -- ninguem grava
// "conta 0".
func (s *Service) autorizaEscrita(ctx context.Context, moderatorID int64) (domain.Ator, Result, error) {
	r, err := s.authorize(ctx, moderatorID)
	if r != OK || err != nil {
		return domain.Ator{}, r, err
	}
	a, doPainel := painelator.Do(ctx)
	// OS DOIS JUNTOS SAO RECUSA, e nao empate a ser desempatado.
	//
	// Antes eu dava preferencia a conta de jogo. Estava errado: preferir e DESCARTAR
	// o outro em silencio, e a linha de auditoria sairia dizendo que uma pessoa fez o
	// que duas informacoes reivindicam. Numa tabela cuja unica razao de existir e
	// dizer QUEM fez, "eu escolhi um dos dois" e a pior resposta possivel.
	//
	// E nao e so teoria: a chamada so tem os dois se alguem montou um pedido
	// estranho, e um pedido estranho e exatamente o que nao deve virar uma gravacao
	// com autor plausivel.
	if moderatorID > 0 && doPainel {
		return domain.Ator{}, Forbidden, nil
	}
	if moderatorID > 0 {
		return domain.AtorDaConta(moderatorID), OK, nil
	}
	if doPainel {
		return domain.AtorDoPainel(a.ID), OK, nil
	}
	return domain.Ator{}, Forbidden, nil
}

// ehAdmin diz se quem pede tem cargo de admin, nos dois caminhos.
//
// SEPARADO DO autorizaEscrita de propósito: aquele responde "pode administrar?", que é
// a mesma pergunta para as vinte e cinco escritas. Esta responde "pode CRIAR
// DINHEIRO?", que é de uma escrita só. Juntar as duas faria toda escrita carregar a
// pergunta mais restritiva na cabeça de quem lê.
func (s *Service) ehAdmin(ctx context.Context, moderatorID int64) bool {
	if moderatorID > 0 {
		papel, err := s.store.AccountRole(ctx, moderatorID)
		return err == nil && papel == "admin"
	}
	if a, doPainel := painelator.Do(ctx); doPainel {
		return a.Papel == "admin"
	}
	return false
}
