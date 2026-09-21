package handler

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func questRewardDB(item int16, lvl int, amount uint8) *fakeDB {
	db := newDB()
	st := world.CharacterState{
		Slot: 0, Name: "Hero", ClassMaster: classMasterMortal, Level: lvl,
		X: 5, Y: 5, HP: 1000, MaxHP: 1000,
	}
	st.Carry[0] = world.Item{Index: item}
	if amount > 1 {
		st.Carry[0].Effects[0] = world.Effect{Effect: efAmount, Value: amount}
	}
	db.loadResult = st
	return db
}

func useQuestItem(t *testing.T, c net.Conn) {
	t.Helper()
	body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: 0}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

func collectQuestResult(t *testing.T, c net.Conn, limit int) (exp int64, coin int32, item []byte, sawPanel bool) {
	t.Helper()
	for range limit {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgExpPanel:
			sawPanel = true
		case protocol.MsgUpdateEtc:
			exp = int64(binary.LittleEndian.Uint64(payload[4:12]))
			coin = int32(binary.LittleEndian.Uint32(payload[28:32]))
		case protocol.MsgSendItem:
			item = payload
		}
	}
	return
}

// Um clique gasta a PILHA INTEIRA e paga por cada unidade: três troféus valem três
// vezes a XP e o gold do degrau, e o espaço fica vazio. Era uma unidade por clique
// até 21/09/2026 (useQuestReward).
func TestQuestItemRewardAllTiersAndStackConsumption(t *testing.T) {
	const pilha = 3
	tiers := []struct {
		item int16
		lvl  int
		exp  int64
		coin int32
	}{{4117, 39, 1000, 2000}, {4118, 115, 2000, 4000}, {4119, 190, 3000, 6000}, {4120, 265, 4000, 8000}, {4121, 320, 5000, 10000}}
	for _, tc := range tiers {
		t.Run(fmt.Sprintf("item-%d", tc.item), func(t *testing.T) {
			addr, stop := startServerClockVol(t, questRewardDB(tc.item, tc.lvl, pilha), map[int]int{int(tc.item): volQuestReward})
			defer stop()
			c := enterWorld(t, addr)
			defer c.Close()
			useQuestItem(t, c)
			exp, coin, item, panel := collectQuestResult(t, c, 20)
			wantExp, wantCoin := tc.exp*pilha, tc.coin*pilha
			if exp != wantExp || coin != wantCoin || !panel {
				t.Errorf("reward = exp %d coin %d panel %v, want %d %d true", exp, coin, panel, wantExp, wantCoin)
			}
			if len(item) < 6 || le16(item[4:6]) != 0 {
				t.Errorf("a pilha não foi gasta inteira: %v", item)
			}
		})
	}
}

// O clique único não pode pagar XP fora da faixa do troféu nem comer os troféus que
// a faixa já não paga: quem cruza o topo no meio da pilha para ali, com o RESTO NA
// MÃO e a linha de nível na tela.
//
// O nível 114 com a Caixa da Sabedoria é o caso de verdade: a faixa do degrau 0
// acaba em 115 (meia-aberta), e uma pilha grande atravessa esse topo.
func TestQuestItemRewardParaNoTopoDaFaixaComORestoNaMao(t *testing.T) {
	const pilha = 120
	db := questRewardDB(4117, 114, pilha)
	// Um passo do 115: o primeiro troféu já sobe o nível e fecha a faixa.
	db.loadResult.Exp = level.NextLevelExp(114) - 1
	addr, stop := startServerClockVol(t, db, map[int]int{4117: volQuestReward})
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()
	useQuestItem(t, c)

	var item []byte
	nivelBarrado := false
	for range 40 {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgSendItem:
			item = payload
		case protocol.MsgMessageBoxOk:
			if noticeCode(t, payload) == NoticeLevelLimit {
				nivelBarrado = true
			}
		}
	}
	if !nivelBarrado {
		t.Error("a pilha parou no topo da faixa e o jogador não foi avisado")
	}
	if len(item) < 8 || le16(item[4:6]) != 4117 {
		t.Fatalf("a pilha sumiu em vez de sobrar: %v", item)
	}
	// Um usado, 119 na mão: o que passou do topo não foi gasto.
	if item[6] != efAmount || item[7] != pilha-1 {
		t.Errorf("sobrou %v, quero %d troféus na mão", item, pilha-1)
	}
}

// Nos dois tetos ao mesmo tempo o troféu não entrega nada, e nada é gasto: é o
// "perder exp no processo" que o clique único não pode causar.
func TestQuestItemRewardNaoGastaNadaNosDoisTetos(t *testing.T) {
	const pilha = 120
	db := questRewardDB(4117, 39, pilha)
	db.loadResult.Exp = level.MaxExp
	db.loadResult.Coin = maxCoin
	addr, stop := startServerClockVol(t, db, map[int]int{4117: volQuestReward})
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()
	useQuestItem(t, c)
	_, _, item, panel := collectQuestResult(t, c, 20)
	if panel {
		t.Error("painel de XP com os dois tetos cheios")
	}
	if len(item) < 8 || le16(item[4:6]) != 4117 || item[6] != efAmount || item[7] != pilha {
		t.Errorf("a pilha foi mexida: %v, quero %d troféus intactos", item, pilha)
	}
}

func TestQuestItemRewardLevelBoundariesRejectWithoutConsumption(t *testing.T) {
	for _, lvl := range []int{38, 115} {
		addr, stop := startServerClockVol(t, questRewardDB(4117, lvl, 3), map[int]int{4117: volQuestReward})
		c := enterWorld(t, addr)
		useQuestItem(t, c)
		if notice := expect(t, c, protocol.MsgMessageBoxOk); noticeCode(t, notice) != NoticeLevelLimit {
			t.Errorf("level %d notice = %d, want NoticeLevelLimit", lvl, noticeCode(t, notice))
		}
		// The code alone renders as nothing: what the player outside the band must
		// actually see is _NN_Level_limit, or the click looks dead.
		if got := decodePanel(expect(t, c, protocol.MsgMessagePanel)); got != "Nível Insuficiente. Isto não pode ser utilizado." {
			t.Errorf("level %d panel = %q", lvl, got)
		}
		item := expect(t, c, protocol.MsgSendItem)
		if le16(item[4:6]) != 4117 || item[7] != 3 {
			t.Errorf("level %d changed rejected stack: %v", lvl, item)
		}
		c.Close()
		stop()
	}
}

func TestQuestItemRewardCaps(t *testing.T) {
	db := questRewardDB(4117, 39, 1)
	db.loadResult.Exp = level.MaxExp - 10
	db.loadResult.Coin = maxCoin - 10
	addr, stop := startServerClockVol(t, db, map[int]int{4117: volQuestReward})
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()
	useQuestItem(t, c)
	exp, coin, item, _ := collectQuestResult(t, c, 10)
	if exp != level.MaxExp || coin != maxCoin {
		t.Errorf("capped reward = exp %d coin %d, want %d %d", exp, coin, level.MaxExp, maxCoin)
	}
	if len(item) < 6 || le16(item[4:6]) != 0 {
		t.Errorf("final unit not consumed: %v", item)
	}
}

func TestQuestItemRewardPartyMemberGetsTenPercent(t *testing.T) {
	db := partyDB()
	leader := db.loads[7]
	leader.Carry[0] = world.Item{Index: 4117}
	db.loads[7] = leader
	addr, stop := startServerClockVol(t, db, map[int]int{4117: volQuestReward})
	defer stop()
	a := enterWorldAs(t, addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb")
	defer b.Close()

	reqPartyFrame(t, a, 1, 2)
	expectPartyFrame(t, b, protocol.MsgSendReqParty)
	acceptPartyFrame(t, b, 1, "Hero")
	for {
		if _, _, ok := readMaybe(t, a); !ok {
			break
		}
	}
	for {
		if _, _, ok := readMaybe(t, b); !ok {
			break
		}
	}

	useQuestItem(t, a)
	leaderExp, _, _, _ := collectQuestResult(t, a, 10)
	memberExp, memberCoin, _, panel := collectQuestResult(t, b, 8)
	if leaderExp != 1000 {
		t.Errorf("consumer exp = %d, want 1000 (no self share)", leaderExp)
	}
	if memberExp != 100 || memberCoin != 0 || !panel {
		t.Errorf("member reward = exp %d coin %d panel %v, want 100 0 true", memberExp, memberCoin, panel)
	}
}
