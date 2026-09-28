package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// GRUPO NÃO FERE, GUILDA SÓ COM A MEDALHA BAIXADA. Decisão do Marco em 26/09:
// "precisamos zerar". É a linha `if (leader == mobleader || Guild == MobGuild)
// dam = 0;` do legado (_MSG_Attack.cpp:1334), que o port só tinha levado aos
// afetos e aos pets.
//
// A prova é o UltimoPvP da vítima: marcarPvP só roda num golpe de PvP que
// acertou com dano. Vários golpes, porque um golpe sozinho pode errar e passaria
// por "zerado" sem o ser.
func TestPvPZeraGrupoEGuildaComMedalha(t *testing.T) {
	casos := []struct {
		nome    string
		montar  func(w *world.World)
		protege bool
	}{
		{"estranhos se ferem", func(*world.World) {}, false},
		{"mesmo grupo não fere", func(w *world.World) {
			addMember(w.Entity(donoID), vitimaID)
			w.Entity(vitimaID).Leader = donoID
		}, true},
		{"mesma guilda de medalha levantada não fere", func(w *world.World) {
			w.Entity(donoID).Guild = 5
			w.Entity(vitimaID).Guild = 5
		}, true},
		{"mesma guilda com a medalha baixada fere", func(w *world.World) {
			w.Entity(donoID).Guild = 5
			w.Entity(vitimaID).Guild = 5
			w.Session(vitimaID).GuildDisable = true
		}, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, w, _, conns := subirRevide(t)
			noLaco(t, w, c.montar)

			feriu := false
			for i := 0; i < 8 && !feriu; i++ {
				tick := serverTime + uint32(1000*(i+1)) // um golpe por cadência
				attackFrame(t, conns[donoID], tick, vitimaID, 0)
				esperarGolpe(t, w, donoID, tick)
				noLaco(t, w, func(w *world.World) { feriu = w.Entity(vitimaID).UltimoPvP != 0 })
			}
			if c.protege && feriu {
				t.Error("o golpe feriu quem está do mesmo lado")
			}
			if !c.protege && !feriu {
				t.Error("oito golpes sem ferir quem não está do mesmo lado: o teste não prova nada")
			}
		})
	}
}
