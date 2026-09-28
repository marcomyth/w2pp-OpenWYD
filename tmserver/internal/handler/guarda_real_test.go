package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func guardaRealTemplate(t *testing.T, arquivo string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "npc", arquivo))
	if err != nil {
		t.Skipf("template %s indisponível: %v", arquivo, err)
	}
	return b
}

// itensEnviados lê o que chega até a conexão ficar quieta: os MsgSendItem
// (place/slot -> índice) e as linhas do painel de mensagens.
func itensEnviados(t *testing.T, cc func() (protocol.Type, []byte, bool)) (map[[2]uint16]uint16, []string) {
	t.Helper()
	got := map[[2]uint16]uint16{}
	var falas []string
	for {
		ty, payload, ok := cc()
		if !ok {
			return got, falas
		}
		switch ty {
		case protocol.MsgSendItem:
			got[[2]uint16{le16(payload[0:2]), le16(payload[2:4])}] = le16(payload[4:6])
		case protocol.MsgMessagePanel:
			falas = append(falas, decodePanel(payload))
		}
	}
}

// Os dois Guarda Real de verdade: o de Hekalotia (clan 7) veste o Manto_Hekalotia,
// o de Akelonia (clan 8) o Manto_Akelonia, e cada um cobra uma Safira. O primeiro
// clique só diz o preço; é o de confirmar que cobra e veste.
func TestGuardaRealVendeACapaPorUmaSafira(t *testing.T) {
	for _, c := range []struct {
		arquivo string
		capa    uint16
	}{
		{"Guarda_Real", capaHekalotia},
		{"Guarda_Real_", capaAkelonia},
	} {
		t.Run(c.arquivo, func(t *testing.T) {
			db := newDB()
			st := baseMortalState(220)
			st.Carry[3] = world.Item{Index: sapphireUnit1}
			db.loadResult = st
			addr, stop, npcID := startServerQuestNPC(t, db, guardaRealTemplate(t, c.arquivo))
			defer stop()
			conn := enterWorld(t, addr)
			defer conn.Close()
			ler := func() (protocol.Type, []byte, bool) { return readMaybe(t, conn) }

			questFrame(t, conn, npcID)
			if got, _ := itensEnviados(t, ler); len(got) != 0 {
				t.Fatalf("o primeiro clique mexeu em item: %v", got)
			}

			send(t, conn, protocol.MsgQuest, protocol.EncodeStandardParm2(int32(npcID), 1))
			got, _ := itensEnviados(t, ler)
			if idx, ok := got[[2]uint16{world.ItemPlaceCarry, 3}]; !ok || idx != 0 {
				t.Errorf("a Safira da vaga 3 = %d ok=%v, queria gasta", idx, ok)
			}
			if idx := got[[2]uint16{world.ItemPlaceEquip, capeEquipSlot}]; idx != c.capa {
				t.Errorf("capa vestida = %d, queria %d", idx, c.capa)
			}
		})
	}
}

// As recusas não gastam a Safira nem mexem na capa: sem Safira, com capa de reino
// já vestida, e com outra capa vestida (trocar calado apagaria a do jogador).
func TestGuardaRealRecusaSemGastar(t *testing.T) {
	for _, c := range []struct {
		nome   string
		safira bool
		capa   int16
		fala   string
	}{
		{"sem safira", false, 0, "Você precisa de 1 Safira."},
		{"já tem capa de reino", true, capaAkelonia, "Você já tem a capa de um reino."},
		{"outra capa vestida", true, greenCapeItem, "Tire a capa que está vestida para receber a capa do reino."},
	} {
		t.Run(c.nome, func(t *testing.T) {
			db := newDB()
			st := baseMortalState(220)
			if c.safira {
				st.Carry[3] = world.Item{Index: sapphireUnit1}
			}
			st.Equip[capeEquipSlot] = world.Item{Index: c.capa}
			db.loadResult = st
			addr, stop, npcID := startServerQuestNPC(t, db, guardaRealTemplate(t, "Guarda_Real"))
			defer stop()
			conn := enterWorld(t, addr)
			defer conn.Close()

			send(t, conn, protocol.MsgQuest, protocol.EncodeStandardParm2(int32(npcID), 1))
			got, falas := itensEnviados(t, func() (protocol.Type, []byte, bool) { return readMaybe(t, conn) })
			if len(got) != 0 {
				t.Errorf("a recusa mexeu em item: %v", got)
			}
			if len(falas) != 1 || falas[0] != c.fala {
				t.Errorf("falas = %q, queria [%q]", falas, c.fala)
			}
		})
	}
}
