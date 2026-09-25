package world

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// TestOPrazoDaPosseCobreTresBatimentos prende os dois números que moram em
// arquivos diferentes.
//
// O intervalo é do mundo (é ele que bate) e o prazo é do store (é a consulta que o
// aplica). Separados, eles podem andar um sem o outro — e se o intervalo passar do
// prazo, o servidor perde as contas dos próprios jogadores sozinho, sem defeito
// nenhum além da aritmética.
func TestOPrazoDaPosseCobreTresBatimentos(t *testing.T) {
	if store.PrazoDaPosse < 3*IntervaloDoBatimento {
		t.Errorf("prazo %v para batimento de %v: cabem menos de três batimentos, e um soluço de rede tira a conta de quem está jogando",
			store.PrazoDaPosse, IntervaloDoBatimento)
	}
}

// posseFake registra o batimento e a soltura, e deixa o teste escolher quais
// contas continuam sendo desta execução.
type posseFake struct {
	NopPersistence
	mu       sync.Mutex
	batidas  [][]int64
	soltas   []int64
	perdidas map[int64]bool
	salvos   int
}

func (p *posseFake) BaterPelasContas(_ context.Context, _ int64, contas []int64) ([]int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.batidas = append(p.batidas, append([]int64(nil), contas...))
	var minhas []int64
	for _, id := range contas {
		if !p.perdidas[id] {
			minhas = append(minhas, id)
		}
	}
	return minhas, nil
}

func (p *posseFake) SoltarPosseDaConta(_ context.Context, accountID, _ int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.soltas = append(p.soltas, accountID)
	return nil
}

func (p *posseFake) SalvarPersonagemComCarga(context.Context, CharacterSave, CargoSave, []int64, []int64, int64, int64, bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.salvos++
	return nil
}

func (p *posseFake) SaveOnShutdown(context.Context, CharacterSave, int64, int64, bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.salvos++
	return nil
}

func (p *posseFake) SaveCargo(context.Context, CargoSave) error { return nil }

func (p *posseFake) leitura() (int, []int64, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.batidas), append([]int64(nil), p.soltas...), p.salvos
}

func esperaAte(t *testing.T, cond func() bool, oQue string) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for time.Now().Before(limite) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("esperei 2s por: %s", oQue)
}

// TestBatimentoRespeitaOIntervalo: o relógio vem de fora justamente para o teste
// não precisar dormir 30 segundos.
func TestBatimentoRespeitaOIntervalo(t *testing.T) {
	pf := &posseFake{}
	w := New(Config{GridDim: 16}, slogDiscard(), pf, nil)
	w.DefineEpocaDoPar(7)
	w.sessions[1] = &Session{Conn: 1, AccountID: 42, Mode: UserPlay}
	w.entities[1] = &Entity{ID: 1, Mode: MobUser, HP: 100}

	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	w.posseTick(base)
	esperaAte(t, func() bool { n, _, _ := pf.leitura(); return n == 1 }, "o primeiro batimento")

	// Cedo demais: não bate de novo.
	w.posseTick(base.Add(IntervaloDoBatimento / 2))
	if n, _, _ := pf.leitura(); n != 1 {
		t.Errorf("bateu %d vezes antes da hora", n)
	}
	// Na hora: bate.
	w.posseTick(base.Add(IntervaloDoBatimento))
	esperaAte(t, func() bool { n, _, _ := pf.leitura(); return n == 2 }, "o segundo batimento")
}

// TestBatimentoParadoNaoBate: o desligamento para o batimento ANTES de esvaziar o
// servidor, senão um batimento atrasado re-carimbaria contas já soltas.
func TestBatimentoParadoNaoBate(t *testing.T) {
	pf := &posseFake{}
	w := New(Config{GridDim: 16}, slogDiscard(), pf, nil)
	w.DefineEpocaDoPar(7)
	w.sessions[1] = &Session{Conn: 1, AccountID: 42, Mode: UserPlay}
	w.entities[1] = &Entity{ID: 1, Mode: MobUser, HP: 100}

	w.PararOBatimento()
	w.posseTick(time.Now())
	time.Sleep(50 * time.Millisecond)
	if n, _, _ := pf.leitura(); n != 0 {
		t.Errorf("bateu %d vezes depois de parado", n)
	}
}

// TestPossePerdidaSalvaEDerruba é o risco que a revisão nomeou: processo vivo, mas
// o banco ficou lento e o prazo venceu. Continuar jogando numa conta que não é mais
// minha é produzir a divergência que a posse existe para impedir.
func TestPossePerdidaSalvaEDerruba(t *testing.T) {
	pf := &posseFake{perdidas: map[int64]bool{42: true}}
	w := New(Config{GridDim: 16}, slogDiscard(), pf, nil)
	w.DefineEpocaDoPar(7)
	// A sessão vem com o canal de fechamento porque ela VAI SER FECHADA: sessão de
	// verdade tem o canal, e uma sem ele só provaria que o teste montou pela metade.
	s := &Session{Conn: 1, AccountID: 42, Mode: UserPlay, closeCh: make(chan struct{})}
	w.sessions[1] = s
	w.entities[1] = &Entity{ID: 1, Mode: MobUser, HP: 100}
	w.SetCargo(42, &CargoState{})

	w.posseTick(time.Now())
	esperaAte(t, func() bool { n, _, _ := pf.leitura(); return n == 1 }, "o batimento")
	// A volta do batimento entra pelo laço; aqui o teste não tem laço rodando,
	// então aplica o que ele aplicaria.
	w.largarContasPerdidas([]int64{42}, nil)

	// A gravação sai fora do laço, então esperar o efeito é a única leitura
	// honesta: conferir na hora dá verde ou vermelho conforme a máquina do dia.
	esperaAte(t, func() bool { _, _, salvos := pf.leitura(); return salvos > 0 },
		"o save da conta perdida antes de derrubar")
	if w.sessions[1] != nil {
		t.Error("a sessão continuou viva numa conta que não é mais desta execução")
	}
}

// TestSairDaSelecaoSoltaAPosse: a conta que logou, ficou na tela de seleção e
// desconectou não tem save de saída. Sem uma soltura própria, ela ficaria presa até
// o prazo vencer — a cada vez.
func TestSairDaSelecaoSoltaAPosse(t *testing.T) {
	pf := &posseFake{}
	w := New(Config{GridDim: 16}, slogDiscard(), pf, nil)
	w.DefineEpocaDoPar(7)
	s := &Session{Conn: 1, AccountID: 42, Mode: UserSelChar}
	w.sessions[1] = s // sem entidade: nunca entrou em jogo

	w.EncerrarSessaoDaConta(s)
	esperaAte(t, func() bool { _, soltas, _ := pf.leitura(); return len(soltas) == 1 }, "a soltura da posse")
	if _, soltas, _ := pf.leitura(); soltas[0] != 42 {
		t.Errorf("soltou a conta %d", soltas[0])
	}
}

// TestSairComPersonagemNaoSoltaDuasVezes: com personagem em jogo, a posse sai
// DENTRO do save do par. Uma soltura solta por cima seria uma corrida contra a
// própria gravação.
func TestSairComPersonagemNaoSoltaDuasVezes(t *testing.T) {
	pf := &posseFake{}
	w := New(Config{GridDim: 16}, slogDiscard(), pf, nil)
	w.DefineEpocaDoPar(7)
	s := &Session{Conn: 1, AccountID: 42, Mode: UserPlay}
	w.sessions[1] = s
	w.entities[1] = &Entity{ID: 1, Mode: MobUser, HP: 100}
	w.SetCargo(42, &CargoState{})

	w.EncerrarSessaoDaConta(s)
	esperaAte(t, func() bool { _, _, salvos := pf.leitura(); return salvos == 1 }, "o save de saída")
	time.Sleep(100 * time.Millisecond)
	if _, soltas, _ := pf.leitura(); len(soltas) != 0 {
		t.Errorf("soltou a posse por fora do save: %v", soltas)
	}
}

// TestOutraSessaoDaContaSeguraAPosse: a posse é da CONTA. Se outra sessão da mesma
// conta ainda está com personagem, soltar aqui arrancaria a conta de quem continua
// jogando.
func TestOutraSessaoDaContaSeguraAPosse(t *testing.T) {
	pf := &posseFake{}
	w := New(Config{GridDim: 16}, slogDiscard(), pf, nil)
	w.DefineEpocaDoPar(7)
	saindo := &Session{Conn: 1, AccountID: 42, Mode: UserSelChar}
	ficando := &Session{Conn: 2, AccountID: 42, Mode: UserPlay}
	w.sessions[1], w.sessions[2] = saindo, ficando
	w.entities[2] = &Entity{ID: 2, Mode: MobUser, HP: 100}

	w.EncerrarSessaoDaConta(saindo)
	time.Sleep(100 * time.Millisecond)
	if _, soltas, _ := pf.leitura(); len(soltas) != 0 {
		t.Errorf("soltou a posse com outra sessão da conta viva: %v", soltas)
	}
}
