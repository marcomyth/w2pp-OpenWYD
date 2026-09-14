package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// startServerNovato é o startServerClock que entrega TAMBÉM o mundo: o kit é
// conferido na bolsa e no baú, que só existem lá dentro.
func startServerNovato(t *testing.T, persist world.Persistence) (string, func(), *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) }})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, persist, d.Handle)
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

// noLacoDoMundo roda fn DENTRO da rotina do mundo e espera ela terminar.
//
// Existe porque o estado do mundo tem um dono só: a rotina do World.Run o muta
// sem tranca nenhuma, e é isso que segura a paridade e impede item duplicado
// (ver a doc do pacote world). Um teste que alcança o estado direto da própria
// rotina lê ao mesmo tempo em que o laço escreve — e o detector de corrida pega,
// de vez em quando, no teste que estiver passando na hora errada.
//
// GoDetached é o caminho que o mundo já oferece para isso: entrega o retorno
// dentro do laço. O canal fecha depois de fn rodar, então o que fn escreveu
// chega ao teste com a ordem garantida.
func noLacoDoMundo(t *testing.T, w *world.World, fn func(*world.World)) {
	t.Helper()
	pronto := make(chan struct{})
	w.GoDetached(func() func(*world.World) {
		return func(w *world.World) {
			defer close(pronto)
			fn(w)
		}
	})
	select {
	case <-pronto:
	case <-time.After(2 * time.Second):
		t.Fatal("a leitura dentro do laço do mundo não voltou")
	}
}

// bolsaDoJogador devolve os itens não vazios da bolsa do único jogador em jogo.
func bolsaDoJogador(t *testing.T, w *world.World) map[int16]world.Item {
	t.Helper()
	achados := map[int16]world.Item{}
	noLacoDoMundo(t, w, func(w *world.World) {
		w.ForEachPlaying(-1, func(_ *world.Session, e *world.Entity) {
			for _, it := range e.Carry {
				if !it.Empty() {
					achados[it.Index] = it
				}
			}
		})
	})
	return achados
}

// ClaimNewbieKit espelha o store real: a primeira chamada de cada conta concede,
// as seguintes não.
func (f *fakeDB) ClaimNewbieKit(_ context.Context, accountID int64, _ string) (bool, error) {
	if f.kitNovatoErr != nil {
		return false, f.kitNovatoErr
	}
	if f.kitNovato == nil {
		f.kitNovato = map[int64]bool{}
	}
	if f.kitNovato[accountID] {
		return false, nil
	}
	f.kitNovato[accountID] = true
	return true, nil
}

// novatoDB: um Mortal de nível 1 (personagem novo de verdade), bolsa vazia.
func novatoDB() *fakeDB {
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Novato", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 1,
	}
	return db
}

// esperarMensagem drena até achar um MSG_MessagePanel que contenha trecho.
func esperarMensagem(t *testing.T, c net.Conn, trecho string) string {
	t.Helper()
	for i := 0; i < 40; i++ {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty != protocol.MsgMessagePanel {
			continue
		}
		texto := decodePanel(payload)
		if strings.Contains(texto, trecho) {
			return texto
		}
	}
	t.Fatalf("nenhuma mensagem contendo %q chegou", trecho)
	return ""
}

// TestNovatoEntregaOKit: o kit chega como TRÊS itens empilhados, não nove soltos.
//
// A conta é gasta na entrega, então esta é a única vez que o jogador vê esses
// itens: se a pilha vier errada, ninguém pede de novo para corrigir.
func TestNovatoEntregaOKit(t *testing.T) {
	addr, stop, w := startServerNovato(t, novatoDB())
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()

	whisperFrame(t, c, "/novato", "")
	esperarMensagem(t, c, "Kit de novato entregue")

	achados := bolsaDoJogador(t, w)
	if len(achados) != 3 {
		t.Fatalf("a bolsa ficou com %d itens, queria 3 (frango, baú e montaria)", len(achados))
	}
	if got := itemAmount(achados[itemFrangoNovato]); got != frangosNoKit {
		t.Errorf("frangos = %d, queria %d numa pilha só", got, frangosNoKit)
	}
	if got := itemAmount(achados[itemBauXPNovato]); got != bausNoKit {
		t.Errorf("baús de XP = %d, queria %d numa pilha só", got, bausNoKit)
	}
	shire, ok := achados[itemShireNovato]
	if !ok {
		t.Fatal("a Shire não veio no kit")
	}
	// O tempo vai nos EFEITOS, não em ExpiresAt: é o que faz o relógio começar
	// quando o novato monta, e não enquanto o kit espera guardado.
	if shire.ExpiresAt != 0 {
		t.Errorf("a Shire veio com prazo já correndo (ExpiresAt=%d)", shire.ExpiresAt)
	}
	var dias uint8
	for _, ef := range shire.Effects {
		if ef.Effect == efWDay {
			dias = ef.Value
		}
	}
	if dias != diasDaMontaria {
		t.Errorf("a Shire veio com %d dias, queria %d", dias, diasDaMontaria)
	}
}

// TestNovatoSoUmaVezPorConta: a segunda vez não entrega nada. A trava é da CONTA,
// não do personagem — apagar o personagem e criar outro não devolve o kit.
func TestNovatoSoUmaVezPorConta(t *testing.T) {
	addr, stop, w := startServerNovato(t, novatoDB())
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()

	whisperFrame(t, c, "/novato", "")
	esperarMensagem(t, c, "Kit de novato entregue")

	whisperFrame(t, c, "/novato", "")
	esperarMensagem(t, c, "já recebeu")

	itens := len(bolsaDoJogador(t, w))
	if itens != 3 {
		t.Errorf("a bolsa ficou com %d itens depois do segundo pedido, queria 3", itens)
	}
}

// TestNovatoSoParaMortal: Arch e Celestial vêm de uma conta que já jogou, e o
// kit é de entrada. A recusa é falada — um comando que não responde nada é lido
// como bug pelo jogador.
func TestNovatoSoParaMortal(t *testing.T) {
	addr, stop, w := startServerNovato(t, celestialDB(classMasterArch))
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()

	whisperFrame(t, c, "/novato", "")
	esperarMensagem(t, c, "só para personagens Mortais")

	for idx := range bolsaDoJogador(t, w) {
		t.Fatalf("o Arch recebeu o item %d mesmo com o kit recusado", idx)
	}
}

// TestNovatoComBolsaCheiaVaiProBau: a trava já foi gasta quando a entrega
// começa, então faltar espaço não pode custar o kit — o que não cabe vai para o
// baú da conta, e a mensagem diz isso.
func TestNovatoComBolsaCheiaVaiProBau(t *testing.T) {
	db := novatoDB()
	st := db.loadResult
	// Enche todos os espaços que o servidor considera acessíveis (30 sem a Bolsa
	// do Andarilho).
	for i := 0; i < baseCarrySlots; i++ {
		st.Carry[i] = world.Item{Index: 1100}
	}
	db.loadResult = st

	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()

	whisperFrame(t, c, "/novato", "")
	esperarMensagem(t, c, "foram para o baú")

	cargo := w.Cargo(7)
	if cargo == nil {
		t.Fatal("a conta não tem baú carregado")
	}
	var noBau int
	for _, it := range cargo.Items {
		if !it.Empty() {
			noBau++
		}
	}
	if noBau != 3 {
		t.Errorf("o baú ficou com %d itens, queria os 3 do kit", noBau)
	}
}
