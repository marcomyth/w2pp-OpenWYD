package handler

import (
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O cliente manda o pacote do Mestre Grifo duas vezes por viagem: Ty 1 na partida
// e Ty 2 no pouso (log de produção, 17/09). O Ty 2 não pode abrir outra viagem.

func servidorDoGrifo(t *testing.T) net.Conn {
	t.Helper()
	st := world.CharacterState{
		Slot: 0, Name: "Hero", Level: 148, X: 2113, Y: 2079,
		HP: 2000, MaxHP: 1000, LastCity: 0, ClassMaster: classMasterMortal,
	}
	addr, stop, _ := startServerMestreGrifo(t, st, true)
	t.Cleanup(stop)
	c := enterWorld(t, addr)
	t.Cleanup(func() { c.Close() })
	return c
}

// pulosAte conta os teleportes do próprio avatar que chegam no prazo.
func pulosAte(t *testing.T, c net.Conn, prazo time.Duration) int {
	t.Helper()
	n := 0
	quadroAte(t, c, prazo, func(h protocol.Header, p []byte) bool {
		if ehPulo(h, p) {
			n++
		}
		return false
	})
	return n
}

// TestGrifoPousoNaoAbreOutraViagem é o defeito visto em produção: o pouso que chega
// depois do teleporte abria uma segunda viagem, e dez segundos depois o jogador era
// puxado de volta ao ponto de chegada.
func TestGrifoPousoNaoAbreOutraViagem(t *testing.T) {
	withMasterGriffTravelDelay(t, 300*time.Millisecond)
	c := servidorDoGrifo(t)

	send(t, c, protocol.MsgMasterGriff, protocol.EncodeStandardParm2(2, 1))
	if n := pulosAte(t, c, 1500*time.Millisecond); n != 1 {
		t.Fatalf("a viagem teleportou %d vezes, quero 1", n)
	}
	send(t, c, protocol.MsgMasterGriff, protocol.EncodeStandardParm2(2, masterGriffLanding))
	if n := pulosAte(t, c, 1500*time.Millisecond); n != 0 {
		t.Errorf("o pouso depois da chegada teleportou %d vez(es); puxa de volta quem já andou", n)
	}
}

// TestGrifoPousoAntesDoTempoLevaNaHora: se o pouso chega antes do tempo de reserva,
// leva na hora, e o tempo de reserva não leva de novo.
func TestGrifoPousoAntesDoTempoLevaNaHora(t *testing.T) {
	withMasterGriffTravelDelay(t, 1500*time.Millisecond)
	c := servidorDoGrifo(t)

	send(t, c, protocol.MsgMasterGriff, protocol.EncodeStandardParm2(2, 1))
	send(t, c, protocol.MsgMasterGriff, protocol.EncodeStandardParm2(2, masterGriffLanding))
	if n := pulosAte(t, c, 700*time.Millisecond); n != 1 {
		t.Fatalf("o pouso antes do tempo teleportou %d vezes em 0,7 s, quero 1 na hora", n)
	}
	if n := pulosAte(t, c, 2*time.Second); n != 0 {
		t.Errorf("o tempo de reserva teleportou de novo (%d)", n)
	}
}

// TestGrifoPousoSemViagemNaoFazNada: um pouso sem partida não teleporta ninguém.
func TestGrifoPousoSemViagemNaoFazNada(t *testing.T) {
	withMasterGriffTravelDelay(t, 200*time.Millisecond)
	c := servidorDoGrifo(t)

	send(t, c, protocol.MsgMasterGriff, protocol.EncodeStandardParm2(2, masterGriffLanding))
	if n := pulosAte(t, c, time.Second); n != 0 {
		t.Errorf("um pouso sem viagem teleportou %d vez(es)", n)
	}
}
