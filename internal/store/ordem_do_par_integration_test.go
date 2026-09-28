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
		500, []domain.Item{{Slot: 0, Index: item}}, nil, nil, 1, 20, false); err != nil {
		t.Fatalf("o par novo: %v", err)
	}

	// O VELHO chega atrasado, com a carga como era antes.
	err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("ordem_par_p", 999, nil),
		0, nil, nil, nil, 1, 10, false)
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
		personagemDoPar("ordem_epoca_p", 0, nil), 500, nil, nil, nil, 5, 900, false); err != nil {
		t.Fatalf("a época velha: %v", err)
	}
	// Época maior, número MENOR: tem de passar.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("ordem_epoca_p", 0, nil), 700, nil, nil, nil, 6, 1, false); err != nil {
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
			personagemDoPar("ordem_sem_epoca_p", 0, nil), ouro, nil, nil, nil, 0, 0, false); err != nil {
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

// TestDeployComSobreposicaoNaoPerdeOSaveDoVelho é o cenário do deploy, e é o que
// sustenta a época vir do boot.
//
// O container novo sobe e pega uma época maior, mas isso NÃO escreve nada em conta
// nenhuma: a época é comparada contra a que está gravada na LINHA DA CONTA. Então o
// save final do container velho, de um jogador que nunca chegou a logar no novo,
// passa. Perder esse save seria perder tudo desde o último save, a cada deploy, e
// em silêncio.
func TestDeployComSobreposicaoNaoPerdeOSaveDoVelho(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComPersonagem(ctx, t, s, "deploy_sobrepoe", 0, 0)
	const velho, novo = int64(5), int64(6)

	// O container velho vinha gravando esta conta.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("deploy_sobrepoe_p", 100, nil), 0, nil, nil, nil, velho, 40, false); err != nil {
		t.Fatal(err)
	}
	// O novo subiu (época 6) e NÃO tocou nesta conta: o jogador continua no velho.
	// O save de desligamento do velho chega agora.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("deploy_sobrepoe_p", 999, nil), 0, nil, nil, nil, velho, 41, false); err != nil {
		t.Fatalf("o save final do container velho foi recusado: %v", err)
	}
	if p, _ := leOuro(ctx, t, s, conta); p != 999 {
		t.Errorf("ouro = %d, queria 999 — o save do velho tinha de passar", p)
	}
	_ = novo
}

// TestSessaoNovaGanhaDaVelhaAtrasada: depois que a sessão nova gravou, o par
// atrasado da velha é recusado e o que a nova pôs continua lá.
func TestSessaoNovaGanhaDaVelhaAtrasada(t *testing.T) {
	s, ctx := freshStore(t)
	const item = int16(4321)
	conta := contaComPersonagem(ctx, t, s, "sessao_nova", 0, 0)

	// A sessão velha, época 5.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("sessao_nova_p", 10, nil), 0, nil, nil, nil, 5, 100, false); err != nil {
		t.Fatal(err)
	}
	// A nova, época 6, põe o item na carga.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("sessao_nova_p", 20, nil),
		50, []domain.Item{{Slot: 0, Index: item}}, nil, nil, 6, 1, false); err != nil {
		t.Fatal(err)
	}
	// O par atrasado da velha chega depois.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("sessao_nova_p", 10, nil), 0, nil, nil, nil, 5, 101, false); !errors.Is(err, ErrParVelho) {
		t.Fatalf("erro = %v, queria ErrParVelho", err)
	}
	p, c := leOuro(ctx, t, s, conta)
	if p != 20 || c != 50 {
		t.Errorf("personagem %d, carga %d; queria 20 e 50", p, c)
	}
	if n := contaItens(ctx, t, s, conta, item); n != 1 {
		t.Errorf("o item da sessão nova existe %d vezes, queria 1", n)
	}
}

// TestCargaSozinhaNaoCarimbaAOrdem é a regra que mantém o deploy seguro.
//
// O container novo drena uma entrega para uma conta que ELE acha offline, porque o
// personagem está no container velho. Se essa gravação carimbasse a época dele na
// conta, roubaria a conta de quem ainda está jogando, e o save de saída do velho
// seria recusado — em silêncio, que é o pior jeito.
func TestCargaSozinhaNaoCarimbaAOrdem(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComPersonagem(ctx, t, s, "carga_nao_carimba", 0, 0)

	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("carga_nao_carimba_p", 100, nil), 0, nil, nil, nil, 5, 40, false); err != nil {
		t.Fatal(err)
	}
	// A carga sozinha, vinda do container novo.
	if err := s.SaveCargoWithDeliveries(ctx, conta, 777, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	var epoca, seq int64
	if err := s.pool.QueryRow(ctx,
		`SELECT par_epoca, par_seq FROM account WHERE id = $1`, conta).Scan(&epoca, &seq); err != nil {
		t.Fatal(err)
	}
	if epoca != 5 || seq != 40 {
		t.Errorf("a carga sozinha carimbou a ordem: (%d, %d), queria (5, 40)", epoca, seq)
	}
	// E o save do container velho continua passando depois dela.
	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("carga_nao_carimba_p", 999, nil), 0, nil, nil, nil, 5, 41, false); err != nil {
		t.Fatalf("o save do velho foi recusado depois da carga sozinha: %v", err)
	}
}

// TestSavePersonagemSozinhoRespeitaAOrdem: um save velho só do personagem passa por
// cima do par novo do mesmo jeito, então ele confere as MESMAS colunas.
func TestSavePersonagemSozinhoRespeitaAOrdem(t *testing.T) {
	s, ctx := freshStore(t)
	conta := contaComPersonagem(ctx, t, s, "so_personagem", 0, 0)

	if err := s.SalvarPersonagemComCarga(ctx, conta,
		personagemDoPar("so_personagem_p", 500, nil), 0, nil, nil, nil, 7, 30, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SalvarPersonagemOrdenado(ctx, conta,
		personagemDoPar("so_personagem_p", 1, nil), 7, 29, false); !errors.Is(err, ErrParVelho) {
		t.Fatalf("erro = %v, queria ErrParVelho", err)
	}
	if p, _ := leOuro(ctx, t, s, conta); p != 500 {
		t.Errorf("ouro = %d, queria 500 — o save velho de personagem passou por cima", p)
	}
	// E o mais novo passa.
	if err := s.SalvarPersonagemOrdenado(ctx, conta,
		personagemDoPar("so_personagem_p", 900, nil), 7, 31, false); err != nil {
		t.Fatal(err)
	}
	if p, _ := leOuro(ctx, t, s, conta); p != 900 {
		t.Errorf("ouro = %d, queria 900", p)
	}
}
