package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// recebidas diz quantas denúncias chegaram ao banco, dando um instante para uma escrita
// fora do laço aparecer.
//
// A ESPERA EXISTE PARA O TESTE PODER FALHAR. A gravação antiga era feita fora do laço do
// mundo, então ler na hora daria zero mesmo se alguém religasse o caminho — o teste
// passaria dizendo "não grava" sem ter medido nada. Esperar um pouco é o que torna o zero
// uma afirmação.
func (r *reportDB) recebidas() []world.PlayerReport {
	time.Sleep(150 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]world.PlayerReport, len(r.feitos))
	copy(out, r.feitos)
	return out
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

// O /REPORTAR MANDA PARA O DISCORD, e não grava mais nada.
//
// Denúncia e suporte saíram do jogo em 25/09/2026. O comando continua RESPONDENDO em vez
// de cair no "comando desconhecido": quem digitava tinha um problema NAQUELE momento, e um
// silêncio manda essa pessoa embora sem saber para onde ir.
//
// A metade que vale mais é a segunda: NADA é gravado. O caminho de gravação continua de pé
// no dbclient e no store — tirá-lo exigiria mexer no .proto e quebrar o build do par do
// site —, e é justamente por ele existir sem ninguém chamar que este teste precisa afirmar
// que ninguém chama. Sem isso, uma religação acidental voltaria a enfileirar denúncia numa
// tela que não existe mais, e ninguém descobriria.
func TestReportarMandaParaODiscordENaoGravaNada(t *testing.T) {
	for _, cmd := range []string{"reportar", "report"} {
		db := newReportDB()
		agora := &atomic.Int64{}
		addr, stop := startServerReport(t, db, agora)
		a := enterWorldAs(t, addr, "tester")

		whisperFrame(t, a, cmd, "alguem esta usando bot aqui")

		if !recebeu(t, a, msgSuportePeloDiscord) {
			t.Errorf("/%s: o jogador nao leu para onde ir", cmd)
		}
		if n := len(db.recebidas()); n != 0 {
			t.Errorf("/%s: gravou %d denuncia(s); o caminho devia estar desligado", cmd, n)
		}
		a.Close()
		stop()
	}
}
