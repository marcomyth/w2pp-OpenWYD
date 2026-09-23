package handler

import (
	"encoding/binary"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// autotradeDB seats a seller (tester, conn 1) and a buyer (tradeb, conn 2) in the
// world with HP>0 (login spawns both in Armia, a village, within view range). The
// seller's account Cargo holds one item to put on sale; the buyer carries gold.
//
// Both are level 1: a level-0 character with no gear and no experience is one
// the dbserver just created, and that one is born in the training field
// (pontoDeEntrada), outside the city a shop needs.
func autotradeDB(sellItem int16) *fakeDB {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7:  {Slot: 0, Name: "Seller", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1000},
		11: {Slot: 0, Name: "Buyer", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1_000_000},
	}
	var cargo world.CargoState
	cargo.Items[0] = world.Item{Index: sellItem}
	db.accounts["tester"].cargo = cargo
	return db
}

func autotradeViewDB(sellItem int16) *fakeDB {
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7:  {Slot: 0, Name: "Seller", X: 2086, Y: 2093, HP: 1000, MaxHP: 1000, Coin: 1000},
		11: {Slot: 0, Name: "Buyer", X: 2086, Y: 2130, HP: 1000, MaxHP: 1000, Coin: 1_000_000},
	}
	var cargo world.CargoState
	cargo.Items[0] = world.Item{Index: sellItem}
	db.accounts["tester"].cargo = cargo
	return db
}

func createMobTradeFields(t *testing.T, payload []byte) (x, y int16, mobID int, title string) {
	t.Helper()
	if len(payload) != 240 {
		t.Fatalf("CreateMobTrade body = %d bytes, want 240", len(payload))
	}
	x = int16(binary.LittleEndian.Uint16(payload[0:2]))
	y = int16(binary.LittleEndian.Uint16(payload[2:4]))
	mobID = int(binary.LittleEndian.Uint16(payload[4:6]))
	title = cstr(payload[216:240])
	return x, y, mobID, title
}

// openShopPayload builds a client MSG_SendAutoTrade selling Cargo[cargoPos] for
// price in shop slot 0.
func openShopPayload(title string, item int16, cargoPos int, price int32) []byte {
	body := protocol.MsgSendAutoTradeBody{Title: title}
	for i := range body.Slots {
		body.Slots[i].CarryPos = -1
	}
	body.Slots[0] = protocol.AutoTradeWireItem{
		Item:     protocol.WireItem{Index: item},
		CarryPos: int8(cargoPos),
		Coin:     price,
	}
	return body.Encode()
}

// reqBuyPayload builds a client MSG_ReqBuy for shop slot pos of seller targetID.
func reqBuyPayload(targetID, pos int, item int16, price, tax int32) []byte {
	body := protocol.MsgReqBuyBody{
		Pos:      int32(pos),
		TargetID: uint16(targetID),
		Price:    price,
		Tax:      tax,
		Item:     protocol.WireItem{Index: item},
	}
	return body.Encode()
}

func TestAutoTradeShopPoseWhenBuyerEntersView(t *testing.T) {
	const sellItem = int16(1030)
	const title = "Loja Longe"
	addr, stop := startServerView(t, autotradeViewDB(sellItem), world.DefaultGridDim)
	defer stop()
	seller := enterWorldAs(t, addr, "tester") // conn 1, inside Armia at (2086,2093)
	defer seller.Close()
	buyer := enterWorldAs(t, addr, "tradeb") // conn 2, outside ViewRange at (2086,2130)
	defer buyer.Close()
	drainRaw(t, seller)
	drainRaw(t, buyer)

	abreBarraca(t, seller, title, 0, 1000, protocol.LojaMoedaOuro)
	if ty, _, ok := readMaybeRaw(t, buyer); ok {
		t.Fatalf("out-of-view buyer got %#x after shop opened, want no shop pose yet", ty)
	}

	actionFrameXY(t, buyer, protocol.MsgAction, serverTime, 2086, 2130, 2086, 2100)
	ty, payload, ok := readMaybeRaw(t, buyer)
	if !ok || ty != protocol.MsgCreateMobTrade {
		t.Fatalf("buyer entering view got %#x ok=%v, want CreateMobTrade", ty, ok)
	}
	if x, y, id, gotTitle := createMobTradeFields(t, payload); id != 1 || x != 2086 || y != 2093 || gotTitle != title {
		t.Fatalf("shop pose = id %d at (%d,%d) title %q, want id 1 at (2086,2093) title %q", id, x, y, gotTitle, title)
	}
	if ty, _, ok = readMaybeRaw(t, buyer); !ok || ty != protocol.MsgPKInfo {
		t.Fatalf("frame after shop pose = %#x ok=%v, want PKInfo", ty, ok)
	}
}

func TestAutoTradeOpenBrowseBuy(t *testing.T) {
	const sellItem = int16(1030)
	const price, tax = int32(200_000), int32(5)
	addr, stop, _ := startServerClock(t, autotradeDB(sellItem))
	defer stop()
	seller := enterWorldAs(t, addr, "tester") // conn 1
	defer seller.Close()
	buyer := enterWorldAs(t, addr, "tradeb") // conn 2
	defer buyer.Close()

	// O vendedor monta a barraca pelo painel e clica na própria barraca. Clicar
	// numa barraca não abre mais a janela antiga: a resposta é o convite para a
	// vitrine. Título, item e preço são conferidos onde eles aparecem hoje, que
	// é a vitrine (lojaservidor_test.go).
	stallID := abreBarraca(t, seller, "Minha Loja", 0, price, protocol.LojaMoedaOuro)
	send(t, seller, protocol.MsgReqTradeList, protocol.EncodeStandardParm(stallID))
	readUntil(t, seller, protocol.MsgLojaMercado)

	// O comprador clica na barraca do vendedor (Parm = conn 1 do vendedor).
	send(t, buyer, protocol.MsgReqTradeList, protocol.EncodeStandardParm(1))
	readUntil(t, buyer, protocol.MsgLojaMercado)

	// Buyer buys slot 0.
	send(t, buyer, protocol.MsgReqBuy, reqBuyPayload(1, 0, sellItem, price, tax))

	// Buyer receives the item (MSG_SendItem: invType@0, slot@2, item.Index@4).
	si, _ := readUntil(t, buyer, protocol.MsgSendItem)
	if place := binary.LittleEndian.Uint16(si[0:2]); place != protocol.ItemPlaceCarry {
		t.Errorf("bought item place = %d, want Carry(%d)", place, protocol.ItemPlaceCarry)
	}
	if got := int16(binary.LittleEndian.Uint16(si[4:6])); got != sellItem {
		t.Errorf("bought item index = %d, want %d", got, sellItem)
	}
	// Buyer gold debited (MSG_UpdateEtc Coin @body28).
	etc, _ := readUntil(t, buyer, protocol.MsgUpdateEtc)
	if coin := int32(binary.LittleEndian.Uint32(etc[28:32])); coin != 1_000_000-price {
		t.Errorf("buyer coin = %d, want %d", coin, 1_000_000-price)
	}

	// Seller receives the cleared cargo slot + the tax-adjusted cargo coin. Tax:
	// imposto = (price/100)*tax = 10000; proceeds = 190000.
	scc, _ := readUntil(t, seller, protocol.MsgUpdateCargoCoin)
	if coin := int32(binary.LittleEndian.Uint32(scc[0:4])); coin != price-(price/100)*tax {
		t.Errorf("seller cargo coin = %d, want %d", coin, price-(price/100)*tax)
	}
}

// TestAutoTradeBuyNoDup: once a slot is bought it is cleared, so a second buy of the
// same slot delivers nothing (the loop serializes buys — no item duplication).
func TestAutoTradeBuyNoDup(t *testing.T) {
	const sellItem = int16(1030)
	const price, tax = int32(50_000), int32(5)
	addr, stop, _ := startServerClock(t, autotradeDB(sellItem))
	defer stop()
	seller := enterWorldAs(t, addr, "tester")
	defer seller.Close()
	buyer := enterWorldAs(t, addr, "tradeb")
	defer buyer.Close()

	abreBarraca(t, seller, "Loja", 0, price, protocol.LojaMoedaOuro)

	// First buy succeeds → item delivered.
	send(t, buyer, protocol.MsgReqBuy, reqBuyPayload(1, 0, sellItem, price, tax))
	si, _ := readUntil(t, buyer, protocol.MsgSendItem)
	if got := int16(binary.LittleEndian.Uint16(si[4:6])); got != sellItem {
		t.Fatalf("first buy item = %d, want %d", got, sellItem)
	}
	readUntil(t, buyer, protocol.MsgUpdateEtc)
	// The buyer is in the seller's view, so it also gets the ItemSold broadcast from
	// its own purchase — drain it so the assertion below sees a clean socket.
	readUntil(t, buyer, protocol.MsgItemSold)

	// Second buy of the same (now empty) slot delivers nothing.
	send(t, buyer, protocol.MsgReqBuy, reqBuyPayload(1, 0, sellItem, price, tax))
	if ty, _, ok := readMaybe(t, buyer); ok {
		t.Fatalf("second buy produced a %#x frame, want none (slot already sold)", ty)
	}
}

// TestAutoTradeCloseOnQuit: sending _MSG_QuitTrade closes an open shop via
// RemoveTrade — the owner gets a QuitTrade signal and a normal CreateMob re-emit,
// and afterwards a browse of the (now closed) shop returns nothing.
func TestAutoTradeCloseOnQuit(t *testing.T) {
	const sellItem = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(sellItem))
	defer stop()
	seller := enterWorldAs(t, addr, "tester")
	defer seller.Close()
	buyer := enterWorldAs(t, addr, "tradeb")
	defer buyer.Close()

	abreBarraca(t, seller, "Loja", 0, 1000, protocol.LojaMoedaOuro)
	// The buyer, in view, received the stall-pose CreateMobTrade on open — drain it so
	// the post-close assertion sees a clean socket.
	readUntil(t, buyer, protocol.MsgCreateMobTrade)

	// Closing (QuitTrade → removeTrade → closeAutoTrade) signals the owner.
	send(t, seller, protocol.MsgQuitTrade, nil)
	readUntil(t, seller, protocol.MsgQuitTrade)

	// The shop is gone: a browse yields nothing.
	send(t, buyer, protocol.MsgReqTradeList, protocol.EncodeStandardParm(1))
	if ty, _, ok := readMaybe(t, buyer); ok {
		t.Fatalf("browse after close produced a %#x frame, want none (shop closed)", ty)
	}
}

// TestAutoTradeOpenRejectsBlacklist: a blacklisted sIndex is refused, so no shop
// opens (the owner gets no list back).
func TestAutoTradeOpenRejectsBlacklist(t *testing.T) {
	const blacklisted = int16(508)
	addr, stop, _ := startServerClock(t, autotradeDB(blacklisted))
	defer stop()
	seller := enterWorldAs(t, addr, "tester")
	defer seller.Close()

	// A barraca agora se monta pelo painel; o item proibido continua recusado, e
	// o que prova isso é a barraca não subir.
	mandaAbrirBarraca(t, seller, "Loja", 0, 1000, protocol.LojaMoedaOuro)
	for {
		ty, _, ok := readMaybe(t, seller)
		if !ok {
			break
		}
		if ty == protocol.MsgLojaAbriu {
			t.Fatalf("a barraca subiu com item proibido")
		}
	}
}

// TestReqBuyRecusaMoedaQueNaoEOuro: a janela antiga do cliente (MSG_ReqBuy) paga
// sempre em ouro, e por isso uma prateleira anunciada em Cash ou em RMT saía por
// esse MESMO número em ouro. As duas conferências anti-adulteração do reqBuy não
// pegam isso: elas comparam preço e item, que conferem, e a moeda não entra em
// nenhuma das duas.
//
// As três moedas passam pela mesma prateleira aqui, e a de ouro NO FIM é o que
// dá valor às outras duas: ela prova que a recusa é por moeda e não uma recusa
// geral. Um guard sabotado para recusar sempre passa nos dois primeiros casos e
// quebra no terceiro.
//
// O item ser vendido no fim prova também o que mais importa numa recusa: as duas
// tentativas negadas não consumiram nada — nem o item do Cargo, nem o ouro.
func TestReqBuyRecusaMoedaQueNaoEOuro(t *testing.T) {
	const sellItem = int16(1030)
	const price, tax = int32(200_000), int32(5)
	addr, stop, _ := startServerClock(t, autotradeDB(sellItem))
	defer stop()
	seller := enterWorldAs(t, addr, "tester") // conn 1
	defer seller.Close()
	buyer := enterWorldAs(t, addr, "tradeb") // conn 2
	defer buyer.Close()

	abreBarraca(t, seller, "Loja", 0, price, protocol.LojaMoedaCash)

	tentaComprarERecusa := func(nome string) {
		t.Helper()
		drena(t, buyer)
		send(t, buyer, protocol.MsgReqBuy, reqBuyPayload(1, 0, sellItem, price, tax))
		avisado := false
		for {
			ty, payload, ok := readMaybe(t, buyer)
			if !ok {
				break
			}
			switch {
			case ty == protocol.MsgSendItem:
				t.Fatalf("%s: o comprador levou o item pagando em OURO uma prateleira em %s", nome, nome)
			case ty == protocol.MsgUpdateEtc:
				t.Fatalf("%s: o ouro do comprador mexeu numa compra que devia ser recusada", nome)
			case ty == protocol.MsgMessageBoxOk && len(payload) >= 4 &&
				Notice(binary.LittleEndian.Uint32(payload[0:4])) == NoticeCantAutoTrade:
				avisado = true
			}
		}
		if !avisado {
			t.Fatalf("%s: a compra não saiu, mas o jogador não foi avisado do motivo", nome)
		}
	}

	tentaComprarERecusa("Cash")

	// O MESMO item, agora anunciado em RMT — e remontando a barraca, porque a
	// moeda de dinheiro real só se escolhe na montagem (ver msgMoedaRMTSoNaMontagem).
	send(t, seller, protocol.MsgQuitTrade, nil)
	abreBarraca(t, seller, "Loja", 0, price, protocol.LojaMoedaRMT)
	tentaComprarERecusa("RMT")

	// E em ouro a porta continua aberta: o item sai, e sai pelo preço certo.
	send(t, seller, protocol.MsgLojaMoeda,
		(&protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaOuro}).Encode())
	drena(t, buyer)
	send(t, buyer, protocol.MsgReqBuy, reqBuyPayload(1, 0, sellItem, price, tax))

	si, _ := readUntil(t, buyer, protocol.MsgSendItem)
	if got := int16(binary.LittleEndian.Uint16(si[4:6])); got != sellItem {
		t.Errorf("item comprado = %d, esperado %d", got, sellItem)
	}
	etc, _ := readUntil(t, buyer, protocol.MsgUpdateEtc)
	if coin := int32(binary.LittleEndian.Uint32(etc[28:32])); coin != 1_000_000-price {
		t.Errorf("ouro do comprador = %d, esperado %d — as recusas não podiam ter cobrado nada",
			coin, 1_000_000-price)
	}
}
