package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A entrega retroativa das peças de nível (migração 0172).
//
// DIVERGÊNCIA DELIBERADA, pedida pelo Marco em 26/09/2026. Quem subia sem
// distribuir pontos, e o TK, a FM e o BM de Destreza, passaram do 29 ao 254 sem
// uma peça: o LevelItem.txt não tem coluna para eles e a entrega desistia calada
// (levelitem.go, desvioDaConstrucao). A regra foi consertada para a frente, e o
// que ficou para trás sai aqui, de uma vez, no login: todo Mortal recebe a peça
// de CADA nível que já passou, montarias inclusive, pela construção de agora.
// Inclusive quem já tinha recebido — "foi erro nosso" —, por isso não há
// conferência contra o que já foi entregue.
//
// A marca Entity.NivelRetroativo é o que impede a repetição. Ela vai ao banco na
// mesma transação que o armazém (SalvarPersonagemComCarga), então as peças e a
// marca gravam juntas ou não gravam.

// nivelRetroativoConcluido marca o personagem que já recebeu tudo. Fica acima
// do maior nível do jogo, para nunca ser confundido com um nível parcial.
const nivelRetroativoConcluido = 1000

// entregaItensDeNivelRetroativos põe no armazém da conta a peça de cada nível
// que o personagem já passou e ainda não recebeu por aqui.
//
// Armazém cheio não perde nada: a entrega para na primeira peça que não cabe, a
// marca guarda o último nível entregue, e o resto sai no próximo login. Só o
// Mortal entra — é a mesma regra da entrega de cada nível.
func (d *Dispatcher) entregaItensDeNivelRetroativos(w *world.World, s *world.Session, e *world.Entity) {
	if d.levelItems == nil || s == nil || e == nil || e.NivelRetroativo >= nivelRetroativoConcluido {
		return
	}
	if e.ClassMaster != classMasterMortal {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		d.log.Warn("itens de nível retroativos: a conta não tem armazém carregado",
			"conn", s.Conn, "personagem", e.Name)
		return
	}

	entregues, faltam := 0, 0
	feito := int(e.NivelRetroativo)
	for nivel := feito + 1; nivel <= int(e.Level); nivel++ {
		item, construcao := pecaDoNivelEm(d.levelItems, e, int32(nivel))
		if item.Empty() {
			if faltam == 0 {
				feito = nivel
			}
			continue
		}
		if faltam > 0 {
			faltam++
			continue
		}
		vaga := vagaParaItemDeNivel(cargo)
		if vaga < 0 {
			faltam++
			continue
		}
		it := world.Item{Index: item.Index}
		for i, ef := range item.Effects {
			it.Effects[i] = world.Effect{Effect: ef[0], Value: ef[1]}
		}
		cargo.Items[vaga] = it
		w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(world.ItemPlaceCargo, vaga, itemToSel(it)))
		entregues++
		feito = nivel
		d.log.Info("item de nível retroativo entregue",
			"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name, "classe", e.Class,
			"construcao", construcao, "nivel", nivel, "item", item.Index, "vaga", vaga)
	}

	if faltam == 0 {
		e.NivelRetroativo = nivelRetroativoConcluido
	} else {
		e.NivelRetroativo = uint16(feito)
	}
	if entregues > 0 {
		sendClientMessage(w, s, fmt.Sprintf("%d itens de nível chegaram ao seu armazém.", entregues))
	}
	if faltam > 0 {
		sendClientMessage(w, s, fmt.Sprintf("Armazém cheio: faltam %d itens de nível. Libere espaço e entre de novo.", faltam))
		d.log.Warn("itens de nível retroativos: armazém cheio",
			"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name, "entregues", entregues,
			"faltam", faltam, "ate_nivel", feito)
	}
}

// entregaRetroativaNoLogin roda a entrega retroativa no fim da entrada no mundo,
// depois do quadro de login: o armazém já está carregado, e o aviso chega a um
// cliente que já desenhou a cena.
func (d *Dispatcher) entregaRetroativaNoLogin(w *world.World, s *world.Session) {
	if e := w.Entity(s.Conn); e != nil {
		d.entregaItensDeNivelRetroativos(w, s, e)
	}
}
