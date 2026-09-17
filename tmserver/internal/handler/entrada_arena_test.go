package handler

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Uma entrada por rodada (entrada_arena.go). Os testes usam o servidor do relógio
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
// tique para a vigia ver a saída.
func saiDaArena(t *testing.T, srv *servidorDoRelogio, conta int64) {
	t.Helper()
	naContaDoRelogio(t, srv, conta, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		d.doTeleport(w, s, 2113, 2079)
	})
	srv.noLaco(t, func(w *world.World, d *Dispatcher) { d.Tick(w) })
}

// esperaRecusa lê até um quadro do tipo pedido com o texto da rodada usada, e
// confere que nenhum teleporte veio junto.
func esperaRecusa(t *testing.T, c net.Conn, tipo protocol.Type, porta string) {
	t.Helper()
	pulou := false
	_, p, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		if ehPulo(h, p) {
			pulou = true
		}
		return h.Type == tipo && strings.Contains(decodePanel(p), "entrada desta rodada")
	})
	if !ok {
		t.Fatalf("%s: a recusa da rodada não chegou", porta)
	}
	if txt := decodePanel(p); !strings.Contains(txt, "min") {
		t.Errorf("%s: a recusa não diz quanto falta: %q", porta, txt)
	}
	drena(t, c)
	if pulou {
		t.Errorf("%s: recusou e mesmo assim teleportou", porta)
	}
}

func usaBilhete(t *testing.T, c net.Conn) {
	t.Helper()
	body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: 0}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

// TestSegundaEntradaRecusadaNasTresPortas: entrou pelo Grifo e saiu; o Grifo, o
// Coveiro e o bilhete recusam, e o bilhete continua na bolsa.
func TestSegundaEntradaRecusadaNasTresPortas(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	saiDaArena(t, srv, 7)
	drena(t, c)

	questFrame(t, c, srv.grifo)
	esperaRecusa(t, c, protocol.MsgMessageChat, "Mestre Grifo")
	questFrame(t, c, srv.coveiro)
	esperaRecusa(t, c, protocol.MsgMessageChat, "Coveiro")

	usaBilhete(t, c)
	pulou := false
	_, p, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		if ehPulo(h, p) {
			pulou = true
		}
		return h.Type == protocol.MsgSendItem
	})
	if !ok {
		t.Fatal("bilhete: o espaço não foi reenviado")
	}
	if slot, idx := binary.LittleEndian.Uint16(p[2:4]), int16(binary.LittleEndian.Uint16(p[4:6])); slot != 0 || idx != itemVelaDoCoveiro {
		t.Errorf("bilhete: espaço %d com item %d, quero o espaço 0 ainda com a Vela (%d)", slot, idx, itemVelaDoCoveiro)
	}
	if pulou {
		t.Error("bilhete: recusou e mesmo assim teleportou")
	}
	naContaDoRelogio(t, srv, 7, func(_ *world.World, _ *Dispatcher, _ *world.Session, e *world.Entity) {
		if e.Carry[0].Index != itemVelaDoCoveiro {
			t.Errorf("a Vela do Coveiro sumiu da bolsa: espaço 0 = %d", e.Carry[0].Index)
		}
	})
}

// TestBilheteRecusadoMostraOAviso: a recusa do bilhete sai no painel do jogador.
func TestBilheteRecusadoMostraOAviso(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	saiDaArena(t, srv, 7)
	drena(t, c)
	usaBilhete(t, c)
	esperaRecusa(t, c, protocol.MsgMessagePanel, "bilhete")
}

// TestMorrerNaArenaFechaARodada: morrer lá dentro é saída. Depois de voltar à
// vida na cidade, a porta recusa; e quem saiu perde a bandeira só depois de vivo.
func TestMorrerNaArenaFechaARodada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.HP = 0
		d.Tick(w)
		if ent := d.entradasDaRodada[donoDe(s)]; !ent.saiu {
			t.Error("morreu na arena e a rodada continuou aberta")
		}
		if e.QuestFlag == 0 {
			t.Error("tirou a bandeira de um morto; ele volta pela cidade no restart")
		}
		e.HP = e.MaxHP
		d.doTeleport(w, s, 2113, 2079)
		d.Tick(w)
		if e.QuestFlag != 0 {
			t.Errorf("vivo de novo e ainda com a bandeira %d", e.QuestFlag)
		}
	})
	drena(t, c)
	questFrame(t, c, srv.grifo)
	esperaRecusa(t, c, protocol.MsgMessageChat, "Mestre Grifo depois da morte")
}

// TestRelogNaoLiberaAEntrada: sair do jogo lá dentro fecha a rodada, e voltar na
// mesma rodada não dá entrada nova.
func TestRelogNaoLiberaAEntrada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	entraPeloGrifo(t, c, srv)
	c.Close()

	fim := time.Now().Add(3 * time.Second)
	for {
		conectado := false
		srv.noLaco(t, func(w *world.World, _ *Dispatcher) {
			w.ForEachPlayer(func(s *world.Session, _ *world.Entity) {
				if s.AccountID == 7 {
					conectado = true
				}
			})
		})
		if !conectado {
			break
		}
		if time.Now().After(fim) {
			t.Fatal("a sessão fechada continuou em jogo")
		}
		time.Sleep(10 * time.Millisecond)
	}
	srv.noLaco(t, func(w *world.World, d *Dispatcher) { d.Tick(w) })

	c2 := enterWorldAs(t, srv.addr, "tester")
	defer c2.Close()
	drena(t, c2)
	usaBilhete(t, c2)
	esperaRecusa(t, c2, protocol.MsgMessagePanel, "bilhete depois do relog")
}

// TestPulsoLiberaNovaEntrada: depois do pulso, a entrada volta; e a expulsão do
// pulso não fecha a rodada nova.
func TestPulsoLiberaNovaEntrada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), 4*questClearTicks-6)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	entraPeloGrifo(t, c, srv)
	saiDaArena(t, srv, 7)
	drena(t, c)
	questFrame(t, c, srv.grifo)
	esperaRecusa(t, c, protocol.MsgMessageChat, "Mestre Grifo antes do pulso")

	// Passa o pulso: faltavam 5 tiques depois da saída.
	srv.anda.Store(true)
	fim := time.Now().Add(5 * time.Second)
	for srv.tiques.Load() < 8 && time.Now().Before(fim) {
		time.Sleep(time.Millisecond)
	}
	srv.anda.Store(false)
	drena(t, c)

	questFrame(t, c, srv.grifo)
	if _, _, ok := quadroAte(t, c, 2*time.Second, ehPulo); !ok {
		t.Fatal("depois do pulso o Grifo não levou de novo")
	}
}

// TestPulsoNaoContaComoSaida: quem está dentro quando o pulso vira é expulso e
// começa a rodada nova com a entrada livre.
func TestPulsoNaoContaComoSaida(t *testing.T) {
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
	if _, _, ok := quadroAte(t, c, 2*time.Second, ehPulo); !ok {
		t.Fatal("a expulsão do pulso fechou a rodada nova")
	}
}

// TestOutraPessoaNaoEAfetada: a rodada fechada é de um personagem só.
func TestOutraPessoaNaoEAfetada(t *testing.T) {
	outro := mortalDoCemiterio()
	outro.Name = "HeroB"
	srv := startServerRelogioDasArenasCom(t, mortalDoCemiterio(), inicioDaVolta, map[int64]world.CharacterState{11: outro})
	a := enterWorldAs(t, srv.addr, "tester")
	defer a.Close()
	entraPeloGrifo(t, a, srv)
	saiDaArena(t, srv, 7)
	drena(t, a)

	b := enterWorldAs(t, srv.addr, "tradeb")
	defer b.Close()
	drena(t, b)
	entraPeloGrifo(t, b, srv)

	drena(t, a)
	questFrame(t, a, srv.grifo)
	esperaRecusa(t, a, protocol.MsgMessageChat, "Mestre Grifo para quem já saiu")
}
