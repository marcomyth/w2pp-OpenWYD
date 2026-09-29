package mountgrowth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/painelator"
)

// A MONTARIA NÃO CONFERIA CARGO NENHUM, e este arquivo é o conserto.
//
// MEDIDO, NÃO SUPOSTO: nem `Service` nem `grpcsrv/mountgrowthadmin.go` liam o cargo de
// quem pedia. Curva de crescimento, absorção e bônus eram as ÚNICAS escritas de
// administração sem essa conferência — NPC, monstro, item base, recompensa diária,
// loja de doação e evento de mundo todas a fazem.
//
// O TAMANHO DO FURO, dito com precisão para ninguém entender a mais nem a menos: só a
// chave do PAINEL abre estes serviços (authz.go:148; a chave do site é recusada), e
// essa chave mora no servidor do paineladm, onde hoje só a staff chega. Então era
// DEFESA QUE FALTAVA, e não uma porta aberta na internet.
//
// MAS ELA PASSARIA A IMPORTAR HOJE. Até agora quem segurava o usuário do painel era a
// lista de somente-leitura, que recusava toda escrita dele. Esta entrega remove essa
// lista. Sem o conferidor aqui, qualquer usuário do painel — inclusive um moderador
// que só deveria olhar — passaria a mexer em montaria. Tirar a tranca de fora sem pôr
// a de dentro é trocar uma defesa por nenhuma.

// ErrSemPermissao é a recusa por cargo.
//
// UM ERRO SENTINELA, E NÃO UM TIPO Result como nos outros serviços, porque os métodos
// daqui devolvem `error` puro e a camada gRPC já os traduz. Introduzir um Result só
// para isto mudaria a assinatura de nove métodos e o mapeamento inteiro — muito
// barulho para a mesma resposta. O que importa é que a camada gRPC devolva
// PermissionDenied e não Internal, e isso está em grpcsrv/mountgrowthadmin.go.
var ErrSemPermissao = errors.New("mountgrowth: sem permissao para editar montaria")

// LeitorDeCargo é a conta de jogo conferida no banco. Satisfeito por *store.Store.
//
// SEPARADO DA INTERFACE Store por escolha: Store é o que este serviço já precisava, e
// a conferência de cargo é uma dependência nova, de outra natureza. Quem monta o
// serviço decide se liga ou não — e main.go liga.
type LeitorDeCargo interface {
	AccountRole(ctx context.Context, id int64) (string, error)
}

// ComCargos liga a conferência de cargo. Sem ela o serviço RECUSA toda escrita.
//
// FALHA FECHADA, e é a metade que importa: se alguém montar este serviço amanhã e
// esquecer de ligar o conferidor, o resultado é "ninguém edita montaria", que alguém
// percebe em minutos. O contrário — montar sem conferidor e deixar todo mundo editar —
// é o defeito que este arquivo existe para consertar, e ele voltaria em silêncio.
func (s *Service) ComCargos(l LeitorDeCargo) { s.cargos = l }

// autorizaEscrita diz se PODE e, junto, QUEM É.
//
// A MESMA RÉGUA DOS OUTROS SERVIÇOS, de propósito: conta de jogo com cargo moderator
// ou admin, ou usuário do painel em exercício. Uma régua diferente aqui seria uma
// segunda regra para alguém descobrir no pior dia.
//
// A ORDEM DE PREFERÊNCIA É EXPLÍCITA: conta de jogo primeiro, usuário do painel
// depois. Se as duas informações chegarem juntas, esta ordem decide, em vez de a ação
// ser atribuída a quem for lido primeiro.
func (s *Service) autorizaEscrita(ctx context.Context, moderatorID int64) (domain.Ator, error) {
	if s.cargos == nil {
		return domain.Ator{}, ErrSemPermissao
	}
	if moderatorID > 0 {
		papel, err := s.cargos.AccountRole(ctx, moderatorID)
		if errors.Is(err, store.ErrNotFound) {
			// NÃO DIZ QUE A CONTA NÃO EXISTE. A resposta é a mesma de "existe e não
			// pode", como nos outros serviços: quem pergunta não descobre contas.
			return domain.Ator{}, ErrSemPermissao
		}
		if err != nil {
			return domain.Ator{}, fmt.Errorf("mountgrowth: lendo o cargo de %d: %w", moderatorID, err)
		}
		if papel != "moderator" && papel != "admin" {
			return domain.Ator{}, ErrSemPermissao
		}
		return domain.AtorDaConta(moderatorID), nil
	}
	// SEM CONTA DE JOGO, PODE SER O PAINEL. O ator já veio conferido contra o banco
	// pelo interceptador, nesta mesma chamada: existe e está ativo, ou nem chegou aqui.
	if a, doPainel := painelator.Do(ctx); doPainel {
		if !painelator.PapelValido(a.Papel) {
			return domain.Ator{}, ErrSemPermissao
		}
		return domain.AtorDoPainel(a.ID), nil
	}
	return domain.Ator{}, ErrSemPermissao
}
