package handler

import (
	"bytes"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Mestre Grifo leva à arena da Quest 256 só o Mortal, com a mesma recusa e a
// mesma fala do NPC da quest (quest256NPC). Antes ele ligava a bandeira para
// qualquer classe, e Arch e Celestial entravam nas arenas de graça.
func TestMestreGrifoRecusaQuemNaoEMortal(t *testing.T) {
	for _, tc := range []struct {
		nome   string
		classe uint8
	}{
		{"Arch", classMasterArch},
		{"Celestial", classMasterCelestial},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			// Nível 50: dentro da faixa do Coveiro, então só a classe pode recusar.
			st := world.CharacterState{
				Slot: 0, Name: "Hero", Level: 50, X: 2113, Y: 2079,
				HP: 1000, MaxHP: 1000, ClassMaster: tc.classe,
			}
			addr, stop, npcID := startServerMestreGrifo(t, st, false)
			defer stop()
			c := enterWorld(t, addr)
			defer c.Close()

			questFrame(t, c, npcID)
			recusou, levou := false, false
			for i := 0; i < 6; i++ {
				h, p, ok := readMaybeHeaderRaw(t, c)
				if !ok {
					break
				}
				if h.Type == protocol.MsgMessageChat && int(h.ID) == npcID &&
					bytes.Contains(p, protocol.ClientText("Seu nível não permite o uso disto.")) {
					recusou = true
				}
				if h.Type == protocol.MsgAction {
					levou = true
				}
			}
			if !recusou {
				t.Errorf("%s não ouviu a recusa do Mestre Grifo", tc.nome)
			}
			if levou {
				t.Errorf("o Mestre Grifo levou um %s para a arena", tc.nome)
			}
		})
	}
}
