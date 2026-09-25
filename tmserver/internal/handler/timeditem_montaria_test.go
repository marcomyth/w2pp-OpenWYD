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

// A montaria da loja que não desconta: vestida com duplo clique, o servidor
// começava o prazo e o cliente seguia mostrando a duração cheia ("4 Dia(s) 0Hora
// 0"); e, com o jogador online, nada reenviava o item nem o tirava quando vencia.

const (
	shireLoja      = 3980
	montariaSlotEq = 1 << mountEquipSlot // nPos que só cabe no Equip[14]
)

func startServerMontaria(t *testing.T, st world.CharacterState, tick bool) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, ItemPos: map[int]int{shireLoja: montariaSlotEq}})
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

// quadroDoSlot lê até chegar um SendItem daquele lugar e slot, ou o prazo vencer.
func quadroDoSlot(t *testing.T, c net.Conn, place, slot int, prazo time.Duration) (world.Item, bool) {
	t.Helper()
	fim := time.Now().Add(prazo)
	for time.Now().Before(fim) {
		ty, p, ok := readMaybe(t, c)
		if !ok || ty != protocol.MsgSendItem || len(p) < 12 {
			continue
		}
		if int(binary.LittleEndian.Uint16(p[0:])) != place || int(binary.LittleEndian.Uint16(p[2:])) != slot {
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

// correndo diz se o cliente recebeu um prazo que já anda: menos que a duração
// cheia. Uma Shire de 4 dias que começou agora chega como 3 Dia(s) 23 Hora(s).
func correndo(it world.Item, cheia time.Duration) bool {
	left := effectsDuration(it.Effects)
	return left > 0 && left < cheia
}

func TestDuploCliqueNaMontariaMandaOPrazoQueJaCorre(t *testing.T) {
	const dias = 4 * 24 * time.Hour
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	st.Carry[0] = world.Item{Index: shireLoja, Effects: durationEffects(dias)}
	addr, stop := startServerMontaria(t, st, false)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: 0,
		DestType: world.ItemPlaceEquip, DestPos: mountEquipSlot}
	send(t, c, protocol.MsgUseItem, body.Encode())

	it, ok := quadroDoSlot(t, c, world.ItemPlaceEquip, mountEquipSlot, 2*time.Second)
	if !ok {
		t.Fatal("o duplo clique não mandou o Equip[14]: o cliente fica com a duração cheia")
	}
	if it.Index != shireLoja || !correndo(it, dias) {
		t.Errorf("Equip[14] = item %d com %v; queria a Shire com menos de 4 dias", it.Index, effectsDuration(it.Effects))
	}
}

func TestPulsoReenviaAMontariaQueCorre(t *testing.T) {
	const dias = 3 * 24 * time.Hour
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	st.Equip[mountEquipSlot] = world.Item{Index: shireLoja, ExpiresAt: time.Now().Add(dias - time.Hour).Unix()}
	addr, stop := startServerMontaria(t, st, true)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	it, ok := quadroDoSlot(t, c, world.ItemPlaceEquip, mountEquipSlot, 3*time.Second)
	if !ok {
		t.Fatal("o pulso não reenviou a montaria: o contador do cliente fica parado")
	}
	if it.Index != shireLoja || !correndo(it, dias) {
		t.Errorf("Equip[14] = item %d com %v; queria a Shire correndo", it.Index, effectsDuration(it.Effects))
	}
}

func TestPulsoTiraAMontariaVencidaSemEsperarOLogin(t *testing.T) {
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	// Vence dois segundos depois do login: dropExpired, que só roda no login,
	// ainda a deixa passar, e só o pulso pode tirá-la.
	st.Equip[mountEquipSlot] = world.Item{Index: shireLoja, ExpiresAt: time.Now().Add(2 * time.Second).Unix()}
	addr, stop := startServerMontaria(t, st, true)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	fim := time.Now().Add(6 * time.Second)
	for time.Now().Before(fim) {
		it, ok := quadroDoSlot(t, c, world.ItemPlaceEquip, mountEquipSlot, time.Until(fim))
		if ok && it.Empty() {
			return
		}
	}
	t.Fatal("a montaria vencida continuou no Equip[14] com o jogador online")
}

func TestPulsoComecaAMontariaVestidaSemPrazo(t *testing.T) {
	const dias = 4 * 24 * time.Hour
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	// Arrastada antes de 16c867bb: no Equip[14], ainda com a duração e sem prazo.
	st.Equip[mountEquipSlot] = world.Item{Index: shireLoja, Effects: durationEffects(dias)}
	addr, stop := startServerMontaria(t, st, true)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	it, ok := quadroDoSlot(t, c, world.ItemPlaceEquip, mountEquipSlot, 3*time.Second)
	if !ok {
		t.Fatal("o pulso não mandou a montaria sem prazo")
	}
	if it.Index != shireLoja || !correndo(it, dias) {
		t.Errorf("Equip[14] = item %d com %v; queria a Shire começada", it.Index, effectsDuration(it.Effects))
	}
}

func TestPulsoTiraDaBolsaAMontariaVencida(t *testing.T) {
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 50}
	// Tirada do corpo depois de começar: "Consumo contínuo, mesmo não equipado".
	st.Carry[3] = world.Item{Index: shireLoja, ExpiresAt: time.Now().Add(2 * time.Second).Unix()}
	addr, stop := startServerMontaria(t, st, true)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	it, ok := quadroDoSlot(t, c, world.ItemPlaceCarry, 3, 6*time.Second)
	if !ok || !it.Empty() {
		t.Fatalf("a montaria vencida continuou na bolsa (quadro=%v, item %d)", ok, it.Index)
	}
}
