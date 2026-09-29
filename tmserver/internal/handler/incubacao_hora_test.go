package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestIncubacaoContaPorHora: cada ponto de incubação do ovo é uma hora de ovo
// equipado, não um segundo (28/09/2026: um ovo de 6-9 h chocava em segundos).
//
// Pelo servidor de verdade, com o tick acelerado a 10 ms: dezenas de ticks
// passam e o contador fica onde está; só o tick da hora (pkTick, o mesmo que
// devolve o ponto de PK e cobra a ração) tira um ponto.
func TestIncubacaoContaPorHora(t *testing.T) {
	const ovo = 2304
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, combatDB(), d.Handle)
	w.SetTickHandler(10*time.Millisecond, d.Tick)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	c := enterWorld(t, ln.Addr().String())
	defer c.Close()

	incubacao := func() (n int) {
		noLaco(t, w, func(w *world.World) {
			w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
				n = itemInstanceAbility(e.Equip[mountEquipSlot], efIncuDelay)
			})
		})
		return n
	}

	noLaco(t, w, func(w *world.World) {
		w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
			e.Equip[mountEquipSlot] = world.Item{Index: ovo, Effects: [3]world.Effect{
				{}, {}, {Effect: efIncuDelay, Value: 5},
			}}
		})
	})

	time.Sleep(300 * time.Millisecond) // ~30 ticks de 10 ms
	if got := incubacao(); got != 5 {
		t.Fatalf("depois de ~30 ticks a incubação foi a %d, quer 5: ela não pode descer a cada tick", got)
	}

	noLaco(t, w, func(w *world.World) {
		d.tickCount = pkPointRecoverPeriod * 1000 // um tick que cai na hora
		d.sweepGuilty(w)
	})
	if got := incubacao(); got != 4 {
		t.Errorf("no tick da hora a incubação foi a %d, quer 4", got)
	}
}
