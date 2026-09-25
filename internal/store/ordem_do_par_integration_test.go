//go:build integration

// Testes de integração da ordem das gravações do par.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"errors"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// TestParVelhoNaoApagaOParNovo é o defeito que a ordem existe para impedir.
//
// As gravações saem em goroutines independentes e podem chegar fora de ordem. Um
// par NOVO marca a entrega como feita e grava o item na carga; um par VELHO chega
// depois e regrava a carga SEM o item, com a linha da caixa postal já em
// 'delivered'. É item comprado com dinheiro que some, e não há como reentregar.
func TestParVelhoNaoApagaOParNovo(t *testing.T) {
	s, ctx := freshStore(t)
	const item = int16(4321)
	conta := contaComPersonagem(ctx, t, s, "ordem_par", 0, 0)

	// O par NOVO chega primeiro: o item está na carga.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("ordem_par_p", 0, nil),
		500, []domain.Item{{Slot: 0, Index: item}}, nil, nil, 1, 20); err != nil {
		t.Fatalf("o par novo: %v", err)
	}

	// O VELHO chega atrasado, com a carga como era antes.
	err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("ordem_par_p", 999, nil),
		0, nil, nil, nil, 1, 10)
	if !errors.Is(err, ErrParVelho) {
		t.Fatalf("erro = %v, queria ErrParVelho", err)
	}

	p, c := leOuro(ctx, t, s, conta)
	if p != 0 || c != 500 {
		t.Errorf("o par velho passou por cima: personagem %d, carga %d; queria 0 e 500", p, c)
	}
	if n := contaItens(ctx, t, s, conta, item); n != 1 {
		t.Errorf("o item existe %d vezes, queria 1 — o par velho apagou o que o novo gravou", n)
	}
}

// TestEpocaNovaPassaPorCima: um processo novo escreve por cima de tudo o que o
// anterior deixou, e é o certo — as gravações em voo do processo morto não existem
// mais. Sem isto, o contador zerado do reinício ficaria abaixo do que o banco
// guardou e NENHUMA gravação passaria mais: perda total, calada.
func TestEpocaNovaPassaPorCima(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComPersonagem(ctx, t, s, "ordem_epoca", 0, 0)

	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("ordem_epoca_p", 0, nil), 500, nil, nil, nil, 5, 900); err != nil {
		t.Fatalf("a época velha: %v", err)
	}
	// Época maior, número MENOR: tem de passar.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("ordem_epoca_p", 0, nil), 700, nil, nil, nil, 6, 1); err != nil {
		t.Fatalf("a época nova foi recusada: %v", err)
	}
	if _, c := leOuro(ctx, t, s, conta); c != 700 {
		t.Errorf("carga = %d, queria 700 — a época nova tem de escrever por cima", c)
	}
}

// TestSemEpocaNaoConfere: época zero é o servidor sem banco de ordem (ou um
// caminho que não numera). A guarda fica desligada em vez de recusar tudo.
func TestSemEpocaNaoConfere(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComPersonagem(ctx, t, s, "ordem_sem_epoca", 0, 0)

	for i, ouro := range []int32{300, 100} {
		if err := s.SalvarPersonagemComCarga(ctx, conta,
			personagemDoPar("ordem_sem_epoca_p", 0, nil), ouro, nil, nil, nil, 0, 0); err != nil {
			t.Fatalf("gravação %d: %v", i, err)
		}
	}
	if _, c := leOuro(ctx, t, s, conta); c != 100 {
		t.Errorf("carga = %d, queria 100: sem época a última gravação vale", c)
	}
}

// TestEpocaDoBancoSobe: cada boot pega um número maior, que é o que torna a época
// confiável sem depender do relógio.
func TestEpocaDoBancoSobe(t *testing.T) {
	s, ctx := freshStore(t)
	primeira, err := s.NovaEpocaDePar(ctx)
	if err != nil {
		t.Fatalf("NovaEpocaDePar: %v", err)
	}
	segunda, err := s.NovaEpocaDePar(ctx)
	if err != nil {
		t.Fatalf("NovaEpocaDePar: %v", err)
	}
	if segunda <= primeira {
		t.Errorf("épocas %d e %d: a segunda tem de ser maior", primeira, segunda)
	}
}
