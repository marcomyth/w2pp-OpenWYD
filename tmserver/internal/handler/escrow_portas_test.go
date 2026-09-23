package handler

import (
	"testing"

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
