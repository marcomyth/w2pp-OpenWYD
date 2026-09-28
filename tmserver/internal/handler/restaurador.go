package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Restauradores (_MSG_UseItem.cpp:5101-5184, EF_VOLATILE 93): the seven items
// 3351..3357 that give an ADULT mount back one or two points of vitality. They
// are the adult's counterpart to the Catalisadores, and they split the lineages
// into the same seven groups — the legacy runs the identical chain of ifs over
// sIndex-2363 instead of sIndex-2333, which is catalisadorGrupo on the cria row
// of the same lineage.
//
// Until this port the item existed, had a name and a tooltip, and fell through
// to rejectUnimplementedConsumable: clicking it did nothing. The donate shop
// sells three of them, and a paid item that does nothing is a chargeback.

// restauradorLo is the first restorer item (Restaurador de Kapel).
const restauradorLo = 3351

// The legacy's vitality window (:5156): a mount at 5 or below, or at 50 or
// above, is refused. Below is a mount the restorer will not rescue; above is
// the ceiling the item does not push past.
const (
	restauradorVitalidadeMin = 5
	restauradorVitalidadeMax = 50
)

// msgRestauradorSemEfeito is this fork's line for the vitality refusal. The
// legacy hands the item back without a word (:5156-5161), which reads as "the
// item is broken" — the same reason ração got msgMountNeedsCure.
const msgRestauradorSemEfeito = "O Restaurador só age em montaria viva com vitalidade entre 6 e 49."

// restauradorGrupo maps an adult mount to the restorer that serves it.
func restauradorGrupo(adulta int16) (int, bool) {
	return catalisadorGrupo(adulta - mountRowSize)
}

func (d *Dispatcher) useRestaurador(w *world.World, s *world.Session, e *world.Entity, body protocol.MsgUseItemBody, src int) {
	// Applied TO the worn mount, like the catalyst (:5105-5119).
	if body.DestType != 0 || body.DestPos != mountEquipSlot {
		d.notify(w, s, NoticeMountNotMatch)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	dst := &e.Equip[mountEquipSlot]
	grupo, ok := restauradorGrupo(dst.Index)
	if !ok || grupo != int(e.Carry[src].Index)-restauradorLo {
		d.notify(w, s, NoticeMountNotMatch)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	vit := dst.Effects[1].Value
	if vit >= restauradorVitalidadeMax || vit <= restauradorVitalidadeMin || mountHP(*dst) <= 0 {
		sendClientMessage(w, s, msgRestauradorSemEfeito)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	// rand()%2+1 (:5163), drawn from the shared MSVC stream so the call order
	// matches the legacy's.
	dst.Effects[1].Value = vit + uint8(w.Rand().Intn(2)+1)
	consumeOneItem(&e.Carry[src])

	// The legacy plays the same "face or bare" animation the failed refine uses
	// (:5167-5172).
	motion := motionRefineFailBare
	if e.Equip[0].Index/10 != 0 {
		motion = motionRefineFailFaced
	}
	sendEmotion(w, s, e, motion, 0)
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceEquip, mountEquipSlot, *dst)
	d.log.Info("restaurador used",
		"conn", s.Conn, "account", s.AccountName, "mount", dst.Index, "grupo", grupo,
		"vitalidade", dst.Effects[1].Value)
}
