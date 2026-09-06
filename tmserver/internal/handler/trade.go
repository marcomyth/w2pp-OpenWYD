package handler

import (
	"context"
	"encoding/binary"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// maxTradeSlot is the absolute upper bound for normal player carry slots. The
// per-character active limit may be lower when Bolsa do Andarilho is inactive.
const maxTradeSlot = maxUnlockedCarry

// trade handles _MSG_Trade (0x0383): validate the offer and confirm; when BOTH
// sides have confirmed a matching trade, perform the atomic swap. Any validation
// failure cancels the trade on both sides (anti-dup). The offer is checked by
// memcmp against the real inventory (anti item-swap during confirm).
func (d *Dispatcher) trade(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP == 0 || s.Mode != world.UserPlay {
		w.AddCrackError(s, 5, 18)
		d.removeTrade(w, s)
		return
	}
	var body protocol.MsgTradeBody
	if err := body.Decode(payload); err != nil {
		d.removeTrade(w, s)
		return
	}
	opp := int(body.OpponentID)
	other := w.Session(opp)
	if opp <= 0 || opp >= world.MaxUser || other == nil || other.Mode != world.UserPlay {
		d.removeTrade(w, s)
		return
	}
	if body.TradeMoney < 0 || body.TradeMoney > e.Coin {
		d.removeTrade(w, s)
		return
	}

	var slots []int
	for i := 0; i < protocol.MaxTrade; i++ {
		if body.Item[i].Index == 0 {
			continue
		}
		pos := int(body.InvenPos[i])
		if pos < 0 || pos >= maxTradeSlot || !carrySlotAccessible(e, pos) || !sameItem(body.Item[i], e.Carry[pos]) {
			d.removeTrade(w, s) // bounds or item changed during confirm
			return
		}
		slots = append(slots, pos)
	}

	s.Trade.Active = true
	s.Trade.OpponentID = opp
	s.Trade.Money = body.TradeMoney
	s.Trade.Slots = slots
	s.Trade.Confirmed = body.MyCheck != 0

	if s.Trade.Confirmed && other.Trade.Active && other.Trade.OpponentID == s.Conn && other.Trade.Confirmed {
		d.executeSwap(w, s, other)
		return
	}
	// First confirm: acknowledge (empty result); the swap fires on the second.
	w.Send(s, protocol.MsgTrade, tradeResultPayload(nil))
}

// executeSwap transfers both offers atomically (validate-all-then-apply-all):
// items are taken from both sides, room is checked, then handed over with money.
// Any shortfall rolls back and cancels the trade.
func (d *Dispatcher) executeSwap(w *world.World, a, b *world.Session) {
	ea, eb := w.Entity(a.Conn), w.Entity(b.Conn)
	if ea == nil || eb == nil {
		d.removeTrade(w, a)
		return
	}
	if !tradeSlotsAccessible(ea, a.Trade.Slots) || !tradeSlotsAccessible(eb, b.Trade.Slots) {
		d.removeTrade(w, a)
		return
	}

	aItems := takeItems(ea, a.Trade.Slots)
	bItems := takeItems(eb, b.Trade.Slots)
	if freeCarry(eb) < len(aItems) || freeCarry(ea) < len(bItems) {
		putBack(ea, a.Trade.Slots, aItems) // not enough room → rollback
		putBack(eb, b.Trade.Slots, bItems)
		d.removeTrade(w, a)
		return
	}
	for _, it := range aItems {
		if dst := firstEmptyAccessibleCarry(eb); dst >= 0 {
			eb.Carry[dst] = it
		}
	}
	for _, it := range bItems {
		if dst := firstEmptyAccessibleCarry(ea); dst >= 0 {
			ea.Carry[dst] = it
		}
	}
	ea.Coin += b.Trade.Money - a.Trade.Money
	eb.Coin += a.Trade.Money - b.Trade.Money

	// Captured before the states are cleared two lines below. Reading Money
	// after that clear is the obvious mistake here, and it records every trade
	// as having involved no gold at all.
	ouroA, ouroB := a.Trade.Money, b.Trade.Money

	a.Trade = world.TradeState{}
	b.Trade = world.TradeState{}
	// Result to each side carries the items they received (UNVERIFIED layout;
	// the real handler re-sends inventory slots via _MSG_SendItem).
	w.Send(a, protocol.MsgTrade, tradeResultPayload(bItems))
	w.Send(b, protocol.MsgTrade, tradeResultPayload(aItems))

	d.recordTrade(w, a, b, ea.Name, eb.Name, ouroA, ouroB, aItems, bItems)

	// Both sides persisted NOW, not at logout.
	//
	// Nothing else here saves, and until this line neither did the trade: items
	// reached Postgres only when a character left play, and there is no periodic
	// save. That opened the shortest path to a duplicate this server has, and it
	// needed no exploit at all — A hands B a sword, B logs out and is saved with
	// it, the process dies before A logs out, and A comes back with the rows it
	// had before the trade. Two swords, one creation, neither player trying.
	//
	// The same window destroys items when the order is reversed: A saves, the
	// server dies, B never saves, the sword is gone and somebody opens a ticket.
	//
	// This shrinks that window from hours to the length of one write. The pattern
	// is the one arch, combine and guild already use; the trade is simply the one
	// that moves the most value and had it missing.
	w.SaveCharacterAsync(a)
	w.SaveCharacterAsync(b)
}

// tradeResultPayload encodes the received items as count + WireItems (placeholder
// result body for testing/observability; UNVERIFIED real layout).
func tradeResultPayload(items []world.Item) []byte {
	b := make([]byte, 1+len(items)*protocol.ItemSize)
	b[0] = byte(len(items))
	for i, it := range items {
		off := 1 + i*protocol.ItemSize
		binary.LittleEndian.PutUint16(b[off:off+2], uint16(it.Index))
		for e := 0; e < 3; e++ {
			b[off+2+e*2] = it.Effects[e].Effect
			b[off+3+e*2] = it.Effects[e].Value
		}
	}
	return b
}

// quitTrade handles _MSG_QuitTrade (0x0384): cancel the trade.
func (d *Dispatcher) quitTrade(w *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	if e := w.Entity(s.Conn); e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		w.AddCrackError(s, 10, 17)
	}
	d.removeTrade(w, s)
}

// removeTrade cancels any active trade on s and its opponent, notifying both.
// It is also the anti-dup hook called when a player drops/uses/attacks mid-trade.
func (d *Dispatcher) removeTrade(w *world.World, s *world.Session) {
	// RemoveTrade in the original also closes an open personal shop (Server.cpp:8124);
	// this is what makes walking/buying/item-ops/quit-trade tear the stall down.
	d.closeAutoTrade(w, s)
	if !s.Trade.Active {
		return
	}
	opp := s.Trade.OpponentID
	s.Trade = world.TradeState{}
	w.Send(s, protocol.MsgQuitTrade, nil)
	if other := w.Session(opp); other != nil && other.Trade.OpponentID == s.Conn {
		other.Trade = world.TradeState{}
		w.Send(other, protocol.MsgQuitTrade, nil)
	}
}

// sameItem reports whether a wire item equals the inventory item (the memcmp
// used to detect an item swapped in during confirmation).
func sameItem(wi protocol.WireItem, it world.Item) bool {
	if wi.Index != it.Index {
		return false
	}
	for i := 0; i < 3; i++ {
		if wi.Effects[i].Effect != it.Effects[i].Effect || wi.Effects[i].Value != it.Effects[i].Value {
			return false
		}
	}
	return true
}

func freeCarry(e *world.Entity) int {
	return freeAccessibleCarry(e)
}

func tradeSlotsAccessible(e *world.Entity, slots []int) bool {
	for _, sl := range slots {
		if sl < 0 || sl >= maxTradeSlot || !carrySlotAccessible(e, sl) || e.Carry[sl].Empty() {
			return false
		}
	}
	return true
}

// takeItems removes the items at the given carry slots, returning them aligned to
// slots (so putBack can restore them on rollback).
func takeItems(e *world.Entity, slots []int) []world.Item {
	out := make([]world.Item, len(slots))
	for i, sl := range slots {
		if sl >= 0 && sl < world.MaxCarry {
			out[i] = e.Carry[sl]
			e.Carry[sl] = world.Item{}
		}
	}
	return out
}

func putBack(e *world.Entity, slots []int, items []world.Item) {
	for i, sl := range slots {
		if sl >= 0 && sl < world.MaxCarry {
			e.Carry[sl] = items[i]
		}
	}
}

// recordTrade writes the trade to the log off the loop (World.GoDetached — not
// World.Go, which is session-bound), the same way duel results are recorded.
//
// A trade that moved nothing is not written: both sides can confirm an empty
// window, and those rows would be noise in the one screen a moderator opens
// when somebody reports a scam.
func (d *Dispatcher) recordTrade(w *world.World, a, b *world.Session,
	nomeA, nomeB string, ouroA, ouroB int32, itensA, itensB []world.Item,
) {
	if ouroA == 0 && ouroB == 0 && len(itensA) == 0 && len(itensB) == 0 {
		return
	}
	rec := world.TradeRecord{
		CharA: nomeA, CharB: nomeB,
		AccountA: a.AccountID, AccountB: b.AccountID,
		GoldA: ouroA, GoldB: ouroB,
		ItemsA: tradeItemsForLog(itensA),
		ItemsB: tradeItemsForLog(itensB),
	}
	p := w.Persistence()
	w.GoDetached(func() func(*world.World) {
		if err := p.RecordTrade(context.Background(), rec); err != nil {
			return func(*world.World) {
				d.log.Warn("trade log: persistence failed",
					"a", rec.CharA, "b", rec.CharB, "err", err)
			}
		}
		return nil
	})
}

func tradeItemsForLog(items []world.Item) []world.TradeItem {
	out := make([]world.TradeItem, 0, len(items))
	for _, it := range items {
		if it.Empty() {
			continue
		}
		t := world.TradeItem{Index: int32(it.Index)}
		for i := 0; i < 3; i++ {
			t.Eff[i] = [2]uint8{it.Effects[i].Effect, it.Effects[i].Value}
		}
		out = append(out, t)
	}
	return out
}
