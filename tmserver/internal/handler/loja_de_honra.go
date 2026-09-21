package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A Loja de Honra: o God of War trocando itens pelos pontos que a lojinha
// aberta rende (shoppoints.go, 3 por quinze minutos e 7 com Fada Azul).
//
// Pedida em 2026-09-20. Até aqui os pontos só entravam: eram creditados, o
// /pontos os mostrava, e não havia onde gastá-los. Esta é a saída.
//
// O NPC é reaproveitado, não criado. O God of War já está de pé em Armia
// (NPCGener #[6063], em 2125,2114) com Merchant 104, e 104 não cai em nenhum
// ramo do roteamento de clique em misc.go — clicar nele não fazia absolutamente
// nada. É a mesma situação de onde saiu a loja do Unicórnio Puro
// (loja_de_emblema.go), e o truque é o mesmo: o servidor continua vendo 104, que
// é como reconhece a loja, e anuncia Merchant 1 ao cliente para que o clique
// chegue como _MSG_REQShopList.
//
// A diferença é o que responde a esse clique. A loja de emblema deixa o cliente
// abrir a janela de loja dele; aqui o servidor NÃO manda MSG_ShopList — manda
// MsgHonraAbre, e quem desenha é o nosso painel (client/gamepatch/honra.cpp). O
// motivo é o preço: a janela do cliente só sabe escrever preço com o cifrão do
// ouro, e o que se paga aqui é ponto. Ver protocol/lojahonra.go.

const (
	// merchantGodOfWar é o Merchant do template God_of_War (byte 104 do
	// STRUCT_MOB, CurrentScore.Merchant). Como no Unicórnio Puro, a loja é
	// reconhecida pelo Merchant e não pelo nome: o painel de NPCs permite trocar
	// o nome exibido.
	merchantGodOfWar = 104

	// honraMotivo é o que aparece no extrato de shop_points_audit.
	honraMotivo = "loja de honra"
)

// itemDeHonra é uma troca oferecida pela loja.
type itemDeHonra struct {
	Indice int16
	Preco  int32 // em pontos de lojinha
}

// estoqueDaLojaDeHonra é o que o God of War oferece, na ordem em que aparece no
// painel. As casas são a POSIÇÃO nesta tabela, e é por elas que a compra se
// refere ao item, então acrescentar no fim é seguro e reordenar troca o que cada
// jogador com o painel aberto vai comprar.
//
// Preços de partida, a calibrar: uma lojinha cheia rende 3 pontos por quinze
// minutos, ou 12 por hora — 288 por dia de barraca de pé, 672 com Fada Azul.
// Todo índice aqui foi conferido no Release/Common/ItemList.csv.
//
// Mora em código, como o estoque do Unicórnio Puro, e pelo mesmo motivo: com
// -npc-editing ligado o Carry do template não chega ao jogo. Se um dia a equipe
// tiver de mexer nisto pelo site, o lugar é uma tabela nova ao lado de
// npc_shop_item — o preço aqui é em pontos, e item_price só sabe de ouro.
var estoqueDaLojaDeHonra = []itemDeHonra{
	{3901, 150}, // Fada_Azul(3dias) - a fada que dobra o próprio ganho
	{3904, 240}, // Fada_Azul(5dias)
	{3907, 320}, // Fada_Azul(7dias)
	{412, 40},   // Poeira_de_Oriharucon
	{413, 40},   // Poeira_de_Lactolerium
	{414, 60},   // Poeira_de_Fada
	{774, 80},   // Pedra_da_Troca_Maior
	{775, 30},   // Pedra_da_Troca_Menor
	{3909, 100}, // Mapa_Vale_Escondido(24h)
}

// ehLojaDeHonra diz se npc é o God of War.
func ehLojaDeHonra(npc *world.Entity) bool {
	return npc != nil && npc.Merchant == merchantGodOfWar
}

// itensDeHonraParaOPainel monta o estoque na forma da linha.
func itensDeHonraParaOPainel() []protocol.HonraItem {
	n := len(estoqueDaLojaDeHonra)
	if n > protocol.HonraMaxItens {
		n = protocol.HonraMaxItens
	}
	itens := make([]protocol.HonraItem, 0, n)
	for i := 0; i < n; i++ {
		it := estoqueDaLojaDeHonra[i]
		itens = append(itens, protocol.HonraItem{
			Slot:   int16(i),
			Indice: it.Indice,
			Preco:  it.Preco,
		})
	}
	return itens
}

// abrirLojaDeHonra responde ao clique no God of War: manda o estoque e o saldo.
//
// O saldo vem do banco, e não de um número guardado na sessão, pelo mesmo motivo
// do /pontos: o ponto é da CONTA, e os outros três personagens dela podem ter
// somado enquanto este estava andando. Leitura fora do laço, como toda leitura
// de banco.
func (d *Dispatcher) abrirLojaDeHonra(w *world.World, s *world.Session, npc *world.Entity) {
	// Guardar qual NPC abriu a loja é o que faz a compra poder exigir presença:
	// sem isto, um cliente remendado compraria de qualquer lugar do mundo.
	s.LojaHonraNPC = npc.ID
	itens := itensDeHonraParaOPainel()
	accountID := s.AccountID
	p := w.Persistence()
	d.log.Info("loja de honra aberta", "conn", s.Conn, "npc", npc.ID, "itens", len(itens))
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.ShopPoints(context.Background(), accountID)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("loja de honra: leitura de saldo falhou", "conta", accountID, "err", err)
				sendClientMessage(w, s, "Não foi possível consultar seus pontos agora.")
				return
			}
			corpo := (&protocol.HonraAbreBody{Saldo: saldo, Itens: itens}).Encode()
			w.Send(s, protocol.MsgHonraAbre, corpo)
		}
	})
}

// honraFecha atende MsgHonraFecha: o painel fechou, então a loja não está mais
// aberta e a compra volta a ser recusada.
func (d *Dispatcher) honraFecha(w *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	s.LojaHonraNPC = 0
}

// honraCompra atende MsgHonraCompra: troca pontos por um item do estoque.
//
// A ordem importa e é esta: confere tudo o que o laço sabe, DEBITA no banco, e
// só depois entrega o item. Debitar primeiro é o que impede a entrega dupla —
// dois pedidos seguidos não podem virar dois itens com saldo para um. Se entre o
// débito e a entrega a mochila encheu, os pontos VOLTAM: o estorno é a única
// forma de o jogador não pagar por nada.
func (d *Dispatcher) honraCompra(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	if s.TradeMode != 0 || s.Trade.Active {
		return // quem está vendendo ou negociando não compra
	}
	// Um pedido por vez. Sem esta trava dois cliques rápidos viram dois débitos
	// no ar ao mesmo tempo, e o segundo pode passar com o saldo do primeiro.
	if s.HonraCobrando {
		return
	}
	var pedido protocol.HonraCompraBody
	if err := pedido.Decode(payload); err != nil {
		return
	}
	npc := w.Entity(s.LojaHonraNPC)
	if !ehLojaDeHonra(npc) {
		return // painel fechado, ou nunca esteve aberto
	}
	// Presença: a loja é do NPC, não do jogador. O mesmo alcance que a
	// experiência de grupo usa para "estava perto do corpo".
	if !pertoDoMob(e, npc) {
		sendClientMessage(w, s, "Você está longe demais do God of War.")
		return
	}
	pos := int(pedido.Slot)
	if pos < 0 || pos >= len(estoqueDaLojaDeHonra) {
		return
	}
	item := estoqueDaLojaDeHonra[pos]
	if item.Indice <= 0 || item.Preco <= 0 {
		return
	}
	if firstFreeTradeSlot(e) < 0 {
		d.notify(w, s, NoticeNoSpaceToTrade)
		return
	}

	s.HonraCobrando = true
	accountID := s.AccountID
	nome := e.Name
	preco := item.Preco
	indice := item.Indice
	p := w.Persistence()
	d.log.Info("loja de honra: cobrando", "conn", s.Conn, "conta", accountID,
		"item", indice, "pontos", preco)
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.AddShopPoints(context.Background(), accountID, -preco, nome, honraMotivo)
		return func(w *world.World, s *world.Session) {
			s.HonraCobrando = false
			if errors.Is(err, world.ErrPontosInsuficientes) {
				sendClientMessage(w, s, fmt.Sprintf(
					"Você precisa de %d pontos de lojinha para trocar por este item.", preco))
				return
			}
			if err != nil {
				d.log.Error("loja de honra: débito falhou", "conta", accountID,
					"pontos", preco, "err", err)
				sendClientMessage(w, s, "A loja de honra está indisponível agora. Tente de novo.")
				return
			}
			d.entregaDeHonra(w, s, accountID, nome, indice, preco, saldo)
		}
	})
}

// entregaDeHonra põe o item comprado na mochila, já com os pontos debitados. Uma
// entrega que não cabe mais devolve os pontos.
//
// Roda no laço, na volta do débito — e por isso tudo é conferido de novo: entre o
// pedido e esta linha o jogador pode ter morrido, deslogado e voltado, ou enchido
// a mochila.
func (d *Dispatcher) entregaDeHonra(w *world.World, s *world.Session, accountID int64,
	nome string, indice int16, preco, saldo int32) {
	e := w.Entity(s.Conn)
	destino := -1
	if e != nil && s.Mode == world.UserPlay {
		destino = firstFreeTradeSlot(e)
	}
	if destino < 0 {
		// Estorno. O crédito pode falhar por sua vez, e aí o log é a única
		// testemunha: é dinheiro de alguém, então ele grita.
		p := w.Persistence()
		d.log.Warn("loja de honra: sem espaço na volta do débito, estornando",
			"conta", accountID, "pontos", preco)
		w.GoDetached(func() func(*world.World) {
			if _, err := p.AddShopPoints(context.Background(), accountID, preco, nome,
				honraMotivo+": estorno"); err != nil {
				d.log.Error("loja de honra: estorno falhou", "conta", accountID,
					"pontos", preco, "err", err)
			}
			return nil
		})
		d.notify(w, s, NoticeNoSpaceToTrade)
		return
	}
	item := world.Item{Index: indice}
	e.Carry[destino] = item
	d.sendSlot(w, s, world.ItemPlaceCarry, destino, item)
	w.Send(s, protocol.MsgHonraSaldo, protocol.EncodeHonraSaldo(saldo))
	d.log.Info("loja de honra: entregue", "conn", s.Conn, "conta", accountID,
		"item", indice, "pontos", preco, "saldo", saldo, "slot", destino)
}
