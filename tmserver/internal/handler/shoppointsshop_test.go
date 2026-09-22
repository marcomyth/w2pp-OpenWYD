package handler

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// carteira é uma persistência de mentira com saldo de pontos de verdade: ela
// debita, recusa quando não cobre e guarda todo movimento. Os testes olham o
// extrato, não só o que chegou ao cliente — uma compra que entrega o item sem
// cobrar passa em qualquer asserção de socket.
type carteira struct {
	*fakeDB
	mu     sync.Mutex
	saldo  int32
	movtos []int32 // deltas na ordem em que ocorreram (negativo = gasto)
}

func novaCarteira(saldo int32, coin int32) *carteira {
	return &carteira{fakeDB: shopDB(coin), saldo: saldo}
}

func (c *carteira) SpendShopPoints(_ context.Context, _ int64, cost int32, _, _ string) (int32, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cost > c.saldo {
		return 0, false, nil
	}
	c.saldo -= cost
	c.movtos = append(c.movtos, -cost)
	return c.saldo, true, nil
}

func (c *carteira) AddShopPoints(_ context.Context, _ int64, delta int32, _, _ string) (int32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saldo += delta
	c.movtos = append(c.movtos, delta)
	return c.saldo, nil
}

func (c *carteira) extrato() (int32, []int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saldo, append([]int32(nil), c.movtos...)
}

// startServerLojaDePontos sobe a loja do shop_test com o slot 0 cobrado em
// pontos. O preço é escrito ANTES de w.Serve: mexer na entidade depois seria
// corrida com o laço, que é dono único do mundo.
func startServerLojaDePontos(t *testing.T, persist world.Persistence, custo int32) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, ItemPrices: map[int]int32{1100: 5_000_000}, ItemNames: map[int]string{1100: "Poção"}})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)

	tmpl := make([]byte, 816)
	copy(tmpl[0:16], "Pontos")
	tmpl[92+12] = 1                                  // Merchant = loja normal
	binary.LittleEndian.PutUint32(tmpl[92+16:], 100) // MaxHp
	binary.LittleEndian.PutUint32(tmpl[92+24:], 100) // Hp
	binary.LittleEndian.PutUint16(tmpl[268:], 1100)  // Carry[0].sIndex
	id := w.SpawnMob(tmpl, 5, 5)
	if id != shopNPCID {
		t.Fatalf("NPC subiu com id %d, quer %d", id, shopNPCID)
	}
	w.Entity(id).ShopPointPrice = map[int]int32{0: custo}

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

// O item custa 50 pontos e o preço de catálogo é 5 milhões de ouro, que o
// jogador NÃO tem: é assim que se vê que a moeda trocou de verdade, e não que a
// compra em ouro passou por acaso.
func TestCompraEmPontosDebitaEEntrega(t *testing.T) {
	db := novaCarteira(200, 1234)
	addr, stop := startServerLojaDePontos(t, db, 50)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	buyFrame(t, c, shopNPCID, 0, 3)

	echo := expect(t, c, protocol.MsgBuy)
	if got := int32(le(echo[8:12])); got != 1234 {
		t.Errorf("ouro no eco = %d, quer 1234 intocado: a compra foi em pontos", got)
	}
	expect(t, c, protocol.MsgUpdateEtc)
	item := expect(t, c, protocol.MsgSendItem)
	if slot := le16(item[2:4]); slot != 3 {
		t.Errorf("slot do item = %d, quer 3", slot)
	}
	if idx := le16(item[4:6]); idx != 1100 {
		t.Errorf("índice do item = %d, quer 1100", idx)
	}

	saldo, movtos := db.extrato()
	if saldo != 150 {
		t.Errorf("saldo = %d, quer 150 (200 - 50)", saldo)
	}
	if len(movtos) != 1 || movtos[0] != -50 {
		t.Errorf("extrato = %v, quer exatamente um gasto de -50", movtos)
	}
}

// Sem saldo: nada de item e nada de movimento na carteira. O importante aqui é o
// item — um servidor que cobra errado é um problema, um que entrega de graça é
// outro bem maior.
func TestCompraEmPontosSemSaldoNaoEntrega(t *testing.T) {
	db := novaCarteira(10, 1234)
	addr, stop := startServerLojaDePontos(t, db, 50)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	buyFrame(t, c, shopNPCID, 0, 3)

	// A recusa chega como texto; o que não pode chegar é MSG_SendItem.
	prazo := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(prazo) {
		tipo, ok := lerTipoOuDesistir(c, 300*time.Millisecond)
		if !ok {
			break
		}
		if tipo == protocol.MsgSendItem || tipo == protocol.MsgBuy {
			t.Fatalf("recebeu %#x sem ter pontos: a compra não podia ter sido entregue", tipo)
		}
	}
	saldo, movtos := db.extrato()
	if saldo != 10 || len(movtos) != 0 {
		t.Errorf("carteira = %d com movimentos %v, quer 10 e nenhum movimento", saldo, movtos)
	}
}

// Preço zero é entrega direta, sem ida ao banco: o banco recusa custo <= 0, e
// mandá-lo para lá trocaria um item de graça por uma mensagem de erro.
func TestCompraEmPontosPrecoZeroEntregaSemDebitar(t *testing.T) {
	db := novaCarteira(0, 1234)
	addr, stop := startServerLojaDePontos(t, db, 0)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	buyFrame(t, c, shopNPCID, 0, 3)
	expect(t, c, protocol.MsgBuy)
	expect(t, c, protocol.MsgUpdateEtc)
	item := expect(t, c, protocol.MsgSendItem)
	if idx := le16(item[4:6]); idx != 1100 {
		t.Fatalf("índice do item = %d, quer 1100", idx)
	}
	if _, movtos := db.extrato(); len(movtos) != 0 {
		t.Errorf("extrato = %v, quer vazio: preço zero não é débito", movtos)
	}
}

// lerTipoOuDesistir lê um quadro dentro do prazo e devolve o tipo, ou desiste.
// Diferente de expect/readFrameHeader, um prazo estourado aqui é resposta, não
// falha: é exatamente assim que se prova que NADA chegou.
func lerTipoOuDesistir(c net.Conn, prazo time.Duration) (protocol.Type, bool) {
	_ = c.SetReadDeadline(time.Now().Add(prazo))
	var sz [2]byte
	if _, err := io.ReadFull(c, sz[:]); err != nil {
		return 0, false
	}
	buf := make([]byte, binary.LittleEndian.Uint16(sz[:]))
	copy(buf, sz[:])
	if _, err := io.ReadFull(c, buf[2:]); err != nil {
		return 0, false
	}
	h, _, _, err := protocol.Decode(buf)
	if err != nil {
		return 0, false
	}
	return h.Type, true
}
