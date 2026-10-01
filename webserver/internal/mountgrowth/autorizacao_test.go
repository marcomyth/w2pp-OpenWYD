package mountgrowth

import (
	"context"
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/painelator"
)

// cargoFalso responde o cargo de uma conta de jogo.
type cargoFalso struct {
	papeis map[int64]string
}

func (c cargoFalso) AccountRole(_ context.Context, id int64) (string, error) {
	p, ok := c.papeis[id]
	if !ok {
		return "", store.ErrNotFound
	}
	return p, nil
}

// lojaVazia satisfaz a interface Store sem fazer nada: estes testes param na
// autorização e nunca chegam a escrever.
type lojaVazia struct{}

func (lojaVazia) ListMountGrowthRates(context.Context) ([]domain.MountGrowthRate, error) {
	return nil, nil
}

func (lojaVazia) SetMountGrowthCurve(context.Context, int16, []int16, domain.Ator, string) error {
	return nil
}
func (lojaVazia) ClearMountGrowthCurve(context.Context, int16, domain.Ator) error { return nil }
func (lojaVazia) ListMountAbsorb(context.Context) ([]domain.MountAbsorb, error)   { return nil, nil }
func (lojaVazia) SetMountAbsorb(context.Context, int16, int16, int16, domain.Ator, string) error {
	return nil
}
func (lojaVazia) ClearMountAbsorb(context.Context, int16, domain.Ator) error  { return nil }
func (lojaVazia) ListMountBonus(context.Context) ([]domain.MountBonus, error) { return nil, nil }
func (lojaVazia) SetMountBonus(context.Context, domain.MountBonus, domain.Ator, string) error {
	return nil
}
func (lojaVazia) ClearMountBonus(context.Context, int16, domain.Ator) error { return nil }
func (lojaVazia) MountConfigVersion(context.Context) (int64, error)         { return 0, nil }

// TestSemConferidorDeCargoORecusaTudo é o teste da FALHA FECHADA.
//
// A montaria não conferia cargo nenhum, e o conserto foi um conferidor que precisa ser
// LIGADO por quem monta o serviço. A metade que importa é esta: se ninguém ligar, o
// serviço recusa tudo. Um serviço que recusa é um problema que alguém percebe em
// minutos; um serviço que aceita todo mundo em silêncio é o defeito que este arquivo
// existe para consertar, e ele voltaria sem ninguém ver.
func TestSemConferidorDeCargoORecusaTudo(t *testing.T) {
	t.Parallel()
	s := New(lojaVazia{})
	// De propósito SEM ComCargos.
	if err := s.Set(context.Background(), 7, "mod", 1, []int16{10}); !errors.Is(err, ErrSemPermissao) {
		t.Fatalf("Set sem conferidor = %v, queria ErrSemPermissao", err)
	}
	if err := s.ClearAbsorb(context.Background(), 7, 1); !errors.Is(err, ErrSemPermissao) {
		t.Errorf("ClearAbsorb sem conferidor = %v, queria ErrSemPermissao", err)
	}
}

// TestAReguaDeCargoNaMontaria: a mesma dos outros serviços.
func TestAReguaDeCargoNaMontaria(t *testing.T) {
	t.Parallel()
	s := New(lojaVazia{})
	s.ComCargos(cargoFalso{papeis: map[int64]string{
		1: "admin",
		2: "moderator",
		3: "player",
	}})
	ctx := context.Background()

	if err := s.Set(ctx, 1, "admin", 1, []int16{10}); err != nil {
		t.Errorf("admin recusado: %v", err)
	}
	if err := s.Set(ctx, 2, "mod", 1, []int16{10}); err != nil {
		t.Errorf("moderador recusado: %v", err)
	}
	if err := s.Set(ctx, 3, "jogador", 1, []int16{10}); !errors.Is(err, ErrSemPermissao) {
		t.Errorf("JOGADOR foi aceito: %v", err)
	}
	// CONTA QUE NÃO EXISTE RECEBE A MESMA RESPOSTA de uma que existe e não pode: quem
	// pergunta não descobre contas.
	if err := s.Set(ctx, 999, "ninguem", 1, []int16{10}); !errors.Is(err, ErrSemPermissao) {
		t.Errorf("conta inexistente = %v, queria a MESMA recusa de quem nao pode", err)
	}
}

// TestOUsuarioDoPainelEditaMontaria: o caminho novo, sem conta de jogo.
func TestOUsuarioDoPainelEditaMontaria(t *testing.T) {
	t.Parallel()
	s := New(lojaVazia{})
	s.ComCargos(cargoFalso{papeis: map[int64]string{}})

	// moderatorID zero e um ator do painel no contexto: é como a chamada chega do
	// paineladm quando quem está logado é um usuário do painel.
	ctx := painelator.NoContexto(context.Background(),
		painelator.Ator{ID: 42, Login: "hanna", Papel: "admin"})
	if err := s.Set(ctx, 0, "hanna", 1, []int16{10}); err != nil {
		t.Errorf("usuario do painel admin recusado: %v", err)
	}

	// Papel que o painel não emite não passa.
	ctx = painelator.NoContexto(context.Background(),
		painelator.Ator{ID: 43, Login: "quem", Papel: "visitante"})
	if err := s.Set(ctx, 0, "quem", 1, []int16{10}); !errors.Is(err, ErrSemPermissao) {
		t.Errorf("papel invalido foi aceito: %v", err)
	}

	// Sem conta e sem painel: recusa.
	if err := s.Set(context.Background(), 0, "", 1, []int16{10}); !errors.Is(err, ErrSemPermissao) {
		t.Errorf("chamada sem ator nenhum foi aceita: %v", err)
	}
}

// TestOsDoisAtoresJuntosSaoRecusados: conta de jogo E usuário do painel na mesma
// chamada não é empate a desempatar.
//
// PREFERIR UM SERIA DESCARTAR O OUTRO EM SILÊNCIO, e a linha de auditoria sairia
// dizendo que uma pessoa fez o que duas informações reivindicam. Numa tabela cuja única
// razão de existir é dizer QUEM fez, "eu escolhi um dos dois" é a pior resposta.
func TestOsDoisAtoresJuntosSaoRecusados(t *testing.T) {
	t.Parallel()
	s := New(lojaVazia{})
	s.ComCargos(cargoFalso{papeis: map[int64]string{1: "admin"}})
	ctx := painelator.NoContexto(context.Background(),
		painelator.Ator{ID: 42, Login: "hanna", Papel: "admin"})

	// Os dois podem, cada um por si. Juntos, não.
	if err := s.Set(ctx, 1, "admin", 1, []int16{10}); !errors.Is(err, ErrSemPermissao) {
		t.Errorf("conta de jogo E usuario do painel juntos foram aceitos: %v", err)
	}
}
