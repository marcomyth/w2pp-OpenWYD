package handler

import (
	"encoding/binary"
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

// TestTrofeuUsoLivre: usar o troféu paga inteiro e é gasto, mesmo com a parte do
// troféu e o total da rodada cheios: o limite é no drop.
func TestTrofeuUsoLivre(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		rate, ok := d.questRates.Tier(0)
		if !ok || rate.MortalExp <= 0 {
			t.Fatalf("troféu do Coveiro sem valor: %+v", rate)
		}
		e.Carry[1] = world.Item{Index: itemQuestRewardBase}
		k := donoDe(s)
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {total: tetoFaixa99, trofeu: tetoFaixa99, avisado: true}}
		exp := e.Exp
		d.useQuestReward(w, s, e, 1)
		if got := e.Exp - exp; got != rate.MortalExp {
			t.Errorf("com a rodada cheia o troféu pagou %d, quero o valor inteiro %d", got, rate.MortalExp)
		}
		if e.Carry[1].Index != 0 {
			t.Errorf("o troféu usado não foi gasto: espaço 1 = %d", e.Carry[1].Index)
		}
		if st := d.xpDaRodada[k]; st.total != tetoFaixa99 || st.trofeu != tetoFaixa99 {
			t.Errorf("o uso somou na rodada: %+v", st)
		}
	})
}

// TestTrofeuCaiAteAMetadeEPara: o troféu cai enquanto a parte reservada é menor
// que a metade do teto; depois não cai, com o aviso uma vez só. O que cai reserva
// o valor no total, e a morte só paga o que sobrou.
func TestTrofeuCaiAteAMetadeEPara(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		rate, _ := d.questRates.Tier(0)
		valor := rate.MortalExp
		metade := int64(tetoFaixa99 / 2)
		k := donoDe(s)
		for i := range e.Carry {
			e.Carry[i] = world.Item{}
		}
		// Falta um pouco menos que dois troféus para a metade: caem dois, o terceiro não.
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {trofeu: metade - valor - 1, total: metade - valor - 1}}
		caiu := 0
		for range 4 {
			if d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}) {
				caiu++
			}
		}
		if caiu != 2 {
			t.Errorf("caíram %d troféus, quero 2", caiu)
		}
		st := d.xpDaRodada[k]
		if st.trofeu != metade-valor-1+2*valor {
			t.Errorf("parte do troféu reservada = %d, quero %d", st.trofeu, metade-valor-1+2*valor)
		}
		if st.total != st.trofeu {
			t.Errorf("o total não recebeu a reserva: total %d, troféu %d", st.total, st.trofeu)
		}
		// Uma espada do mesmo saque continua caindo.
		if !d.putMobDrop(w, e, world.Item{Index: 1100}) {
			t.Error("o resto do saque parou junto com o troféu")
		}
		// A morte agora só cabe no que o troféu deixou.
		if cabe, _ := d.cabeNaRodada(s, e); cabe != tetoFaixa99-st.total {
			t.Errorf("depois da reserva cabem %d, quero %d", cabe, tetoFaixa99-st.total)
		}
	})
	avisos := 0
	quadroAte(t, c, time.Second, func(h protocol.Header, p []byte) bool {
		if h.Type == protocol.MsgMessagePanel && strings.Contains(decodePanel(p), "recebeu os troféus desta rodada") {
			avisos++
		}
		return false
	})
	if avisos != 1 {
		t.Errorf("o aviso dos troféus saiu %d vezes em duas recusas, quero 1", avisos)
	}
}

// TestTrofeuSoCaiSeOValorCabeNoTotal: quem encheu o total matando não ganha troféu
// na arena; com espaço exato para o troféu inteiro ele cai, com um a menos não.
func TestTrofeuSoCaiSeOValorCabeNoTotal(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		valor := d.valorDoTrofeu(world.Item{Index: itemQuestRewardBase})
		if valor <= 0 {
			t.Fatal("troféu do Coveiro sem valor")
		}
		k := donoDe(s)
		casos := []struct {
			nome  string
			total int64
			cai   bool
		}{
			{"total cheio de mortes", tetoFaixa99, false},
			{"espaço exato para um troféu", tetoFaixa99 - valor, true},
			{"espaço um abaixo do troféu", tetoFaixa99 - valor + 1, false},
		}
		for _, cs := range casos {
			for i := range e.Carry {
				e.Carry[i] = world.Item{}
			}
			d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {total: cs.total, avisado: true, avisadoTrofeu: true}}
			if got := d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}); got != cs.cai {
				t.Errorf("%s: caiu = %v, quero %v", cs.nome, got, cs.cai)
			}
			if cs.cai {
				if st := d.xpDaRodada[k]; st.total != tetoFaixa99 || st.trofeu != valor {
					t.Errorf("%s: reserva = %+v, quero total %d e troféu %d", cs.nome, st, tetoFaixa99, valor)
				}
			}
		}
	})
}

// TestTrofeuComDobroCaiODobro: com o dobro, a metade é a do teto do dobro.
func TestTrofeuComDobroCaiODobro(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		k := donoDe(s)
		for i := range e.Carry {
			e.Carry[i] = world.Item{}
		}
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{k: {trofeu: tetoFaixa99 / 2, avisadoTrofeu: true}}
		if d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}) {
			t.Error("sem dobro e com a metade cheia, o troféu caiu")
		}
		d.expEvents.DoubleMode = true
		if !d.putMobDrop(w, e, world.Item{Index: itemQuestRewardBase}) {
			t.Error("com dobro a metade dobra, e o troféu não caiu")
		}
		d.expEvents.DoubleMode = false
	})
}

// TestTrofeuNaoVaiAoBauNemAoChao: o troféu é do personagem. Com o guarda do baú
// do lado, levar à bolsa para o baú é recusado; soltar no chão também. Uma espada
// passa pelos dois caminhos, para o teste não passar por outra trava.
func TestTrofeuNaoVaiAoBauNemAoChao(t *testing.T) {
	srv := startServerRelogioDasArenas(t, mortalDoCemiterio(), inicioDaVolta)
	c := enterWorldAs(t, srv.addr, "tester")
	defer c.Close()

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		tmpl := make([]byte, 816)
		copy(tmpl[0:16], "CargoGuard")
		tmpl[92+12] = 2 // Merchant = guarda do baú
		binary.LittleEndian.PutUint32(tmpl[92+16:], 100)
		binary.LittleEndian.PutUint32(tmpl[92+24:], 100)
		guarda := w.SpawnMob(tmpl, e.X+1, e.Y)
		if guarda < 0 {
			t.Fatal("o guarda do baú não nasceu")
		}
		cargo := w.Cargo(s.AccountID)
		if cargo == nil {
			t.Fatal("a conta não tem baú carregado")
		}
		leva := func(slot, destino int) {
			body := protocol.MsgTradingItemBody{SrcPlace: uint8(world.ItemPlaceCarry), SrcSlot: uint8(slot),
				DestPlace: uint8(world.ItemPlaceCargo), DestSlot: uint8(destino), WarpID: int32(guarda)}
			d.tradingItem(w, s, protocol.Header{}, body.Encode())
		}
		e.Carry[1] = world.Item{Index: itemQuestRewardBase}
		e.Carry[2] = world.Item{Index: 1100}
		cargo.Items[5], cargo.Items[6] = world.Item{}, world.Item{}
		leva(1, 5)
		if e.Carry[1].Index != itemQuestRewardBase || cargo.Items[5].Index != 0 {
			t.Errorf("o troféu foi ao baú: bolsa %d, baú %d", e.Carry[1].Index, cargo.Items[5].Index)
		}
		leva(2, 6)
		if cargo.Items[6].Index != 1100 {
			t.Fatalf("a espada não foi ao baú (%d); o teste não chegou à trava do troféu", cargo.Items[6].Index)
		}
	})
	// A recusa do baú também reenvia o espaço; o chão é conferido à parte.
	drena(t, c)

	naContaDoRelogio(t, srv, 7, func(w *world.World, d *Dispatcher, s *world.Session, e *world.Entity) {
		solta := func(slot int) {
			body := protocol.MsgDropItemBody{SourType: int32(world.ItemPlaceCarry), SourPos: int32(slot),
				GridX: uint16(e.X), GridY: uint16(e.Y + 2)}
			d.dropItem(w, s, protocol.Header{}, body.Encode())
		}
		e.Carry[3] = world.Item{Index: 1100}
		solta(1)
		if e.Carry[1].Index != itemQuestRewardBase {
			t.Errorf("o troféu foi ao chão: espaço 1 = %d", e.Carry[1].Index)
		}
		solta(3)
		if e.Carry[3].Index != 0 {
			t.Fatalf("a espada não foi ao chão (%d); o teste não chegou à trava do troféu", e.Carry[3].Index)
		}
	})
	// A recusa do chão reenvia o espaço do troféu, que o cliente tirou da tela.
	if _, _, ok := quadroAte(t, c, 2*time.Second, func(h protocol.Header, p []byte) bool {
		return h.Type == protocol.MsgSendItem && len(p) >= 6 && binary.LittleEndian.Uint16(p[0:2]) == uint16(world.ItemPlaceCarry) &&
			binary.LittleEndian.Uint16(p[2:4]) == 1 && int16(binary.LittleEndian.Uint16(p[4:6])) == itemQuestRewardBase
	}); !ok {
		t.Error("a recusa do chão não reenviou o espaço do troféu")
	}
}

// TestTetoNaParteDoGrupo: os 10% do troféu contam no total da rodada de QUEM
// RECEBE, com corte.
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
		d.xpDaRodada = map[donoDaEntrada]xpDaRodada{donoDe(sRecebe): {total: tetoFaixa99 - 50, avisado: true}}
		antes := recebe.Exp
		d.grantQuestPartyExp(w, usa, 1000)
		if got := recebe.Exp - antes; got != 50 {
			t.Errorf("a parte do grupo pagou %d a quem tinha 50 de rodada", got)
		}
		if st := d.xpDaRodada[donoDe(sRecebe)]; st.total != tetoFaixa99 {
			t.Errorf("total de quem recebe = %d, quero %d", st.total, tetoFaixa99)
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
		if got := d.cortaXPDaRodada(w, s, e, 1000); got != 0 {
			t.Errorf("sem dobro e com o teto cheio pagou %d", got)
		}
		d.expEvents.DoubleMode = true
		if teto, _ := d.tetoDoGanho(e); teto != domain.DefaultRoundXPCapDouble[0] {
			t.Errorf("com dobro o teto é %d, quero %d", teto, domain.DefaultRoundXPCapDouble[0])
		}
		if got := d.cortaXPDaRodada(w, s, e, 1000); got != 1000 {
			t.Errorf("com dobro, o teto dobrado ainda tem espaço e pagou %d", got)
		}
		d.expEvents.DoubleMode = false
		e.ClassMaster = classMasterArch
		if got := d.cortaXPDaRodada(w, s, e, 5_000_000); got != 5_000_000 {
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
		if cabe, _ := d.cabeNaRodada(s, e); cabe != 0 {
			t.Errorf("depois do relog cabem %d; o relog zerou o teto", cabe)
		}
		d.Tick(w) // o pulso
		if cabe, _ := d.cabeNaRodada(s, e); cabe != tetoFaixa99 {
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
			d.cortaXPDaRodada(w, s, e, 100)
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
