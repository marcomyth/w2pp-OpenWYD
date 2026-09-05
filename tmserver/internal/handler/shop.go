package handler

import (
	"encoding/binary"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// reqShopList handles _MSG_REQShopList (0x027B): the client clicked a merchant
// NPC. The shop list is the NPC's own Carry[] (SendFunc.cpp:SendShopList).
// _MSG_REQShopList.cpp: Merchant 1 → ShopType 1, Merchant 19 → ShopType 3.
func (d *Dispatcher) reqShopList(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if s.Mode != world.UserPlay || len(payload) < 2 {
		return
	}
	target := int(binary.LittleEndian.Uint16(payload[0:2]))
	npc := w.Entity(target)
	if npc == nil || npc.Mode == world.MobEmpty || npc.Merchant == 0 {
		return // not a merchant
	}
	// Merchant==2 is the cargo guard (Guarda-Carga): it opens the account
	// warehouse, not a buy/sell list. UNVERIFIED: the Merchant==2 tagging of the
	// Release/ NPCs is not yet confirmed by capture.
	if npc.Merchant == 2 {
		d.openCargo(w, s)
		return
	}
	shopType := int32(1)
	if npc.Merchant == 19 {
		shopType = 3
	}
	var list [27]protocol.SelItem
	for i := 0; i < 27; i++ {
		c := npc.Carry[protocol.ShopSlot(i)]
		list[i] = protocol.SelItem{
			Index: uint16(c.Index),
			Eff: [3][2]uint8{
				{c.Effects[0].Effect, c.Effects[0].Value},
				{c.Effects[1].Effect, c.Effects[1].Value},
				{c.Effects[2].Effect, c.Effects[2].Value},
			},
		}
	}
	body := protocol.EncodeShopListBody(shopType, list, 0) // Tax 0 (city-tax table UNVERIFIED)
	w.SendTo(s, protocol.Header{Type: protocol.MsgShopList, ID: protocol.IDScene}, body)
	d.log.Info("shop opened", "conn", s.Conn, "npc", target, "merchant", npc.Merchant)
}

// buy handles _MSG_Buy (0x0379): purchase a shop item from an NPC. Price =
// itemPrices[index] (no city tax — village shortcut). The original accepts
// Price==0 (free item) and rejects only negative prices / insufficient gold.
// Debits gold, adds the item to the player's Carry, echoes MSG_Buy (new Coin) +
// MSG_UpdateEtc.
func (d *Dispatcher) buy(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if s.Mode != world.UserPlay || len(payload) < 6 {
		return
	}
	target := int(binary.LittleEndian.Uint16(payload[0:2]))
	npcPos := int(int16(binary.LittleEndian.Uint16(payload[2:4])))
	myPos := int(int16(binary.LittleEndian.Uint16(payload[4:6])))
	npc := w.Entity(target)
	e := w.Entity(s.Conn)
	if npc == nil || npc.Merchant == 0 || e == nil {
		return
	}
	if npcPos < 0 || npcPos >= world.MaxCarry || !carrySlotAccessible(e, myPos) {
		return
	}
	item := npc.Carry[npcPos]
	if item.Index == 0 {
		return // empty shop slot
	}
	// Destination slot occupied: re-sync the client with what is REALLY in that slot,
	// exactly as the original (_MSG_Buy.cpp:158-162). Without this the client keeps a
	// stale "empty" view (e.g. a class/body item it doesn't render in the bag) and
	// retries the same occupied slot forever — every buy silently fails (B11).
	if e.Carry[myPos].Index != 0 {
		d.log.Info("buy resync (dest occupied)", "conn", s.Conn, "npcPos", npcPos, "wantItem", item.Index, "myPos", myPos, "destItem", e.Carry[myPos].Index)
		w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, myPos, itemToSel(e.Carry[myPos])))
		return
	}
	price, ok := d.itemPrices[int(item.Index)]
	if !ok || price < 0 || price > e.Coin {
		d.log.Info("buy denied", "conn", s.Conn, "item", item.Index, "price", price, "gold", e.Coin)
		return
	}
	e.Coin -= price
	e.Carry[myPos] = item
	d.log.Info("buy ok", "conn", s.Conn, "npcPos", npcPos, "item", item.Index, "myPos", myPos, "price", price, "gold", e.Coin)
	// Reply in the EXACT order of the original (_MSG_Buy.cpp:271-296): the MSG_Buy
	// echo (ID=ESCENE_FIELD, new Coin) first, then SendEtc, and the SendItem LAST —
	// the client commits the bought item to the bag on the SendItem, so it must arrive
	// after the buy/gold acknowledgement or the item appears one purchase behind (B?).
	echo := make([]byte, len(payload))
	copy(echo, payload)
	if len(echo) >= 12 {
		binary.LittleEndian.PutUint32(echo[8:12], uint32(e.Coin)) // Coin @body8
	}
	w.SendTo(s, protocol.Header{Type: protocol.MsgBuy, ID: protocol.IDScene}, echo)
	d.sendEtc(w, s, e)
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, myPos, itemToSel(item)))
}

// itemToSel converts a world inventory item to the wire STRUCT_ITEM form. A
// timed item (ExpiresAt set) reports its expiry instead of its stored effects —
// see expiryEffects. Every item that reaches a client goes through here, so this
// is the one place the conversion has to be right.
func itemToSel(it world.Item) protocol.SelItem {
	eff := it.Effects
	if it.ExpiresAt != 0 {
		eff = expiryEffects(it.Index, it.ExpiresAt, time.Now())
	}
	return protocol.SelItem{
		Index: uint16(it.Index),
		Eff: [3][2]uint8{
			{eff[0].Effect, eff[0].Value},
			{eff[1].Effect, eff[1].Value},
			{eff[2].Effect, eff[2].Value},
		},
	}
}

// Fairy item range (Basedef.cpp BASE_CheckFairyDate bails outside it).
const (
	fairyFirstIndex = 3900
	fairyLastIndex  = 3913
)

// expiryEffects writes an item's expiry into the three effect slots, DERIVED
// from ExpiresAt on every send rather than stored and ticked down. ExpiresAt
// survives restarts and is the value dropExpired enforces, so deriving keeps what
// the player sees and the moment the item actually dies from ever disagreeing.
//
// The legacy has TWO schemes here and they are not interchangeable:
//
//   - Fairies (3900-3913) hold a COUNTDOWN — BASE_CheckFairyDate reads
//     stEffect[0..2] as days/hours/minutes left and decrements them every minute
//     (Basedef.cpp:7437). The catalog seeds it, e.g.
//     "3900,Fada_Verde(3dias),…,EF_WDAY,3,EF_HOUR,0,EF_MIN,0".
//   - Everything else timed — the Bolsa do Andarilho (BASE_SetItemDate on
//     Carry[60]/[61], _MSG_UseItem.cpp:5834), the premium equips 3980-3989 and the
//     mount costumes 4150-4188 — holds an ABSOLUTE DATE: BASE_SetItemDate writes
//     EF_WDAY = day of month, EF_WMONTH = month (1-12), EF_YEAR = year-100, and
//     BASE_CheckItemDate compares those against today (Basedef.cpp:7408-7434).
//
// Sending a countdown where the client expects a date shows the wrong expiry, so
// the range decides. The three values replace whatever the item held; every item
// that carries an expiry today is created with no effects of its own. Anything
// already expired reports zeroes and is dropped on the next load.
func expiryEffects(index int16, expiresAt int64, now time.Time) [3]world.Effect {
	if index >= fairyFirstIndex && index <= fairyLastIndex {
		left := time.Unix(expiresAt, 0).Sub(now)
		if left < 0 {
			left = 0
		}
		days := int64(left.Hours()) / 24
		if days > 255 {
			days = 255
		}
		return [3]world.Effect{
			{Effect: efWDay, Value: uint8(days)},
			{Effect: efHour, Value: uint8(int64(left.Hours()) % 24)},
			{Effect: efMin, Value: uint8(int64(left.Minutes()) % 60)},
		}
	}
	// Absolute date. Year is stored as year-100 the way the legacy tm_year does
	// (BASE_SetItemDate: `year - 100`), i.e. years since 2000.
	exp := time.Unix(expiresAt, 0).In(now.Location())
	year := exp.Year() - 2000
	if year < 0 {
		year = 0
	}
	if year > 255 {
		year = 255
	}
	return [3]world.Effect{
		{Effect: efWDay, Value: uint8(exp.Day())},
		{Effect: efWMonth, Value: uint8(int(exp.Month()))},
		{Effect: efYear, Value: uint8(year)},
	}
}

// sell handles _MSG_Sell (0x037A): sell a Carry item to an NPC. Sell price =
// Price/4 (→/2 if >10000, →*2/3 if 5000<x<=10000); no city tax. Credits gold,
// clears the slot, echoes MSG_Sell + MSG_UpdateEtc. Only MyType=1 (Carry) for now.
func (d *Dispatcher) sell(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if s.Mode != world.UserPlay || len(payload) < 6 {
		return
	}
	target := int(binary.LittleEndian.Uint16(payload[0:2]))
	myType := int(int16(binary.LittleEndian.Uint16(payload[2:4])))
	myPos := int(int16(binary.LittleEndian.Uint16(payload[4:6])))
	npc := w.Entity(target)
	e := w.Entity(s.Conn)
	if npc == nil || npc.Merchant == 0 || e == nil || myType != 1 {
		return // only inventory (Carry) sell supported this pass
	}
	if !carrySlotAccessible(e, myPos) || e.Carry[myPos].Index == 0 {
		return
	}
	price := d.itemPrices[int(e.Carry[myPos].Index)]
	sp := price / 4
	if sp > 10000 {
		sp /= 2
	} else if sp > 5000 {
		sp = 2 * sp / 3
	}
	e.Coin += sp
	e.Carry[myPos] = world.Item{}
	d.log.Info("sell ok", "conn", s.Conn, "slot", myPos, "gain", sp, "gold", e.Coin)
	w.SendTo(s, protocol.Header{Type: protocol.MsgSell, ID: protocol.IDScene}, payload)
	// Clear the sold slot on the client (sIndex 0) + refresh gold.
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, myPos, protocol.SelItem{}))
	d.sendEtc(w, s, e)
}

// expiryFromEffects is the inverse of expiryEffects: it reads a date or a
// countdown out of three effect slots and returns the Unix instant it means.
// Only ExpiresAt actually expires an item here (dropExpired), so an authoring
// path — /gm item today, an admin item editor later — has to convert rather than
// store, or the item would display a validity it never honours.
//
// It reports false when the slots carry no expiry, which is the common case: a
// refine or a damage bonus is a plain effect and must be left alone.
func expiryFromEffects(index int16, eff [3]world.Effect, now time.Time) (int64, bool) {
	var day, month, year, hour, minute int
	var haveDay, haveMonth, haveYear, haveClock bool
	for _, e := range eff {
		switch e.Effect {
		case efWDay:
			day, haveDay = int(e.Value), true
		case efWMonth:
			month, haveMonth = int(e.Value), true
		case efYear:
			year, haveYear = int(e.Value), true
		case efHour:
			hour, haveClock = int(e.Value), true
		case efMin:
			minute, haveClock = int(e.Value), true
		}
	}

	// Fairies count down: EF_WDAY/EF_HOUR/EF_MIN are time REMAINING.
	if index >= fairyFirstIndex && index <= fairyLastIndex {
		if !haveDay && !haveClock {
			return 0, false
		}
		left := time.Duration(day)*24*time.Hour + time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute
		if left <= 0 {
			return 0, false
		}
		return now.Add(left).Unix(), true
	}

	// Everything else carries an absolute date: EF_WDAY is the day of the month,
	// EF_WMONTH the month, EF_YEAR the year less 2000 (the legacy's tm_year-100).
	if !haveDay || !haveMonth || !haveYear {
		return 0, false
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, false
	}
	// End of day: the legacy compares whole dates, so the item lives through the
	// day it names rather than dying at its midnight.
	exp := time.Date(2000+year, time.Month(month), day, 23, 59, 59, 0, now.Location())
	return exp.Unix(), true
}
