package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A LIXEIRA EM LOTE: o jogador marca vários itens da mochila e apaga de uma vez.
//
// Ela existe porque o descarte de um item só (0x02E4) não serve para um lote — ver
// o comentário em protocol/lixeira.go. E ela existe AGORA porque largar item no chão
// foi desligado: sem ela, o jogador fica sem nenhuma forma de se livrar de um item.
//
// ITEM POR ITEM, E NÃO TUDO-OU-NADA. O que passa é apagado, e o resto volta com o
// motivo. É a mesma regra do 0x02E4 repetido, e é a que não faz um slot bloqueado
// no fim da lista anular os dezenove apagamentos legítimos que vieram antes.

// lixeiraApaga atende o MsgLixeiraApaga.
func (d *Dispatcher) lixeiraApaga(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)

	// PACOTE RUIM: nada é apagado e nem se diz quais slots ficaram, porque não dá
	// para confiar em nenhum número que veio dentro dele.
	corpo, err := protocol.DecodeLixeiraApaga(payload)
	if err != nil {
		d.log.Info("lixeira: pacote recusado", "conn", s.Conn, "erro", err)
		d.lixeiraResponde(w, s, 0, nil)
		return
	}

	// MORTO OU FORA DE JOGO: o lote inteiro cai, e SEM AddCrackError.
	//
	// O 0x02E4 conta isso como trapaça, e aqui não é: a janela da lixeira fica
	// aberta enquanto o jogador leva um golpe, e o clique que sai no mesmo segundo
	// da morte é tempo de rede, não fraude. Marcar como trapaça puniria quem morreu
	// na hora errada.
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		d.lixeiraResponde(w, s, 0, lixeiraTudoRecusado(corpo.Itens, protocol.LixeiraMotivoNaoPode))
		return
	}

	// EM TROCA: o lote inteiro cai, E A TROCA CONTINUA. O 0x02E4 cancelaria a troca;
	// aqui isso seria um estrago desproporcional — o jogador clicou na lixeira, não
	// na troca, e cancelar faria ele perder o que já tinha combinado com outra
	// pessoa.
	if s.Trade.Active {
		d.lixeiraResponde(w, s, 0, lixeiraTudoRecusado(corpo.Itens, protocol.LixeiraMotivoEmTroca))
		return
	}

	var apagados uint8
	var recusas []protocol.LixeiraRecusa
	vistos := make(map[int16]bool, len(corpo.Itens))
	for _, item := range corpo.Itens {
		slot := int(item.Slot)
		switch {
		case vistos[item.Slot]:
			// O MESMO SLOT DUAS VEZES: a primeira vale, a segunda é recusada. Sem
			// isto, a segunda passagem veria o slot já vazio e diria "vazio" — um
			// motivo que descreve o efeito da primeira linha e não o erro real.
			recusas = append(recusas, protocol.LixeiraRecusa{Slot: item.Slot, Motivo: protocol.LixeiraMotivoRepetido})
			continue
		case !carrySlotAccessible(e, slot):
			recusas = append(recusas, protocol.LixeiraRecusa{Slot: item.Slot, Motivo: protocol.LixeiraMotivoSlotBloqueado})
			continue
		case e.Carry[slot].Empty():
			recusas = append(recusas, protocol.LixeiraRecusa{Slot: item.Slot, Motivo: protocol.LixeiraMotivoVazio})
			continue
		case e.Carry[slot].Index != item.Indice:
			// O ÍNDICE MUDOU DESDE QUE ELE MARCOU. É a conferência que o 0x02E4 não
			// tem, e a razão principal de este pacote existir: entre marcar e
			// confirmar, um drop ou um arrastar muda o que está no slot, e apagar o
			// que está lá agora seria apagar o item errado — sem volta.
			recusas = append(recusas, protocol.LixeiraRecusa{Slot: item.Slot, Motivo: protocol.LixeiraMotivoOutroItem})
			continue
		}
		vistos[item.Slot] = true
		idx := e.Carry[slot].Index
		e.Carry[slot] = world.Item{}
		d.sendSlot(w, s, world.ItemPlaceCarry, slot, e.Carry[slot])
		d.log.Info("item deleted (lote)", "conn", s.Conn, "slot", slot, "index", idx)
		apagados++
	}
	d.lixeiraResponde(w, s, apagados, recusas)
}

// lixeiraTudoRecusado monta a recusa do lote inteiro com um motivo só.
func lixeiraTudoRecusado(itens []protocol.LixeiraPedido, motivo uint8) []protocol.LixeiraRecusa {
	out := make([]protocol.LixeiraRecusa, 0, len(itens))
	for _, i := range itens {
		out = append(out, protocol.LixeiraRecusa{Slot: i.Slot, Motivo: motivo})
	}
	return out
}

// lixeiraResponde manda o 0x0F51.
//
// SEMPRE, e uma vez só. A janela do cliente espera a resposta e, sem ela, diz "sem
// resposta do servidor" depois de cinco segundos — o silêncio viraria um erro que
// não aconteceu.
func (d *Dispatcher) lixeiraResponde(w *world.World, s *world.Session, apagados uint8, recusas []protocol.LixeiraRecusa) {
	w.SendTo(s, protocol.Header{Type: protocol.MsgLixeiraResultado, ID: uint16(s.Conn)},
		protocol.EncodeLixeiraResultado(apagados, recusas))
}
