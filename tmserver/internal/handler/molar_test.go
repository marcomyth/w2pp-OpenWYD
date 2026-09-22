package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const itemMolarGargula = 4122

// molarDB veste as cinco peças do set com o refino pedido e põe o molar na
// primeira vaga da bolsa. sancInicial < 0 deixa as peças sem refino nenhum, que
// é o caso que exige o Bootstrap.
func molarDB(sancInicial int, jaUsou uint8) *fakeDB {
	db := newDB()
	st := world.CharacterState{Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000, Level: 220}
	st.Carry[0] = world.Item{Index: itemMolarGargula}
	for slot := molarPrimeiraPec; slot < molarPrimeiraPec+molarPecasDoSet; slot++ {
		peca := world.Item{Index: int16(1461 + slot)} // peças do set de Osso, quaisquer
		if sancInicial >= 0 {
			refine.Bootstrap(&peca)
			if sancInicial > 0 {
				refine.Set(&peca, sancInicial, 0)
			}
		}
		st.Equip[slot] = peca
	}
	st.MolarGargula = jaUsou
	db.loadResult = st
	return db
}

func molarFrame(t *testing.T, c net.Conn) {
	t.Helper()
	body := protocol.MsgUseItemBody{
		SourType: world.ItemPlaceCarry, SourPos: 0,
		DestType: world.ItemPlaceCarry, DestPos: 0,
	}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

// equipItemFromWire drena até achar o SendItem de uma vaga de EQUIPAMENTO.
func equipItemFromWire(t *testing.T, c net.Conn, slot int) world.Item {
	t.Helper()
	for i := 0; i < 20; i++ {
		p := expect(t, c, protocol.MsgSendItem)
		if le16(p[0:2]) == uint16(world.ItemPlaceEquip) && le16(p[2:4]) == uint16(slot) {
			return decodeItem(p[4:])
		}
	}
	t.Fatalf("nenhum SendItem para a vaga de equipamento %d", slot)
	return world.Item{}
}

// O que o item promete: as cinco peças vestidas vão a +7 de uma vez.
func TestMolarSobeOSetParaMaisSete(t *testing.T) {
	vols := map[int]int{itemMolarGargula: volMolarGargula}
	addr, stop := startServerClockVol(t, molarDB(3, 0), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	molarFrame(t, c)
	for slot := molarPrimeiraPec; slot < molarPrimeiraPec+molarPecasDoSet; slot++ {
		it := equipItemFromWire(t, c, slot)
		if got := refine.Level(it); got != molarSancAlvo {
			t.Errorf("vaga %d ficou +%d, queria +%d", slot, got, molarSancAlvo)
		}
	}
}

// Peça sem refino nenhum não tem onde guardar o nível: o Bootstrap planta o par
// EF_SANC antes do Set, senão o molar seria consumido sem efeito.
func TestMolarRefinaPecaVirgem(t *testing.T) {
	vols := map[int]int{itemMolarGargula: volMolarGargula}
	addr, stop := startServerClockVol(t, molarDB(-1, 0), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	molarFrame(t, c)
	it := equipItemFromWire(t, c, molarPrimeiraPec)
	if got := refine.Level(it); got != molarSancAlvo {
		t.Errorf("peça virgem ficou +%d, queria +%d", got, molarSancAlvo)
	}
}

// Uma vez por personagem: com a marca gravada o molar é recusado e devolvido.
func TestMolarSoValeUmaVezPorPersonagem(t *testing.T) {
	vols := map[int]int{itemMolarGargula: volMolarGargula}
	addr, stop := startServerClockVol(t, molarDB(3, molarJaUsado), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	molarFrame(t, c)
	if code := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); code != NoticeAlreadyDone {
		t.Fatalf("aviso = %d, queria NoticeAlreadyDone", code)
	}
	it := decodeItem(slotItem(t, c, 0))
	if it.Index != itemMolarGargula {
		t.Errorf("molar na bolsa = %d, queria %d (recusa não consome)", it.Index, itemMolarGargula)
	}
}

// Set já acima do alvo: o molar não desce refino nenhum e NÃO é consumido —
// gastar o item sem fazer nada seria o pior resultado para quem clicou.
func TestMolarNaoConsomeQuandoNaoHaOQueSubir(t *testing.T) {
	vols := map[int]int{itemMolarGargula: volMolarGargula}
	addr, stop := startServerClockVol(t, molarDB(9, 0), vols)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	molarFrame(t, c)
	if code := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); code != NoticeCantUseHere {
		t.Fatalf("aviso = %d, queria NoticeCantUseHere", code)
	}
	it := decodeItem(slotItem(t, c, 0))
	if it.Index != itemMolarGargula {
		t.Errorf("molar na bolsa = %d, queria %d", it.Index, itemMolarGargula)
	}
}

// O teto do teleporte subiu para 351: um nível que a faixa antiga (199..253)
// recusava agora entra.
func TestMolarGargulaTeleportaAteTrezentosECinquentaEUm(t *testing.T) {
	tmpl := questNPCTemplate("Gargula_Molar", 100, 15, 0)
	db := newDB()
	db.loadResult = baseMortalState(351)
	addr, stop, npcID := startServerQuestNPC(t, db, tmpl)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	questFrame(t, c, npcID)
	body := expectAction(t, c)
	if !inRange(body.TargetX, 817) || !inRange(body.TargetY, 4062) {
		t.Fatalf("destino = %d,%d, queria perto de 817,4062", body.TargetX, body.TargetY)
	}
}

// E acima do teto continua recusando.
func TestMolarGargulaAcimaDoTetoNaoTeleporta(t *testing.T) {
	tmpl := questNPCTemplate("Gargula_Molar", 100, 15, 0)
	db := newDB()
	db.loadResult = baseMortalState(352)
	addr, stop, npcID := startServerQuestNPC(t, db, tmpl)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	questFrame(t, c, npcID)
	if code := noticeCode(t, expect(t, c, protocol.MsgMessageBoxOk)); code != NoticeReqNotMet {
		t.Fatalf("aviso = %d, queria NoticeReqNotMet", code)
	}
}
