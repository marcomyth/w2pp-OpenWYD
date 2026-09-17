package handler

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Entrar nas arenas da Quest 256 não tem limite por rodada (pedido da Hanna de
// 17/09, que tirou a regra de uma entrada e uma saída). Ficam o guarda por nível
// e o relógio de 600 s. Os testes usam o servidor do relógio
// (startServerRelogioDasArenas): Mestre Grifo e Coveiro em Armia, o Mortal 50 com
// a Vela do Coveiro no espaço 0 e o tique parado, andando só quando o teste pede.

const inicioDaVolta = 3*questClearTicks + 1

// entraPeloGrifo entra na arena e espera o relógio, que é o último quadro da entrada.
func entraPeloGrifo(t *testing.T, c net.Conn, srv *servidorDoRelogio) {
	t.Helper()
	questFrame(t, c, srv.grifo)
	if _, _, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, _ []byte) bool {
		return h.Type == protocol.MsgStartTime
	}); !ok {
		t.Fatal("a entrada pelo Mestre Grifo não chegou")
	}
}

// drena lê o que houver por um instante, para a próxima leitura começar limpa.
func drena(t *testing.T, c net.Conn) {
	t.Helper()
	quadroAte(t, c, 400*time.Millisecond, func(protocol.Header, []byte) bool { return false })
}

// naContaDoRelogio roda fn no laço sobre o personagem da conta.
func naContaDoRelogio(t *testing.T, srv *servidorDoRelogio, conta int64, fn func(*world.World, *Dispatcher, *world.Session, *world.Entity)) {
	t.Helper()
	achou := false
	srv.noLaco(t, func(w *world.World, d *Dispatcher) {
		w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
			if s.AccountID == conta {
				achou = true
				fn(w, d, s, e)
			}
		})
	})
	if !achou {
		t.Fatalf("a conta %d não está em jogo", conta)
	}
}

// saiDaArena põe o personagem de volta ao lado do Grifo, por teleporte, e roda um
// tique.
func saiDaArena(t *testing.T, srv *servidorDoRelogio, conta int64) {
	t.Helper()
	naContaDoRelogio(t, srv, conta, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		d.doTeleport(w, s, 2113, 2079)
	})
	srv.noLaco(t, func(w *world.World, d *Dispatcher) { d.Tick(w) })
}

func usaBilhete(t *testing.T, c net.Conn) {
	t.Helper()
	body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: 0}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

// esperaEntrada lê até o pulo e confere que o personagem está no Cemitério com a
// bandeira do Coveiro. Uma recusa com texto no caminho falha na hora.
func esperaEntrada(t *testing.T, c net.Conn, srv *servidorDoRelogio, porta string) {
	t.Helper()
	recusa := ""
	_, _, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		if h.Type == protocol.MsgMessageChat || h.Type == protocol.MsgMessagePanel {
			if txt := decodePanel(p); strings.Contains(txt, "rodada") {
				recusa = txt
			}
		}
		return ehPulo(h, p)
	})
	if !ok {
		t.Fatalf("%s: não entrou (recusa: %q)", porta, recusa)
	}
	drena(t, c)
	naContaDoRelogio(t, srv, 7, func(_ *world.World, _ *Dispatcher, _ *world.Session, e *world.Entity) {
		if !quest256Steps[0].area.contains(e.X, e.Y) {
			t.Errorf("%s: pulou para %d,%d, fora do Cemitério", porta, e.X, e.Y)
		}
		if e.QuestFlag != quest256Steps[0].flag {
			t.Errorf("%s: bandeira %d, quero %d", porta, e.QuestFlag, quest256Steps[0].flag)
		}
	})
}

// TestEntraDeNovoNaMesmaRodadaPelasTresPortas: entrou pelo Grifo e saiu; o Grifo,
// o Coveiro e o bilhete levam de novo, todos antes do pulso.
func TestEntraDeNovoNaMesmaRodadaPelasTresPortas(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	saiDaArena(t, srv, 7)
	drena(t, c)

	questFrame(t, c, srv.grifo)
	esperaEntrada(t, c, srv, "Mestre Grifo, segunda vez")
	saiDaArena(t, srv, 7)
	drena(t, c)

	// O Coveiro toma a Vela (_MSG_Quest.cpp:316).
	questFrame(t, c, srv.coveiro)
	esperaEntrada(t, c, srv, "Coveiro, terceira vez")
	saiDaArena(t, srv, 7)
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(_ *world.World, d *Dispatcher, _ *world.Session, e *world.Entity) {
		if e.Carry[0].Index != 0 {
			t.Errorf("o Coveiro não tomou a Vela: espaço 0 = %d", e.Carry[0].Index)
		}
		e.Carry[0] = world.Item{Index: itemVelaDoCoveiro}
		if d.tickCount/questClearTicks != inicioDaVolta/questClearTicks {
			t.Fatalf("o teste passou do pulso (tickCount %d); a prova é na mesma rodada", d.tickCount)
		}
	})
	usaBilhete(t, c)
	esperaEntrada(t, c, srv, "bilhete, quarta vez")
}

// TestMorrerNaArenaNaoFechaAPorta: morrer lá dentro e voltar à vida na cidade não
// impede entrar de novo na mesma rodada.
func TestMorrerNaArenaNaoFechaAPorta(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.HP = 0
		d.Tick(w)
		e.HP = e.MaxHP
		d.doTeleport(w, s, 2113, 2079)
		d.Tick(w)
	})
	drena(t, c)
	questFrame(t, c, srv.grifo)
	esperaEntrada(t, c, srv, "Mestre Grifo depois da morte")
}

// TestPulsoExpulsaEAPortaSegueAberta: quem está dentro quando o pulso vira é
// expulso, e o Grifo leva de novo logo depois.
func TestPulsoExpulsaEAPortaSegueAberta(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), 4*questClearTicks-3)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	srv.anda.Store(true)
	if _, _, ok := quadroAte(t, c, 5*time.Second, ehPulo); !ok {
		t.Fatal("o pulso não expulsou")
	}
	fim := time.Now().Add(5 * time.Second)
	for srv.tiques.Load() < 6 && time.Now().Before(fim) {
		time.Sleep(time.Millisecond)
	}
	srv.anda.Store(false)
	drena(t, c)

	questFrame(t, c, srv.grifo)
	esperaEntrada(t, c, srv, "Mestre Grifo depois do pulso")
}
