package control

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gamev1 "github.com/jeanluca/w2pp-openwyd/api/game/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// SetAplicadorDePasse liga a troca de moldura em jogo. Opcional, como o
// SetBlockRunner: sem ele a chamada responde que não está ligada, em vez de
// responder que trocou e não ter trocado nada.
func (s *Server) SetAplicadorDePasse(a AplicadorDePasse) { s.passe = a }

// nivelMaxDoPasse é o que o cliente sabe desenhar.
const nivelMaxDoPasse = 4

// SetPassLevel troca a moldura de quem está jogando e a redesenha para quem o vê.
//
// ELA NÃO ESCREVE NO BANCO, e essa divisão é o desenho: o nível mora na conta, quem
// o grava é o painel, e isto é a cortesia que poupa a pessoa de relogar — igual ao
// DeliverNow. Se esta chamada falhar, nada se perde: o nível gravado vale no próximo
// login.
//
// found=false é a conta desconectada, e NÃO é erro: é o estado normal de quase toda
// conta em quase todo momento. Quem chama trata os dois casos igual e o valor existe
// para a tela poder dizer o que houve.
func (s *Server) SetPassLevel(ctx context.Context, req *gamev1.SetPassLevelRequest,
) (*gamev1.SetPassLevelResponse, error) {
	if s.passe == nil {
		return nil, status.Error(codes.Unavailable, "a troca de passe em jogo nao esta ligada neste servidor")
	}
	nome := strings.TrimSpace(req.GetAccountName())
	if nome == "" {
		return nil, status.Error(codes.InvalidArgument, "account name is required")
	}
	// FORA DA FAIXA É RECUSA, e não um número preso em silêncio. O byte vai direto
	// para o pacote; um valor que o cliente não sabe desenhar é bug de quem chamou, e
	// prender caladamente esconderia esse bug atrás de uma moldura que parece certa.
	nivel := req.GetLevel()
	if nivel < 0 || nivel > nivelMaxDoPasse {
		return nil, status.Errorf(codes.InvalidArgument,
			"nivel %d fora de 0..%d", nivel, nivelMaxDoPasse)
	}

	// A conta é achada pelo NOME, como no resto deste arquivo, e o id sai da sessão:
	// o laço não fala com o banco, e mandar o id de fora abriria um caminho para
	// mexer numa conta que não é a que o painel pensa que é.
	type alvo struct {
		personagem string
		achou      bool
	}
	out, err := noLoop(ctx, s.world, func(w *world.World) alvo {
		var a alvo
		var accountID int64
		w.ForEachSession(func(sess *world.Session, _ *world.Entity) {
			if accountID == 0 && strings.EqualFold(strings.TrimSpace(sess.AccountName), strings.TrimSpace(nome)) {
				accountID = sess.AccountID
			}
		})
		if accountID == 0 {
			return a
		}
		a.personagem, a.achou = s.passe(w, accountID, uint8(nivel))
		return a
	})
	if err != nil {
		return nil, err
	}
	if out.achou {
		s.log.Info("passe: moldura trocada em jogo",
			"conta", nome, "nivel", nivel, "personagem", out.personagem)
	}
	return &gamev1.SetPassLevelResponse{Found: out.achou, CharacterName: out.personagem}, nil
}
