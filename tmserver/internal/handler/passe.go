package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// PasseNivelMax é o maior nível que o cliente sabe desenhar. Acima disso ele não
// desenha nada, o que é o melhor dos estragos — e ainda assim o servidor prende.
const PasseNivelMax = 4

// AplicarPasse troca a moldura do passe de quem está EM JOGO e a redesenha para
// todos que o veem.
//
// O QUE ELA NÃO FAZ: escrever no banco. O nível mora na conta e quem o grava é quem
// chamou — esta função é a cortesia que poupa a pessoa de relogar, igual à entrega
// imediata. Falhar aqui não perde nada: o nível gravado vale no próximo login.
//
// POR QUE O REDESENHO É UM CreateMob INTEIRO, e não um pacote de "mudou a moldura":
// não existe pacote novo. O byte da moldura viaja dentro do CreateMob, então mandar
// o CreateMob de novo é o único jeito de o cliente reler o byte. É o mesmo caminho
// que o jogo já usa quando alguém entra na tela.
//
// E ELE VAI PARA A PRÓPRIA PESSOA TAMBÉM, e não só para quem está em volta: a
// moldura aparece em volta do próprio nome, e sem o pacote de si mesma a pessoa
// pagaria pelo passe e não veria nada até relogar.
//
// Devolve o nome do personagem e se a conta estava conectada. Não conectada NÃO é
// erro: é o estado normal de quase toda conta em quase todo momento.
func (d *Dispatcher) AplicarPasse(w *world.World, accountID int64, nivel uint8) (string, bool) {
	if nivel > PasseNivelMax {
		nivel = PasseNivelMax
	}
	var personagem string
	var achou bool
	w.ForEachSession(func(s *world.Session, e *world.Entity) {
		if s == nil || s.AccountID != accountID {
			return
		}
		achou = true
		// A SESSÃO SEMPRE, a entidade só se houver. Alguém parado na tela de escolher
		// personagem tem sessão e não tem entidade; guardar na sessão faz a moldura
		// valer no personagem que ele escolher daqui a pouco, sem relogar.
		s.PasseNivel = nivel
		if e == nil {
			return
		}
		e.PasseNivel = nivel
		personagem = e.Name
		d.reanunciaPasse(w, s, e)
	})
	return personagem, achou
}

// reanunciaPasse manda o CreateMob de novo: para a própria pessoa e para quem a vê.
func (d *Dispatcher) reanunciaPasse(w *world.World, s *world.Session, e *world.Entity) {
	ty, body := createMobViewPacket(w, e, 0)
	w.SendTo(s, protocol.Header{Type: ty, ID: protocol.IDScene}, body)
	w.ForEachInView(e.ID, func(vs *world.Session, _ *world.Entity) {
		w.SendTo(vs, protocol.Header{Type: ty, ID: protocol.IDScene}, body)
	})
}
