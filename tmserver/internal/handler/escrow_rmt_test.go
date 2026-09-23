package handler

import (
	"bytes"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O escrow da venda em dinheiro real trava o item NO SLOT DO BAÚ, e o que ele
// impede não é o item sair de lá — é o item MUDAR.
//
// Metade dos caminhos que alcançam o baú deixam o item exatamente onde está e
// mudam o que ele é: refino, gema base, adamantita, feijão mágico, acelerador de
// ovo e troca de classe. Se qualquer um deles pegar um item anunciado, a
// conferência da compra (itemsEqual) passa a falhar — e aí o comprador que já
// pagou o Pix não recebe nada, porque o item que ele comprou deixou de existir
// como era.
//
// O refino é o caso escolhido para os testes por ser o pior dos seis: ele é o
// único que não confere proximidade do guarda do baú, então alcança o item de
// qualquer lugar do mapa.

const itemAnuncioDeTeste = int64(4242)

// escrowFixture é a fixture de refino com um baú carregado: o slot 0 tem o item
// TRAVADO por um anúncio, e o slot 1 tem o MESMO item destravado.
//
// Os dois lado a lado são de propósito: é o que separa "a trava funciona" de "o
// refino parou de funcionar". Um guard sabotado para recusar sempre passa no
// primeiro caso e quebra no segundo.
func escrowFixture(t *testing.T) *refineFixture {
	t.Helper()
	f := newRefineFixture(t, alwaysRate(100), nil)
	// A conta 0 não existe para o mundo (world.Cargo recusa zero, porque zero é
	// "sem conta" e não "conta número zero"), e a fixture de refino nasce assim.
	f.s.AccountID = 7
	var cargo world.CargoState
	cargo.Items[0] = world.Item{Index: itemArmor, AnuncioRMT: itemAnuncioDeTeste}
	cargo.Items[1] = world.Item{Index: itemArmor}
	f.w.SetCargo(f.s.AccountID, &cargo)
	return f
}

// refinaNoBau arrasta a poeira do slot 0 da mochila sobre um slot do BAÚ.
func refinaNoBau(f *refineFixture, dust int16, slotDoBau int) {
	f.e.Carry[0] = world.Item{Index: dust}
	body := protocol.MsgUseItemBody{
		SourType: world.ItemPlaceCarry, SourPos: 0,
		DestType: world.ItemPlaceCargo, DestPos: int32(slotDoBau),
	}
	f.d.refineItem(f.w, f.s, f.e, body, 0, f.d.itemVolatiles[int(dust)])
}

func TestEscrowRecusaAlterarItemAnunciado(t *testing.T) {
	f := escrowFixture(t)
	cargo := f.w.Cargo(f.s.AccountID)
	antes := cargo.Items[0]

	refinaNoBau(f, itemPoeiraLac, 0)

	if cargo.Items[0] != antes {
		t.Errorf("o item anunciado mudou: %+v -> %+v", antes, cargo.Items[0])
	}
	// A poeira também não pode ser consumida: cobrar o material por uma recusa
	// seria o defeito pior — o jogador paga e não recebe nem o erro.
	if f.e.Carry[0].Index != itemPoeiraLac {
		t.Errorf("a poeira foi gasta numa recusa: %+v", f.e.Carry[0])
	}
}

// E o MESMO refino, no MESMO item, no slot vizinho sem marca, funciona. É este
// caso que dá valor ao de cima: sem ele, apagar o corpo inteiro do refineItem
// passaria no primeiro teste.
func TestEscrowNaoAtrapalhaItemSemMarca(t *testing.T) {
	f := escrowFixture(t)
	cargo := f.w.Cargo(f.s.AccountID)

	refinaNoBau(f, itemPoeiraLac, 1)

	if cargo.Items[1].Effects[0] != (world.Effect{Effect: efSanc, Value: 1}) {
		t.Errorf("o item SEM marca não refinou: Effects[0] = %+v, quero {EF_SANC 1}",
			cargo.Items[1].Effects[0])
	}
	if !f.e.Carry[0].Empty() {
		t.Errorf("a poeira não foi consumida numa refinação que deu certo: %+v", f.e.Carry[0])
	}
}

// A marca vale para o slot, não para o índice do item: dois itens iguais, um
// anunciado e outro não, seguem caminhos diferentes. Sem isto a trava poderia
// ter sido escrita comparando o índice e ninguém notaria nos testes de cima.
func TestEscrowTravaOSlotENaoOTipoDeItem(t *testing.T) {
	f := escrowFixture(t)
	cargo := f.w.Cargo(f.s.AccountID)

	if cargo.Items[0].Index != cargo.Items[1].Index {
		t.Fatalf("a fixture perdeu o ponto: os dois slots têm de ter o MESMO item")
	}

	refinaNoBau(f, itemPoeiraLac, 0)
	refinaNoBau(f, itemPoeiraLac, 1)

	if refineLevelDoSlot(cargo, 0) != 0 {
		t.Errorf("o slot travado refinou")
	}
	if refineLevelDoSlot(cargo, 1) == 0 {
		t.Errorf("o slot livre não refinou")
	}
}

func refineLevelDoSlot(c *world.CargoState, slot int) uint8 {
	if c.Items[slot].Effects[0].Effect == efSanc {
		return c.Items[slot].Effects[0].Value
	}
	return 0
}

// O jogador precisa SABER por que não aconteceu nada. Devolver nil calado é o
// pior defeito de suporte que existe: os sete chamadores do itemSlot tratam nil
// como "esse slot não existe" e voltam sem dizer nada, então o jogador arrasta a
// poeira e a tela não reage. Ele não sabe descrever e ninguém sabe reproduzir.
func TestEscrowAvisaOJogador(t *testing.T) {
	db := newDB()
	var mochila [world.MaxCarry]world.Item
	mochila[0] = world.Item{Index: itemPoeiraLac}
	db.loads = map[int64]world.CharacterState{
		7: {Slot: 0, Name: "Dono", Level: 50, HP: 1000, MaxHP: 1000, Carry: mochila},
	}
	var cargo world.CargoState
	cargo.Items[0] = world.Item{Index: itemArmor, AnuncioRMT: itemAnuncioDeTeste}
	db.accounts["tester"].cargo = cargo

	addr, stop := startServerClockVol(t, db, map[int]int{itemPoeiraLac: volDustLac})
	defer stop()
	c := enterWorldAs(t, addr, "tester")
	defer c.Close()
	drena(t, c)

	body := protocol.MsgUseItemBody{
		SourType: world.ItemPlaceCarry, SourPos: 0,
		DestType: world.ItemPlaceCargo, DestPos: 0,
	}
	send(t, c, protocol.MsgUseItem, body.Encode())

	esperado := protocol.EncodeMessagePanelBody(msgItemAVenda)
	avisado := false
	for {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		// A comparacao e contra o corpo CODIFICADO, e nao contra o texto em Go:
		// o servidor converte a mensagem de UTF-8 para a tabela do cliente antes
		// de mandar, entao "à venda" nao chega como os mesmos bytes que a
		// constante tem aqui. Codificar os dois lados e o unico jeito honesto de
		// perguntar "foi ESTA mensagem".
		if ty == protocol.MsgMessagePanel && bytes.Equal(payload, esperado) {
			avisado = true
		}
	}
	if !avisado {
		t.Errorf("a recusa nao chegou ao jogador: ele arrastou a poeira e a tela nao disse nada")
	}
}
