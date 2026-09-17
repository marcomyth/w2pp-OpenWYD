package handler

import (
	"context"
	"fmt"
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

// servidorDoRelogio é o servidor do Mestre Grifo (startServerMestreGrifo) com um
// Coveiro ao lado e o relógio das arenas na mão do teste.
type servidorDoRelogio struct {
	addr           string
	grifo, coveiro int
	// anda solta o tique. Com ele desligado a entrada acontece com tickCount
	// exatamente no valor que o teste pediu; o teste liga quando quer ver o
	// relógio virar.
	anda atomic.Bool
	// tiques conta os tiques rodados desde que anda ligou. saiuNoTique é o
	// primeiro deles que terminou com o jogador fora do Cemitério (0 = ainda lá).
	tiques, saiuNoTique atomic.Int32
	// pendente é uma função que o teste quer rodar DENTRO do laço, uma vez, com o
	// tique parado (entrada_arena_test.go e outros). feito avisa que ela rodou.
	pendente atomic.Pointer[func(*world.World, *Dispatcher)]
	feito    chan struct{}
}

// noLaco roda fn no laço do mundo e espera ela terminar.
func (srv *servidorDoRelogio) noLaco(t *testing.T, fn func(*world.World, *Dispatcher)) {
	t.Helper()
	srv.pendente.Store(&fn)
	select {
	case <-srv.feito:
	case <-time.After(5 * time.Second):
		t.Fatal("a função não rodou no laço")
	}
}

// startServerRelogioDasArenas sobe o servidor com tickCount já no valor pedido.
// A escrita vem antes do Serve, quando ainda não há laço nenhum para disputá-la;
// daí em diante só o laço mexe nele, pelo Tick.
func startServerRelogioDasArenas(t *testing.T, st world.CharacterState, tickCount int) *servidorDoRelogio {
	t.Helper()
	return startServerRelogioDasArenasCom(t, st, tickCount, nil)
}

// startServerRelogioDasArenasCom é o mesmo servidor com personagens por conta
// (fakeDB.loads), para testes com mais de uma pessoa.
func startServerRelogioDasArenasCom(t *testing.T, st world.CharacterState, tickCount int, porConta map[int64]world.CharacterState) *servidorDoRelogio {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Relógio de parede parado, como em TestRelogioEsvaziaAArena: nenhum evento
	// marcado por hora entra no meio da contagem.
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) }})
	d.tickCount = tickCount
	db := newDB()
	db.loadResult = st
	db.loads = porConta
	// Conta que o newDB não tem entra como "conta<id>", para testes com mais de
	// duas pessoas (populacao_arenas_test.go).
	for id, pc := range porConta {
		existe := false
		for _, a := range db.accounts {
			existe = existe || a.id == id
		}
		if !existe {
			db.accounts[fmt.Sprintf("conta%d", id)] = &fakeAccount{id: id, pass: "secret",
				chars: []world.CharSummary{{Slot: 0, Name: pc.Name, Class: 1, Level: pc.Level}}}
		}
	}
	w := world.New(world.Config{GridDim: world.DefaultGridDim}, log, db, d.Handle)
	srv := &servidorDoRelogio{addr: ln.Addr().String()}
	cemiterio := quest256Steps[0].area
	srv.feito = make(chan struct{}, 1)
	w.SetTickHandler(time.Millisecond, func(w *world.World) {
		if fn := srv.pendente.Swap(nil); fn != nil {
			(*fn)(w, d)
			srv.feito <- struct{}{}
		}
		if !srv.anda.Load() {
			return
		}
		d.Tick(w)
		n := srv.tiques.Add(1)
		w.ForEachPlayer(func(_ *world.Session, e *world.Entity) {
			if !cemiterio.contains(e.X, e.Y) {
				srv.saiuNoTique.CompareAndSwap(0, n)
			}
		})
	})
	srv.grifo = w.SpawnMob(mestreGrifoTemplate(), 2116, 2080)
	srv.coveiro = w.SpawnMob(questNPCTemplate("Coveiro", 100, 0, 0), 2110, 2080)
	if srv.grifo < 0 || srv.coveiro < 0 {
		t.Fatalf("NPCs não nasceram: grifo=%d coveiro=%d", srv.grifo, srv.coveiro)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	})
	return srv
}

// mortalDoCemiterio é um Mortal na faixa do Coveiro, ao lado do Mestre Grifo em
// Armia, com a Vela do Coveiro no slot 0: serve às três portas da mesma arena.
func mortalDoCemiterio() world.CharacterState {
	st := world.CharacterState{
		Slot: 0, Name: "Hero", Level: 50, X: 2113, Y: 2079,
		HP: 1000, MaxHP: 1000, LastCity: 0, ClassMaster: classMasterMortal,
	}
	st.Carry[0] = world.Item{Index: itemVelaDoCoveiro}
	return st
}

// quadroAte lê até um quadro que `quer` aceite, por tempo de parede. O leitor do
// harness desiste em 300 ms, e um silêncio desses no meio do prazo não é
// resposta: só o prazo encerra a espera.
func quadroAte(t *testing.T, c net.Conn, prazo time.Duration, quer func(protocol.Header, []byte) bool) (protocol.Header, []byte, bool) {
	t.Helper()
	fim := time.Now().Add(prazo)
	for time.Now().Before(fim) {
		h, p, ok := readMaybeHeaderRaw(t, c)
		if ok && quer(h, p) {
			return h, p, true
		}
	}
	return protocol.Header{}, nil, false
}

// ehPulo diz se o quadro é o teleporte do próprio avatar (MsgAction, Effect 1).
func ehPulo(h protocol.Header, p []byte) bool {
	var b protocol.MsgActionBody
	return h.Type == protocol.MsgAction && b.Decode(p) == nil && b.Effect == 1
}

// TestRelogioDasArenasNaEntrada: quem entra numa arena vê quanto falta para o
// relógio esvaziá-la, pelas três portas — Mestre Grifo, bilhete e NPC — e em
// qualquer ponto da volta. O tickCount já traz voltas inteiras atrás, para a
// conta ser pelo resto e não pelo total.
func TestRelogioDasArenasNaEntrada(t *testing.T) {
	voltas := []struct {
		nome      string
		tickCount int
		segundos  int32
	}{
		{"começo da volta", 3*questClearTicks + 1, 599},
		{"fim da volta", 3*questClearTicks + 599, 1},
		{"a limpeza acabou de rodar", 4 * questClearTicks, 600},
	}
	portas := []struct {
		nome  string
		entra func(t *testing.T, c net.Conn, srv *servidorDoRelogio)
	}{
		{"Mestre Grifo", func(t *testing.T, c net.Conn, srv *servidorDoRelogio) {
			questFrame(t, c, srv.grifo)
		}},
		{"bilhete", func(t *testing.T, c net.Conn, _ *servidorDoRelogio) {
			body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: 0}
			send(t, c, protocol.MsgUseItem, body.Encode())
		}},
		{"Coveiro", func(t *testing.T, c net.Conn, srv *servidorDoRelogio) {
			questFrame(t, c, srv.coveiro)
		}},
	}
	for _, porta := range portas {
		for _, v := range voltas {
			t.Run(porta.nome+"/"+v.nome, func(t *testing.T) {
				srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), v.tickCount)
				c := enterWorld(t, srv.addr)
				defer c.Close()

				porta.entra(t, c, srv)
				pulou := false
				h, p, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
					if ehPulo(h, p) {
						pulou = true
					}
					return h.Type == protocol.MsgStartTime
				})
				if !ok {
					t.Fatal("entrou na arena e o relógio não veio")
				}
				// O cliente só desenha o contador no campo em que está: o relógio
				// vem depois do pulo que o põe na arena, como na Água e no Orc.
				if !pulou {
					t.Error("o relógio chegou antes do teleporte para a arena")
				}
				if h.ID != protocol.IDScene {
					t.Errorf("HEADER.ID = %d, quero IDScene como os outros relógios", h.ID)
				}
				if got, _ := protocol.StandardParm(p); got != v.segundos {
					t.Errorf("tickCount %d: a tela mostra %d s, quero %d", v.tickCount, got, v.segundos)
				}
			})
		}
	}
}

// TestRelogioDasArenasBateComORecall: o número que a tela mostra é o mesmo que o
// relógio cumpre. O tique fica parado até o contador chegar; aí anda, e o
// jogador tem de ser devolvido a Armia exatamente no tique que o contador
// anunciou: nem antes, nem depois. É também a prova de que a entrada não tirou
// ninguém do alcance do relógio.
func TestRelogioDasArenasBateComORecall(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), 3*questClearTicks+590)
	c := enterWorld(t, srv.addr)
	defer c.Close()

	questFrame(t, c, srv.grifo)
	var eu uint16
	_, p, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		if ehPulo(h, p) {
			eu = h.ID // o pulo vai com o id do próprio jogador no cabeçalho
		}
		return h.Type == protocol.MsgStartTime
	})
	if !ok || eu == 0 {
		t.Fatalf("a entrada pelo Mestre Grifo não chegou inteira: relógio=%v pulo=%d", ok, eu)
	}
	mostrou, _ := protocol.StandardParm(p)
	if mostrou != 10 {
		t.Errorf("a tela mostra %d s, quero 10", mostrou) // segue: o tique do recall diz o resto
	}

	srv.anda.Store(true)
	// Dez tiques de 1 ms; o prazo largo é para máquina carregada, não para o relógio.
	_, p, ok = quadroAte(t, c, 5*time.Second, func(h protocol.Header, p []byte) bool {
		return h.ID == eu && ehPulo(h, p)
	})
	if !ok {
		t.Fatalf("o relógio virou (%d tiques) e o jogador continuou na arena", srv.tiques.Load())
	}
	var recall protocol.MsgActionBody
	if err := recall.Decode(p); err != nil {
		t.Fatal(err)
	}
	if recall.TargetX < 2086 || recall.TargetX > 2100 || recall.TargetY < 2093 || recall.TargetY > 2107 {
		t.Errorf("recall para %d,%d, quero o nascimento de Armia", recall.TargetX, recall.TargetY)
	}
	// O quadro sai pela rede enquanto o tique ainda está terminando; a marca é
	// gravada logo depois, no mesmo laço.
	fim := time.Now().Add(time.Second)
	for srv.saiuNoTique.Load() == 0 && time.Now().Before(fim) {
		time.Sleep(time.Millisecond)
	}
	if got := srv.saiuNoTique.Load(); got != mostrou {
		t.Errorf("a tela mostrou %d s e o jogador saiu da arena no tique %d", mostrou, got)
	}
}
