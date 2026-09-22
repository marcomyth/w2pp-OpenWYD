package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Personal shop / autotrade (issue #115, TMSrv/_MSG_SendAutoTrade.cpp,
// _MSG_ReqBuy.cpp, _MSG_ReqTradeList.cpp, SendFunc.cpp:SendAutoTrade). A player
// opens an AFK stall that sells items out of the account Cargo (warehouse) by
// CargoPos; the offered item stays referenced in the Cargo and only moves on a
// completed buy, so closing the shop never returns anything. All state lives on the
// Session (session-only, never persisted) and every handler runs inside the loop
// goroutine, so the buy transaction is atomic without locks. Closing the shop is
// closeAutoTrade, NOT _MSG_Deprivate (which is a guild op) and — diverging from the
// legacy — no longer RemoveTrade: see the note on removeTrade in trade.go.

// autoTradePriceMax is the per-item price ceiling (_MSG_SendAutoTrade.cpp:76).
const autoTradePriceMax = 1_999_999_999

// autoTradeViewRange is VIEWGRIDX/VIEWGRIDY (33): a browser/buyer must be within
// this box of the shop owner (_MSG_ReqBuy.cpp:62, _MSG_ReqTradeList.cpp:40).
const autoTradeViewRange = world.NoViewRange

// autoTradeTaxThreshold is the price below which no city tax is charged
// (_MSG_ReqBuy.cpp:138): imposto = (Price/100)*Tax only when Price >= 100000.
const autoTradeTaxThreshold = 100000

// autoTradeBlacklist is the set of sIndex the original refuses to put on sale
// (_MSG_SendAutoTrade.cpp:85): quest/bound/event items that must never be traded.
var autoTradeBlacklist = map[int16]bool{
	508: true, 3993: true, 747: true, 509: true, 522: true,
	526: true, 527: true, 528: true, 529: true, 530: true, 531: true, 446: true,
}

// inAutoTradeForbiddenRect reports whether (x,y) is inside the hardcoded no-shop
// rectangle inside a village (_MSG_SendAutoTrade.cpp:57).
func inAutoTradeForbiddenRect(x, y int16) bool {
	return x >= 2123 && x <= 2148 && y >= 2139 && y <= 2157
}

// autoTradeInRange reports whether b is within the autotrade view box of a.
func autoTradeInRange(a, b *world.Entity) bool {
	dx := int(a.X) - int(b.X)
	dy := int(a.Y) - int(b.Y)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= autoTradeViewRange && dy <= autoTradeViewRange
}

// sendAutoTrade handles _MSG_SendAutoTrade (0x0397): open a personal shop. Items
// are validated against the seller's account Cargo (memcmp anti item-swap) and the
// sIndex blacklist; the shop is village-only. On success it stores the shop, sets
// TradeMode, sends the owner its own list, and multicasts the stall pose.
// APOSENTADA (18/09/2026). A janela de barraca do cliente saiu de cena: ela só
// sabe de ouro, e quem monta a barraca agora é o painel da Loja do Servidor
// (lojaabrir.go), onde o vendedor escolhe também a moeda de cada item. O
// GamePatch já toma o clique do botão na barra, então este pacote não deveria
// mais chegar; se chegar — cliente antigo, ou alguém falando direto com o
// servidor —, a resposta é recusar, e não abrir uma barraca pela metade, sem
// moeda escolhida.
func (d *Dispatcher) sendAutoTrade(w *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	d.log.Info("autotrade: janela antiga recusada, a barraca se monta pelo painel", "conn", s.Conn)
	d.notify(w, s, NoticeCantAutoTrade)
}

// shopStocked reports whether a shop has anything left to sell. It reads the same
// two fields a buy clears, so a stall whose last item sells goes unstocked on the
// spot and stops earning.
func shopStocked(shop *world.AutoTradeState) bool {
	for i := range shop.Slots {
		if shop.Slots[i].CargoPos >= 0 && !shop.Slots[i].Item.Empty() {
			return true
		}
	}
	return false
}

// shopAt resolves the entity id a client sent into the shop behind it: the
// seller's session and the body the buyer has to stand next to. It accepts both
// shapes — the clone mob and the legacy pose — and it is the reason the callers
// no longer test the id against MaxUser: with a stall of its own, a shop id is
// legitimately a mob id.
func shopAt(w *world.World, id int) (*world.Session, *world.Entity) {
	if id <= 0 || id >= world.MaxMob {
		return nil, nil
	}
	e := w.Entity(id)
	if e == nil {
		return nil, nil
	}
	s := shopSessionOf(w, e)
	if s == nil {
		return nil, nil
	}
	return s, e
}

// reqTradeList handles _MSG_ReqTradeList (0x039A): browse another player's shop.
func (d *Dispatcher) reqTradeList(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP == 0 || s.Mode != world.UserPlay {
		w.AddCrackError(s, 10, 87)
		return
	}
	autoID, ok := protocol.StandardParm(payload)
	if !ok {
		return
	}
	// No MaxUser bound here any more: with the stall raised as its own body, the
	// id the client clicked is a mob id. shopAt is what validates it, and it
	// answers nil for anything that is not actually somebody's open shop.
	seller, te := shopAt(w, int(autoID))
	if seller == nil {
		return
	}
	if !autoTradeInRange(e, te) {
		d.log.Info("autotrade list too far", "conn", s.Conn, "stall", autoID, "seller", seller.Conn)
		return
	}
	// A janela antiga do cliente nao entra mais em cena: ela só sabe de ouro, e
	// o que existe hoje é uma vitrine só, com as três moedas. Clicar numa
	// barraca leva para lá — é o mesmo gesto, com outro destino.
	w.SendTo(s, protocol.Header{Type: protocol.MsgLojaMercado, ID: protocol.IDScene}, nil)
}

// reqBuy handles _MSG_ReqBuy (0x0398): buy one item from a shop. The whole
// transaction runs in a single loop pass (validate-all-then-apply): the item moves
// from the seller's Cargo to the buyer's Carry, gold moves from the buyer's coin to
// the seller's Cargo coin (minus city tax), and the shop/Cargo slots clear. Because
// the loop serializes events, a second buyer of the same slot finds it empty.
func (d *Dispatcher) reqBuy(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP == 0 || s.Mode != world.UserPlay {
		w.AddCrackError(s, 10, 86)
		d.removeTrade(w, s)
		return
	}
	if s.TradeMode != 0 || s.Trade.Active {
		return // a seller/trader can't buy
	}
	var m protocol.MsgReqBuyBody
	if err := m.Decode(payload); err != nil {
		return
	}
	targetID := int(m.TargetID)
	seller, te := shopAt(w, targetID)
	if seller == nil {
		return
	}
	// Against the STALL, not the seller. That is the whole point of the clone:
	// the seller is somewhere else, and measuring the buyer's distance to him
	// would fail every purchase the moment he walked off — or, worse, let someone
	// buy from a stall they are nowhere near because its owner happens to be.
	if !autoTradeInRange(e, te) {
		d.log.Info("autotrade buy too far", "conn", s.Conn, "stall", targetID, "seller", seller.Conn)
		return
	}
	pos := int(m.Pos)
	if pos < 0 || pos >= world.MaxAutoTrade {
		return
	}
	slot := &seller.AutoTrade.Slots[pos]
	cpos := slot.CargoPos
	if cpos < 0 || cpos >= world.MaxCargo || slot.Item.Empty() {
		return // already sold / empty slot
	}
	// Esta porta só vende em OURO, e a recusa aqui é o que impede que ela venda o
	// resto de graça.
	//
	// Este pacote é o da janela antiga do cliente, que nasceu quando ouro era a
	// única moeda: ele traz preço e item e o servidor confere os dois. A vitrine
	// nova pôs moeda POR PRATELEIRA (AutoTrade.Moeda, lojaservidor.go), e este
	// caminho nunca aprendeu a olhar para ela — pagava sempre com e.Coin, pelo
	// mesmo número. Uma prateleira anunciada em Cash saía pelo valor dela em ouro.
	//
	// As duas conferências anti-adulteração logo abaixo não pegam isso, e é por
	// isso que a recusa precisa ser explícita: o preço e o item CONFEREM: o que
	// não confere é a moeda, e ela não entra em nenhuma das duas comparações.
	//
	// Recusa por moeda, e não remoção da rota: a pose legada ainda depende deste
	// pacote (quitTrade e closeAutoTrade a tratam), e ali a barraca é o próprio
	// corpo do vendedor. Tirar a rota derrubaria essa barraca junto.
	if seller.AutoTrade.Moeda[pos] != protocol.LojaMoedaOuro {
		d.log.Info("autotrade buy recusada: a janela antiga so paga em ouro",
			"conn", s.Conn, "stall", targetID, "slot", pos,
			"moeda", seller.AutoTrade.Moeda[pos])
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}
	// Anti-tamper: the offer (tax + price + item) must match exactly, AND the stored
	// offer must still match the live Cargo item (two memcmp, _MSG_ReqBuy.cpp:72-88).
	sellerCargo := w.Cargo(seller.AccountID)
	if sellerCargo == nil {
		return
	}
	cargoItem := sellerCargo.Items[cpos]
	if m.Tax != int32(seller.AutoTrade.Tax) || m.Price != slot.Price ||
		!sameItem(m.Item, slot.Item) || !itemsEqual(slot.Item, cargoItem) {
		d.removeTrade(w, s)
		return
	}
	if e.Coin < m.Price {
		d.notify(w, s, NoticeNotEnoughMoney)
		return
	}
	if int64(sellerCargo.Coin)+int64(m.Price) > maxCoin {
		d.notify(w, s, NoticeCantGetMore2G)
		return
	}
	dst := firstFreeTradeSlot(e)
	if dst < 0 {
		d.notify(w, s, NoticeNoSpaceToTrade)
		return
	}

	// Apply atomically. Tax only bites at/above the threshold (_MSG_ReqBuy.cpp:138).
	e.Carry[dst] = cargoItem
	e.Coin -= m.Price
	var imposto int32
	if m.Price >= autoTradeTaxThreshold {
		imposto = (m.Price / 100) * m.Tax
	}
	sellerCargo.Coin += m.Price - imposto
	sellerCargo.Items[cpos] = world.Item{}
	*slot = world.AutoTradeSlot{CargoPos: -1} // clear the CORRECT slot (not the legacy memset bug)

	// S→C: buyer gets the item + updated coin; seller gets the cleared cargo slot +
	// cargo coin; the shop's viewers get _MSG_ItemSold to drop it from the listing.
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, dst, itemToSel(cargoItem)))
	d.sendEtc(w, s, e)
	w.Send(seller, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCargo, cpos, protocol.SelItem{}))
	w.Send(seller, protocol.MsgUpdateCargoCoin, protocol.EncodeUpdateCargoCoin(sellerCargo.Coin))
	w.BroadcastInView(targetID, protocol.MsgItemSold, protocol.EncodeStandardParm2(int32(targetID), int32(pos)))
	d.notify(w, seller, NoticeItemSold)
	d.log.Info("autotrade buy", "buyer", s.Conn, "seller", targetID, "item", cargoItem.Index, "price", m.Price, "tax", imposto)

	// Both sides persisted NOW, for the reason spelled out in trade.go: the two
	// halves of this sale live on DIFFERENT connections, and each one only
	// reached Postgres when its own connection ended. Buyer logs out and is saved
	// holding the item, the process dies before the seller logs out, and the
	// seller's stale cargo row still has it. One item, two owners, nobody
	// cheating.
	//
	// The buyer's half is a character save (the item landed in Carry); the
	// seller's is a cargo save, because a personal shop sells straight out of the
	// account warehouse. SaveCargoThen with an empty continuation: the seller is
	// still playing, so the cargo is saved and kept loaded rather than released.
	w.SaveCharacterAsync(s)
	w.SaveCargoThen(seller, func(*world.World, *world.Session) {})
}

// shopPinsOwner reports whether an open personal shop still pins its owner in
// place — the legacy behaviour, and now only the fallback shape.
//
// It is the gate behind every "you can't do that while your shop is open" refusal
// (attacking, dropping, picking up, dragging an item). Those all exist for one
// reason: the seller's own body WAS the stall, so letting him act meant letting a
// shop walk, swing and loot. Once the stall is a clone that reason is gone, and
// the owner goes back to being an ordinary player who happens to own a shop.
//
// The shop's own safety does not rest on these refusals and never did: what makes
// the sale safe is the memcmp in reqBuy against the live Cargo slot. An owner who
// shuffles his warehouse under an open shop does not duplicate anything — he makes
// the next purchase of that slot fail.
func shopPinsOwner(s *world.Session) bool {
	if s.TradeMode == 0 {
		return false
	}
	return s.AutoTrade == nil || s.AutoTrade.CloneID < world.MaxUser
}

// shopStallID is the entity id that IS this session's shop: the clone when one
// was raised, and the seller's own conn when it was not (legacy pose fallback).
func shopStallID(s *world.Session) int {
	if s.AutoTrade != nil && s.AutoTrade.CloneID >= world.MaxUser {
		return s.AutoTrade.CloneID
	}
	return s.Conn
}

// raiseShopStall puts the stall into the world and shows it to everyone in view.
//
// It prefers a clone — its own body, which is what frees the seller to walk — and
// falls back to the legacy pose (_MSG_SendAutoTrade.cpp:112-120), where the
// seller's own body becomes the stall, when no clone could be raised. The pose is
// selected by the MSG_CreateMobTrade Type, not by a CreateType value; Score.Con
// is zeroed for parity either way. The clone's size is not set by that packet —
// see shopCloneCon and sendStallScale.
func (d *Dispatcher) raiseShopStall(w *world.World, s *world.Session, e *world.Entity) {
	tab := make([]byte, 26)

	if id := w.SpawnShopClone(s.Conn, e.Name); id != 0 {
		s.AutoTrade.CloneID = id
		ce := w.Entity(id)
		data := createMobFrom(ce, 0)
		data.Con = 0
		body := protocol.EncodeCreateMobTradeBody(data, tab, s.AutoTrade.Title)
		// One broadcast reaches everyone INCLUDING the owner: BroadcastInView
		// skips the session whose conn equals the source id, and the source here
		// is the clone's mob id, which no session's conn can be. Sending the owner
		// a separate copy would deliver the stall to him twice.
		w.BroadcastInView(id, protocol.MsgCreateMobTrade, body)
		w.BroadcastInView(id, protocol.MsgUpdateScore, stallScaleBody(ce))
		// The owner's own body stays a normal avatar. Nothing to re-send: he was
		// never put into the pose.
		return
	}

	// Fallback: no clone template, no free cell, or no free mob slot. The shop
	// still opens the legacy way — the seller IS the stall, so shopPinsOwner keeps
	// his old restrictions and walking closes the shop (movement.go).
	d.log.Info("autotrade sem clone, usando a pose do legado", "conn", s.Conn)
	data := createMobFrom(e, 0)
	data.Con = 0 // _MSG_SendAutoTrade.cpp:118
	body := protocol.EncodeCreateMobTradeBody(data, tab, s.AutoTrade.Title)
	w.SendTo(s, protocol.Header{Type: protocol.MsgCreateMobTrade, ID: protocol.IDScene}, body)
	w.BroadcastInView(s.Conn, protocol.MsgCreateMobTrade, body)
}

// shopCloneCon is the Score.Con that sizes a clone stall on the client:
// WYD.exe 7662 (0x50D43F) scales a mob to (Con/2000 + 1) × 0.9, reading Con
// signed. 0 gives 0.9: Marco asked for 0.45 first and then twice that, once he
// saw it in game (17/09).
//
// It cannot ride in the MSG_CreateMobTrade itself. For a titled stall the client
// rewrites that packet before using it (0x483BF2): face forced to 230, the
// Carbúnculo, equipment cleared, and Con forced to 15000. For a player the same
// scale function clamps Con to 500 and the stall comes out at ~1.0 — that is the
// legacy's rabbit. For a clone, a mob, nothing clamps it: 7.65×, a giant sitting
// over its own seller and eating the clicks meant for the ground.
//
// The fix is the MSG_UpdateScore right behind it: its handler (0x5118BE) copies
// the score into the entity and recomputes the scale from the new Con (0x511EF4),
// for any entity, not just the local player.
const shopCloneCon int16 = 0

// shopCloneMerchant is the Score.Merchant a clone stall is shown with. The client
// draws a mob's name plate — here, the stall's title — only under the mouse
// unless the low nibble of Merchant is 1..14 (0x4FA230), which is what keeps a
// service NPC's name up. The clone's own Merchant is 0 on the server on purpose
// (see SpawnShopClone), so the plate is set on the wire only. It changes nothing
// else a click does: the click tests the title first (0x4604D2) and the
// can-attack test answers the same for 0 and 1 (0x4601A0).
const shopCloneMerchant uint8 = 1

// stallScaleBody is the MSG_UpdateScore that resizes a clone stall. The rest of
// the score is the clone's own, so the handler's copy leaves nothing else changed.
func stallScaleBody(e *world.Entity) []byte {
	body := protocol.EncodeUpdateScore(protocol.ScoreData{
		Level: e.Level, Ac: e.AC, Damage: e.Damage,
		MaxHp: e.MaxHP, Hp: e.HP, MaxMp: e.MaxMP, Mp: e.MP,
		Str: e.Str, Int: e.Int, Dex: e.Dex, Con: shopCloneCon,
	})
	body[12] = shopCloneMerchant // Score.Merchant; ScoreData has no field for it
	return body
}

// sendStallScale follows a clone stall's MSG_CreateMobTrade to one viewer with
// its resize. Every path that reveals an entity calls it right after the create
// packet; for anything that is not a clone stall it sends nothing.
func sendStallScale(w *world.World, s *world.Session, e *world.Entity) {
	if world.IsPlayer(e.ID) || shopSessionOf(w, e) == nil {
		return
	}
	w.SendTo(s, protocol.Header{Type: protocol.MsgUpdateScore, ID: uint16(e.ID)}, stallScaleBody(e))
}

// closeAutoTrade shuts an open personal shop: it settles the shop-points clock,
// takes the stall down (the clone, or the legacy pose reverted with a normal
// MSG_CreateMob — RemoveTrade, Server.cpp:8138-8145) and closes the shop UI on the
// owner. No-op when no shop is open.
//
// It is called from exactly five places, and the shortness of that list is the
// design: /fecharloja, the owner's own quit-trade (only for the legacy pose — with
// a clone, closing the window no longer closes the shop), the end of the session
// (SessionEnd and both character-select paths), this file's anti-tamper refusals,
// and walking while in the legacy pose. It is NOT called from removeTrade any more — see the note
// there. A shop that came down for any other reason would be a shop the player
// cannot keep.
func (d *Dispatcher) closeAutoTrade(w *world.World, s *world.Session) {
	if s.AutoTrade == nil && s.TradeMode == 0 {
		return
	}
	// Settle the shop clock before the state goes away: the owner is owed every
	// quarter-hour the stall actually completed, and closing is the one moment
	// that number can still be read.
	d.creditShopPoints(w, s)

	clone := 0
	if s.AutoTrade != nil {
		clone = s.AutoTrade.CloneID
	}
	s.AutoTrade = nil
	s.TradeMode = 0
	w.Send(s, protocol.MsgQuitTrade, nil)
	// O aviso vai DEPOIS de a barraca sair do ar. Avisando antes, a pagina que
	// os outros recebem ainda tem a barraca dentro - foi o que os testes
	// pegaram, e seria um item fantasma na vitrine de todo mundo.
	d.mercadoMudou(w)

	if clone != 0 {
		// The stall was its own body: take it down and leave the owner alone. He
		// was never in the pose, so re-sending his avatar would be a pointless
		// CreateMob for an entity nobody's client got wrong.
		w.DespawnShopClone(clone, s.Conn)
		return
	}

	e := w.Entity(s.Conn)
	if e == nil || e.Mode != world.MobUser {
		return
	}
	body := protocol.EncodeCreateMobBody(createMobFrom(e, 0))
	w.SendTo(s, protocol.Header{Type: protocol.MsgCreateMob, ID: protocol.IDScene}, body)
	w.BroadcastInView(s.Conn, protocol.MsgCreateMob, body)
}

// fecharLojinha is /fecharloja: the one way an owner closes a clone stall, now
// that closing the window leaves it standing. It closes the legacy pose too, so
// the command means the same thing whatever shape the shop took.
func (d *Dispatcher) fecharLojinha(w *world.World, s *world.Session) {
	if s.AutoTrade == nil && s.TradeMode == 0 {
		sendClientMessage(w, s, "Você não tem lojinha aberta.")
		return
	}
	d.closeAutoTrade(w, s)
	sendClientMessage(w, s, "Lojinha fechada.")
}

// firstFreeTradeSlot returns the first empty Carry slot in the currently unlocked
// tradeable region, or -1 if it is full.
func firstFreeTradeSlot(e *world.Entity) int {
	for i := 0; i < activeCarryLimit(e); i++ {
		if e.Carry[i].Empty() {
			return i
		}
	}
	return -1
}

// wireFromItem converts a world item to the wire STRUCT_ITEM (WireItem) form.
func wireFromItem(it world.Item) protocol.WireItem {
	wi := protocol.WireItem{Index: it.Index}
	for i := 0; i < 3; i++ {
		wi.Effects[i] = protocol.WireEffect{Effect: it.Effects[i].Effect, Value: it.Effects[i].Value}
	}
	return wi
}

// itemsEqual reports whether two world items are byte-identical (index + effects) —
// the memcmp used to confirm the shop offer still matches the live Cargo slot.
func itemsEqual(a, b world.Item) bool {
	if a.Index != b.Index {
		return false
	}
	for i := 0; i < 3; i++ {
		if a.Effects[i] != b.Effects[i] {
			return false
		}
	}
	return true
}
