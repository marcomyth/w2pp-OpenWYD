package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os sete portões do Coliseu, pelo mesmo caminho dos do campo de treino
// (campo_de_treino_portoes.go): o boot semeia todo portão do InitItem aberto e
// sem desenho, e só os que um sistema assume vão ao cliente. Estes só são
// desenhados com o Coliseu ligado — desligado, a arena fica como sempre esteve.
//
// Ao ligar, eles tomam o estado do boot do legado (Server.cpp:4075-4076):
// entrada aberta, os cinco de dentro trancados. A partir daí só o relógio os
// move, e um jogador com a chave deles (EF_KEYID 9, a mesma nos dois modelos)
// abre um trancado pelo caminho genérico de gate.go, como no legado.
//
// UNVERIFIED, como nos outros: que o cliente 7662 barre a passagem só com estes
// pacotes. Quem levanta o chão sob o portão é o cliente; o servidor não tem
// checagem de altura no movimento do jogador.

// portoesDoColiseuIDs devolve os ids de chão dos sete portões, procurando-os na
// primeira chamada. Um portão ausente do InitItem fica com id -1.
func (d *Dispatcher) portoesDoColiseuIDs(w *world.World) [7]int {
	c := &d.coliseu
	if c.portoes[0] == 0 {
		for i := range c.portoes {
			c.portoes[i] = -1
		}
		w.ForEachStaticItem(func(g *world.GroundItem) {
			for i, p := range coliseuPortoes {
				if c.portoes[i] < 0 && g.Item.Index == p.item && g.X == p.x && g.Y == p.y {
					c.portoes[i] = g.ID
				}
			}
		})
	}
	return c.portoes
}

// portoesDoColiseu muda os portões de fora (interno false, SetColoseumDoor) ou
// os de dentro (SetColoseumDoor2) para state, e mostra a mudança a quem está
// vendo: abrir vai como MSG_UpdateItem, trancar como MSG_CreateItem, que é o que
// carrega a altura que fecha a passagem (Server.cpp:6443-6537). O legado só manda
// quando o estado muda, e este também.
func (d *Dispatcher) portoesDoColiseu(w *world.World, interno bool, state int16) {
	for i, id := range d.portoesDoColiseuIDs(w) {
		if coliseuPortoes[i].interno != interno {
			continue
		}
		g := w.GroundItem(id)
		if g == nil || g.State == state {
			continue
		}
		g.State = state
		d.mostraPortaoDoColiseu(w, g)
	}
}

// mostraPortaoDoColiseu manda o estado atual de g a todos que o têm na visão.
// Quem ainda não o via recebe o desenho inteiro.
func (d *Dispatcher) mostraPortaoDoColiseu(w *world.World, g *world.GroundItem) {
	key := gateSeenKey(g.ID)
	upd := (&protocol.MsgUpdateItemBody{ItemID: int32(world.GroundItemIDOffset + g.ID), State: int32(g.State)}).Encode()
	w.ForEachInViewAt(g.X, g.Y, -1, func(s *world.Session, _ *world.Entity) {
		if w.MarkSeen(s, key) || g.State != world.StateOpen {
			w.SendTo(s, protocol.Header{Type: protocol.MsgCreateItem, ID: protocol.IDScene}, d.gateCreateBody(g))
			return
		}
		w.SendTo(s, protocol.Header{Type: protocol.MsgUpdateItem, ID: protocol.IDScene}, upd)
	})
}

// syncPortoesDoColiseu desenha para s os portões que entram na visão e retira
// os que saem, a metade de itens do GridMulticast. Desligado, não desenha nada.
func (d *Dispatcher) syncPortoesDoColiseu(w *world.World, s *world.Session, x, y int16) {
	if s == nil || !d.coliseu.ligado {
		return
	}
	for _, id := range d.portoesDoColiseuIDs(w) {
		g := w.GroundItem(id)
		if g == nil {
			continue
		}
		key := gateSeenKey(g.ID)
		if chebyshev(x, y, g.X, g.Y) <= world.ViewRange {
			if w.MarkSeen(s, key) {
				w.SendTo(s, protocol.Header{Type: protocol.MsgCreateItem, ID: protocol.IDScene}, d.gateCreateBody(g))
			}
			continue
		}
		if w.Seen(s, key) {
			w.UnmarkSeen(s, key)
			w.SendTo(s, protocol.Header{Type: protocol.MsgDecayItem, ID: protocol.IDScene},
				protocol.EncodeDecayItemBody(uint16(world.GroundItemIDOffset+g.ID)))
		}
	}
}

// ligarColiseu liga o interruptor: portões no estado de boot do legado e
// desenhados para quem está perto. Devolve false se já estava ligado.
func (d *Dispatcher) ligarColiseu(w *world.World) bool {
	c := &d.coliseu
	if c.ligado {
		return false
	}
	c.ligado = true
	for i, id := range d.portoesDoColiseuIDs(w) {
		if g := w.GroundItem(id); g != nil {
			g.State = world.StateOpen
			if coliseuPortoes[i].interno {
				g.State = world.StateLocked
			}
		}
	}
	w.ForEachPlaying(-1, func(s *world.Session, e *world.Entity) {
		d.syncPortoesDoColiseu(w, s, e.X, e.Y)
	})
	d.log.Info("coliseu: ligado")
	return true
}

// desligarColiseu encerra o que estiver correndo e devolve a arena ao que ela é
// sem o Coliseu: monstros das ondas fora, limites desligados, portões abertos e
// apagados da tela de quem os via. Devolve false se já estava desligado.
func (d *Dispatcher) desligarColiseu(w *world.World) bool {
	c := &d.coliseu
	if !c.ligado {
		return false
	}
	if c.ondas.Encerrar().Fim {
		d.apagarOndasDoColiseu(w)
	}
	c.batalha.Encerrar()
	c.ligado = false
	d.atualizaAnonimato(w)
	d.redesenhaArena(w, false)
	for _, id := range d.portoesDoColiseuIDs(w) {
		g := w.GroundItem(id)
		if g == nil {
			continue
		}
		g.State = world.StateOpen
		key := gateSeenKey(g.ID)
		decay := protocol.EncodeDecayItemBody(uint16(world.GroundItemIDOffset + g.ID))
		w.ForEachPlaying(-1, func(s *world.Session, _ *world.Entity) {
			if w.Seen(s, key) {
				w.UnmarkSeen(s, key)
				w.SendTo(s, protocol.Header{Type: protocol.MsgDecayItem, ID: protocol.IDScene}, decay)
			}
		})
	}
	d.log.Info("coliseu: desligado")
	return true
}
