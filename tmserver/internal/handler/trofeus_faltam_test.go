package handler

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Quantos troféus ainda cabem na rodada (tetorodada.go, trofeusQueCabem): o número
// que o jogador vê na entrada, a cada troféu e no /xp, e a trava do drop usam a
// mesma conta.

// TestTrofeusQueCabemBateComATrava: a conta de uma vez só dá o mesmo que deixar
// cair troféu por troféu pela regra da trava (parte do troféu abaixo da metade E
// valor inteiro cabendo no total), em bordas e em valores que não dividem o teto.
func TestTrofeusQueCabemBateComATrava(t *testing.T) {
	umPorUm := func(teto int64, st xpDaRodada, valor int64) int64 {
		n := int64(0)
		for st.trofeu < teto/2 && valor <= teto-st.total {
			n++
			st.trofeu += valor
			st.total = min(st.total+valor, teto)
		}
		return n
	}
	for _, teto := range []int64{604951, 2669386, 1000, 7} {
		for _, valor := range []int64{1, 3, 4000, 8000, 100000, teto / 2, teto/2 + 1, teto} {
			if valor <= 0 {
				continue
			}
			for _, trofeu := range []int64{0, 1, teto/2 - valor - 1, teto/2 - valor, teto/2 - 1, teto / 2, teto} {
				for _, extra := range []int64{0, 1, valor - 1, valor, 3 * valor, teto} {
					st := xpDaRodada{trofeu: max(trofeu, 0), total: min(max(trofeu, 0)+extra, teto)}
					if got, quer := trofeusQueCabem(teto, st, valor), umPorUm(teto, st, valor); got != quer {
						t.Errorf("teto %d, valor %d, %+v: conta %d, um por um %d", teto, valor, st, got, quer)
					}
				}
			}
		}
	}
}

// mensagensDeTrofeu lê do painel, em ordem, as mensagens de troféu da rodada.
func mensagensDeTrofeu(t *testing.T, c net.Conn) []string {
	t.Helper()
	var msgs []string
	quadroAte(t, c, time.Second, func(h protocol.Header, p []byte) bool {
		if h.Type == protocol.MsgMessagePanel {
			if txt := decodePanel(p); strings.Contains(txt, "Troféu") || strings.Contains(txt, "troféus desta rodada") {
				msgs = append(msgs, txt)
			}
		}
		return false
	})
	return msgs
}

func iguais(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTrofeusNaEntrada: entrar na arena diz quantos troféus a rodada dá.
func TestTrofeusNaEntrada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	questFrame(t, c, srv.grifo)
	_, p, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		return h.Type == protocol.MsgMessagePanel && strings.HasPrefix(decodePanel(p), "Troféus nesta rodada:")
	})
	if !ok {
		t.Fatal("a entrada pelo Grifo não disse quantos troféus a rodada dá")
	}
	var quer int64
	naContaDoRelogio(t, srv, 7, func(_ *world.World, d *Dispatcher, _ *world.Session, e *world.Entity) {
		teto, _ := d.tetoDoGanho(e)
		valor := d.valorDoTrofeu(world.Item{Index: itemQuestRewardBase})
		if valor <= 0 {
			t.Fatal("troféu do Coveiro sem valor")
		}
		quer = trofeusQueCabem(teto, xpDaRodada{}, valor)
	})
	if quer <= 0 {
		t.Fatalf("rodada vazia dá %d troféus; o teste precisa de pelo menos 1", quer)
	}
	if got, want := decodePanel(p), fmt.Sprintf("Troféus nesta rodada: %d.", quer); got != want {
		t.Errorf("entrada: %q, quero %q", got, want)
	}
}

// TestTrofeusFaltamDiminuemEParam: a cada troféu que cai o número desce; no último
// sai o aviso de sempre, uma vez, e o seguinte não cai nem avisa de novo.
func TestTrofeusFaltamDiminuemEParam(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		valor := d.valorDoTrofeu(world.Item{Index: itemQuestRewardBase})
		metade := int64(tetoFaixa99 / 2)
		for i := range e.Carry {
			e.Carry[i] = world.Item{}
		}
		// Faltam três pela metade, com o total folgado.
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {trofeu: metade - 2*valor - 1, total: metade - 2*valor - 1}}
		if n, _ := d.trofeusDaArena(s, e, 0); n != 3 {
			t.Fatalf("preparo: faltam %d, quero 3", n)
		}
		caiu := 0
		for range 4 {
			if d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}) {
				caiu++
			}
		}
		if caiu != 3 {
			t.Errorf("caíram %d troféus, quero 3", caiu)
		}
	})
	msgs := mensagensDeTrofeu(t, c)
	if len(msgs) != 3 || msgs[0] != "Troféu: faltam 2 nesta rodada." || msgs[1] != "Troféu: faltam 1 nesta rodada." ||
		!strings.HasPrefix(msgs[2], "Você já recebeu os troféus desta rodada.") {
		t.Errorf("mensagens %q, quero faltam 2, faltam 1 e o aviso dos troféus uma vez", msgs)
	}
}

// TestMorteQueEncheOTotalReduzOFaltam: XP de morte entre dois troféus come a sobra
// do total, e a mensagem do troféu seguinte já conta com isso.
func TestMorteQueEncheOTotalReduzOFaltam(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		valor := d.valorDoTrofeu(world.Item{Index: itemQuestRewardBase})
		for i := range e.Carry {
			e.Carry[i] = world.Item{}
		}
		// Parte do troféu vazia; o total só tem espaço para quatro.
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {total: tetoFaixa99 - 4*valor}}
		if !d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}) {
			t.Fatal("o primeiro troféu não caiu")
		}
		if pago := d.cortaXPDaRodada(w, s, e, valor); pago != valor {
			t.Fatalf("a morte pagou %d, quero %d", pago, valor)
		}
		if !d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}) {
			t.Fatal("o segundo troféu não caiu")
		}
	})
	if msgs := mensagensDeTrofeu(t, c); !iguais(msgs, []string{"Troféu: faltam 3 nesta rodada.", "Troféu: faltam 1 nesta rodada."}) {
		t.Errorf("mensagens %q, quero faltam 3 e depois faltam 1 (a morte comeu um)", msgs)
	}
}

// TestXPMostraOsTrofeusDaRodada: o /xp de quem está na faixa de uma quest diz
// quantos troféus faltam e de qual quest.
func TestXPMostraOsTrofeusDaRodada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	var quer string
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		valor := d.valorDoTrofeu(world.Item{Index: itemQuestRewardBase})
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {total: tetoFaixa99 - 5*valor}}
		quer = "Troféus da rodada: faltam 5 (quest Coveiro)"
		d.showXPBonus(w, s)
	})
	achou := false
	quadroAte(t, c, time.Second, func(h protocol.Header, p []byte) bool {
		if h.Type == protocol.MsgMessagePanel && decodePanel(p) == quer {
			achou = true
			return true
		}
		return false
	})
	if !achou {
		t.Errorf("o /xp não mostrou %q", quer)
	}
}
