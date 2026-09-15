package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestItemDeNivelSoParaMortal: o legado só chama DoItemLevel com
// ClassMaster == MORTAL, nos oito pontos que sobem nível (SendFunc.cpp:1042,
// _MSG_Attack.cpp:1772 e _MSG_UseItem.cpp:1028, 1045, 1080, 2397, 2435 e 2475).
// Aqui a peça ia para qualquer evolução: um Arch ou um Celestial que passasse
// pelo 29 recebia a Luvas_Sombrias no armazém.
func TestItemDeNivelSoParaMortal(t *testing.T) {
	for _, c := range []struct {
		nome   string
		cm     uint8
		recebe bool
	}{
		{"Mortal recebe", classMasterMortal, true},
		{"Arch não recebe", classMasterArch, false},
		{"Celestial não recebe", classMasterCelestial, false},
	} {
		t.Run(c.nome, func(t *testing.T) {
			d, w, stop := startServerItemDeNivel(t)
			defer stop()

			var nivel int32
			var armazem [world.MaxCargo]world.Item
			noJogador(t, w, func(w *world.World, s *world.Session, e *world.Entity) {
				comoFoemaInt(e, 28, 0)
				e.ClassMaster = c.cm
				e.Exp = level.NextLevelExpTier(28, c.cm) // um nível só: 28 -> 29
				d.applyLevelUps(w, s, e)
				nivel = e.Level
				if cg := w.Cargo(s.AccountID); cg != nil {
					armazem = cg.Items
				}
			})

			if nivel != 29 {
				t.Fatalf("nível = %d, queria 29", nivel)
			}
			if c.recebe {
				if !mesmaPeca(armazem[0], pecaDo29) {
					t.Errorf("vaga 0 = %+v, queria a peça do 29 (%+v)", armazem[0], pecaDo29)
				}
				return
			}
			for vaga, it := range armazem {
				if !it.Empty() {
					t.Errorf("vaga %d recebeu %+v; fora do Mortal o legado não entrega peça de nível", vaga, it)
				}
			}
		})
	}
}
