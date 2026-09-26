package handler

import (
	"encoding/binary"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// carryCoin lê o Coin que MSG_UpdateCarry carrega no fim do corpo, depois dos 64
// slots (protocol.EncodeUpdateCarryBody).
func carryCoin(t *testing.T, payload []byte) int32 {
	t.Helper()
	off := world.MaxCarry * protocol.ItemSize
	if len(payload) < off+4 {
		t.Fatalf("corpo de UpdateCarry curto demais: %d bytes, esperado ao menos %d", len(payload), off+4)
	}
	return int32(binary.LittleEndian.Uint32(payload[off:]))
}

// TestTrocaAvisaOsDoisLadosDoOuro: o swap tem de terminar com SendCarry nos dois
// lados, e cada um tem de trazer o saldo JÁ acertado.
//
// Este é o teste que faltava quando a troca de gold foi reportada como duplicação
// em jogo: o servidor descontava certo e não mandava nada, então cada cliente
// seguia com a conta que ele próprio tinha feito. Quem pagou continuava vendo o
// ouro (a troca parecia de graça) e quem recebeu via o crédito — a mesma troca
// lida dos dois lados como ouro duplicado. O legado fecha o swap com SendCarry
// nos dois (_MSG_Trade.cpp:335-336) exatamente por isso.
func TestTrocaAvisaOsDoisLadosDoOuro(t *testing.T) {
	addr, stop, _ := startServerClock(t, tradeDB())
	defer stop()
	a := enterWorldAs(t, addr, "tester") // conn 1, 1000 de ouro
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb") // conn 2, 1000 de ouro
	defer b.Close()

	tradeConfirm(t, a, 2, world.Item{Index: 1100}, 0, 100)
	if ty, _, ok := readMaybe(t, a); !ok || ty != protocol.MsgCNFCheck {
		t.Fatalf("confirmação de A = %#x ok=%v, esperado CNFCheck", ty, ok)
	}
	if ty, _, ok := readMaybe(t, b); !ok || ty != protocol.MsgTrade {
		t.Fatalf("B não recebeu a oferta de A: %#x ok=%v", ty, ok)
	}
	tradeConfirm(t, b, 1, world.Item{Index: 2200}, 0, 50)

	// A pagou 100 e recebeu 50; B pagou 50 e recebeu 100.
	pa, _ := readUntil(t, a, protocol.MsgUpdateCarry)
	if got := carryCoin(t, pa); got != 950 {
		t.Errorf("ouro anunciado a A = %d, esperado 950 (1000 - 100 + 50)", got)
	}
	pb, _ := readUntil(t, b, protocol.MsgUpdateCarry)
	if got := carryCoin(t, pb); got != 1050 {
		t.Errorf("ouro anunciado a B = %d, esperado 1050 (1000 - 50 + 100)", got)
	}
}

// tradeMoneyFixture monta os dois lados de um swap sem subir servidor: só o que
// checkTradeMoney lê (Coin das entidades, Money das sessões).
func tradeMoneyFixture(t *testing.T, coinA, coinB, moneyA, moneyB int32) (
	*Dispatcher, *world.World, *world.Session, *world.Session, *world.Entity, *world.Entity,
) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, nil, d.Handle)
	sa := &world.Session{Conn: 1, Mode: world.UserPlay}
	sb := &world.Session{Conn: 2, Mode: world.UserPlay}
	sa.Trade = world.TradeState{Active: true, OpponentID: 2, Confirmed: true, Money: moneyA}
	sb.Trade = world.TradeState{Active: true, OpponentID: 1, Confirmed: true, Money: moneyB}
	ea := &world.Entity{Mode: world.MobUser, Name: "A", Coin: coinA}
	eb := &world.Entity{Mode: world.MobUser, Name: "B", Coin: coinB}
	return d, w, sa, sb, ea, eb
}

// TestTrocaRecusaOuroQueNaoExisteMais: o saldo é conferido quando a oferta é
// montada, mas o jogador continua jogando até a confirmação — comprar num NPC ou
// depositar no baú derruba o ouro prometido na janela. Aplicar assim mesmo deixava
// Coin negativo de um lado e creditava o outro em cheio: ouro criado, sem exploit.
func TestTrocaRecusaOuroQueNaoExisteMais(t *testing.T) {
	d, w, sa, sb, ea, eb := tradeMoneyFixture(t, 50, 1000, 100, 0) // A promete 100, tem 50
	if d.checkTradeMoney(w, sa, sb, ea, eb) {
		t.Fatal("o swap seguiu com ouro que o ofertante não tem mais")
	}
	if ea.Coin != 50 || eb.Coin != 1000 {
		t.Errorf("a recusa mexeu no ouro: A=%d B=%d, esperado 50/1000", ea.Coin, eb.Coin)
	}
	if sa.Trade.Active || sb.Trade.Active {
		t.Error("a recusa deixou a janela de troca aberta")
	}
}

// TestTrocaRecusaAcimaDe2G: Coin é int32 e o legado corta em 2 bilhões
// (_MSG_Trade.cpp:270). Sem o teto a soma estoura o int32 e o saldo de quem
// recebeu vira negativo — uma troca legítima que apaga uma fortuna.
func TestTrocaRecusaAcimaDe2G(t *testing.T) {
	const quaseDoisG = 1_999_999_000
	d, w, sa, sb, ea, eb := tradeMoneyFixture(t, 1_000_000, quaseDoisG, 2_000, 0)
	if d.checkTradeMoney(w, sa, sb, ea, eb) {
		t.Fatal("o swap seguiu levando B acima do teto de 2 bilhões")
	}
	if eb.Coin != quaseDoisG {
		t.Errorf("ouro de B = %d, esperado intocado em %d", eb.Coin, quaseDoisG)
	}
}

// TestTrocaDeOuroValidaPassa guarda o caminho normal: o que cabe nos dois lados
// não pode ser recusado pelas guardas acima.
func TestTrocaDeOuroValidaPassa(t *testing.T) {
	d, w, sa, sb, ea, eb := tradeMoneyFixture(t, 1000, 1000, 1000, 250)
	if !d.checkTradeMoney(w, sa, sb, ea, eb) {
		t.Fatal("uma troca de ouro perfeitamente válida foi recusada")
	}
}
