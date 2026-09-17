package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/level"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O teto de XP por rodada (tetorodada.go). Os testes usam o servidor do relógio
// (startServerRelogioDasArenas) e rodam as funções DENTRO do laço (noLaco), com
// o contador da rodada posto à mão: o Mortal 50 está na faixa 1-99.

const tetoFaixa99 = 604951 // domain.DefaultRoundXPCap[0]

func TestTetoDaFaixaEOPadraoDecidido(t *testing.T) {
	if domain.DefaultRoundXPCap[0] != tetoFaixa99 {
		t.Fatalf("padrão da faixa 1-99 = %d, quero %d", domain.DefaultRoundXPCap[0], tetoFaixa99)
	}
	for nivel, faixa := range map[int32]int{1: 0, 99: 0, 100: 1, 199: 1, 200: 2, 250: 2, 299: 2, 300: 3, 349: 3, 350: 4, 398: 4, 399: 4} {
		if got := faixaDoTeto(nivel); got != faixa {
			t.Errorf("nível guardado %d na faixa %d, quero %d", nivel, got, faixa)
		}
	}
}

// ganhoDaMorte é o que grantExp pagaria sem teto, pela mesma conta.
func ganhoDaMorte(d *Dispatcher, e, mob *world.Entity) int64 {
	termos := d.termosDe(e)
	return level.ExpReward(level.ExpRewardInput{
		Zone:   level.ZoneForKill(int32(mob.X), int32(mob.Y), int32(e.X), int32(e.Y)),
		MobExp: mob.Exp, KillerLevel: e.Level, MobLevel: mob.Level, Tier: tierOf(e),
		ExpBonus: termos.bonus, FairyContent: termos.fada, KillingBlow: &termos.golpe,
		Events: d.expEvents, Config: d.xpConfig,
	})
}

// TestTetoNaMorte: abaixo do teto paga inteira; na borda paga só o que cabe;
// com o teto cheio paga zero.
func TestTetoNaMorte(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		id := w.SpawnMobAt(world.MobSpawn{Template: expMobTemplate(e.Level, 100_000, 0), X: e.X + 1, Y: e.Y, GenIndex: -1})
		if id < 0 {
			t.Fatal("o mob não nasceu")
		}
		mob := w.Entity(id)
		ganho := ganhoDaMorte(d, e, mob)
		if ganho < 2 {
			t.Fatalf("a morte paga %d; o teste precisa de pelo menos 2", ganho)
		}
		casos := []struct {
			nome    string
			jaTinha int64
			quero   int64
		}{
			{"abaixo do teto", 0, ganho},
			{"na borda", tetoFaixa99 - ganho/2, ganho / 2},
			{"teto cheio", tetoFaixa99, 0},
		}
		for _, cs := range casos {
			d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {total: cs.jaTinha, avisado: true}}
			antes := e.Exp
			d.grantExp(w, s, e, mob, d.termosDe(e))
			if got := e.Exp - antes; got != cs.quero {
				t.Errorf("%s: pagou %d, quero %d (morte de %d)", cs.nome, got, cs.quero, ganho)
			}
			if got := d.xpDaRodada[donoDe(s)].total; got != cs.jaTinha+cs.quero {
				t.Errorf("%s: contador %d, quero %d", cs.nome, got, cs.jaTinha+cs.quero)
			}
		}
	})
}

// TestTetoNoTrofeu: com a metade troféu cheia o troféu é recusado e fica na
// bolsa, sem XP e sem ouro; na borda paga o que cabe e é gasto.
func TestTetoNoTrofeu(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		rate, ok := d.questRates.Tier(0)
		if !ok || rate.MortalExp <= 100 {
			t.Fatalf("troféu do Coveiro sem valor útil: %+v", rate)
		}
		e.Carry[1] = world.Item{Index: itemQuestRewardBase}
		k := donoDe(s)

		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {total: tetoFaixa99 / 2, trofeu: tetoFaixa99 / 2}}
		exp, ouro := e.Exp, e.Coin
		d.useQuestReward(w, s, e, 1)
		if e.Carry[1].Index != itemQuestRewardBase {
			t.Errorf("metade troféu cheia: o troféu saiu da bolsa (espaço 1 = %d)", e.Carry[1].Index)
		}
		if e.Exp != exp || e.Coin != ouro {
			t.Errorf("metade troféu cheia: pagou %d de XP e %d de ouro", e.Exp-exp, e.Coin-ouro)
		}

		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {total: tetoFaixa99/2 - 100, trofeu: tetoFaixa99/2 - 100}}
		exp = e.Exp
		d.useQuestReward(w, s, e, 1)
		if got := e.Exp - exp; got != 100 {
			t.Errorf("na borda: pagou %d, quero 100", got)
		}
		if e.Carry[1].Index != 0 {
			t.Errorf("na borda o troféu pagou e devia ser gasto; espaço 1 = %d", e.Carry[1].Index)
		}
	})
	if _, p, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		return h.Type == protocol.MsgMessagePanel && strings.Contains(decodePanel(p), "ficou na bolsa")
	}); !ok {
		t.Errorf("a recusa do troféu não avisou o jogador (%q)", decodePanel(p))
	}
}

// TestTetoNaParteDoGrupo: os 10% do troféu contam no total e na metade troféu de
// QUEM RECEBE.
func TestTetoNaParteDoGrupo(t *testing.T) {
	outro := mortalDoCemiterio()
	outro.Name = "HeroB"
	srv := startServerRelogioDasArenasCom(t, mortalDoCemiterio(), inicioDaVolta, map[int64]world.CharacterState{11: outro})
	a := enterWorldAs(t, srv.addr, "tester")
	defer a.Close()
	b := enterWorldAs(t, srv.addr, "tradeb")
	defer b.Close()

	srv.noLaco(t, func(w *world.World, d *Dispatcher) {
		var usa, recebe *world.Entity
		var sRecebe *world.Session
		w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
			switch s.AccountID {
			case 7:
				usa = e
			case 11:
				recebe, sRecebe = e, s
			}
		})
		if usa == nil || recebe == nil {
			t.Fatal("os dois não estão em jogo")
		}
		usa.Leader = recebe.ID
		recebe.PartyList[0] = usa.ID
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(sRecebe): {total: 1000, trofeu: tetoFaixa99/2 - 50}}
		antes := recebe.Exp
		d.grantQuestPartyExp(w, usa, 1000)
		if got := recebe.Exp - antes; got != 50 {
			t.Errorf("a parte do grupo pagou %d a quem tinha 50 de metade troféu", got)
		}
		st := d.xpDaRodada[donoDe(sRecebe)]
		if st.trofeu != tetoFaixa99/2 || st.total != 1050 {
			t.Errorf("contador de quem recebe = %+v, quero troféu %d e total 1050", st, tetoFaixa99/2)
		}
	})
}

// TestTetoComDobroEClasse: o dobro troca pelo teto do dobro; Arch não tem teto.
func TestTetoComDobroEClasse(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		k := donoDe(s)
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {total: tetoFaixa99, avisado: true}}
		if got := d.cortaXPDaRodada(w, s, e, 1000, false); got != 0 {
			t.Errorf("sem dobro e com o teto cheio pagou %d", got)
		}
		d.expEvents.DoubleMode = true
		if teto, _ := d.tetoDoGanho(e); teto != domain.DefaultRoundXPCapDouble[0] {
			t.Errorf("com dobro o teto é %d, quero %d", teto, domain.DefaultRoundXPCapDouble[0])
		}
		if got := d.cortaXPDaRodada(w, s, e, 1000, false); got != 1000 {
			t.Errorf("com dobro, o teto dobrado ainda tem espaço e pagou %d", got)
		}
		d.expEvents.DoubleMode = false
		e.ClassMaster = classMasterArch
		if got := d.cortaXPDaRodada(w, s, e, 5_000_000, false); got != 5_000_000 {
			t.Errorf("um Arch passou pelo teto e recebeu %d", got)
		}
		e.ClassMaster = classMasterMortal
	})
}

// TestTetoZeraNoPulsoENaoNoRelog: o pulso abre a rodada; sair e voltar do jogo não.
func TestTetoZeraNoPulsoENaoNoRelog(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), 4*questClearTicks-1)
	c := enterWorldAs(t, srv.addr, "tester")

	naContaDoRelogio(t, srv, 7, func(_ *world.World, d *Dispatcher, s *world.Session, _ *world.Entity) {
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {total: tetoFaixa99, avisado: true}}
	})
	c.Close()
	fim := time.Now().Add(3 * time.Second)
	for {
		conectado := false
		srv.noLaco(t, func(w *world.World, _ *Dispatcher) {
			w.ForEachPlayer(func(s *world.Session, _ *world.Entity) { conectado = conectado || s.AccountID == 7 })
		})
		if !conectado {
			break
		}
		if time.Now().After(fim) {
			t.Fatal("a sessão fechada continuou em jogo")
		}
		time.Sleep(10 * time.Millisecond)
	}
	c2 := enterWorldAs(t, srv.addr, "tester")
	defer c2.Close()
	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		if cabe, _ := d.cabeNaRodada(s, e, false); cabe != 0 {
			t.Errorf("depois do relog cabem %d; o relog zerou o teto", cabe)
		}
		d.Tick(w) // o pulso
		if cabe, _ := d.cabeNaRodada(s, e, false); cabe != tetoFaixa99 {
			t.Errorf("depois do pulso cabem %d, quero o teto inteiro (%d)", cabe, tetoFaixa99)
		}
	})
}

// TestPoeiraDeFadaIsentaDoTeto: com o teto cheio a Poeira ainda dá o nível.
func TestPoeiraDeFadaIsentaDoTeto(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {total: tetoFaixa99, trofeu: tetoFaixa99 / 2, avisado: true}}
		nivel := e.Level
		e.Carry[1] = world.Item{Index: 414}
		d.useFairyDust(w, s, e, 1)
		if e.Level != nivel+1 {
			t.Errorf("com o teto cheio a Poeira levou do nível %d ao %d, quero %d", nivel, e.Level, nivel+1)
		}
	})
}

// TestAvisoDoTetoUmaVezPorRodada: o aviso sai no primeiro corte, não a cada morte.
func TestAvisoDoTetoUmaVezPorRodada(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(s): {total: tetoFaixa99 - 10}}
		for range 3 {
			d.cortaXPDaRodada(w, s, e, 100, false)
		}
	})
	avisos := 0
	quadroAte(t, c, time.Second, func(h protocol.Header, p []byte) bool {
		if h.Type == protocol.MsgMessagePanel && strings.Contains(decodePanel(p), "limite de XP desta rodada") {
			avisos++
		}
		return false
	})
	if avisos != 1 {
		t.Errorf("o aviso de teto saiu %d vezes em três cortes, quero 1", avisos)
	}
}
