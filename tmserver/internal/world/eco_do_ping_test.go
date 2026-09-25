package world

import (
	"io"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// TestOPingVoltaParaQuemMandou é o contrato com o cliente do jogo.
//
// O GamePatch 1.0.2 manda o ping depois de 15 s sem receber NENHUM pacote e usa o eco
// para saber que o servidor está vivo. Na recepção ele olha só o TIPO, mas a resposta
// é o cabeçalho sozinho: 12 bytes, corpo vazio.
func TestOPingVoltaParaQuemMandou(t *testing.T) {
	addr, stop := testWorld(t, nil, NopPersistence{})
	defer stop()
	c := dialClient(t, addr)
	defer c.Close()

	sendFrame(t, c, protocol.Header{Type: protocol.MsgPing, ID: 7}, nil)

	h, corpo := readFrame(t, c)
	if h.Type != protocol.MsgPing {
		t.Fatalf("tipo = %#x, queria %#x", h.Type, protocol.MsgPing)
	}
	if len(corpo) != 0 {
		t.Errorf("o eco veio com %d bytes de corpo, queria vazio", len(corpo))
	}
	if h.ID != 7 {
		t.Errorf("id = %d, queria 7 — o eco devolve o id que veio", h.ID)
	}
}

// TestOEcoNaoVaiParaOutraSessao: o eco só serve a quem mandou o ping. Mandá-lo a
// outra conexão ligaria a regra de silêncio dela sem ela ter pedido — e a partir daí
// 40 s sem pacote viram queda anunciada.
func TestOEcoNaoVaiParaOutraSessao(t *testing.T) {
	addr, stop := testWorld(t, nil, NopPersistence{})
	defer stop()
	quemManda := dialClient(t, addr)
	defer quemManda.Close()
	oOutro := dialClient(t, addr)
	defer oOutro.Close()

	sendFrame(t, quemManda, protocol.Header{Type: protocol.MsgPing, ID: 1}, nil)
	if h, _ := readFrame(t, quemManda); h.Type != protocol.MsgPing {
		t.Fatalf("quem mandou recebeu %#x", h.Type)
	}
	// E o outro não recebe nada. Um prazo curto basta: o eco sai na chegada.
	_ = oOutro.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var sz [2]byte
	if _, err := io.ReadFull(oOutro, sz[:]); err == nil {
		t.Error("a outra sessão recebeu um quadro que não pediu")
	}
}

// TestOPingNaoChegaAoDispatcher: ele é tratado no laço e não vira quadro de jogo. Se
// chegasse ao dispatcher, um ping viraria uma mensagem desconhecida a cada 15 s de
// silêncio.
func TestOPingNaoChegaAoDispatcher(t *testing.T) {
	chegou := make(chan protocol.Type, 4)
	h := func(_ *World, _ *Session, hd protocol.Header, _ []byte) {
		select {
		case chegou <- hd.Type:
		default:
		}
	}
	addr, stop := testWorld(t, h, NopPersistence{})
	defer stop()
	c := dialClient(t, addr)
	defer c.Close()

	sendFrame(t, c, protocol.Header{Type: protocol.MsgPing, ID: 1}, nil)
	if hd, _ := readFrame(t, c); hd.Type != protocol.MsgPing {
		t.Fatalf("eco = %#x", hd.Type)
	}
	select {
	case ty := <-chegou:
		t.Errorf("o ping chegou ao dispatcher como %#x", ty)
	default:
	}
}
