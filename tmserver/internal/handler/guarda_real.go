package handler

import (
	"fmt"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Guarda Real (Merchant 100, EF_GRADE0 13) vende a capa do reino por uma Safira.
//
// REGRA NOSSA, não do legado (Marco, 27/09/2026). No original ele só levava o
// Mortal de nível 199 a 253 para (1740, 1725), onde os Cav. Mortal soltam o
// Emblema do Reino, que o Rei trocava pela capa (QUEST_CAPAREAL,
// _MSG_Quest.cpp:2342-2361). Agora a capa sai do próprio Guarda: o primeiro
// clique diz o preço, e o clique de confirmar cobra uma Safira e veste a capa. A
// área antiga fica livre para uma quest nova.
//
// Cada reino tem o seu Guarda (o byte Clan do template): Guarda_Real, clan 7, dá o
// Manto_Hekalotia; Guarda_Real_, clan 8, o Manto_Akelonia — as mesmas capas que o
// Rei entrega no modo 0 (_MSG_Quest.cpp:1042-1050).
//
// A capa só entra no slot vazio. Quem já tem capa de reino é avisado, e quem veste
// outra capa tem de tirá-la antes: trocar calado apagaria a capa do jogador.

const (
	guardaRealPreco = 1   // Safiras
	capaHekalotia   = 545 // Manto_Hekalotia
	capaAkelonia    = 546 // Manto_Akelonia
)

// capaDoGuardaReal é a capa que o Guarda do reino clan entrega.
func capaDoGuardaReal(clan uint8) (int16, bool) {
	switch clan {
	case clanHekalotia:
		return capaHekalotia, true
	case clanAkelonia:
		return capaAkelonia, true
	}
	return 0, false
}

func (d *Dispatcher) guardaReal(w *world.World, s *world.Session, e, npc *world.Entity, confirm int) {
	capa, ok := capaDoGuardaReal(npc.Clan)
	if !ok {
		d.notify(w, s, NoticeReqNotMet)
		return
	}
	nome := strings.ReplaceAll(d.itemName(capa), "_", " ")
	if reino, _ := capeKingdomMode(e.Equip[capeEquipSlot].Index); reino != 0 {
		sendClientMessage(w, s, "Você já tem a capa de um reino.")
		return
	}
	if !e.Equip[capeEquipSlot].Empty() {
		sendClientMessage(w, s, "Tire a capa que está vestida para receber a capa do reino.")
		return
	}
	if confirm == 0 {
		sendClientMessage(w, s, fmt.Sprintf("Entregue 1 Safira e receba o %s.", nome))
		return
	}

	changed, pagamento := sapphirePaymentPlan(e, guardaRealPreco)
	switch pagamento {
	case sapphireShort:
		sendClientMessage(w, s, "Você precisa de 1 Safira.")
		return
	case sapphireNoRoom:
		sendClientMessage(w, s, "Libere espaço no inventário para o troco das safiras.")
		return
	}
	e.Equip[capeEquipSlot] = world.Item{Index: capa}
	e.Clan = npc.Clan
	for _, slot := range changed {
		d.sendSlot(w, s, world.ItemPlaceCarry, slot, e.Carry[slot])
	}
	d.sendSlot(w, s, world.ItemPlaceEquip, capeEquipSlot, e.Equip[capeEquipSlot])
	d.refreshScore(e)
	d.sendScore(w, s, e)
	sendClientMessage(w, s, fmt.Sprintf("Você recebeu o %s.", nome))
	d.log.Info("guarda real: capa do reino entregue",
		"conn", s.Conn, "conta", s.AccountID, "personagem", e.Name, "clan", npc.Clan, "capa", capa)
}
