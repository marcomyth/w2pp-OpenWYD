package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// AS GUARDAS DA FORÇA ELEMENTAL, COM DOIS JOGADORES DE VERDADE.
//
// Tem de ser no fio. tiqueAlcancaJogador termina num gate de sessão em jogo, e
// sem ele TODA resposta é "não alcança" — um teste sem sessão passa com qualquer
// guarda removida, porque nunca chega nelas. Foi o que aconteceu: cinco
// sabotagens seguidas não quebraram nada no teste unitário.
//
// Aqui cada caso muda UMA coisa a partir de um cenário que alcança, e prova que
// aquela guarda sozinha basta para barrar.
func TestForcaElementalGuardasNoFio(t *testing.T) {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Beast", Class: 2, X: 40, Y: 40,
		HP: 5000, MaxHP: 5000, MP: 30_000, MaxMP: 30_000, Level: 399, Int: 100,
	}
	spells := content.NewSkillData([]content.Spell{
		{Index: 52, ManaSpent: 25, InstanceType: 4, InstanceValue: 220, MaxTarget: 5, Aggressive: 1},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log, Spells: spells, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 64, Now: clock.Load}, log, db, d.Handle)
	w.SetTickHandler(10*time.Millisecond, d.Tick)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	bm := enterWorldAs(t, ln.Addr().String(), "tester")
	defer bm.Close()
	outro := enterWorldAs(t, ln.Addr().String(), "tradeb")
	defer outro.Close()
	esvaziar(bm, outro)

	const casterID, alvoID = 1, 2

	// O cenário que ALCANÇA: os dois em campo aberto, o lançador em modo PK.
	// Sem provar que existe um caso positivo, nenhum caso negativo significa nada.
	noLaco(t, w, func(w *world.World) {
		c, a := w.Entity(casterID), w.Entity(alvoID)
		if c == nil || a == nil {
			t.Fatal("os dois jogadores tinham de estar no mundo")
		}
		c.X, c.Y, a.X, a.Y = 40, 40, 41, 40
		c.PKMode = true
		c.PKPoint, a.PKPoint = 50, 50
		c.Leader, a.Leader = 0, 0
		if !d.tiqueAlcancaJogador(w, c, a) {
			t.Fatal("em campo aberto e em modo PK o tique TEM de alcançar")
		}
	})

	casos := []struct {
		nome  string
		mexer func(caster, alvo *world.Entity)
	}{
		{"sem modo PK", func(c, _ *world.Entity) { c.PKMode = false }},
		{"lançador na cidade", func(c, _ *world.Entity) { c.X, c.Y = 2100, 2100 }},
		{"alvo na cidade", func(_, a *world.Entity) { a.X, a.Y = 2100, 2100 }},
		{"lançador mais caótico", func(c, a *world.Entity) { c.PKPoint, a.PKPoint = 5, 50 }},
		{"mesmo grupo", func(c, a *world.Entity) { c.Leader, a.Leader = 9, 9 }},
		// Barrado por TRÊS caminhos independentes: a varredura já pula o próprio
		// lançador (thunderTargets), a primeira linha de tiqueAlcancaJogador
		// compara os ids, e a guarda de grupo considera todo mundo do próprio
		// grupo. Este caso verifica o resultado, não qual dos três agiu — tirar a
		// comparação de ids sozinha não quebra o teste, e isso é esperado.
		{"contra si mesmo", nil},
	}
	cidadeVale := world.Village(2100, 2100) >= 0
	for _, caso := range casos {
		if !cidadeVale && (caso.nome == "lançador na cidade" || caso.nome == "alvo na cidade") {
			continue
		}
		noLaco(t, w, func(w *world.World) {
			c, a := w.Entity(casterID), w.Entity(alvoID)
			// Repõe o cenário que alcança antes de cada caso.
			c.X, c.Y, a.X, a.Y = 40, 40, 41, 40
			c.PKMode = true
			c.PKPoint, a.PKPoint = 50, 50
			c.Leader, a.Leader = 0, 0
			if caso.mexer == nil { // contra si mesmo
				if d.tiqueAlcancaJogador(w, c, c) {
					t.Errorf("%s: o tique NÃO podia alcançar", caso.nome)
				}
				return
			}
			caso.mexer(c, a)
			if d.tiqueAlcancaJogador(w, c, a) {
				t.Errorf("%s: o tique NÃO podia alcançar", caso.nome)
			}
		})
	}

	// O alvo sai de jogo (volta ao char-select) e o tique para de alcançá-lo na
	// hora, mesmo com tudo o mais em ordem.
	noLaco(t, w, func(w *world.World) {
		c, a := w.Entity(casterID), w.Entity(alvoID)
		c.X, c.Y, a.X, a.Y = 40, 40, 41, 40
		c.PKMode = true
		s := w.Session(alvoID)
		anterior := s.Mode
		s.Mode = world.UserSelChar
		defer func() { s.Mode = anterior }()
		if d.tiqueAlcancaJogador(w, c, a) {
			t.Error("alvo fora de jogo: o tique NÃO podia alcançar")
		}
	})
}
