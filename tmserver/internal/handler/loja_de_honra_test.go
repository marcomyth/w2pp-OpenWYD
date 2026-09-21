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

// A Loja de Honra, de ponta a ponta.
//
// O que estes testes guardam é o que não dá para ver na tela: que o clique no
// God of War responde com o NOSSO pacote e não com a janela de loja do cliente,
// que quem paga é o saldo de pontos da conta, e que nenhum dos caminhos de recusa
// entrega item — nem o sem saldo, nem o de longe, nem o do painel que nunca
// abriu. Um item entregue sem cobrança é dinheiro do servidor indo embora, e é o
// tipo de erro que só aparece no extrato semanas depois.

// godOfWarTemplate é o God_of_War como o jogo o traz: Merchant 104 nos dois
// bytes e Carry vazio (Release/TMsrv/run/npc/God_of_War tem 75 no byte 17 e 104
// no 104; o que o roteamento lê é o segundo).
func godOfWarTemplate() []byte {
	tmpl := make([]byte, 816)
	copy(tmpl[0:16], "God_of_War")
	tmpl[17] = merchantGodOfWar
	tmpl[56+12] = merchantGodOfWar
	tmpl[92+12] = merchantGodOfWar
	binary.LittleEndian.PutUint32(tmpl[92+16:], 19000)
	binary.LittleEndian.PutUint32(tmpl[92+24:], 19000)
	return tmpl
}

func startServerHonra(t *testing.T, persist world.Persistence) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)
	if id := w.SpawnMob(godOfWarTemplate(), 5, 5); id != shopNPCID {
		t.Fatalf("o God of War nasceu como %d, esperado %d", id, shopNPCID)
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

// contaComPontos é uma conta com saldo de pontos e a mochila vazia, no lugar que
// o teste pedir.
func contaComPontos(pontos int32, x, y int16) *fakeDB {
	db := newDB()
	db.pontosLojinha = pontos
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Vendedora", X: x, Y: y, HP: 1000, MaxHP: 1000, Coin: 0,
	}
	return db
}

// clicaNoGodOfWar manda o clique que o cliente manda (o CreateMob anuncia
// Merchant 1, então o clique chega como _MSG_REQShopList).
func clicaNoGodOfWar(t *testing.T, c net.Conn) {
	t.Helper()
	pedido := make([]byte, 4)
	binary.LittleEndian.PutUint16(pedido, uint16(shopNPCID))
	send(t, c, protocol.MsgREQShopList, pedido)
}

func compraDeHonra(t *testing.T, c net.Conn, slot int) {
	t.Helper()
	send(t, c, protocol.MsgHonraCompra, (&protocol.HonraCompraBody{Slot: int16(slot)}).Encode())
}

// TestGodOfWarApareceComoLoja: o cliente decide pelo Merchant do CreateMob o que
// o clique manda. O servidor vê 104 — que é como reconhece a loja —, e o cliente
// tem de receber 1, senão o clique cai em _MSG_Quest e nada acontece.
func TestGodOfWarApareceComoLoja(t *testing.T) {
	god := &world.Entity{ID: shopNPCID, Merchant: merchantGodOfWar}
	if got := createMobFrom(god, 0).Merchant; got != 1 {
		t.Errorf("CreateMob do God of War com Merchant %d; o cliente só manda o clique com 1", got)
	}
}

// TestLojaDeHonraAbreComEstoqueESaldo: o clique traz o nosso pacote, com o saldo
// da conta e o estoque, e NÃO traz a janela de loja do cliente — ela escreveria
// preço em ouro.
func TestLojaDeHonraAbreComEstoqueESaldo(t *testing.T) {
	db := contaComPontos(500, 5, 5)
	addr, stop := startServerHonra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	corpo := expect(t, c, protocol.MsgHonraAbre)
	var abre protocol.HonraAbreBody
	if err := abre.Decode(corpo); err != nil {
		t.Fatalf("abertura ilegível: %v", err)
	}
	if abre.Saldo != 500 {
		t.Errorf("saldo na abertura = %d, esperado 500", abre.Saldo)
	}
	if len(abre.Itens) != len(estoqueDaLojaDeHonra) {
		t.Fatalf("a loja abriu com %d itens, esperado %d", len(abre.Itens), len(estoqueDaLojaDeHonra))
	}
	for i, it := range abre.Itens {
		quer := estoqueDaLojaDeHonra[i]
		if it.Slot != int16(i) || it.Indice != quer.Indice || it.Preco != quer.Preco {
			t.Errorf("casa %d = slot %d item %d por %d; esperado slot %d item %d por %d",
				i, it.Slot, it.Indice, it.Preco, i, quer.Indice, quer.Preco)
		}
	}
	// A janela do cliente não pode abrir junto: seriam duas lojas na tela, e a
	// dela cobraria ouro.
	for i := 0; i < 4; i++ {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgShopList {
			t.Fatal("o God of War mandou a janela de loja do cliente junto com o painel")
		}
	}
}

// TestLojaDeHonraTrocaPontosPorItem é o pedido inteiro: dois cliques e o item
// está na mochila, com os pontos debitados da CONTA e o saldo novo de volta no
// painel.
func TestLojaDeHonraTrocaPontosPorItem(t *testing.T) {
	db := contaComPontos(100, 5, 5)
	addr, stop := startServerHonra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	expect(t, c, protocol.MsgHonraAbre)

	// Casa 3 do estoque: Poeira_de_Oriharucon por 40 pontos.
	const casa = 3
	quer := estoqueDaLojaDeHonra[casa]
	compraDeHonra(t, c, casa)

	chegou, saldoNoPainel := false, int32(-1)
	for i := 0; i < 8; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgSendItem:
			if int16(le16(p[4:6])) == quer.Indice {
				chegou = true
			}
		case protocol.MsgHonraSaldo:
			saldo, err := protocol.DecodeHonraSaldo(p)
			if err != nil {
				t.Fatalf("saldo ilegível: %v", err)
			}
			saldoNoPainel = saldo
		}
	}
	if !chegou {
		t.Errorf("o item %d não chegou à mochila", quer.Indice)
	}
	if saldoNoPainel != 100-quer.Preco {
		t.Errorf("saldo devolvido ao painel = %d, esperado %d", saldoNoPainel, 100-quer.Preco)
	}
	if got := db.pontosLojinha0(); got != 100-quer.Preco {
		t.Errorf("carteira da conta = %d, esperado %d", got, 100-quer.Preco)
	}
}

// TestLojaDeHonraSemPontosNaoEntrega: o débito é o que decide, e ele acontece
// ANTES da entrega. Sem saldo o jogador é avisado e nada se move.
func TestLojaDeHonraSemPontosNaoEntrega(t *testing.T) {
	db := contaComPontos(10, 5, 5)
	addr, stop := startServerHonra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	expect(t, c, protocol.MsgHonraAbre)

	const casa = 3 // 40 pontos, e a conta tem 10
	compraDeHonra(t, c, casa)

	avisou, entregou := false, false
	for i := 0; i < 8; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessagePanel {
			avisou = true
		}
		if ty == protocol.MsgSendItem && le16(p[4:6]) != 0 {
			entregou = true
		}
	}
	if !avisou {
		t.Error("sem pontos, o jogador não foi avisado")
	}
	if entregou {
		t.Error("sem pontos, o item foi entregue mesmo assim")
	}
	if got := db.pontosLojinha0(); got != 10 {
		t.Errorf("carteira mexeu numa compra recusada: %d, esperado 10", got)
	}
}

// TestLojaDeHonraLongeDoNPCNaoVende: a loja é do NPC. Com o painel aberto e o
// personagem do outro lado do mapa, a compra é recusada — senão um cliente
// remendado compraria de qualquer lugar do mundo.
func TestLojaDeHonraLongeDoNPCNaoVende(t *testing.T) {
	db := contaComPontos(500, 200, 200) // o God of War está em 5,5
	addr, stop := startServerHonra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	expect(t, c, protocol.MsgHonraAbre)
	compraDeHonra(t, c, 3)

	entregou := false
	for i := 0; i < 8; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem && le16(p[4:6]) != 0 {
			entregou = true
		}
	}
	if entregou {
		t.Error("a compra saiu com o personagem longe do NPC")
	}
	if got := db.pontosLojinha0(); got != 500 {
		t.Errorf("carteira mexeu numa compra de longe: %d, esperado 500", got)
	}
}

// TestLojaDeHonraSemPainelAbertoNaoVende: o pedido de compra sozinho, sem o
// clique que abre a loja, não compra nada. É o que impede uma compra fabricada
// por um cliente remendado que nunca chegou perto do NPC.
func TestLojaDeHonraSemPainelAbertoNaoVende(t *testing.T) {
	db := contaComPontos(500, 5, 5)
	addr, stop := startServerHonra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	compraDeHonra(t, c, 3)
	for i := 0; i < 6; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem && le16(p[4:6]) != 0 {
			t.Fatal("comprou sem nunca ter aberto a loja")
		}
		_ = p
	}
	if got := db.pontosLojinha0(); got != 500 {
		t.Errorf("carteira mexeu sem a loja aberta: %d, esperado 500", got)
	}
}
