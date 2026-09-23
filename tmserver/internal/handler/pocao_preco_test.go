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

// O PREÇO DA PILHA (pedido de 22/09/2026).
//
// Em todo o resto do jogo o preço de vitrine é por COMPRA: o legado lê
// g_pItemList[índice].Price e não olha a quantidade (_MSG_Buy.cpp:104), de modo
// que uma pilha de dez sai pelo preço de uma. São 252 vagas em 46 lojas assim
// hoje, e mexer nisso de uma vez reprecificaria o jogo inteiro.
//
// As duas poções de 500 divergem, e só elas: a pilha paga por unidade. Estes
// testes guardam as duas metades da regra — que a poção multiplica E que o resto
// do jogo continua não multiplicando.

// lojaComItem sobe um servidor com UM mercador vendendo index na vaga 0, em
// pilha de qtd (qtd 0 = sem EF_AMOUNT).
func lojaComItem(t *testing.T, persist world.Persistence, prices map[int]int32, index int, qtd int) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, ItemPrices: prices})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)

	tmpl := make([]byte, 816)
	copy(tmpl[0:16], "ShopKeeper")
	tmpl[92+12] = 1                                  // CurrentScore.Merchant = loja normal
	binary.LittleEndian.PutUint32(tmpl[92+16:], 100) // MaxHp
	binary.LittleEndian.PutUint32(tmpl[92+24:], 100) // Hp
	binary.LittleEndian.PutUint16(tmpl[268:], uint16(index))
	if qtd > 0 {
		tmpl[268+2] = 61 // EF_AMOUNT
		tmpl[268+3] = byte(qtd)
	}
	if id := w.SpawnMob(tmpl, 5, 5); id != shopNPCID {
		t.Fatalf("o mercador nasceu com id %d, queria %d", id, shopNPCID)
	}

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
	}
}

// compraECobra faz uma compra e devolve quanto de ouro saiu.
func compraECobra(t *testing.T, addr string, ouroInicial int32) int32 {
	t.Helper()
	c := enterWorld(t, addr)
	defer c.Close()
	buyFrame(t, c, shopNPCID, 0, 3)
	echo := expect(t, c, protocol.MsgBuy)
	return ouroInicial - int32(le(echo[8:12]))
}

// TestPocaoEmPilhaPagaPorUnidade: a pilha de 120 da Ultra Poção de Cura custa
// 120 × 2.000. Sem isso a poção de 500 sai a dezesseis de ouro.
func TestPocaoEmPilhaPagaPorUnidade(t *testing.T) {
	const (
		ouro  = 1_000_000
		preco = 2000
		pilha = 120
	)
	for _, pocao := range []int{ultraPocaoCura, ultraPocaoMana} {
		addr, stop := lojaComItem(t, shopDB(ouro), map[int]int32{pocao: preco}, pocao, pilha)
		gasto := compraECobra(t, addr, ouro)
		stop()
		if want := int32(preco * pilha); gasto != want {
			t.Errorf("poção %d: a pilha de %d custou %d, esperava %d", pocao, pilha, gasto, want)
		}
	}
}

// TestPocaoAvulsaPagaOPrecoCheio: sem pilha, o preço é o do catálogo — a regra
// multiplica, não substitui.
func TestPocaoAvulsaPagaOPrecoCheio(t *testing.T) {
	const (
		ouro  = 10_000
		preco = 2000
	)
	addr, stop := lojaComItem(t, shopDB(ouro), map[int]int32{ultraPocaoCura: preco}, ultraPocaoCura, 0)
	defer stop()
	if gasto := compraECobra(t, addr, ouro); gasto != preco {
		t.Errorf("a poção avulsa custou %d, esperava %d", gasto, preco)
	}
}

// TestOutroItemEmPilhaContinuaPagandoUmaVez é a outra metade da regra, e a que
// impede que a divergência escorra: uma ração de sessenta, um pergaminho de dez,
// um Pedido de Caça — nada disso pode ter mudado de preço.
func TestOutroItemEmPilhaContinuaPagandoUmaVez(t *testing.T) {
	const (
		ouro  = 1_000_000
		preco = 3000
	)
	// 2420 é a Ração de Porco, que quatro lojas vendem em pilha de sessenta.
	const racaoDePorco = 2420
	addr, stop := lojaComItem(t, shopDB(ouro), map[int]int32{racaoDePorco: preco}, racaoDePorco, 60)
	defer stop()
	if gasto := compraECobra(t, addr, ouro); gasto != preco {
		t.Errorf("a ração em pilha de 60 custou %d, esperava %d (o preço de uma compra)", gasto, preco)
	}
}

// TestPocaoCaraDemaisNaoDaAVolta: preço alto vezes pilha grande estoura o int32
// do Coin, e o estouro é PERIGOSO do lado positivo.
//
// O preço é escolhido a dedo: 35.791.395 × 120 = 4.294.967.400, que truncado a
// 32 bits vira 104. Sem o teto, a pilha de 120 poções de 500 sairia por cento e
// quatro moedas — e o guard de preço negativo que já existia NÃO pega este caso,
// que é justamente por isso que o teto existe.
func TestPocaoCaraDemaisNaoDaAVolta(t *testing.T) {
	const (
		ouro  = 2_000_000_000
		preco = 35_791_395
	)
	addr, stop := lojaComItem(t, shopDB(ouro), map[int]int32{ultraPocaoCura: preco}, ultraPocaoCura, 120)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()
	buyFrame(t, c, shopNPCID, 0, 3)
	if ty, payload, ok := readMaybe(t, c); ok {
		gasto := int32(ouro)
		if ty == protocol.MsgBuy && len(payload) >= 12 {
			gasto = ouro - int32(le(payload[8:12]))
		}
		t.Errorf("a compra impossível foi aceita (%#x) e cobrou %d; devia ser recusada", ty, gasto)
	}
}
