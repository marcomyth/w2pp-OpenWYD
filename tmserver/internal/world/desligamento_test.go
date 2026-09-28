package world

import (
	"context"
	"sync"
	"testing"
	"time"
)

// gravadorLento é o banco dos testes de desligamento: cada save demora
// `demora`, ou fica preso — ignorando o contexto, como um banco que não
// responde — até `libera` fechar, quando `trava` está ligado.
type gravadorLento struct {
	NopPersistence
	demora time.Duration
	trava  bool
	libera chan struct{}

	mu        sync.Mutex
	gravouEm  []time.Time
	soltouAo  []bool
	soltas    []int64
	aoMesmo   int
	maxJuntos int
}

func (g *gravadorLento) SaveOnShutdown(_ context.Context, _ CharacterSave, _, _ int64, soltarPosse bool) error {
	g.mu.Lock()
	g.aoMesmo++
	g.maxJuntos = max(g.maxJuntos, g.aoMesmo)
	g.gravouEm = append(g.gravouEm, time.Now())
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.aoMesmo--
		g.soltouAo = append(g.soltouAo, soltarPosse)
		g.mu.Unlock()
	}()
	if g.trava {
		<-g.libera
		return context.DeadlineExceeded
	}
	time.Sleep(g.demora)
	return nil
}

func (g *gravadorLento) SoltarPosseDaConta(_ context.Context, conta, _ int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.soltas = append(g.soltas, conta)
	return nil
}

// mundoParaDesligar monta um mundo parado com n jogadores em jogo, cada um com
// mochila viva e na sua conta.
func mundoParaDesligar(t *testing.T, cfg Config, p Persistence, n int) *World {
	t.Helper()
	if cfg.GridDim == 0 {
		cfg.GridDim = 16
	}
	w := New(cfg, slogDiscard(), p, nil)
	w.DefineEpocaDoPar(7)
	for i := 1; i <= n; i++ {
		w.sessions[i] = &Session{Conn: i, AccountID: int64(100 + i), Mode: UserPlay, closeCh: make(chan struct{})}
		w.entities[i] = &Entity{ID: i, Mode: MobUser, HP: 100, Level: 50}
	}
	return w
}

// O save sai na hora, e não depois do aviso. Em 27/09/2026 ele esperava os 2 s
// do aviso primeiro, e a plataforma matou o processo no meio dos saves.
func TestDesligamentoGravaAntesDaEsperaDoAviso(t *testing.T) {
	const aviso = 300 * time.Millisecond
	g := &gravadorLento{}
	w := mundoParaDesligar(t, Config{ShutdownGrace: aviso}, g, 1)

	inicio := time.Now()
	w.shutdown(true)
	levou := time.Since(inicio)

	if len(g.gravouEm) != 1 {
		t.Fatalf("gravou %d vezes, queria 1", len(g.gravouEm))
	}
	if atraso := g.gravouEm[0].Sub(inicio); atraso > aviso/3 {
		t.Errorf("o save começou %v depois do sinal; tem de começar antes da espera do aviso (%v)", atraso, aviso)
	}
	// E o aviso ainda ganha o tempo dele antes de as sessões fecharem.
	if levou < aviso {
		t.Errorf("o desligamento levou %v; as sessões fecharam antes dos %v do aviso", levou, aviso)
	}
}

// Sem ninguém avisado, não há o que esperar.
func TestDesligamentoSemAvisoNaoEspera(t *testing.T) {
	g := &gravadorLento{}
	w := mundoParaDesligar(t, Config{ShutdownGrace: time.Minute}, g, 1)
	inicio := time.Now()
	w.shutdown(false)
	if levou := time.Since(inicio); levou > time.Second {
		t.Errorf("levou %v sem ter avisado ninguém", levou)
	}
}

// Seis jogadores não esperam na fila um do outro.
func TestDesligamentoGravaEmParalelo(t *testing.T) {
	const demora = 200 * time.Millisecond
	g := &gravadorLento{demora: demora}
	w := mundoParaDesligar(t, Config{}, g, 6)

	inicio := time.Now()
	w.shutdown(false)
	levou := time.Since(inicio)

	if len(g.gravouEm) != 6 {
		t.Fatalf("gravou %d, queria 6", len(g.gravouEm))
	}
	if levou >= 3*demora {
		t.Errorf("seis saves de %v levaram %v: estão em fila", demora, levou)
	}
	if g.maxJuntos < 2 {
		t.Errorf("no máximo %d save ao mesmo tempo", g.maxJuntos)
	}
	if n := w.SavesFalhados(); n != 0 {
		t.Errorf("SavesFalhados = %d, queria 0", n)
	}
}

// Um banco que não responde não segura o desligamento além do prazo, e a falha
// fica contada — o processo sai dizendo o que não gravou, em vez de morrer calado.
func TestDesligamentoTemPrazo(t *testing.T) {
	g := &gravadorLento{trava: true, libera: make(chan struct{})}
	t.Cleanup(func() { close(g.libera) })
	w := mundoParaDesligar(t, Config{ShutdownSaveDeadline: 100 * time.Millisecond}, g, 2)

	feito := make(chan struct{})
	go func() { w.shutdown(false); close(feito) }()
	select {
	case <-feito:
	case <-time.After(2 * time.Second):
		t.Fatal("o desligamento não respeitou o prazo com o banco travado")
	}
	if n := w.SavesFalhados(); n != 2 {
		t.Errorf("SavesFalhados = %d, queria 2", n)
	}
}

// O save de desligamento solta a posse, como o de logout; e a conta que não
// tinha nada a gravar também é devolvida. Sem isso o servidor novo recusava o
// login por até 90 s.
func TestDesligamentoSoltaAPosse(t *testing.T) {
	g := &gravadorLento{}
	w := mundoParaDesligar(t, Config{}, g, 1)
	// Uma conta na tela de seleção, sem personagem e sem carga.
	w.sessions[5] = &Session{Conn: 5, AccountID: 555, Mode: UserSelChar, closeCh: make(chan struct{})}

	w.shutdown(false)

	if len(g.soltouAo) != 1 || !g.soltouAo[0] {
		t.Errorf("save do jogador em jogo soltou a posse? %v; queria [true]", g.soltouAo)
	}
	if len(g.soltas) != 1 || g.soltas[0] != 555 {
		t.Errorf("posses soltas sem save = %v; queria [555]", g.soltas)
	}
}

// O save periódico dispara no horário da sessão e não antes, e não mexe em quem
// não está em jogo.
func TestSavePeriodico(t *testing.T) {
	g := &gravadorLento{}
	w := mundoParaDesligar(t, Config{}, g, 1)
	w.sessions[2] = &Session{Conn: 2, AccountID: 202, Mode: UserSelChar, closeCh: make(chan struct{})}

	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	gravacoes := func() int {
		g.mu.Lock()
		defer g.mu.Unlock()
		return len(g.gravouEm)
	}

	w.savePeriodicoTick(base) // marca o horário, não grava
	w.savePeriodicoTick(base.Add(IntervaloDoSavePeriodico / 2))
	w.saveWG.Wait()
	if n := gravacoes(); n != 0 {
		t.Fatalf("gravou %d vezes antes da hora", n)
	}

	w.savePeriodicoTick(base.Add(IntervaloDoSavePeriodico))
	w.saveWG.Wait()
	if n := gravacoes(); n != 1 {
		t.Fatalf("depois de um intervalo gravou %d vezes, queria 1", n)
	}

	w.savePeriodicoTick(base.Add(IntervaloDoSavePeriodico + time.Minute))
	w.saveWG.Wait()
	if n := gravacoes(); n != 1 {
		t.Errorf("gravou de novo um minuto depois (%d)", n)
	}

	w.savePeriodicoTick(base.Add(2*IntervaloDoSavePeriodico + time.Second))
	w.saveWG.Wait()
	if n := gravacoes(); n != 2 {
		t.Errorf("depois de dois intervalos gravou %d vezes, queria 2", n)
	}
	// O save periódico não é de saída: a posse fica.
	for _, soltou := range g.soltouAo {
		if soltou {
			t.Error("o save periódico soltou a posse de quem continua em jogo")
		}
	}
}

// Quem sai do jogo recomeça a contagem: o próximo personagem não herda o
// horário do anterior.
func TestSavePeriodicoRecomecaAoSairDoJogo(t *testing.T) {
	g := &gravadorLento{}
	w := mundoParaDesligar(t, Config{}, g, 1)
	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	w.savePeriodicoTick(base)
	if w.sessions[1].proximoSavePeriodico.IsZero() {
		t.Fatal("a sessão em jogo não ganhou horário")
	}
	w.sessions[1].Mode = UserSelChar
	w.savePeriodicoTick(base.Add(time.Second))
	if !w.sessions[1].proximoSavePeriodico.IsZero() {
		t.Error("a sessão fora de jogo manteve o horário do personagem anterior")
	}
}
