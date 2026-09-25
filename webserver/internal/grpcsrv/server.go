// Package grpcsrv implements the web-api's gRPC AccountWebService (api/web/v1)
// over the account service. It is the edge the Next.js BFF calls server-side
// over gRPC+mTLS (web-platform-plan.md); the browser never reaches here.
package grpcsrv

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/account"
)

// Accounts is the account-logic surface the server depends on (satisfied by
// *account.Service). Kept as an interface so the server is unit-testable.
type Accounts interface {
	Create(ctx context.Context, name, password, email string) (account.CreateResult, int64, error)
	Verify(ctx context.Context, name, password string) (ok bool, accountID int64, blocked bool, role, discordID string, err error)
	// VincularDiscord guarda o Discord da pessoa depois do OAuth do site.
	VincularDiscord(ctx context.Context, accountID int64, discordID string) error
}

// Server implements webv1.AccountWebServiceServer.
type Server struct {
	webv1.UnimplementedAccountWebServiceServer
	accounts Accounts
}

// New builds the AccountWebService over the given account logic.
func New(a Accounts) *Server { return &Server{accounts: a} }

// CreateAccount registers a new account. Business outcomes (name taken, invalid
// input) ride in the response enum; only infra failures become gRPC errors.
func (s *Server) CreateAccount(ctx context.Context, req *webv1.CreateAccountRequest) (*webv1.CreateAccountResponse, error) {
	res, id, err := s.accounts.Create(ctx, req.GetName(), req.GetPassword(), req.GetEmail())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create account: %v", err)
	}
	return &webv1.CreateAccountResponse{Result: createResultToProto(res), AccountId: id}, nil
}

// VerifyCredentials validates name + password for the BFF session cookie.
func (s *Server) VerifyCredentials(ctx context.Context, req *webv1.VerifyCredentialsRequest) (*webv1.VerifyCredentialsResponse, error) {
	ok, id, blocked, role, discord, err := s.accounts.Verify(ctx, req.GetName(), req.GetPassword())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "verify credentials: %v", err)
	}
	// O Discord sai na MESMA resposta do papel: a página precisa dos dois no mesmo
	// instante, e uma segunda chamada por um campo seria uma ida a mais em todo login.
	return &webv1.VerifyCredentialsResponse{
		Ok: ok, AccountId: id, Blocked: blocked, Role: role, DiscordId: discord,
	}, nil
}

// SetMyDiscordLink guarda o Discord da conta depois do OAuth do site.
//
// As recusas previstas viajam no enum e NÃO como erro de gRPC, como no resto deste
// serviço: a página precisa saber O QUE dizer à pessoa, e um código de transporte não
// diz. Só falha de infraestrutura vira erro.
//
// TROCAR o Discord de uma conta que já tem OUTRO é recusado, e só a staff desfaz —
// sem isso, quem tomasse uma conta trocaria o vínculo em silêncio e levaria junto o
// cargo que o Discord dá.
func (s *Server) SetMyDiscordLink(ctx context.Context, req *webv1.SetMyDiscordLinkRequest) (*webv1.SetMyDiscordLinkResponse, error) {
	err := s.accounts.VincularDiscord(ctx, req.GetAccountId(), req.GetDiscordId())
	switch {
	case err == nil:
		return &webv1.SetMyDiscordLinkResponse{Result: webv1.DiscordLinkResult_DISCORD_LINK_RESULT_OK}, nil
	case errors.Is(err, store.ErrDiscordInvalido):
		return &webv1.SetMyDiscordLinkResponse{Result: webv1.DiscordLinkResult_DISCORD_LINK_RESULT_INVALID}, nil
	case errors.Is(err, store.ErrDiscordEmOutraConta):
		return &webv1.SetMyDiscordLinkResponse{
			Result: webv1.DiscordLinkResult_DISCORD_LINK_RESULT_ALREADY_LINKED_ELSEWHERE,
		}, nil
	case errors.Is(err, store.ErrContaTemOutroDiscord):
		return &webv1.SetMyDiscordLinkResponse{
			Result: webv1.DiscordLinkResult_DISCORD_LINK_RESULT_ACCOUNT_HAS_ANOTHER,
		}, nil
	case errors.Is(err, store.ErrNotFound):
		// A conta vem da SESSÃO, então isto não é um estado que o site alcance: é
		// conta apagada no meio do caminho. INVALID é o mais honesto que o contrato
		// tem — e o log diz a conta, nunca o Discord.
		return &webv1.SetMyDiscordLinkResponse{Result: webv1.DiscordLinkResult_DISCORD_LINK_RESULT_INVALID}, nil
	}
	// Erro desconhecido NÃO vira recusa: uma recusa inventada mandaria a pessoa
	// procurar problema na conta dela.
	return nil, status.Errorf(codes.Internal, "vincular discord: %v", err)
}

// createResultToProto maps the domain outcome to the wire enum.
func createResultToProto(r account.CreateResult) webv1.CreateResult {
	switch r {
	case account.CreateOK:
		return webv1.CreateResult_CREATE_RESULT_OK
	case account.CreateNameTaken:
		return webv1.CreateResult_CREATE_RESULT_NAME_TAKEN
	case account.CreateFechado:
		// NÃO cai no INVALID: nada no pedido estava errado. Ver o comentário no
		// CREATE_RESULT_CLOSED do proto.
		return webv1.CreateResult_CREATE_RESULT_CLOSED
	default:
		return webv1.CreateResult_CREATE_RESULT_INVALID
	}
}
