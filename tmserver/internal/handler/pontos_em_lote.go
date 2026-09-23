package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As linhas que o lote responde. Todas dizem o NÚMERO, porque "não cabe" sem
// dizer quanto cabe obriga o jogador a descobrir no tentativa e erro.
const (
	msgLotePontosDeMenos = "Pontos insuficientes: você pediu %d e tem %d."
	msgLoteNaoCabe       = "Não cabe: o campo aceita mais %d ponto(s) agora."
	msgLoteCheio         = "Este campo já está no limite."
	msgLoteFeito         = "%d ponto(s) aplicado(s)."
)

// nomesDosAtributos e nomesDasAprendizagens só entram no log, para um relato de
// jogo virar uma linha procurável.
var (
	nomesDosAtributos     = [4]string{"FOR", "INT", "DES", "CON"}
	nomesDasAprendizagens = [4]string{"Aprender Arma", "Magia Branca", "Magia Negra", "Magia Especial"}
)

// pontosEmLote gasta vários pontos de uma vez num campo da janela de Personagem
// (protocol/pontos_em_lote.go). É TUDO OU NADA: o que não couber inteiro não é
// gasto, e a recusa diz qual das duas paredes bateu.
//
// As regras não são reescritas aqui. Um ponto de atributo passa por
// somaUmAtributo e um de aprendizagem por somaUmaAprendizagem — as mesmas
// funções que o `+` de um clique usa —, então o lote não pode divergir do clique.
func (d *Dispatcher) pontosEmLote(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	var body protocol.MsgPontosEmLoteBody
	if err := body.Decode(payload); err != nil {
		return
	}
	if !pedidoDeLoteValido(body) {
		return
	}
	campo := int(body.Campo)
	pedido := int(body.Quantidade)

	switch body.Tipo {
	case protocol.PontosAtributo:
		d.loteDeAtributo(w, s, e, campo, pedido)
	case protocol.PontosAprendizagem:
		d.loteDeAprendizagem(w, s, e, campo, pedido)
	}
}

// pedidoDeLoteValido confere o que veio do cliente ANTES de qualquer gasto: o
// quadro tem de ser um dos dois, o campo tem de existir e a quantidade tem de ser
// um número de verdade. O teto de PontosEmLoteMax não é regra de jogo — é o que
// impede um corpo absurdo de virar um laço longo dentro do laço do mundo.
func pedidoDeLoteValido(body protocol.MsgPontosEmLoteBody) bool {
	if body.Tipo != protocol.PontosAtributo && body.Tipo != protocol.PontosAprendizagem {
		return false
	}
	campo := int(body.Campo)
	pedido := int(body.Quantidade)
	return campo >= 0 && campo <= 3 && pedido >= 1 && pedido <= protocol.PontosEmLoteMax
}

// loteDeAtributo: FOR, INT, DES, CON. A única parede é o monte de pontos.
func (d *Dispatcher) loteDeAtributo(w *world.World, s *world.Session, e *world.Entity, campo, pedido int) {
	tenho := int(e.ScoreBonus)
	if pedido > tenho {
		sendClientMessage(w, s, fmt.Sprintf(msgLotePontosDeMenos, pedido, tenho))
		return
	}
	for i := 0; i < pedido; i++ {
		if !somaUmAtributo(e, campo) {
			// Não deve acontecer: a conta acima já garantiu o monte. Se acontecer,
			// para aqui em vez de seguir gastando — e o log diz que a garantia
			// falhou.
			d.log.Warn("lote de atributo parou no meio",
				"conn", s.Conn, "campo", campo, "pedido", pedido, "aplicados", i)
			break
		}
	}
	d.refreshScore(e)
	d.sendScore(w, s, e)
	d.sendEtc(w, s, e)
	sendClientMessage(w, s, fmt.Sprintf(msgLoteFeito, pedido))
	d.log.Info("pontos em lote",
		"conn", s.Conn, "account", s.AccountName, "quadro", "atributo",
		"campo", nomesDosAtributos[campo], "pontos", pedido, "sobrou", e.ScoreBonus)
}

// loteDeAprendizagem: Aprender Arma, Magia Branca, Negra, Especial. Duas paredes
// — o monte de pontos e o que ainda cabe no campo — e cada uma tem sua linha.
func (d *Dispatcher) loteDeAprendizagem(w *world.World, s *world.Session, e *world.Entity, campo, pedido int) {
	tenho := int(e.SpecialBonus)
	if pedido > tenho {
		sendClientMessage(w, s, fmt.Sprintf(msgLotePontosDeMenos, pedido, tenho))
		return
	}
	cabe := cabeNaAprendizagem(e, campo)
	if cabe <= 0 {
		sendClientMessage(w, s, msgLoteCheio)
		return
	}
	if pedido > cabe {
		sendClientMessage(w, s, fmt.Sprintf(msgLoteNaoCabe, cabe))
		return
	}
	for i := 0; i < pedido; i++ {
		if !somaUmaAprendizagem(e, campo) {
			d.log.Warn("lote de aprendizagem parou no meio",
				"conn", s.Conn, "campo", campo, "pedido", pedido, "aplicados", i)
			break
		}
	}
	d.refreshScore(e)
	d.sendScore(w, s, e)
	d.sendEtc(w, s, e)
	sendClientMessage(w, s, fmt.Sprintf(msgLoteFeito, pedido))
	d.log.Info("pontos em lote",
		"conn", s.Conn, "account", s.AccountName, "quadro", "aprendizagem",
		"campo", nomesDasAprendizagens[campo], "pontos", pedido, "sobrou", e.SpecialBonus)
}
