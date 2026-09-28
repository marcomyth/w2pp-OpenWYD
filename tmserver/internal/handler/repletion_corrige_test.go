package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func startServerRepletion(t *testing.T, persist world.Persistence) (string, func(), *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// nPos do ItemList: 4 é peito, 8 é calça, 16 é luva.
	d := New(Config{Log: log, ItemPos: map[int]int{1345: 4, 1348: 8, 1501: 16}})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}, w
}

func pecaDeRepletion(idx int16, add uint8, valor uint8, defesa uint8) world.Item {
	return world.Item{Index: idx, Effects: [3]world.Effect{
		{Effect: 43, Value: 6}, {Effect: add, Value: valor}, {Effect: 3, Value: defesa},
	}}
}

// As peças que a Repletion antiga deixou com Defesa 35-50 junto de Dano ou Magia
// saem do carregamento com Defesa 30 — no equipamento, na bolsa e no armazém.
func TestLoginCorrigeRepletionAberrante(t *testing.T) {
	db := newDB()
	st := world.CharacterState{Slot: 0, Name: "Tanque", X: 5, Y: 5, Level: 1, HP: 1000, MaxHP: 1000}
	st.Equip[2] = pecaDeRepletion(1345, 60, 10, 35) // Túnica: Defesa 35 + Magia 10
	st.Carry[0] = pecaDeRepletion(1348, 60, 10, 40) // Calça: Defesa 40 + Magia 10
	db.loadResult = st
	var cargo world.CargoState
	cargo.Items[0] = pecaDeRepletion(1501, 60, 8, 50) // Manoplas: Defesa 50 + Magia 8
	db.accounts["tester"].cargo = cargo

	addr, stop, w := startServerRepletion(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	var equip, bolsa, armazem world.Item
	noLaco(t, w, func(w *world.World) {
		e := w.Entity(1)
		equip, bolsa = e.Equip[2], e.Carry[0]
		if cg := w.Cargo(db.accounts["tester"].id); cg != nil {
			armazem = cg.Items[0]
		}
	})
	for _, c := range []struct {
		onde      string
		it        world.Item
		add, soma uint8
		defesa    world.Effect
	}{
		{"equipamento", equip, 60, 10, world.Effect{Effect: 3, Value: 30}},
		{"bolsa", bolsa, 60, 10, world.Effect{Effect: 3, Value: 30}},
		// A luva não fica com Defesa nenhuma: vira Skill 12.
		{"armazém", armazem, 60, 8, world.Effect{Effect: 74, Value: 12}},
	} {
		if c.it.Effects[2] != c.defesa || c.it.Effects[1] != (world.Effect{Effect: c.add, Value: c.soma}) {
			t.Errorf("%s: peça ficou %+v; quer %+v e o add mantido", c.onde, c.it.Effects, c.defesa)
		}
	}
}
