package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// useBirthAccelerator is the Acelerador de Nascimento (3438, EF_VOLATILE 196):
// drag it onto a mount egg and the egg gains one refine level, with no roll.
//
// NO LEGACY COUNTERPART: _MSG_UseItem.cpp has no branch for EF_VOLATILE 196, so
// there is nothing to port and nothing to match byte for byte. The behaviour is
// taken from the tooltip the shipped client already draws for the item —
// "Aumenta o valor da refinação em +1 independente da previsão do nascimento da
// montaria. Aplicável a todos ovos de montaria." — because that text is what the
// player is promised before they spend the item, and it is the only
// specification that exists.
//
// Two clauses of that tooltip carry weight:
//
//   - "+1 independente da previsão do nascimento": the incubation timer a failed
//     refine stamps on the egg (efIncuDelay) does NOT block this, which is the
//     whole point of an accelerator. A dust refuses with NoticeIncuWaitMore while
//     that timer runs; this does not.
//   - "aplicável a todos ovos de montaria": eggs only. Anything else is refused
//     rather than silently eaten — a premium item that vanishes with no message
//     reads as theft.
//
// The gained level can hatch the egg, on the same threshold a dust uses
// (hatchEgg reads the instance EF_INCUBATE), so an accelerator is a guaranteed
// step toward the mount rather than a way around the last one.
// eggRefineCeiling mirrors refine's own maxSancLvl (internal/refine/sanc.go).
// Duplicated on purpose: it is unexported there, and the check has to run before
// the write for the reason spelled out at its use site.
const eggRefineCeiling = 15

func (d *Dispatcher) useBirthAccelerator(w *world.World, s *world.Session, e *world.Entity, body protocol.MsgUseItemBody, src int) {
	dst := d.itemSlot(w, s, e, int(body.DestType), int(body.DestPos))
	// Logged on EVERY use, before the gates. A refusal here is indistinguishable
	// on the client from "the item does nothing" — the player only sees a line —
	// and the two have completely different causes: a target that is not an egg,
	// versus a use that carried no target at all because the item was clicked
	// instead of dragged onto the egg.
	target := int16(0)
	if dst != nil {
		target = dst.Index
	}
	d.log.Info("birth accelerator attempt",
		"conn", s.Conn, "account", s.AccountName,
		"dest_type", body.DestType, "dest_pos", body.DestPos, "target", target)

	if dst == nil || dst.Empty() {
		return // no target slot: the legacy drops the message, and so do we
	}
	if !isEgg(*dst) {
		// Refused, NOT consumed. NoticeCantUseHere carries _NN_Cant_Use_That_Here,
		// so the player is told why instead of watching the item disappear.
		d.refineReject(w, s, e, src, NoticeCantUseHere)
		return
	}
	if d.itemAbility(*dst, efNoSanc) != 0 {
		d.refineReject(w, s, e, src, NoticeCantRefineMore)
		return
	}

	// A never-refined egg has nowhere to store a level yet, and refine.Set is a
	// no-op on one — the accelerator would be eaten for nothing. Bootstrap plants
	// the EF_SANC pair, and fails only when all three effect slots hold real
	// effects, which is the legacy's own refusal to refine such an item.
	if refine.Level(*dst) == 0 && !refine.Bootstrap(dst) {
		d.refineReject(w, s, e, src, NoticeCantRefineMore)
		return
	}

	level := refine.Level(*dst) + 1
	// The ceiling is checked BEFORE the write, not clamped after: refine.Set
	// encodes an out-of-range level as +0 (the legacy quirk, refine/sanc.go), so
	// writing +16 would not cap the egg — it would wipe its refine entirely.
	if level > eggRefineCeiling {
		d.refineReject(w, s, e, src, NoticeCantRefineMore)
		return
	}
	// A success clears the pity counter, exactly as refineSucceed does: the
	// accelerator is a won refine, not a bypass of the refine system.
	refine.Set(dst, level, 0)

	t := refineTarget{item: dst, place: int(body.DestType), slot: int(body.DestPos)}
	d.notify(w, s, NoticeRefineSuccess)
	if isEgg(*dst) {
		d.hatchEgg(w, s, t, level)
	}

	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	d.sendSlot(w, s, t.place, t.slot, *t.item)
	d.log.Info("birth accelerator used",
		"conn", s.Conn, "account", s.AccountName, "egg", dst.Index, "level", level)
}
