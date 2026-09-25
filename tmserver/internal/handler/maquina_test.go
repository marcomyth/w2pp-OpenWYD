package handler

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func TestMaquinaConhecida(t *testing.T) {
	casos := []struct {
		nome string
		m    [4]int32
		quer bool
	}{
		{"GUID de placa", [4]int32{0x1a2b3c4d, 0x11112222, -0x5eadbeef, 0x7}, true},
		{"cliente sem placa (zero)", [4]int32{}, false},
		{"pacote curto no legado (0xFF)", [4]int32{-1, -1, -1, -1}, false},
		{"só um campo preenchido", [4]int32{0, 0, 0, 1}, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := maquinaConhecida(c.m); got != c.quer {
				t.Fatalf("maquinaConhecida(%v) = %v, quer %v", c.m, got, c.quer)
			}
		})
	}
}

// lojaNa monta uma sessão com barraca abastecida, aberta em openedAt, na máquina m.
func lojaNa(conn int, m [4]int32, openedAt uint32) *world.Session {
	s := &world.Session{Conn: conn, Maquina: m, Mode: world.UserPlay}
	s.AutoTrade = stockedShop()
	s.AutoTrade.OpenedAt = openedAt
	s.AutoTrade.PaidUntil = openedAt
	return s
}

func percorre(ss ...*world.Session) func(func(*world.Session)) {
	return func(fn func(*world.Session)) {
		for _, s := range ss {
			fn(s)
		}
	}
}

func TestLojaPrecedidaNaMaquina(t *testing.T) {
	pcA := [4]int32{1, 2, 3, 4}
	pcB := [4]int32{5, 6, 7, 8}
	const now = uint32(10 * 60 * 1000)

	t.Run("mesma máquina: só a mais nova fica sem render", func(t *testing.T) {
		velha, nova := lojaNa(1, pcA, 0), lojaNa(2, pcA, 60_000)
		todas := percorre(velha, nova)
		if lojaPrecedidaNaMaquina(now, velha, todas) {
			t.Error("a barraca mais antiga foi dada como precedida; é ela que rende")
		}
		if !lojaPrecedidaNaMaquina(now, nova, todas) {
			t.Error("a segunda barraca do mesmo computador não foi dada como precedida")
		}
	})

	t.Run("máquinas diferentes rendem as duas", func(t *testing.T) {
		a, b := lojaNa(1, pcA, 0), lojaNa(2, pcB, 60_000)
		todas := percorre(a, b)
		if lojaPrecedidaNaMaquina(now, a, todas) || lojaPrecedidaNaMaquina(now, b, todas) {
			t.Error("duas pessoas em computadores diferentes não podem se travar")
		}
	})

	t.Run("máquina desconhecida não entra na trava", func(t *testing.T) {
		a, b := lojaNa(1, [4]int32{}, 0), lojaNa(2, [4]int32{}, 60_000)
		if lojaPrecedidaNaMaquina(now, b, percorre(a, b)) {
			t.Error("clientes sem placa de rede foram tratados como um computador só")
		}
	})

	t.Run("barraca vazia não tira o prêmio da outra", func(t *testing.T) {
		vazia, cheia := lojaNa(1, pcA, 0), lojaNa(2, pcA, 60_000)
		vazia.AutoTrade.Slots[0] = world.AutoTradeSlot{CargoPos: -1}
		if lojaPrecedidaNaMaquina(now, cheia, percorre(vazia, cheia)) {
			t.Error("uma barraca vazia, que não rende, travou a abastecida")
		}
	})

	t.Run("empate: exatamente uma rende", func(t *testing.T) {
		a, b := lojaNa(1, pcA, 60_000), lojaNa(2, pcA, 60_000)
		todas := percorre(a, b)
		pa, pb := lojaPrecedidaNaMaquina(now, a, todas), lojaPrecedidaNaMaquina(now, b, todas)
		if pa == pb {
			t.Errorf("no empate precedida(a)=%v precedida(b)=%v: quer uma de cada", pa, pb)
		}
	})

	t.Run("a volta do relógio não inverte quem é mais antiga", func(t *testing.T) {
		// A velha abriu pouco antes do uint32 virar; a nova, depois. Comparar
		// OpenedAt direto daria a nova como mais antiga.
		velha, nova := lojaNa(1, pcA, ^uint32(0)-30_000), lojaNa(2, pcA, 10_000)
		agora := uint32(60_000)
		todas := percorre(velha, nova)
		if lojaPrecedidaNaMaquina(agora, velha, todas) || !lojaPrecedidaNaMaquina(agora, nova, todas) {
			t.Error("na volta do relógio a barraca mais nova passou a render no lugar da antiga")
		}
	})
}

// enterWorldNaMaquina é o enterWorldAs com a placa de rede do login preenchida.
func enterWorldNaMaquina(t *testing.T, addr, conta string, m [4]int32) net.Conn {
	t.Helper()
	var b protocol.MsgAccountLoginBody
	copy(b.AccountName[:], conta)
	copy(b.AccountPassword[:], "secret")
	b.ClientVersion = protocol.AppVersion
	b.AdapterName = m
	c := dial(t, addr)
	send(t, c, protocol.MsgAccountLogin, b.Encode())
	if ty, _ := read(t, c); ty != protocol.MsgCNFAccountLogin {
		t.Fatalf("login %s falhou: %#x", conta, ty)
	}
	var corpo protocol.MsgCharacterLoginBody
	send(t, c, protocol.MsgCharacterLogin, corpo.Encode())
	if ty, _ := read(t, c); ty != protocol.MsgCNFCharacterLogin {
		t.Fatalf("entrada no mundo de %s falhou: %#x", conta, ty)
	}
	drainLoginScore(t, c)
	return c
}

// duasBarracas sobe um servidor de pontos e abre uma barraca em cada uma de duas
// contas, nas máquinas dadas. Devolve o banco, o relógio, as conexões e o stop.
func duasBarracas(t *testing.T, maqA, maqB [4]int32) (*fakeDB, func(uint32), net.Conn, net.Conn, func()) {
	t.Helper()
	db := autotradeDB(1030)
	var carga world.CargoState
	carga.Items[0] = world.Item{Index: 1030}
	db.accounts["tradeb"].cargo = carga
	addr, stop, clock := startServerPontos(t, db)
	a := enterWorldNaMaquina(t, addr, "tester", maqA)
	b := enterWorldNaMaquina(t, addr, "tradeb", maqB)
	abreBarraca(t, a, "Loja A", 0, 250_000, protocol.LojaMoedaOuro)
	// Um minuto entre as duas: é o que decide qual das duas é a mais antiga.
	clock.Add(60_000)
	abreBarraca(t, b, "Loja B", 0, 250_000, protocol.LojaMoedaOuro)
	return db, func(ms uint32) { clock.Add(ms) }, a, b, func() {
		_ = a.Close()
		_ = b.Close()
		stop()
	}
}

// pontosAssentados espera a carteira parar de mudar e devolve o valor.
func pontosAssentados(db *fakeDB) int32 {
	esperaPontos(db, 2*time.Second)
	time.Sleep(300 * time.Millisecond)
	return db.pontosLojinha0()
}

// O caso do print de 25/09: duas contas, um computador, duas barracas.
func TestDuasBarracasNoMesmoComputadorRendemComoUma(t *testing.T) {
	pc := [4]int32{0x1a2b3c4d, 0x11112222, 0x33334444, 0x55556666}
	db, anda, _, b, stop := duasBarracas(t, pc, pc)
	defer stop()

	// A segunda barraca é avisada na hora em que sobe.
	var avisada bool
	for _, l := range linhasDeAviso(t, b) {
		if strings.Contains(l, "Outra lojinha deste computador") {
			avisada = true
		}
	}
	if !avisada {
		t.Error("quem abriu a segunda barraca do computador não foi avisado de que ela não rende")
	}

	anda(shopPointsWindowMs + 60_000)
	if p := pontosAssentados(db); p != shopPointsBase {
		t.Fatalf("duas barracas do mesmo computador renderam %d pontos numa janela; quer %d (uma só)",
			p, shopPointsBase)
	}
}

// O controle do teste acima: sem ele, um harness que nunca pagasse a segunda
// barraca passaria lá também.
func TestDuasBarracasEmComputadoresDiferentesRendemAsDuas(t *testing.T) {
	db, anda, _, _, stop := duasBarracas(t, [4]int32{1, 1, 1, 1}, [4]int32{2, 2, 2, 2})
	defer stop()

	anda(shopPointsWindowMs + 60_000)
	if p := pontosAssentados(db); p != 2*shopPointsBase {
		t.Fatalf("duas pessoas em computadores diferentes renderam %d pontos; quer %d", p, 2*shopPointsBase)
	}
}

// Quando a primeira barraca cai, a segunda passa a render — do momento em que
// cai, e não desde que abriu: o tempo que ela passou atrás da outra não vira
// crédito atrasado.
func TestSegundaBarracaAssumeQuandoAPrimeiraCai(t *testing.T) {
	pc := [4]int32{9, 9, 9, 9}
	db, anda, a, _, stop := duasBarracas(t, pc, pc)
	defer stop()

	// Duas janelas com as duas de pé: rende só a primeira.
	anda(2*shopPointsWindowMs + 60_000)
	if p := pontosAssentados(db); p != 2*shopPointsBase {
		t.Fatalf("com as duas de pé, duas janelas renderam %d; quer %d", p, 2*shopPointsBase)
	}

	// A primeira cai com a queda da conexão, e passa uma janela.
	_ = a.Close()
	time.Sleep(200 * time.Millisecond)
	anda(shopPointsWindowMs + 60_000)
	time.Sleep(300 * time.Millisecond)
	if p := db.pontosLojinha0(); p != 3*shopPointsBase {
		t.Fatalf("depois de a primeira cair a carteira tem %d; quer %d (a segunda rende UMA janela, sem atrasados)",
			p, 3*shopPointsBase)
	}
}
