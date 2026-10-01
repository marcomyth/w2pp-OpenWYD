package handler

import (
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// OS DOIS PAINÉIS PELO FIO, com o número de pacote de verdade.
//
// Os testes de refino_lote_test.go e montaria_lote_test.go chamam refinoExecuta e
// montariaExecuta direto: provam a regra, e não provam que o pacote CHEGA nelas.
// E foi exatamente aí que o refino quebrou uma vez sem ninguém ver: ele nasceu em
// 0x0F50/0x0F51, a lixeira em lote chegou à main com o mesmo par, e a tabela de
// rotas é um mapa — o segundo registro cobre o primeiro, calado, e todo teste que
// chama a função direto continua verde.

// respostaDoPainel espera a resposta do tipo pedido, e falha se antes dela chegar
// a resposta de OUTRO painel: é assim que o teste vê um pacote atendido pelo
// tratador errado, ou uma resposta que não devia ter saído.
func respostaDoPainel(t *testing.T, c net.Conn, quer protocol.Type) []byte {
	t.Helper()
	_, corpo, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, _ []byte) bool {
		switch h.Type {
		case protocol.MsgRefinoResultado, protocol.MsgMontariaResultado, protocol.MsgLixeiraResultado:
			if h.Type != quer {
				t.Fatalf("chegou a resposta %s, e a pedida era %s", formatType(h.Type), formatType(quer))
			}
			return true
		}
		return false
	})
	if !ok {
		t.Fatalf("o servidor nao respondeu o %s", formatType(quer))
	}
	return corpo
}

func TestPaineisRespondemPeloNumeroDeles(t *testing.T) {
	// O par do refino é o que o cliente (refinorede.h) tem escrito. Mudar aqui sem
	// mudar lá deixa o painel mudo.
	if protocol.MsgRefinoPede != 0x0F52 || protocol.MsgRefinoResultado != 0x0F53 {
		t.Fatalf("refino em %s/%s, e o contrato com o cliente e 0x0f52/0x0f53",
			formatType(protocol.MsgRefinoPede), formatType(protocol.MsgRefinoResultado))
	}
	if protocol.MsgMontariaPede != 0x0F62 || protocol.MsgMontariaResultado != 0x0F63 {
		t.Fatalf("montaria em %s/%s, e o contrato com o cliente e 0x0f62/0x0f63",
			formatType(protocol.MsgMontariaPede), formatType(protocol.MsgMontariaResultado))
	}

	srv, c := mesaDaLixeira(t)
	const conta = 7

	naContaDoRelogio(t, srv, conta, func(_ *world.World, d *Dispatcher, _ *world.Session, e *world.Entity) {
		// O servidor do harness sobe sem catálogo: a poeira precisa do volátil dela
		// para ser poeira. A taxa é a que o harness tiver: aqui só se conta poeira
		// gasta, porque a regra do refino é dos outros testes.
		d.itemVolatiles = map[int]int{itemPoeiraLac: volDustLac, itemAmagoAndaluzN: volAmago}
		e.Carry[0] = pilhaDe(itemPoeiraLac, 10)
		e.Carry[1] = world.Item{Index: itemArmor}
		e.Carry[2] = pilhaDeAmago(itemAmagoAndaluzN, 5)
		m := world.Item{Index: itemAdultaAndaluzN}
		putShort(&m.Effects[0], mountFedValue)
		m.Effects[1].Effect = 30
		e.Equip[mountEquipSlot] = m
	})

	// MORTO: o pedido cai calado e não gasta nada. A resposta que chega depois tem
	// de ser a do pedido seguinte, com o personagem vivo.
	pedeRefino := protocol.RefinoPedeBody{Lugar: world.ItemPlaceCarry, Slot: 1, SlotPoeira: 0, Alvo: 9, MaxPoeiras: 3}
	pedeMontaria := protocol.MontariaPedeBody{SlotAmago: 2, Pilhas: 1}
	naContaDoRelogio(t, srv, conta, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		vivo := e.HP
		e.HP = 0
		// Pelo Handle, e não pela função: é a tabela de rotas que está em prova.
		d.Handle(w, s, protocol.Header{Type: protocol.MsgRefinoPede}, pedeRefino.Encode())
		d.Handle(w, s, protocol.Header{Type: protocol.MsgMontariaPede}, pedeMontaria.Encode())
		e.HP = vivo
		if itemAmount(e.Carry[0]) != 10 || itemAmount(e.Carry[2]) != 5 {
			t.Errorf("morto, e o lote gastou: poeira %d, amago %d", itemAmount(e.Carry[0]), itemAmount(e.Carry[2]))
		}
	})

	// O REFINO, em 0x0F52: o teto de três poeiras, e a pilha desce três.
	send(t, c, protocol.MsgRefinoPede, pedeRefino.Encode())
	var r protocol.RefinoResultadoBody
	if err := r.Decode(respostaDoPainel(t, c, protocol.MsgRefinoResultado)); err != nil {
		t.Fatal(err)
	}
	if r.Motivo != protocol.RefinoTeto || r.Usadas != 3 || r.Sucessos+r.Falhas != 3 || r.Lugar != world.ItemPlaceCarry || r.Slot != 1 {
		t.Errorf("refino pelo fio = %+v, quero TETO com 3 usadas no carry 1", r)
	}

	// A MONTARIA, em 0x0F62: a pilha de cinco inteira.
	send(t, c, protocol.MsgMontariaPede, pedeMontaria.Encode())
	var m protocol.MontariaResultadoBody
	if err := m.Decode(respostaDoPainel(t, c, protocol.MsgMontariaResultado)); err != nil {
		t.Fatal(err)
	}
	if m.Motivo != protocol.MontariaAcabouPacs || m.Usados != 5 || m.Pilhas != 1 || m.Montaria != itemAdultaAndaluzN {
		t.Errorf("montaria pelo fio = %+v, quero ACABOU_PACS com 5 usados e 1 pilha", m)
	}

	naContaDoRelogio(t, srv, conta, func(_ *world.World, _ *Dispatcher, _ *world.Session, e *world.Entity) {
		if got := itemAmount(e.Carry[0]); got != 7 {
			t.Errorf("poeira = %d, quero 7 (10 - 3)", got)
		}
		if !e.Carry[2].Empty() {
			t.Errorf("a pilha de amago devia ter acabado: %+v", e.Carry[2])
		}
	})

	// O PEDIDO DE REFINO NO NÚMERO ANTIGO (0x0F50) é da lixeira, e não pode apagar
	// nada. Existe uma GamePatch.dll de referência de 24/09/2026 que manda o refino
	// em 0x0F50; o corpo dela, lido como lote da lixeira, vira "apague o slot da
	// poeira se o índice dele for o nível alvo" — e a poeira é 412 ou 413, nunca 1
	// a 11. Quem segura é a conferência de índice da lixeira; este teste é o que
	// avisa se um dia ela sair.
	antigo := protocol.RefinoPedeBody{Lugar: world.ItemPlaceCarry, Slot: 1, SlotPoeira: 0, Alvo: 9}
	send(t, c, protocol.MsgLixeiraApaga, antigo.Encode())
	corpo := respostaDoPainel(t, c, protocol.MsgLixeiraResultado)
	if corpo[0] != 0 {
		t.Errorf("o pedido antigo do refino apagou %d item(ns) pela lixeira", corpo[0])
	}
	naContaDoRelogio(t, srv, conta, func(_ *world.World, _ *Dispatcher, _ *world.Session, e *world.Entity) {
		if got := itemAmount(e.Carry[0]); got != 7 || e.Carry[1].Index != itemArmor {
			t.Errorf("o pedido antigo mexeu na mochila: poeira %d, item %d", got, e.Carry[1].Index)
		}
	})
}
