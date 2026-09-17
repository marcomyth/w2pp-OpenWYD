package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O guarda das arenas devolve o Mortal vivo cujo nível saiu da faixa da quest lá
// dentro. O Cemitério (Coveiro) vai do guardado 39 ao 114: 115 é o 116 da tela.

// TestGuardaDevolveQuemPassouDaFaixa: 114 fica; subir para 115 lá dentro devolve
// à cidade com o aviso, no tique seguinte, e fecha a rodada.
func TestGuardaDevolveQuemPassouDaFaixa(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	entraPeloGrifo(t, c, srv)
	cemiterio := quest256Steps[0].area

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, _ *world.Session, e *world.Entity) {
		e.Level = 114
		d.Tick(w)
		if !cemiterio.contains(e.X, e.Y) {
			t.Error("o 114 foi devolvido; ainda está na faixa do Coveiro")
		}
	})
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		e.Level = 115
		d.Tick(w)
		if cemiterio.contains(e.X, e.Y) {
			t.Error("subiu para 115 e continuou no Cemitério")
		}
		if e.QuestFlag != 0 {
			t.Errorf("devolvido com a bandeira %d", e.QuestFlag)
		}
		d.Tick(w) // a vigia vê a saída
		if ent, ok := d.entradasDaRodada[donoDe(s)]; !ok || !ent.saiu {
			t.Error("a devolução por nível não fechou a rodada")
		}
	})
	if _, _, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		return h.Type == protocol.MsgMessagePanel && strings.Contains(decodePanel(p), "passou do limite desta quest")
	}); !ok {
		t.Error("a devolução por nível não avisou o jogador")
	}
}

// TestGuardaEsperaOMortoReviver: morto fora da faixa fica até reviver.
func TestGuardaEsperaOMortoReviver(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	entraPeloGrifo(t, c, srv)
	cemiterio := quest256Steps[0].area

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, _ *world.Session, e *world.Entity) {
		e.Level, e.HP = 115, 0
		d.guardQuest256Areas(w)
		if !cemiterio.contains(e.X, e.Y) {
			t.Error("devolveu um morto; ele volta pela cidade no restart")
		}
		e.HP = e.MaxHP
		d.guardQuest256Areas(w)
		if cemiterio.contains(e.X, e.Y) {
			t.Error("vivo de novo e fora da faixa, continuou no Cemitério")
		}
	})
}
