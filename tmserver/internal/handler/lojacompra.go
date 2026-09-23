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

// repartirImposto manda o imposto da venda para os cofres das cidades, em vez de
// deixá-lo evaporar como antes.
//
// Com o mercado global uma venda tem duas cidades: a da barraca e a de quem
// comprou. A divisão é 80% para a cidade onde a barraca está — é ela que
// hospeda o vendedor e cobra a taxa anunciada — e 20% para a cidade de onde
// partiu a compra. Fora de cidade não há cofre, e nesse caso a parte do
// comprador segue junto com a do vendedor, em vez de sumir.
//
// O cofre é o TaxVault da zona, o mesmo que o líder da guilda dona da cidade
// saca em guild.go. Até aqui NADA depositava nele: o imposto da lojinha era
// descontado do vendedor e desaparecia no ar. Agora ele tem para onde ir.
func (d *Dispatcher) repartirImposto(w *world.World, s *world.Session, imposto int32,
	cidadeDaBarraca, cidadeDoComprador int) {
	if imposto <= 0 {
		return
	}
	valeComprador := cidadeDoComprador >= 0 && cidadeDoComprador < len(d.guildZones)
	valeBarraca := cidadeDaBarraca >= 0 && cidadeDaBarraca < len(d.guildZones)
	if !valeBarraca && !valeComprador {
		return
	}
	doComprador := int64(imposto) * 20 / 100
	daBarraca := int64(imposto) - doComprador
	switch {
	case !valeComprador:
		daBarraca, doComprador = int64(imposto), 0
	case !valeBarraca:
		doComprador, daBarraca = int64(imposto), 0
	case cidadeDoComprador == cidadeDaBarraca:
		daBarraca, doComprador = int64(imposto), 0
	}
	if daBarraca > 0 {
		z := &d.guildZones[cidadeDaBarraca]
		z.TaxVault += daBarraca
		d.persistGuildZone(w, s, *z)
	}
	if doComprador > 0 {
		z := &d.guildZones[cidadeDoComprador]
		z.TaxVault += doComprador
		d.persistGuildZone(w, s, *z)
	}
	d.log.Info("loja: imposto repartido", "total", imposto, "cidade_barraca", cidadeDaBarraca,
		"parte_barraca", daBarraca, "cidade_comprador", cidadeDoComprador,
		"parte_comprador", doComprador)
}

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
		d.repartirImposto(w, s, imposto, world.Village(barraca.X, barraca.Y),
			world.Village(e.X, e.Y))
	case protocol.LojaMoedaCash, protocol.LojaMoedaRMT:
		// Aqui é a emenda: enquanto o saldo não estiver ligado ao banco, a compra
		// é recusada e NADA se move. Ver lojasaldo.go.
		if err := saldoContas.Transfere(s.AccountID, vendedor.AccountID, moeda, preco); err != nil {
			d.log.Info("loja: compra em cash/rmt recusada", "conn", s.Conn, "moeda", moeda,
				"erro", err)
			d.notify(w, s, NoticeNotEnoughMoney)
			return
		}
		// O banco é a verdade, mas quem mostra o saldo no painel é a sessão: ela
		// acompanha a transferência que acabou de dar certo, dos dois lados.
		if moeda == protocol.LojaMoedaCash {
			s.Cash -= preco
			vendedor.Cash += preco
		} else {
			s.Rmt -= preco
			vendedor.Rmt += preco
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
	// O aviso aos outros sai no fim, com a prateleira ja vazia: avisar antes
	// mandaria a todos uma vitrine com o item que acabou de ser vendido.
	d.mercadoMudou(w)
}
