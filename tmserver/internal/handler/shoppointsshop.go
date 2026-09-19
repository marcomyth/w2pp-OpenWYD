package handler

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Loja que cobra em PONTOS DE LOJINHA (0060_shop_points).
//
// Um slot de loja de NPC com price_points preenchido no painel troca a moeda
// daquele item: o servidor debita a carteira da conta em vez do Coin do
// personagem. Nada disso existe no legado — a moeda também não existe lá.
//
// O cliente não participa da decisão e não precisa de patch: a desmontagem do
// wyd.exe (0x476d30-0x4775d6) mostra que o caminho até montar MSG_Buy não lê o
// ouro do jogador nem o preço, então quem escolhe a moeda é o servidor. O que o
// cliente NÃO cede é a exibição — a janela desenha o preço em ouro do ItemList
// dele, e é por isso que o custo em pontos é anunciado no chat ao abrir a loja.

const compraEmPontosMotivo = "loja de pontos"

// comprarComPontos é a compra de um slot precificado em pontos. Ela sai do laço
// para falar com o banco e volta, então é a única compra do jogo que não é
// resolvida no mesmo instante em que chega.
//
// A ordem é: cobrar primeiro, entregar depois. O contrário — entregar e cobrar —
// dá o item de graça a qualquer erro de banco, e um item na bolsa não volta.
func (d *Dispatcher) comprarComPontos(w *world.World, s *world.Session, e *world.Entity,
	npcPos, myPos int, item world.Item, custo int32, payload []byte) {
	if s.AccountID == 0 {
		return
	}
	// Um preço negativo só chega aqui por dado corrompido: o banco tem CHECK >= 0.
	// Recusar é melhor do que virar um crédito.
	if custo < 0 {
		d.log.Warn("compra em pontos: preço negativo no catálogo",
			"conn", s.Conn, "npc_pos", npcPos, "item", item.Index, "custo", custo)
		return
	}
	// Trava local antes da ida ao banco. Sem ela, um jogador que clica cinco vezes
	// põe cinco débitos em voo: o banco cobraria os cinco, e só o primeiro teria
	// um slot livre para entregar.
	if s.CompraEmPontos {
		return
	}

	// Preço zero não é um débito: o banco recusa custo <= 0, e mandá-lo para lá
	// só para levar um erro de volta seria trocar um presente por uma falha.
	if custo == 0 {
		d.entregarCompraEmPontos(w, s, e, myPos, item, payload)
		d.log.Info("compra em pontos", "conn", s.Conn, "conta", s.AccountID,
			"item", item.Index, "custo", 0)
		return
	}

	s.CompraEmPontos = true
	conta, nome := s.AccountID, e.Name
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, pago, err := p.SpendShopPoints(context.Background(), conta, custo, nome, compraEmPontosMotivo)
		return func(w *world.World, s *world.Session) {
			s.CompraEmPontos = false
			if err != nil {
				d.log.Warn("compra em pontos: débito falhou", "conta", conta,
					"item", item.Index, "custo", custo, "err", err)
				sendClientMessage(w, s, "Não foi possível cobrar seus pontos agora. Tente novamente em instantes.")
				return
			}
			if !pago {
				sendClientMessage(w, s, fmt.Sprintf(
					"Pontos insuficientes: este item custa %d pontos.", custo))
				return
			}
			e := w.Entity(s.Conn)
			// O jogador mexeu na bolsa enquanto o banco respondia, ou caiu. Os
			// pontos JÁ saíram, então a única saída honesta é devolvê-los — o
			// item não tem para onde ir.
			if e == nil || s.Mode != world.UserPlay || !carrySlotAccessible(e, myPos) || e.Carry[myPos].Index != 0 {
				d.estornarPontos(w, conta, nome, custo, item.Index)
				if e != nil {
					sendClientMessage(w, s, "A bolsa mudou durante a compra; seus pontos foram devolvidos.")
				}
				return
			}
			d.entregarCompraEmPontos(w, s, e, myPos, item, payload)
			d.log.Info("compra em pontos", "conn", s.Conn, "conta", conta,
				"item", item.Index, "custo", custo, "saldo", saldo)
			sendClientMessage(w, s, fmt.Sprintf("Você gastou %d pontos. Saldo: %d.", custo, saldo))
		}
	})
}

// entregarCompraEmPontos põe o item na bolsa e responde ao cliente na MESMA ordem
// que a compra em ouro (_MSG_Buy.cpp:271-296): o eco do MSG_Buy primeiro, o
// SendEtc depois e o SendItem por último, porque é no SendItem que o cliente
// grava o item na bolsa. O Coin vai no eco inalterado — a compra não encostou
// nele, e o cliente redesenha o mesmo número.
func (d *Dispatcher) entregarCompraEmPontos(w *world.World, s *world.Session, e *world.Entity,
	myPos int, item world.Item, payload []byte) {
	e.Carry[myPos] = item
	echo := make([]byte, len(payload))
	copy(echo, payload)
	if len(echo) >= 12 {
		binary.LittleEndian.PutUint32(echo[8:12], uint32(e.Coin))
	}
	w.SendTo(s, protocol.Header{Type: protocol.MsgBuy, ID: protocol.IDScene}, echo)
	d.sendEtc(w, s, e)
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, myPos, itemToSel(item)))
}

// estornarPontos devolve um débito que não virou item. GoDetached e não Go: o
// estorno é da CONTA e tem de acontecer mesmo que a sessão já tenha ido embora —
// que é justamente um dos casos em que ele é chamado.
func (d *Dispatcher) estornarPontos(w *world.World, conta int64, nome string, custo int32, item int16) {
	p := w.Persistence()
	w.GoDetached(func() func(*world.World) {
		saldo, err := p.AddShopPoints(context.Background(), conta, custo, nome, compraEmPontosMotivo+" (estorno)")
		if err != nil {
			// Alto no log porque é o dinheiro de alguém: o débito está gravado no
			// extrato e o estorno não, então isto é o bastante para devolver à mão.
			d.log.Error("compra em pontos: ESTORNO FALHOU", "conta", conta,
				"nome", nome, "pontos", custo, "item", item, "err", err)
			return nil
		}
		d.log.Info("compra em pontos estornada", "conta", conta, "pontos", custo,
			"item", item, "saldo", saldo)
		return nil
	})
}

// anunciarPrecoEmPontos escreve no chat o que a loja cobra em pontos, ao abri-la.
//
// É a única forma de o jogador saber o preço sem trocar arquivo do cliente: a
// janela de loja desenha o preço que está no ItemList.bin DELE, que é o preço em
// ouro do item e não tem nada a ver com o custo em pontos. Enquanto o tooltip do
// cliente não for atualizado, esta linha é o preço.
//
// Silenciosa quando a loja não tem nenhum item em pontos, que é o caso de todos
// os vendedores do jogo.
func (d *Dispatcher) anunciarPrecoEmPontos(w *world.World, s *world.Session, npc *world.Entity) {
	if npc == nil || len(npc.ShopPointPrice) == 0 {
		return
	}
	// Na ordem em que a janela desenha, não na ordem do mapa: o jogador está
	// lendo a lista enquanto lê isto, e um mapa em Go sai embaralhado a cada
	// abertura.
	linhas := 0
	for i := 0; i < maxShopSlots; i++ {
		pos := protocol.ShopSlot(i)
		custo, emPontos := npc.ShopPointPrice[pos]
		if !emPontos {
			continue
		}
		idx := int(npc.Carry[pos].Index)
		if idx == 0 {
			continue
		}
		if linhas == 0 {
			sendClientMessage(w, s, "Esta loja cobra em PONTOS DE LOJINHA (veja o seu saldo com /pontos):")
		}
		linhas++
		sendClientMessage(w, s, fmt.Sprintf("  %s — %d pontos", d.nomeDeItem(idx), custo))
	}
}

// nomeDeItem devolve o nome do catálogo, ou o índice cru quando o catálogo não
// está montado. O número sozinho não diz nada ao jogador, mas diz tudo a quem lê
// o log de um servidor rodando sem -content.
func (d *Dispatcher) nomeDeItem(index int) string {
	if nome, ok := d.itemNames[index]; ok && nome != "" {
		return nome
	}
	return fmt.Sprintf("item %d", index)
}
