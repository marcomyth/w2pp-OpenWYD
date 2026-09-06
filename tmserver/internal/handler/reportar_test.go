package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// reportDB is chatDB that also captures what was filed.
type reportDB struct {
	*fakeDB
	mu     sync.Mutex
	feitos []world.PlayerReport
	erro   error
}

func newReportDB() *reportDB { return &reportDB{fakeDB: chatDB()} }

func (r *reportDB) RecordReport(_ context.Context, rep world.PlayerReport) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.erro != nil {
		return r.erro
	}
	r.feitos = append(r.feitos, rep)
	return nil
}

// esperaDenuncias waits briefly for the detached write to land, the way
// fakeDB.lastTrade does: the report is filed off the loop, so asserting
// immediately would race the goroutine rather than the behavior.
func (r *reportDB) esperaDenuncias(t *testing.T, quantas int) []world.PlayerReport {
	t.Helper()
	prazo := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		n := len(r.feitos)
		if n >= quantas || time.Now().After(prazo) {
			out := make([]world.PlayerReport, n)
			copy(out, r.feitos)
			r.mu.Unlock()
			return out
		}
		r.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
}

// startServerReport is startServerClock with a clock the test can move.
//
// The shared helper pins the dispatcher at time.Unix(0,0), which is fine for
// everything that does not measure elapsed time and useless for the report
// cooldown — the only way to test it there would be to sleep a real minute.
func startServerReport(t *testing.T, persist world.Persistence, agora *atomic.Int64) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relogio := &atomic.Uint32{}
	relogio.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(agora.Load(), 0) }})
	w := world.New(world.Config{GridDim: 16, Now: relogio.Load}, log, persist, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	}
}

func TestReportarGuardaOMomento(t *testing.T) {
	// The whole point: staff gets the server's own answer to "what was going on"
	// instead of a screenshot and a story.
	db := newReportDB()
	agora := &atomic.Int64{}
	addr, stop := startServerReport(t, db, agora)
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()

	whisperFrame(t, a, "reportar", "o cara ali esta usando bot")

	feitos := db.esperaDenuncias(t, 1)
	if len(feitos) != 1 {
		t.Fatalf("denúncias = %d, want 1", len(feitos))
	}
	r := feitos[0]
	if r.Text != "o cara ali esta usando bot" {
		t.Errorf("texto = %q", r.Text)
	}
	if r.Character != "Hero" {
		t.Errorf("personagem = %q, want Hero", r.Character)
	}
	// Without the position the report is "somewhere on the map".
	if r.X != 5 || r.Y != 5 {
		t.Errorf("posição = %d,%d, want 5,5", r.X, r.Y)
	}
	if r.Account == "" {
		t.Error("a denúncia não sabe de qual conta veio")
	}
}

func TestReportarAvisaOJogador(t *testing.T) {
	// The player asked for help; the answer must not wait on Postgres. Here the
	// write fails on purpose and the player is told it went in anyway — a failed
	// write is ours to see in the log, not theirs to read on screen.
	db := newReportDB()
	db.erro = errors.New("falha de teste")
	agora := &atomic.Int64{}
	addr, stop := startServerReport(t, db, agora)
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()

	whisperFrame(t, a, "reportar", "o banco vai falhar")
	ty, payload, ok := readMaybe(t, a)
	if !ok || ty != protocol.MsgMessageChat {
		t.Fatalf("got %#x ok=%v, want MessageChat", ty, ok)
	}
	if !strings.Contains(string(payload), "Reportado") {
		t.Errorf("resposta = %q, want confirmando", payload)
	}
}

func TestReportarSemTextoEnsinaOComando(t *testing.T) {
	// A bare /reportar is somebody who does not know the syntax, not a complaint.
	// Filing it would put a blank row at the top of the queue.
	db := newReportDB()
	agora := &atomic.Int64{}
	addr, stop := startServerReport(t, db, agora)
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()

	whisperFrame(t, a, "reportar", "")
	ty, payload, ok := readMaybe(t, a)
	if !ok || ty != protocol.MsgMessageChat {
		t.Fatalf("got %#x ok=%v, want MessageChat", ty, ok)
	}
	if !strings.Contains(string(payload), "Escreva o que houve") {
		t.Errorf("resposta = %q, want ensinando a usar", payload)
	}
	if n := len(db.esperaDenuncias(t, 0)); n != 0 {
		t.Errorf("denúncias = %d, want 0", n)
	}
}

func TestReportarTemEsperaEntreUmEOutro(t *testing.T) {
	// Without this one annoyed player writes hundreds of rows in a minute and
	// buries the queue — the tool for handling grief becomes what is griefed.
	db := newReportDB()
	agora := &atomic.Int64{}
	addr, stop := startServerReport(t, db, agora)
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()

	whisperFrame(t, a, "reportar", "primeira")
	if _, _, ok := readMaybe(t, a); !ok {
		t.Fatal("sem resposta da primeira")
	}
	whisperFrame(t, a, "reportar", "segunda, logo em seguida")
	ty, payload, ok := readMaybe(t, a)
	if !ok || ty != protocol.MsgMessageChat {
		t.Fatalf("got %#x ok=%v, want MessageChat", ty, ok)
	}
	if !strings.Contains(string(payload), "Espere") {
		t.Errorf("resposta = %q, want avisando da espera", payload)
	}
	if n := len(db.esperaDenuncias(t, 2)); n != 1 {
		t.Errorf("denúncias = %d, want 1 — a segunda passou pela espera", n)
	}
}

func TestDepoisDaEsperaReportaDeNovo(t *testing.T) {
	// The gate must not block somebody who genuinely has a second problem.
	db := newReportDB()
	agora := &atomic.Int64{}
	addr, stop := startServerReport(t, db, agora)
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()

	whisperFrame(t, a, "reportar", "primeira")
	if _, _, ok := readMaybe(t, a); !ok {
		t.Fatal("sem resposta da primeira")
	}
	agora.Store(int64(reportEspera/time.Second) + 1)

	whisperFrame(t, a, "reportar", "segunda, bem depois")
	if _, _, ok := readMaybe(t, a); !ok {
		t.Fatal("sem resposta da segunda")
	}
	if n := len(db.esperaDenuncias(t, 2)); n != 2 {
		t.Errorf("denúncias = %d, want 2 — a espera prendeu quem tinha outro problema", n)
	}
}

func TestReportarVeQuemEstaPorPerto(t *testing.T) {
	// The bystander list is what turns "somebody here is botting" into something
	// checkable. Names only — no positions, no accounts.
	db := newReportDB()
	agora := &atomic.Int64{}
	addr, stop := startServerReport(t, db, agora)
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()

	whisperFrame(t, a, "reportar", "esse ai do lado")

	feitos := db.esperaDenuncias(t, 1)
	if len(feitos) != 1 {
		t.Fatalf("denúncias = %d, want 1", len(feitos))
	}
	if len(feitos[0].Nearby) == 0 {
		t.Fatal("ninguém por perto, com outro jogador no mesmo lugar")
	}
	achou := false
	for _, n := range feitos[0].Nearby {
		if n == "HeroB" {
			achou = true
		}
	}
	if !achou {
		t.Errorf("por perto = %v, want incluindo HeroB", feitos[0].Nearby)
	}
	// The reporter is not in their own bystander list: ForEachInView excludes the
	// source, and a report that named its own author would read as two people.
	for _, n := range feitos[0].Nearby {
		if n == "Hero" {
			t.Error("o próprio denunciante entrou na lista de quem estava por perto")
		}
	}
}
