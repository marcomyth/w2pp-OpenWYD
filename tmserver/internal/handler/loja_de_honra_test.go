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

// godOfWarTemplate é o God_of_War JÁ MARCADO como loja de honra, que é o estado
// em que ele chega ao mundo: o arquivo traz 104 (Release/TMsrv/run/npc/God_of_War
// tem 75 no byte 17 e 104 no 104), e marcaLojaDeHonra troca por 201 no
// nascimento. Quem prova essa troca é TestSoOGodOfWarViraLojaDeHonra.
func godOfWarTemplate() []byte {
	tmpl := make([]byte, 816)
	copy(tmpl[0:16], "God_of_War")
	tmpl[17] = merchantLojaDeHonra
	tmpl[56+12] = merchantLojaDeHonra
	tmpl[92+12] = merchantLojaDeHonra
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

// startServerHonraAndando é o arranque para os testes que ANDAM: mapa grande o
// bastante para caber a caminhada e relógio na mão do teste, porque a trava
// anti-speedhack compara o tique que o cliente manda com o relógio do servidor.
func startServerHonraAndando(t *testing.T, persist world.Persistence) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	relogio := func() uint32 { return serverTime }
	w := world.New(world.Config{GridDim: 512, Now: relogio}, log, persist, d.Handle)
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
// o clique manda. O servidor vê o Merchant da loja, e o cliente tem de receber 1,
// senão o clique cai em _MSG_Quest e nada acontece.
func TestGodOfWarApareceComoLoja(t *testing.T) {
	god := &world.Entity{ID: shopNPCID, Merchant: merchantLojaDeHonra}
	if got := createMobFrom(god, 0).Merchant; got != 1 {
		t.Errorf("CreateMob do God of War com Merchant %d; o cliente só manda o clique com 1", got)
	}
}

// TestSoOGodOfWarViraLojaDeHonra é o teste que faltava quando a loja foi escrita, e
// a falta dele custou renomear dois NPCs no banco de teste: o Merchant 104 do
// template do God_of_War é compartilhado com o Treinador2 e o Uxmal, então ele não
// identifica loja nenhuma. Quem identifica é o template.
func TestSoOGodOfWarViraLojaDeHonra(t *testing.T) {
	casos := []struct {
		template string
		vira     bool
	}{
		{"God_of_War", true},
		{"god_of_war", true}, // o painel não garante a caixa do nome
		{" God_of_War ", true},
		{"Treinador2", false},
		{"Uxmal", false},
		{"", false},
	}
	for _, c := range casos {
		// Nasce como o arquivo dele traz: 104, o Merchant compartilhado.
		e := &world.Entity{ID: shopNPCID, Merchant: 104}
		marcaLojaDeHonra(e, c.template)
		if ehLojaDeHonra(e) != c.vira {
			t.Errorf("template %q: virou loja de honra = %v, esperado %v",
				c.template, ehLojaDeHonra(e), c.vira)
		}
		if !c.vira && e.Merchant != 104 {
			t.Errorf("template %q: o Merchant mudou para %d sem precisar", c.template, e.Merchant)
		}
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
	// O painel escreve "a cada 15 min com a lojinha aberta: +3 pontos" com estes
	// dois números; sem eles ele teria de repetir a regra do servidor por conta
	// própria, e mentiria no dia em que ela mudasse.
	if abre.PorJanela != shopPointsBase {
		t.Errorf("ganho por janela = %d, esperado %d", abre.PorJanela, shopPointsBase)
	}
	if abre.MinutosJanela != shopPointsWindowMs/60000 {
		t.Errorf("janela de %d min, esperado %d", abre.MinutosJanela, shopPointsWindowMs/60000)
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
		// A aba viaja com o item: o cliente não tem catálogo para deduzi-la.
		if it.Categoria != quer.Cat {
			t.Errorf("casa %d na categoria %d, esperado %d", i, it.Categoria, quer.Cat)
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

// TestEstoqueDeHonraTemCategoriaConhecida: a aba de cada item é escrita a mão na
// tabela, e um número fora das quatro abas faria o item desaparecer de todas menos
// a Todos. É o tipo de erro de digitação que não aparece na tela.
func TestEstoqueDeHonraTemCategoriaConhecida(t *testing.T) {
	for i, it := range estoqueDaLojaDeHonra {
		switch it.Cat {
		case protocol.HonraCatNenhuma, protocol.HonraCatArmas,
			protocol.HonraCatSet, protocol.HonraCatConsumo:
		default:
			t.Errorf("casa %d (item %d) tem categoria %d, que não é aba nenhuma",
				i, it.Indice, it.Cat)
		}
		if it.Preco <= 0 {
			t.Errorf("casa %d (item %d) custa %d pontos", i, it.Indice, it.Preco)
		}
	}
}

// TestLojaDeHonraContaOGanhoDaFadaAzul: o número que o painel escreve é o DESTE
// jogador. Quem está com uma Fada Azul ganha 7 por quinze minutos, e é 7 que tem
// de aparecer — o cliente não tem como saber da fada sozinho.
func TestLojaDeHonraContaOGanhoDaFadaAzul(t *testing.T) {
	db := contaComPontos(0, 5, 5)
	st := db.loadResult
	st.Equip[fairyEquipSlot] = world.Item{Index: 3901} // Fada_Azul(3dias)
	db.loadResult = st
	addr, stop := startServerHonra(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	var abre protocol.HonraAbreBody
	if err := abre.Decode(expect(t, c, protocol.MsgHonraAbre)); err != nil {
		t.Fatalf("abertura ilegível: %v", err)
	}
	if abre.PorJanela != shopPointsFairy {
		t.Errorf("com Fada Azul o ganho por janela = %d, esperado %d",
			abre.PorJanela, shopPointsFairy)
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

// TestLojaDeHonraFechaQuandoOJogadorSeAfasta: andar com a loja aberta vale, e até
// o NPC sair de vista a loja continua de pé — é o que o jogo faz com as lojas dele
// (_MSG_Buy.cpp:60 responde com _MSG_CloseShop a uma compra fora da vista). Passou
// disso, o servidor manda fechar e a compra para de valer.
func TestLojaDeHonraFechaQuandoOJogadorSeAfasta(t *testing.T) {
	db := contaComPontos(500, 5, 5)
	addr, stop := startServerHonraAndando(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	clicaNoGodOfWar(t, c)
	expect(t, c, protocol.MsgHonraAbre)

	// Trinta tiles: longe, mas ainda na vista (VIEWGRID é 33). A loja fica.
	actionFrame(t, c, serverTime, 35)
	for i := 0; i < 6; i++ {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgHonraFechou {
			t.Fatal("a loja fechou com o NPC ainda na vista")
		}
	}

	// Mais trinta: agora o NPC ficou para trás.
	actionFrame(t, c, serverTime, 65)
	fechou := false
	for i := 0; i < 8 && !fechou; i++ {
		ty, _, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgHonraFechou {
			fechou = true
		}
	}
	if !fechou {
		t.Fatal("o jogador saiu de vista e a loja não fechou")
	}

	// E a compra não vale mais: o servidor esqueceu o NPC.
	compraDeHonra(t, c, 3)
	for i := 0; i < 6; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem && le16(p[4:6]) != 0 {
			t.Fatal("comprou depois de a loja ter fechado por distância")
		}
	}
	if got := db.pontosLojinha0(); got != 500 {
		t.Errorf("carteira mexeu depois do fechamento: %d, esperado 500", got)
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
