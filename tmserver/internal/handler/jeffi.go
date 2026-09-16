package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Jeffi (Merchant 12, JEFFI — _MSG_Quest.cpp:514-610), the alchemist of Armia
// and Erion. Two services, chosen by what the player wears in the circle slot:
//
//   - a Pedaço do Círculo Divino (447) or its Comp version (692) there is purified
//     into one of the three Círculos Divinos Puros, for 1M or 5M gold;
//   - otherwise every 10 Restos de Oriharucon/Lactolerium in the bag become one
//     Poeira, all of them for a single 1M gold.
//
// The legacy never reads confirm here: one click on Jeffi does the work.
const merchantJeffi = 12

const (
	jeffiCircleSlot = 13

	itemCirclePiece     int16 = 447 // Pedaço_do_Círculo_Divino
	itemCirclePure      int16 = 448 // Círculo_Divino_Puro, 448-450
	itemCircleCompPiece int16 = 692 // Pedaço_do_Círc_Div_Comp
	itemCircleCompPure  int16 = 693 // Círculo_Divino_Comp_Puro, 693-695
	jeffiCircleVariants       = 3   // rand() % 3 over the three EF_INITn circles

	jeffiCirclePrice     int32 = 1000000
	jeffiCircleCompPrice int32 = 5000000

	itemRestoOri   int16 = 419 // Resto_de_Oriharucon
	itemRestoLac   int16 = 420 // Resto_de_Lactolerium
	jeffiPoeiraOri int16 = 412 // Poeira_de_Oriharucon
	jeffiPoeiraLac int16 = 413 // Poeira_de_Lactolerium

	jeffiRestosPerPoeira       = 10
	jeffiPoeiraPrice     int32 = 1000000
)

// The NPC's lines, copied from Language.txt (152, 157, 166) as the fallback for
// a server booted without -content.
const (
	msgNeed1000000Gold = "Requer 1000000 gold."
	msgNeed5000000Gold = "Para refinar o item são necessários 5000000 Gold."
	msgNeed10Particle  = "Você pode agrupar Ori/Lac em 10 unidades."
)

// jeffi is the JEFFI case of _MSG_Quest.
func (d *Dispatcher) jeffi(w *world.World, s *world.Session, e, npc *world.Entity) {
	circle := e.Equip[jeffiCircleSlot].Index
	d.log.Info("jeffi", "conn", s.Conn, "account", s.AccountName, "circle", circle, "coin", e.Coin)
	if circle == itemCirclePiece || circle == itemCircleCompPiece {
		d.jeffiPurifyCircle(w, s, e, npc)
		return
	}
	d.jeffiPoeira(w, s, e, npc)
}

// jeffiPurifyCircle is the first branch (:517-560): the piece in the circle slot
// becomes a pure circle, picked at random among the three.
func (d *Dispatcher) jeffiPurifyCircle(w *world.World, s *world.Session, e, npc *world.Entity) {
	slot := &e.Equip[jeffiCircleSlot]
	price, pure := jeffiCirclePrice, itemCirclePure
	key, fallback := "_NN_Need_1000000_Gold", msgNeed1000000Gold
	if slot.Index == itemCircleCompPiece {
		price, pure = jeffiCircleCompPrice, itemCircleCompPure
		key, fallback = "_NN_Need_5000000_Gold", msgNeed5000000Gold
	}
	if e.Coin < price {
		d.say(w, npc, key, fallback)
		return
	}
	e.Coin -= price
	from := slot.Index
	// Only sIndex changes (:536/:541): the piece's effects ride along.
	slot.Index = pure + int16(w.Rand().Intn(jeffiCircleVariants))

	d.sendSlot(w, s, world.ItemPlaceEquip, jeffiCircleSlot, *slot)
	d.sendEtc(w, s, e)
	d.say(w, npc, "_NN_Processing_Complete", msgProcessingComplete)
	// The legacy also flashes SetAffect(conn, 44, 20, 20) here, a cosmetic effect
	// deferred like the one on the Sephira books (useSkillBook).
	d.sendScore(w, s, e)
	d.log.Info("jeffi circle purified", "conn", s.Conn, "account", s.AccountName,
		"from", from, "to", slot.Index, "price", price)
}

// jeffiPoeira is the second branch (:562-607): the Restos become Poeiras.
func (d *Dispatcher) jeffiPoeira(w *world.World, s *world.Session, e, npc *world.Entity) {
	limit := activeCarryLimit(e)
	ori := countCarried(e, limit, itemRestoOri)
	lac := countCarried(e, limit, itemRestoLac)
	if ori < jeffiRestosPerPoeira && lac < jeffiRestosPerPoeira {
		d.say(w, npc, "_NN_Need_10_Particle", msgNeed10Particle)
		return
	}
	if e.Coin < jeffiPoeiraPrice {
		d.say(w, npc, "_NN_Need_1000000_Gold", msgNeed1000000Gold)
		return
	}

	// Ori before Lac on every round, as the legacy loop orders the Combine calls
	// (:579-591) — that order is also the order the prizes draw rand().
	made := 0
	for ori >= jeffiRestosPerPoeira || lac >= jeffiRestosPerPoeira {
		progressed := false
		if ori >= jeffiRestosPerPoeira && jeffiCombine(w, e, limit, itemRestoOri, jeffiPoeiraOri) {
			ori -= jeffiRestosPerPoeira
			made++
			progressed = true
		}
		if lac >= jeffiRestosPerPoeira && jeffiCombine(w, e, limit, itemRestoLac, jeffiPoeiraLac) {
			lac -= jeffiRestosPerPoeira
			made++
			progressed = true
		}
		if !progressed {
			break // the bag is full: what is left stays as Restos
		}
	}
	if made == 0 {
		d.notify(w, s, NoticeNoEmptySlot)
		return
	}

	// One price for the whole batch (:593), however many Poeiras came out.
	e.Coin -= jeffiPoeiraPrice
	d.say(w, npc, "_NN_Processing_Complete", msgProcessingComplete)
	d.sendScore(w, s, e)
	d.sendCarry(w, s, e)
	d.sendEtc(w, s, e)
	d.log.Info("jeffi poeira", "conn", s.Conn, "account", s.AccountName,
		"made", made, "price", jeffiPoeiraPrice)
}

// jeffiCombine is Combine (Server.cpp:9448): takes 10 of item from the bag and
// puts one prize, carrying three random EF_UNIQUE values, in the first free slot.
//
// DELIBERATE DIVERGENCE on two points, both of them the legacy eating the
// player's Restos:
//   - the legacy clears whole stacks until it has cleared as much as it COUNTED,
//     and its count only stops after crossing 10 — a stack of 8 next to one of 20
//     costs 28 Restos for one Poeira. Here exactly 10 go.
//   - with a full bag the legacy drops the Poeira on the floor (:9527), and this
//     port has no ground item for it. Here the combine does not happen and the
//     Restos stay in the bag.
//
// And one for the bag: the Poeira lands on a Poeira pile below 120 before it
// takes a free slot. The Restos drop in piles of 120, so one click can make a
// hundred Poeiras, and one per slot, as the legacy does, would stop the batch at
// the few free slots. The EF_UNIQUE draws still happen, in the legacy order.
func jeffiCombine(w *world.World, e *world.Entity, limit int, item, prize int16) bool {
	// Take first, then look for room: using up a stack frees its slot. The copy
	// puts the bag back when there is no room after all.
	before := e.Carry
	need := jeffiRestosPerPoeira
	for i := 0; i < limit && need > 0; i++ {
		it := &e.Carry[i]
		if it.Index != item {
			continue
		}
		if n := itemAmount(*it); n > need {
			setItemAmount(it, n-need)
			need = 0
		} else {
			*it = world.Item{}
			need -= n
		}
	}
	pile := jeffiPoeiraPile(e, limit, prize)
	slot := firstEmptyCarrySlot(e.Carry[:], limit)
	if need > 0 || (pile < 0 && slot < 0) {
		e.Carry = before
		return false
	}
	// rand() % 100, rand() % 100, rand() % 50 + 50, in that order (:9498-9505).
	p := world.Item{Index: prize}
	p.Effects[0] = world.Effect{Effect: efUnique, Value: uint8(w.Rand().Intn(100))}
	p.Effects[1] = world.Effect{Effect: efUnique, Value: uint8(w.Rand().Intn(100))}
	p.Effects[2] = world.Effect{Effect: efUnique, Value: uint8(w.Rand().Intn(50) + 50)}
	// The Poeira stacks here, and a stackable saved without EF_AMOUNT kills the
	// client at login (countStacksMissingAmount): the count takes the first draw's
	// slot, as setItemAmount would.
	p.Effects[0] = world.Effect{Effect: efAmount, Value: 1}
	if pile >= 0 {
		setItemAmount(&e.Carry[pile], itemAmount(e.Carry[pile])+1)
		return true
	}
	e.Carry[slot] = p
	return true
}

// jeffiPoeiraPile is the first pile of this Poeira in the bag with room for one
// more, or -1. The EF_UNIQUE draws do not split piles (stackIdentity).
func jeffiPoeiraPile(e *world.Entity, limit int, prize int16) int {
	for i := 0; i < limit; i++ {
		it := e.Carry[i]
		if sameStackClass(it, world.Item{Index: prize}) && itemAmount(it) < maxStackAmount && canWriteItemAmount(it) {
			return i
		}
	}
	return -1
}

// countCarried sums the units of item in the accessible bag, stacks counted by
// their EF_AMOUNT.
func countCarried(e *world.Entity, limit int, item int16) int {
	n := 0
	for i := 0; i < limit; i++ {
		if e.Carry[i].Index == item {
			n += itemAmount(e.Carry[i])
		}
	}
	return n
}
