package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As duas portas que mexem no baú DIRETAMENTE, sem passar pelo `itemSlot`, e que
// por isso precisam da recusa escrita nelas: a montagem da barraca e a compra
// pela janela antiga. A compra pelo painel tem a dela em anuncio_rmt_test.go.
//
// O caminho de verdade é o primeiro: a barraca em dinheiro real caiu, o anúncio
// dela continua de pé, e o vendedor monta tudo de novo — em OURO, que é a moeda
// que ele sempre usou. Sem a recusa, o mesmo item é vendido duas vezes: uma por
// ouro aqui, e outra pelo Pix que já estava anunciado.

// NÃO SE MONTA BARRACA COM ITEM JÁ PRESO NUM ANÚNCIO.
func TestBarracaRecusaItemJaAnunciado(t *testing.T) {
	db := bancoDeAnuncio()
	cargo := db.accounts["tester"].cargo
	cargo.Items[0].AnuncioRMT = 777 // anúncio de uma barraca anterior, ainda vivo
	db.accounts["tester"].cargo = cargo

	addr, stop, w := startServerNovato(t, db)
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	mandaAbrirBarraca(t, c, "Loja", 0, 1000, protocol.LojaMoedaOuro)

	if !recebeu(t, c, msgItemJaAnunciado) {
		t.Error("recusou em silencio, ou deixou montar")
	}
	drena(t, c)
	if v := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos); v.Qtd != 0 {
		t.Errorf("a barraca subiu com %d prateleira(s) sobre um item ja vendido", v.Qtd)
	}
	if bau := bauDoVendedor(t, w); bau.Items[0].AnuncioRMT != 777 {
		t.Errorf("a marca do anuncio mudou para %d", bau.Items[0].AnuncioRMT)
	}
}

// E o contrário, para a recusa não virar uma trava que nunca deixa vender: item
// SEM marca monta normalmente.
func TestBarracaSobeComItemSemMarca(t *testing.T) {
	addr, stop, _ := startServerNovato(t, bancoDeAnuncio())
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	abreBarraca(t, c, "Loja", 0, 1000, protocol.LojaMoedaOuro)

	if v := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos); v.Qtd != 1 {
		t.Errorf("a vitrine tem %d oferta(s), quero 1", v.Qtd)
	}
}

// A JANELA ANTIGA TAMBÉM NÃO LEVA ITEM PRESO.
//
// Com a recusa na montagem, esta é uma segunda camada: hoje a marca não aparece
// mais no meio de uma barraca aberta. Ela existe porque as duas conferências
// anti-adulteração do `_MSG_ReqBuy` comparam índice e efeitos, e a marca não é
// nem um nem outro — o item CONFERE e mesmo assim já tem dono. Quem for ligar a
// retirada imediata do vendedor online (que mexe no baú com a barraca de pé) vai
// esbarrar exatamente aqui.
func TestJanelaAntigaNaoLevaItemPresoNoEscrow(t *testing.T) {
	const sellItem = int16(1030)
	const price, tax = int32(50_000), int32(5)
	addr, stop, w := startServerNovato(t, autotradeDB(sellItem))
	defer stop()
	seller := enterWorldAs(t, addr, "tester")
	defer seller.Close()
	buyer := enterWorldAs(t, addr, "tradeb")
	defer buyer.Close()

	abreBarraca(t, seller, "Loja", 0, price, protocol.LojaMoedaOuro)

	// A marca chega DEPOIS de a barraca subir, e por dentro do laço, que é quem é
	// dono do baú. É o único jeito de montar este caso agora que a montagem
	// recusa item marcado.
	noLacoDoMundo(t, w, func(w *world.World) {
		if c := w.Cargo(7); c != nil {
			c.Items[0].AnuncioRMT = 888
		}
	})
	drena(t, buyer)

	send(t, buyer, protocol.MsgReqBuy, reqBuyPayload(1, 0, sellItem, price, tax))

	for {
		ty, _, ok := readMaybe(t, buyer)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem {
			t.Fatal("a janela antiga entregou um item preso num anuncio em dinheiro real")
		}
	}
	noLacoDoMundo(t, w, func(w *world.World) {
		c := w.Cargo(7)
		if c == nil || c.Items[0].Empty() {
			t.Error("o item saiu do bau do vendedor")
		}
	})
}

// A CONEXÃO É REAPROVEITADA, E POR ISSO A VOLTA DO BANCO CONFERE A CONTA.
//
// O `allocConn` entrega o primeiro número livre. Se o vendedor cai enquanto o
// pedido está no banco, o número dele volta para a fila e a próxima pessoa que
// entrar recebe o MESMO. Procurar a sessão só pelo número devolveria a sessão
// dela.
//
// O estrago não é teórico: a volta conferiria o baú da conta errada e, se por
// azar o item batesse — e aqui bate de propósito, é o mesmo índice —, marcaria o
// baú do segundo com o anúncio do primeiro e subiria uma barraca que ele não
// pediu, na sessão dele.
func TestVoltaDoBancoNaoPegaOutraContaNaMesmaConexao(t *testing.T) {
	db := bancoDeAnuncio()
	db.portaoAnuncio = make(chan struct{})
	// O intruso tem o MESMO item no MESMO slot. É o caso ruim: sem a conferência
	// de conta, todas as outras validações passariam.
	var cargoIntruso world.CargoState
	cargoIntruso.Items[0] = world.Item{Index: 1030}
	cargoIntruso.Items[0].Effects[0] = world.Effect{Effect: 3, Value: 9}
	cargoIntruso.Items[0].Effects[1] = world.Effect{Effect: 4, Value: 11}
	db.accounts["tradeb"].cargo = cargoIntruso
	db.loads[11] = world.CharacterState{Slot: 0, Name: "Intruso", Level: 1, HP: 1000, MaxHP: 1000}

	addr, stop, w := startServerNovato(t, db)
	defer stop()

	vendedor := enterWorldAs(t, addr, "tester")
	drena(t, vendedor)
	mandaAbrirBarraca(t, vendedor, "Loja", 0, 5000, protocol.LojaMoedaRMT)

	// O vendedor cai com o pedido preso no banco. Espera a sessão realmente sair
	// do mundo, senão o número não volta para a fila.
	esperaConn(t, w, 1, false)
	_ = vendedor.Close()
	esperaConn(t, w, 1, true)

	// O intruso entra e recebe o número 1, que era do vendedor.
	intruso := enterWorldAs(t, addr, "tradeb")
	defer intruso.Close()
	drena(t, intruso)

	close(db.portaoAnuncio) // a volta do banco acontece agora

	// Nada do intruso é tocado, e a barraca do vendedor não sobe na sessão dele.
	esperaCancelamento(t, db)
	noLacoDoMundo(t, w, func(w *world.World) {
		c := w.Cargo(11)
		if c == nil {
			t.Fatal("o bau do intruso nao foi carregado")
		}
		if c.Items[0].AnuncioRMT != 0 {
			t.Errorf("o bau do intruso foi marcado com o anuncio %d do vendedor",
				c.Items[0].AnuncioRMT)
		}
	})
	if v := pedeVitrine(t, intruso, 0, protocol.LojaFiltroTodos); v.Qtd != 0 {
		t.Errorf("subiu %d prateleira(s) na sessao de quem nao pediu barraca nenhuma", v.Qtd)
	}
}

// esperaConn espera a conexão existir (vazia=false) ou sumir (vazia=true) do
// mundo. A troca de dono de um número de conexão é assíncrona, e sem esperar o
// teste corre contra o laço.
func esperaConn(t *testing.T, w *world.World, conn int, vazia bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		var tem bool
		noLacoDoMundo(t, w, func(w *world.World) {
			w.ForEachSession(func(s *world.Session, _ *world.Entity) {
				if s != nil && s.Conn == conn {
					tem = true
				}
			})
		})
		if tem != vazia {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("a conexao %d nao ficou vazia=%v a tempo", conn, vazia)
}

// esperaCancelamento espera a compensação chegar ao banco. Ela roda por
// GoDetached, então não é síncrona com o fechamento do portão.
func esperaCancelamento(t *testing.T, db *fakeDB) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if len(db.cancelados()) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("os anuncios nasceram e ninguem os cancelou; eles prendem o slot para sempre")
}
