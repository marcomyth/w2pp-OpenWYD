package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Compra pela Loja do Servidor.
//
// A compra do jogo (MSG_ReqBuy) só sabe de ouro e confere preço e item que o
// CLIENTE mandou. Aqui é o contrário: o painel manda apenas qual barraca e qual
// posição, e o servidor lê da própria barraca o item, o preço e a moeda — nada
// do que o cliente diz sobre valor é aceito.
//
// As regras de compra continuam as do jogo, e de propósito: a barraca precisa
// estar ao alcance (as lojinhas ficam na cidade, e a vitrine já marca o que está
// perto), o item precisa continuar no Cargo do vendedor, e o imposto da cidade
// incide a partir do mesmo piso.
//
// Ouro está inteiro. Cash e RMT passam por saldoContas, que hoje recusa — ver a
// emenda em lojasaldo.go.

// lojaCompra atende MsgLojaCompra.
func (d *Dispatcher) lojaCompra(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	if s.TradeMode != 0 || s.Trade.Active {
		return // quem está vendendo ou negociando não compra
	}
	var pedido protocol.LojaCompraBody
	if err := pedido.Decode(payload); err != nil {
		return
	}

	vendedor, barraca := shopAt(w, int(pedido.Vendedor))
	if vendedor == nil || vendedor.Conn == s.Conn {
		return // barraca que não existe, ou a própria
	}
	if !autoTradeInRange(e, barraca) {
		d.log.Info("loja: compra longe demais", "conn", s.Conn, "barraca", pedido.Vendedor)
		return
	}
	pos := int(pedido.Slot)
	if pos < 0 || pos >= world.MaxAutoTrade {
		return
	}
	slot := &vendedor.AutoTrade.Slots[pos]
	cpos := slot.CargoPos
	if cpos < 0 || cpos >= world.MaxCargo || slot.Item.Empty() {
		return // vendido enquanto o painel olhava
	}
	cargoVendedor := w.Cargo(vendedor.AccountID)
	if cargoVendedor == nil {
		return
	}
	// O item precisa continuar sendo o mesmo que foi anunciado: entre a montagem
	// da barraca e este instante o Cargo pode ter mudado.
	itemCargo := cargoVendedor.Items[cpos]
	if !itemsEqual(slot.Item, itemCargo) {
		return
	}
	preco := slot.Price
	moeda := vendedor.AutoTrade.Moeda[pos]
	// O painel manda a moeda que ele mostrou ao jogador; se o vendedor trocou
	// nesse meio-tempo, a compra não sai — ninguém paga em moeda que não viu.
	if pedido.Moeda != moeda {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}

	destino := firstFreeTradeSlot(e)
	if destino < 0 {
		d.notify(w, s, NoticeNoSpaceToTrade)
		return
	}

	var imposto int32
	switch moeda {
	case protocol.LojaMoedaOuro:
		if e.Coin < preco {
			d.notify(w, s, NoticeNotEnoughMoney)
			return
		}
		if int64(cargoVendedor.Coin)+int64(preco) > maxCoin {
			d.notify(w, s, NoticeCantGetMore2G)
			return
		}
		if preco >= autoTradeTaxThreshold {
			imposto = (preco / 100) * int32(vendedor.AutoTrade.Tax)
		}
		e.Coin -= preco
		cargoVendedor.Coin += preco - imposto
	case protocol.LojaMoedaCash, protocol.LojaMoedaRMT:
		// Aqui é a emenda: enquanto o saldo não estiver ligado ao banco, a compra
		// é recusada e NADA se move. Ver lojasaldo.go.
		if err := saldoContas.Transfere(s.AccountID, vendedor.AccountID, moeda, preco); err != nil {
			d.log.Info("loja: compra em cash/rmt recusada", "conn", s.Conn, "moeda", moeda,
				"erro", err)
			d.notify(w, s, NoticeNotEnoughMoney)
			return
		}
	default:
		return
	}

	// Daqui para baixo a venda aconteceu: o item sai do Cargo do vendedor e entra
	// na mochila do comprador, e a barraca perde o slot.
	e.Carry[destino] = itemCargo
	cargoVendedor.Items[cpos] = world.Item{}
	*slot = world.AutoTradeSlot{CargoPos: -1}
	vendedor.AutoTrade.Moeda[pos] = protocol.LojaMoedaOuro

	w.Send(s, protocol.MsgSendItem,
		protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, destino, itemToSel(itemCargo)))
	d.sendEtc(w, s, e)
	w.Send(vendedor, protocol.MsgSendItem,
		protocol.EncodeSendItemBody(protocol.ItemPlaceCargo, cpos, protocol.SelItem{}))
	if moeda == protocol.LojaMoedaOuro {
		w.Send(vendedor, protocol.MsgUpdateCargoCoin,
			protocol.EncodeUpdateCargoCoin(cargoVendedor.Coin))
	}
	// Quem estiver olhando a barraca no jeito antigo vê o item sair da lista.
	w.BroadcastInView(int(pedido.Vendedor), protocol.MsgItemSold,
		protocol.EncodeStandardParm2(pedido.Vendedor, int32(pos)))
}
