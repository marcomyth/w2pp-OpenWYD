package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A entrega de item por nível (DoItemLevel, Server.cpp:9744).
//
// Ao subir de nível o jogo escolhe uma peça pela ficha e a põe no ARMAZÉM da
// conta — não na bolsa. É o que veste o personagem no caminho do 29 ao 254:
// cinco peças de set, doze armas e uma montaria, a Cavalo_Leve_N do nível 149.
//
// Estava no conteúdo e nunca foi lido: o LevelItem.txt viaja no Release desde
// sempre, o legado o carrega em três lugares, e do lado Go não havia uma única
// referência ao arquivo. Por isso ninguém recebia nada.

// construcaoDoPersonagem é o `type` do legado: qual atributo BASE é o maior.
// Força vence empate com Int e Des; Int vence Des; e qualquer outra coisa —
// Constituição maior, ou empate entre os que sobraram — cai no 3. A ordem das
// comparações é a do original (Server.cpp:9748-9758), e ela não é simétrica:
// dois atributos empatados no topo não caem no 0 nem no 1, caem no 3.
//
// Lê o BaseScore de propósito. Os campos vivos (e.Str, e.Int…) são base mais
// equipamento, e usá-los faria a peça do nível depender do que o jogador está
// vestindo na hora — inclusive da peça anterior desta mesma entrega.
func construcaoDoPersonagem(e *world.Entity) int {
	f, i, d, c := e.BaseStr, e.BaseInt, e.BaseDex, e.BaseCon
	switch {
	case f > i && f > d && f > c:
		return 0
	case i > f && i > d && i > c:
		return 1
	case d > f && d > i && d > c:
		return 2
	default:
		return 3
	}
}

// entregaItemDeNivel dá ao personagem a peça do nível que ele acabou de
// alcançar, se houver uma.
//
// Chamado UMA vez por subida, com o nível final — é o que o legado faz: os
// quatro pontos que chamam DoItemLevel chamam depois do CheckGetLevel, com o
// nível já atualizado. Quem pula três níveis de uma vez recebe a peça do nível
// em que parou, não as três.
//
// A peça vai para o armazém da conta, como no original (ITEM_PLACE_CARGO). Se
// não couber, o legado desiste calado e o item some. Aqui o item também não é
// entregue — a fidelidade da regra fica —, mas fica registrado com nome, conta e
// item, porque um item que some sem rastro é um chamado de suporte sem resposta,
// e a equipe já tem uma lista desses para investigar.
func (d *Dispatcher) entregaItemDeNivel(w *world.World, s *world.Session, e *world.Entity) {
	if d.levelItems == nil || s == nil || e == nil {
		return
	}
	item := d.levelItems.Para(int(e.Class), construcaoDoPersonagem(e), e.Level)
	if item.Empty() {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		d.log.Warn("item de nível: a conta não tem armazém carregado",
			"conn", s.Conn, "personagem", e.Name, "nivel", e.Level, "item", item.Index)
		return
	}
	vaga := firstEmptyCargoSlot(cargo)
	if vaga < 0 {
		d.log.Warn("item de nível: armazém cheio, o item não foi entregue",
			"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name,
			"nivel", e.Level, "item", item.Index)
		return
	}
	it := world.Item{Index: item.Index}
	for i, ef := range item.Effects {
		it.Effects[i] = world.Effect{Effect: ef[0], Value: ef[1]}
	}
	cargo.Items[vaga] = it
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(world.ItemPlaceCargo, vaga, itemToSel(it)))
	sendClientMessage(w, s, "Um item chegou ao seu armazém. ["+d.itemName(item.Index)+"]")
	d.log.Info("item de nível entregue",
		"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name, "classe", e.Class,
		"construcao", construcaoDoPersonagem(e), "nivel", e.Level, "item", item.Index, "vaga", vaga)
}
