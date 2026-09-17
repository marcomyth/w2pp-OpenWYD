package handler

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A fada que some: a loja vende as fadas sem efeito nenhum, e o jogador as poe no
// Equip[13] arrastando ou com duplo clique — o MsgTradingItem, que não começava o
// tempo. O primeiro pulso de minuto lia duração zero e apagava a fada.

const (
	fadaVermelha3dias = 3902
	fadaSlotEquip     = 1 << fairyEquipSlot // nPos que só cabe no Equip[13]
)

// startServerFada sobe o servidor com o catálogo que a fada precisa: onde ela
// cabe e quantos dias o nome dá. tick liga o laço (10 ms por tique), para o pulso
// de 60 tiques chegar em menos de um segundo.
func startServerFada(t *testing.T, st world.CharacterState, tick bool) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log,
		ItemPos:       map[int]int{fadaVermelha3dias: fadaSlotEquip, fadaDoValeIndex: fadaSlotEquip},
		ItemDurations: map[int]int{fadaVermelha3dias: 3, fadaDoValeIndex: 7},
	})
	db := newDB()
	db.loadResult = st
	w := world.New(world.Config{GridDim: 16}, log, db, d.Handle)
	if tick {
		w.SetTickHandler(10*time.Millisecond, d.Tick)
	}
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

// quadroDoEquip13 lê até chegar um SendItem do Equip[13] ou o prazo vencer.
func quadroDoEquip13(t *testing.T, c net.Conn, prazo time.Duration) (world.Item, bool) {
	t.Helper()
	fim := time.Now().Add(prazo)
	for time.Now().Before(fim) {
		ty, p, ok := readMaybe(t, c)
		if !ok || ty != protocol.MsgSendItem || len(p) < 12 {
			continue
		}
		if binary.LittleEndian.Uint16(p[0:]) != world.ItemPlaceEquip || binary.LittleEndian.Uint16(p[2:]) != fairyEquipSlot {
			continue
		}
		it := world.Item{Index: int16(binary.LittleEndian.Uint16(p[4:]))}
		for i := 0; i < 3; i++ {
			it.Effects[i] = world.Effect{Effect: p[6+2*i], Value: p[7+2*i]}
		}
		return it, true
	}
	return world.Item{}, false
}

// Arrastar a fada da bolsa para o Equip[13] começa o tempo, e o cliente recebe
// a fada já com ele.
func TestArrastarAFadaComecaOTempo(t *testing.T) {
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	st.Carry[0] = world.Item{Index: fadaVermelha3dias} // como a loja vende: sem efeito
	addr, stop := startServerFada(t, st, false)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	tradeItemFrame(t, c, world.ItemPlaceCarry, 0, world.ItemPlaceEquip, fairyEquipSlot, 0)
	it, ok := quadroDoEquip13(t, c, 2*time.Second)
	if !ok {
		t.Fatal("nenhum SendItem do Equip[13] depois de arrastar a fada")
	}
	if it.Index != fadaVermelha3dias {
		t.Fatalf("Equip[13] = item %d, queria a fada %d", it.Index, fadaVermelha3dias)
	}
	if got := effectsDuration(it.Effects); got != 3*24*time.Hour {
		t.Errorf("a fada arrastada chegou com %v de tempo, queria 3 dias", got)
	}
}

// Uma fada que já está no Equip[13] sem tempo — arrastada antes deste conserto,
// ou posta por outro caminho — começa a contar no pulso, e não é apagada.
func TestPulsoComecaAFadaSemTempoEmVezDeApagar(t *testing.T) {
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	st.Equip[fairyEquipSlot] = world.Item{Index: fadaVermelha3dias}
	addr, stop := startServerFada(t, st, true)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	it, ok := quadroDoEquip13(t, c, 3*time.Second)
	if !ok {
		t.Fatal("o pulso não mandou o Equip[13]")
	}
	if it.Index != fadaVermelha3dias {
		t.Fatalf("o pulso deixou o Equip[13] com o item %d; a fada sem tempo foi apagada", it.Index)
	}
	if effectsDuration(it.Effects) <= 0 {
		t.Errorf("a fada ficou no Equip[13] sem tempo nenhum: %v", it.Effects)
	}
}

// A Fada do Vale é fada: gasta só vestida, um minuto por pulso.
func TestFadaDoValeGastaVestida(t *testing.T) {
	if !isFairy(fadaDoValeIndex) {
		t.Fatal("a Fada do Vale não conta como fada")
	}
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	st.Equip[fairyEquipSlot] = world.Item{Index: fadaDoValeIndex, Effects: durationEffects(7 * 24 * time.Hour)}
	addr, stop := startServerFada(t, st, true)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	it, ok := quadroDoEquip13(t, c, 3*time.Second)
	if !ok {
		t.Fatal("o pulso não mexeu na Fada do Vale")
	}
	if got, want := effectsDuration(it.Effects), 7*24*time.Hour-time.Minute; it.Index != fadaDoValeIndex || got != want {
		t.Errorf("Fada do Vale depois do pulso = item %d com %v, queria %d com %v", it.Index, got, fadaDoValeIndex, want)
	}
}
