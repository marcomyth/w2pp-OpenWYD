package handler

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os preços do catálogo (Release/Common/ItemList.csv): o NPC paga 60.000 por um
// Resto de Oriharucon e 100.000 por um de Lactolerium.
var precosDaVenda = map[int]int32{419: 480000, 420: 800000, 411: 80000}

func sellFrame(t *testing.T, c net.Conn, npc, carrySlot int) {
	t.Helper()
	body := make([]byte, 6)
	binary.LittleEndian.PutUint16(body[0:2], uint16(npc))
	binary.LittleEndian.PutUint16(body[2:4], 1) // MyType 1 = bolsa
	binary.LittleEndian.PutUint16(body[4:6], uint16(carrySlot))
	send(t, c, protocol.MsgSell, body)
}

// vendedorCom é uma conta com ouro e uma pilha na vaga 0 da bolsa.
func vendedorCom(coin int32, item int16, qtd uint8) *fakeDB {
	db := shopDB(coin)
	st := db.loadResult
	st.Carry[0] = world.Item{Index: item, Effects: [3]world.Effect{{Effect: efAmount, Value: qtd}}}
	db.loadResult = st
	return db
}

func TestVendaDePilhaDeResto(t *testing.T) {
	casos := []struct {
		nome string
		item int16
		qtd  uint8
		quer int32
	}{
		// Literais de propósito: o número que o Marco pediu, não uma conta que
		// concorde com qualquer mudança no código.
		{"120 Restos de Oriharucon pagam 120 vezes", 419, 120, 7_200_000},
		{"120 Restos de Lactolerium pagam 120 vezes", 420, 120, 12_000_000},
		{"um Resto solto paga um", 419, 1, 60_000},
		// Fora da lista a venda continua pagando uma vez: é o que impede comprar
		// a pilha no NPC e revendê-la com lucro.
		{"dez Poeiras pagam uma vez", 411, 10, 10_000},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			addr, stop := startServerShop(t, vendedorCom(0, c.item, c.qtd), precosDaVenda)
			defer stop()
			conn := enterWorld(t, addr)
			defer conn.Close()

			sellFrame(t, conn, shopNPCID, 0)
			expect(t, conn, protocol.MsgSell)
			expect(t, conn, protocol.MsgSendItem) // a vaga vendida some
			etc := expect(t, conn, protocol.MsgUpdateEtc)
			if got := int32(le(etc[28:32])); got != c.quer {
				t.Fatalf("ouro depois da venda = %d, quer %d", got, c.quer)
			}
		})
	}
}

// Uma venda que passaria os 2 bilhões é recusada: o item fica e o ouro não muda.
// O legado encostava no teto e o resto sumia.
func TestVendaQuePassaDoTetoRecusa(t *testing.T) {
	const ouro = 1_995_000_000
	addr, stop := startServerShop(t, vendedorCom(ouro, 420, 120), precosDaVenda)
	defer stop()
	conn := enterWorld(t, addr)
	defer conn.Close()

	sellFrame(t, conn, shopNPCID, 0)
	avisou, devolveu := false, false
	for i := 0; i < 8; i++ {
		ty, p, ok := readMaybe(t, conn)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgSell, protocol.MsgUpdateEtc:
			t.Fatalf("a venda passou (%#x) com o ouro acima do teto", ty)
		case protocol.MsgMessagePanel:
			if strings.Contains(decodePanel(p), "carregar mais ouro") {
				avisou = true
			}
		case protocol.MsgSendItem:
			if le16(p[4:6]) == 420 {
				devolveu = true
			}
		}
	}
	if !avisou {
		t.Error("a recusa não avisou o jogador")
	}
	if !devolveu {
		t.Error("a pilha não foi reenviada ao cliente depois da recusa")
	}
}
