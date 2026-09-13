package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestAreasDeLimpezaBatemComOLegado prende a lista às dez chamadas que o
// original faz em ProcessSecMinTimer.cpp:562-572. Portar só as cinco arenas que
// apareceram na reclamação deixaria as outras cinco com o mesmo defeito, então a
// lista é conferida inteira, na ordem, com as nove ClearAreaQuest separadas da
// única ClearArea (a Lanhouse, que não mexe em bandeira).
func TestAreasDeLimpezaBatemComOLegado(t *testing.T) {
	quero := []areaDeLimpeza{
		{"Cemitério (Coveiro)", 2379, 2076, 2426, 2133, true},
		{"Capa Verde", 2232, 1564, 2263, 1592, true},
		{"Reset de habilidades (Armia)", 2640, 1966, 2670, 2004, true},
		{"Jardim dos Deuses (Carbuncle)", 2228, 1700, 2257, 1728, true},
		{"Reset de habilidades (Erion)", 1950, 1586, 1988, 1614, true},
		{"Coração do Kaizen", 459, 3887, 497, 3916, true},
		{"Hidras", 658, 3728, 703, 3762, true},
		{"Elfos", 1312, 4027, 1348, 4055, true},
		{"Quest Gárgula", 793, 4046, 827, 4080, true},
		{"Lanhouse", 3570, 3446, 3965, 3711, false},
	}
	if len(areasDeLimpeza) != len(quero) {
		t.Fatalf("o relógio limpa %d áreas, o legado limpa %d", len(areasDeLimpeza), len(quero))
	}
	for i, q := range quero {
		if areasDeLimpeza[i] != q {
			t.Errorf("área %d: %+v, esperado %+v", i, areasDeLimpeza[i], q)
		}
	}
}

// TestLimpezaPegaABordaDaCaixa: ClearAreaQuest pula com `< x1 || > x2`, então a
// própria borda ENTRA. É de propósito que isso não case com questArea.contains,
// do guarda, que é exclusiva — as duas bordas divergem no original.
func TestLimpezaPegaABordaDaCaixa(t *testing.T) {
	jardim := areasDeLimpeza[3]
	dentro := [][2]int16{
		{2228, 1700}, // canto de cima, exatamente na borda
		{2257, 1728}, // canto de baixo, exatamente na borda
		{2240, 1714}, // meio
	}
	for _, p := range dentro {
		if !jardim.contem(p[0], p[1]) {
			t.Errorf("(%d,%d) devia estar dentro do Jardim", p[0], p[1])
		}
	}
	fora := [][2]int16{{2227, 1714}, {2258, 1714}, {2240, 1699}, {2240, 1729}}
	for _, p := range fora {
		if jardim.contem(p[0], p[1]) {
			t.Errorf("(%d,%d) NÃO devia estar dentro do Jardim", p[0], p[1])
		}
	}
	// E o guarda continua exclusivo: a borda que a limpeza pega, ele deixa.
	guarda := quest256Steps[1].area
	if guarda.contains(2228, 1700) {
		t.Error("o guarda virou inclusivo; ele é exclusivo no original")
	}
}

// TestRelogioEsvaziaAArena é o pedido de ponta a ponta: um personagem parado
// numa área de quest é devolvido à cidade quando o relógio vira, e um que está
// longe não é tocado.
//
// A área escolhida é a Lanhouse porque guardQuest256Areas não olha para ela — o
// que sobrar do teste é obra do relógio, e só dele. Nas cinco arenas o guarda
// já expulsa quem está com a bandeira errada no tique seguinte, e um teste ali
// não saberia dizer qual dos dois agiu.
//
// O caso que a Hanna viu — passar do nível e continuar dentro — é este mesmo
// caminho: o relógio não olha nível nem bandeira, tira todo mundo que está na
// caixa. É por isso que ele conserta as cinco quests de uma vez.
func TestRelogioEsvaziaAArena(t *testing.T) {
	db := &fakeDB{accounts: map[string]*fakeAccount{
		"dentro": {id: 40, pass: "secret", role: "player", chars: []world.CharSummary{{Slot: 0, Name: "Dentro"}}},
		"longe":  {id: 41, pass: "secret", role: "player", chars: []world.CharSummary{{Slot: 0, Name: "Longe"}}},
	}}
	db.loads = map[int64]world.CharacterState{
		// dentro da Lanhouse (3570,3446 a 3965,3711), e com nível acima da faixa
		// de qualquer arena — o relógio não se importa com isso, e é o ponto
		40: {Slot: 0, Name: "Dentro", Level: 250, X: 3700, Y: 3500, HP: 1000, MaxHP: 1000},
		41: {Slot: 0, Name: "Longe", Level: 250, X: 2100, Y: 2100, HP: 1000, MaxHP: 1000},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) }})
	w := world.New(world.Config{GridDim: 4096, Now: clock.Load}, log, db, d.Handle)
	// 1 ms por tique: os 600 tiques do relógio cabem em pouco mais de meio
	// segundo, sem mexer no período que o jogo usa de verdade.
	w.SetTickHandler(time.Millisecond, func(w *world.World) { clock.Add(100); d.Tick(w) })
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

	dentro := enterWorldAs(t, ln.Addr().String(), "dentro")
	defer dentro.Close()
	longe := enterWorldAs(t, ln.Addr().String(), "longe")
	defer longe.Close()

	// O recall chega como MsgAction com Effect 1 no próprio avatar. O leitor do
	// harness desiste em 300 ms e o relógio leva 600, então aqui se espera por
	// tempo de parede e não por número de leituras: um silêncio de 300 ms no meio
	// do caminho é normal, não é resposta.
	levado := func(c net.Conn, id int, prazo time.Duration) bool {
		fim := time.Now().Add(prazo)
		for time.Now().Before(fim) {
			h, p, ok := readMaybeHeaderRaw(t, c)
			if !ok {
				continue // deadline de leitura vencido, ainda dentro do prazo
			}
			if h.Type != protocol.MsgAction || int(h.ID) != id {
				continue
			}
			var b protocol.MsgActionBody
			if b.Decode(p) == nil && b.Effect == 1 {
				return true
			}
		}
		return false
	}
	if !levado(dentro, 1, 3*time.Second) {
		t.Error("o relógio virou e quem estava na área continuou lá")
	}
	// o relógio já virou acima; meio segundo cobre a volta inteira do laço
	if levado(longe, 2, 500*time.Millisecond) {
		t.Error("o relógio levou alguém que estava longe de qualquer área")
	}
}
