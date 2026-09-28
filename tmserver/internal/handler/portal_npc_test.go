package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestPortalNaoPousaEmCimaDaPatrulha é o caminho de 26/09/2026: os jogadores do
// Kaizen gravavam a Gema ao lado da Patrulha e voltavam de Armia pelo Pergaminho
// de Portal. O DoTeleport do legado pousa numa casa livre (GetEmptyMobGrid); o
// port pousava na casa exata e, se ela fosse do NPC, o tirava do grid — e com ele
// da vista de todo mundo, até o reinício.
func TestPortalNaoPousaEmCimaDaPatrulha(t *testing.T) {
	tmpl, err := os.ReadFile(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "npc", "Patrulha_"))
	if err != nil {
		t.Fatalf("template real da Patrulha_: %v", err)
	}
	const portal = 776
	db := gemaEstelarDB(5, 5, 20, 20, world.Item{Index: portal})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, ItemVolatiles: map[int]int{portal: volPortalScroll}})
	w := world.New(world.Config{GridDim: 64}, log, db, d.Handle)
	npc := w.SpawnMob(tmpl, 20, 20)
	if npc < 0 {
		t.Fatal("a Patrulha_ não nasceu")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}()

	c := enterWorld(t, ln.Addr().String())
	defer c.Close()
	useItemFrame(t, c, 0)
	expect(t, c, protocol.MsgSendItem) // o pergaminho gasto
	action := expectAction(t, c)
	if action.TargetX == 20 && action.TargetY == 20 {
		t.Fatal("o portal pousou em cima da Patrulha_")
	}
	if chebyshev(action.TargetX, action.TargetY, 20, 20) > 1 {
		t.Errorf("o portal pousou em (%d,%d), longe demais da Gema em (20,20)", action.TargetX, action.TargetY)
	}
	noLaco(t, w, func(w *world.World) {
		if id, ok := w.EntityAt(20, 20); !ok || id != npc {
			t.Errorf("a casa da Patrulha_ ficou com %d (ok=%v), want %d", id, ok, npc)
		}
		if e := w.Entity(1); e == nil || e.X != action.TargetX || e.Y != action.TargetY {
			t.Errorf("o servidor pôs o jogador em %+v, e o cliente em (%d,%d)", e, action.TargetX, action.TargetY)
		}
	})
}
