package handler

import (
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// contagemDeSaves lê os três contadores de uma vez, sob o mutex.
func contagemDeSaves(db *fakeDB) (pares, personagemSozinho, cargaSozinha int) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.paresSalvos, db.personagemSozinho, db.cargaSozinha
}

// TestSaqueEQuedaNaoDuplica é o teste do defeito medido.
//
// O jogador saca ouro da carga: na memória o ouro sai da carga e entra no
// personagem. Enquanto isso eram DUAS gravações, uma queda entre elas deixava o
// mesmo ouro nos dois lados do banco, e o próximo login via o dobro.
//
// O que se prova aqui: a queda (a desconexão) produz UMA gravação, com as duas
// metades, e a soma das duas é a de antes. Nenhuma metade vai sozinha — é isso
// que o contador de sozinhas guarda, porque uma gravação a mais que ninguém
// contasse seria exatamente o buraco de volta.
func TestSaqueEQuedaNaoDuplica(t *testing.T) {
	const ouro = int32(1000)
	db := cargoDB(0, ouro, 0, 0)
	addr, stop, _ := startServerClock(t, db)
	defer stop()
	c := enterWorld(t, addr)

	withdrawFrame(t, c, ouro)
	expect(t, c, protocol.MsgWithdraw)
	expect(t, c, protocol.MsgUpdateCargoCoin)

	// A queda.
	c.Close()
	esperaPar(t, db)

	pares, sozinhoP, sozinhaC := contagemDeSaves(db)
	if pares == 0 {
		t.Fatal("a saída não gravou o par personagem+carga")
	}
	if sozinhoP != 0 || sozinhaC != 0 {
		t.Errorf("gravações soltas: %d só do personagem, %d só da carga — cada uma é uma janela de dupe",
			sozinhoP, sozinhaC)
	}

	personagem, _ := db.lastSavedChar()
	carga, _ := db.lastSavedCargo()
	if soma := personagem.Coin + carga.Coin; soma != ouro {
		t.Errorf("soma gravada = %d (personagem %d + carga %d), queria %d",
			soma, personagem.Coin, carga.Coin, ouro)
	}
	if personagem.Coin != ouro {
		t.Errorf("o personagem foi gravado com %d de ouro, queria %d", personagem.Coin, ouro)
	}
	if carga.Coin != 0 {
		t.Errorf("a carga foi gravada com %d de ouro, queria 0 — o saque já tinha saído dela", carga.Coin)
	}
}

// TestSaqueDeItemEQuedaNaoDuplica é o mesmo, com item em vez de ouro: o item
// sacado tem de aparecer na mochila gravada e NÃO na carga gravada, e as duas
// metades têm de ter ido juntas.
func TestSaqueDeItemEQuedaNaoDuplica(t *testing.T) {
	const indice = int16(2200)
	db := cargoDB(0, 0, 0, indice)
	addr, stop := startServerCargoGuard(t, db)
	defer stop()
	c := enterWorld(t, addr)

	tradeItemFrame(t, c, world.ItemPlaceCargo, 0, world.ItemPlaceCarry, 5, cargoGuardID)
	expect(t, c, protocol.MsgTradingItem)

	c.Close()
	esperaPar(t, db)

	pares, sozinhoP, sozinhaC := contagemDeSaves(db)
	if pares == 0 {
		t.Fatal("a saída não gravou o par personagem+carga")
	}
	if sozinhoP != 0 || sozinhaC != 0 {
		t.Errorf("gravações soltas: %d só do personagem, %d só da carga", sozinhoP, sozinhaC)
	}
	personagem, _ := db.lastSavedChar()
	carga, _ := db.lastSavedCargo()
	if !hasItem(personagem.Carry, indice) {
		t.Errorf("o item sacado não está na mochila gravada: %+v", personagem.Carry)
	}
	if hasItem(carga.Items, indice) {
		t.Errorf("o item sacado continua na carga gravada — é a cópia a mais: %+v", carga.Items)
	}
}

// esperaPar espera a gravação da saída chegar. A desconexão grava fora do laço,
// numa goroutine, então não há mensagem para aguardar: sobra esperar o efeito,
// com prazo, em vez de dormir um tanto e torcer.
func esperaPar(t *testing.T, db *fakeDB) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for time.Now().Before(limite) {
		if pares, sozinhoP, sozinhaC := contagemDeSaves(db); pares+sozinhoP+sozinhaC > 0 {
			// Uma pausa curta depois da primeira gravação: se houver uma segunda,
			// solta, é ela que o teste precisa ver.
			time.Sleep(150 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a saída não gravou nada em 3 segundos")
}

// TestCriarGuildaComOuroDaCargaFechaAConta: o cenário que juntou os dois defeitos
// do dia. A pessoa saca o ouro da carga e cria a guilda; o /create grava o
// personagem antes de cobrar, e essa gravação tem de levar a carga junto.
//
// A conta que tem de fechar depois de tudo: o que ficou no personagem, mais o que
// ficou na carga, mais o custo da guilda, é o que havia no começo. Sobrando,
// duplicou; faltando, sumiu.
func TestCriarGuildaComOuroDaCargaFechaAConta(t *testing.T) {
	const sobra = int32(777)
	total := guildCreateCost + sobra

	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Hero", X: 5, Y: 5, HP: 1000, MaxHP: 1000,
		Clan: 7, Citizen: 1, Coin: 0,
	}
	db.accounts["tester"].cargo = world.CargoState{Coin: total}
	db.guildaConfereOuroGravado = true

	addr, stop, _ := startServerClock(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	withdrawFrame(t, c, total)
	expect(t, c, protocol.MsgWithdraw)
	drainRaw(t, c)

	whisperFrame(t, c, "create", "Lobos")
	linha := ""
	for i := 0; i < 8 && linha == ""; i++ {
		ty, p, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgMessagePanel {
			linha = decodePanel(p)
		}
	}
	if !criouGuilda(db, "Lobos") {
		t.Fatalf("a guilda não foi criada; resposta = %q", linha)
	}
	esperaPar(t, db)

	pares, sozinhoP, sozinhaC := contagemDeSaves(db)
	if pares == 0 {
		t.Fatal("o /create gravou sem levar a carga junto")
	}
	if sozinhoP != 0 || sozinhaC != 0 {
		t.Errorf("gravações soltas no caminho da guilda: %d só do personagem, %d só da carga",
			sozinhoP, sozinhaC)
	}
	personagem, _ := db.lastSavedChar()
	carga, _ := db.lastSavedCargo()
	if soma := personagem.Coin + carga.Coin + guildCreateCost; soma != total {
		t.Errorf("a conta não fecha: personagem %d + carga %d + custo %d = %d, queria %d",
			personagem.Coin, carga.Coin, guildCreateCost, soma, total)
	}
}
