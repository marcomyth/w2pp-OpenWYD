package grpcsrv

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// TestCadaRecusaDoDiscordTemSeuResultado.
//
// A página diz coisas diferentes para "esse Discord já é de outra conta", "essa conta
// já tem outro" e "isso não é um Discord". Se as três virassem a mesma, a pessoa
// ficaria tentando de novo sem saber o que corrigir — e uma delas ela não pode
// corrigir sozinha, precisa da staff.
func TestCadaRecusaDoDiscordTemSeuResultado(t *testing.T) {
	casos := []struct {
		nome string
		erro error
		quer webv1.DiscordLinkResult
	}{
		{"vinculou", nil, webv1.DiscordLinkResult_DISCORD_LINK_RESULT_OK},
		{"nao e snowflake", store.ErrDiscordInvalido, webv1.DiscordLinkResult_DISCORD_LINK_RESULT_INVALID},
		{"ja em outra conta", store.ErrDiscordEmOutraConta,
			webv1.DiscordLinkResult_DISCORD_LINK_RESULT_ALREADY_LINKED_ELSEWHERE},
		{"a conta ja tem outro", store.ErrContaTemOutroDiscord,
			webv1.DiscordLinkResult_DISCORD_LINK_RESULT_ACCOUNT_HAS_ANOTHER},
		// Conta apagada no meio do caminho: a conta vem da sessão, então o site não
		// alcança este estado. INVALID é o mais honesto que o contrato tem.
		{"conta que nao existe", store.ErrNotFound, webv1.DiscordLinkResult_DISCORD_LINK_RESULT_INVALID},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &fakeAccounts{discordErr: c.erro}
			s := New(f)
			resp, err := s.SetMyDiscordLink(context.Background(), &webv1.SetMyDiscordLinkRequest{
				AccountId: 42, DiscordId: "123456789012345678",
			})
			if err != nil {
				t.Fatalf("erro de gRPC inesperado: %v", err)
			}
			if resp.GetResult() != c.quer {
				t.Errorf("result = %v, queria %v", resp.GetResult(), c.quer)
			}
			if f.discordConta != 42 || f.discordID != "123456789012345678" {
				t.Errorf("o store recebeu conta %d e id %q", f.discordConta, f.discordID)
			}
		})
	}
}

// TestErroNovoDoDiscordViraInternal: um erro que este handler não conhece NÃO pode
// virar recusa. Uma recusa inventada mandaria a pessoa procurar problema na conta
// dela, e não existe problema nenhum lá.
func TestErroNovoDoDiscordViraInternal(t *testing.T) {
	s := New(&fakeAccounts{discordErr: errors.New("o banco caiu")})
	if _, err := s.SetMyDiscordLink(context.Background(), &webv1.SetMyDiscordLinkRequest{
		AccountId: 42, DiscordId: "123456789012345678",
	}); status.Code(err) != codes.Internal {
		t.Errorf("código = %v, queria Internal", status.Code(err))
	}
}

// TestOVinculoUsaAContaDaSessao: um pedido que escolhe a conta vincula o Discord de
// outra pessoa.
func TestOVinculoUsaAContaDaSessao(t *testing.T) {
	f := &fakeAccounts{}
	s := New(f)
	if _, err := s.SetMyDiscordLink(context.Background(), &webv1.SetMyDiscordLinkRequest{
		AccountId: 777, DiscordId: "999999999999999999",
	}); err != nil {
		t.Fatal(err)
	}
	if f.discordConta != 777 {
		t.Errorf("vinculou na conta %d, queria 777", f.discordConta)
	}
}

// TestODiscordSaiNoVerifyCredentials: a página precisa dele no MESMO instante em que
// precisa do papel, e é por isso que ele viaja na resposta do login em vez de numa
// chamada nova.
func TestODiscordSaiNoVerifyCredentials(t *testing.T) {
	f := &fakeAccounts{verifyOK: true, verifyID: 7, verifyRole: "player",
		verifyDiscord: "123456789012345678"}
	s := New(f)
	resp, err := s.VerifyCredentials(context.Background(), &webv1.VerifyCredentialsRequest{
		Name: "ana", Password: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetDiscordId() != "123456789012345678" {
		t.Errorf("discord_id = %q", resp.GetDiscordId())
	}
	// E o resto da resposta continua o mesmo: acrescentar um campo não pode mexer no
	// que o site já lia.
	if !resp.GetOk() || resp.GetAccountId() != 7 || resp.GetRole() != "player" {
		t.Errorf("a resposta do login mudou: %+v", resp)
	}
}

// TestSemVinculoODiscordVemVazio: quem nunca vinculou recebe string vazia, e não uma
// ausência que a página tenha de adivinhar.
func TestSemVinculoODiscordVemVazio(t *testing.T) {
	s := New(&fakeAccounts{verifyOK: true, verifyID: 7, verifyRole: "player"})
	resp, err := s.VerifyCredentials(context.Background(), &webv1.VerifyCredentialsRequest{
		Name: "ana", Password: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetDiscordId() != "" {
		t.Errorf("discord_id = %q, queria vazio", resp.GetDiscordId())
	}
}
