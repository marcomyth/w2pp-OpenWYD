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

// msgSemRcoins é a recusa de compra por falta de Rcoins.
//
// ELA É LITERAL, e não uma entrada da tabela do cliente, porque o legado não tem a
// moeda: o _NN_Not_Enough_Money fala em gold, e não existe chave para Rcoins no
// Language.txt. Inventar uma chave não adiantaria — o cliente resolve contra o
// arquivo DELE, e uma chave que ele não conhece não desenha nada.
//
// "Rcoins" é o nome que o jogador vê no site e no painel (a tela de conta diz "Saldo
// de Rcoins"), e é o nome que ele precisa reconhecer aqui.
const msgSemRcoins = "Você não possui Rcoins suficientes."

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

	// A OFERTA QUE SUMIU PASSA A FALAR, e essa era a recusa muda que mais doía.
	//
	// A vitrine é uma fotografia: entre ela e o clique, outra pessoa compra, o
	// vendedor recolhe o item ou fecha a barraca. Todos esses caminhos devolviam
	// NADA — o botão não fazia coisa nenhuma —, e quem estava do outro lado clicava
	// de novo achando que o clique tinha falhado.
	//
	// A frase é a do próprio cliente (_NN_ItemSold, "O item foi vendido."), que
	// descreve o que a pessoa precisa saber: aquela oferta não está mais lá. Ela
	// serve às três causas, porque para quem compra as três são a mesma coisa.
	vendedor, barraca := shopAt(w, int(pedido.Vendedor))
	if vendedor == nil {
		d.notify(w, s, NoticeItemSold)
		return
	}
	if vendedor.Conn == s.Conn {
		// A própria barraca. O painel não oferece isso, então é cliente remendado:
		// continua mudo, como todo pedido que o jogo não produz.
		return
	}
	pos := int(pedido.Slot)
	if pos < 0 || pos >= world.MaxAutoTrade {
		return
	}
	slot := &vendedor.AutoTrade.Slots[pos]
	cpos := slot.CargoPos
	if cpos < 0 || cpos >= world.MaxCargo || slot.Item.Empty() {
		d.notify(w, s, NoticeItemSold) // vendido enquanto o painel olhava
		return
	}
	cargoVendedor := w.Cargo(vendedor.AccountID)
	if cargoVendedor == nil {
		// O baú do vendedor não está carregado: a barraca existe na tela e não tem
		// nada atrás. Para quem compra é o mesmo que ter sumido.
		d.notify(w, s, NoticeItemSold)
		return
	}
	// O item precisa continuar sendo o mesmo que foi anunciado: entre a montagem
	// da barraca e este instante o Cargo pode ter mudado.
	itemCargo := cargoVendedor.Items[cpos]
	if !itemsEqual(slot.Item, itemCargo) {
		d.notify(w, s, NoticeItemSold)
		return
	}
	preco := slot.Price
	moeda := vendedor.AutoTrade.Moeda[pos]
	// ITEM PRESO NO ESCROW NAO SAI POR AQUI - EXCETO PELA PORTA DELE.
	//
	// A marca diz que existe um anuncio em dinheiro real vivo sobre este item. Uma
	// compra em OURO ou em CASH levando-o tiraria o item de baixo de um anuncio
	// ativo: sobraria uma oferta na vitrine sem nada atras, e alguem pagaria por
	// ela.
	//
	// Mas a compra em dinheiro real E a porta desse anuncio. Recusa-la seria
	// recusar a unica venda que a marca existe para permitir.
	//
	// E ELA ERA RECUSADA. Esta conferencia ficava ANTES da leitura da moeda, e o
	// item de uma prateleira em dinheiro real esta SEMPRE marcado - entao a recusa
	// pegava tambem a compra legitima, e a mensagem de "ainda nao esta aberta" logo
	// abaixo era codigo morto. O teste que deveria pega-la montava uma prateleira em
	// RMT com o item NAO marcado, que e um estado que o jogo nao produz.
	//
	// A recusa continua aqui e nao so no itemSlot porque esta funcao mexe no bau
	// DIRETAMENTE, sem passar por la. Armadilha que nao cobre todos os caminhos nao
	// e armadilha.
	if itemCargo.AnuncioRMT != 0 && moeda != protocol.LojaMoedaRMT {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}
	// O painel manda a moeda que ele mostrou ao jogador; se o vendedor trocou
	// nesse meio-tempo, a compra não sai — ninguém paga em moeda que não viu.
	if pedido.Moeda != moeda {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}

	// A COMPRA EM DINHEIRO REAL NAO ACONTECE AQUI. Ela COMECA aqui.
	//
	// Nada se move neste instante: o item fica no bau do vendedor, preso pela
	// marca, e o comprador nao paga nada ainda. O que sai daqui e uma cobranca e um
	// aviso de onde pagar. A venda so acontece quando o dinheiro cair, noutro
	// processo, pela confirmacao.
	//
	// E SAI ANTES DA CONFERENCIA DE ESPACO, de proposito: aquela pergunta e sobre a
	// MOCHILA, e o item comprado por dinheiro real nao vai para a mochila - vai para
	// a caixa postal, e de la para o bau, no login ou no DeliverNow. Recusar por
	// mochila cheia seria recusar por um motivo que nao se aplica.
	if moeda == protocol.LojaMoedaRMT {
		// O MERCADO PODE ESTAR FECHADO, e a trava vale aqui também e não só na montagem
		// da barraca.
		//
		// Parece redundante — se ninguém anuncia, ninguém compra — e não é: anúncio aberto
		// ANTES do fechamento continua na vitrine, e um cliente remendado pode mandar a
		// compra de um anúncio que ele descobriu de outro jeito. Travar só a porta de
		// entrada deixaria o que já estava dentro comprável, e aí entra dinheiro de verdade
		// num mercado que a Hanna decidiu manter fechado.
		if !d.podeUsarRMT(s) {
			sendClientMessage(w, s, msgRMTNaoEstaAberta)
			return
		}
		d.abreCobrancaPix(w, s, itemCargo.AnuncioRMT, preco)
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
	case protocol.LojaMoedaCash:
		// Cash é a carteira de verdade: `account.donate_balance`, que a recarga
		// credita. Ver lojasaldo.go.
		if err := saldoContas.Transfere(s.AccountID, vendedor.AccountID, moeda, preco); err != nil {
			d.log.Info("loja: compra em cash recusada", "conn", s.Conn, "erro", err)
			// A FRASE TEM DE DIZER A MOEDA CERTA. O _NN_Not_Enough_Money do cliente é
			// "Não possui gold suficiente.", e quem tentava comprar em Rcoins lia isso
			// com a bolsa cheia de ouro — a recusa mandava conferir a moeda errada.
			sendClientMessage(w, s, msgSemRcoins)
			return
		}
		// O banco é a verdade, mas quem mostra o saldo no painel é a sessão: ela
		// acompanha a transferência que acabou de dar certo, dos dois lados.
		s.Cash -= preco
		vendedor.Cash += preco
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
