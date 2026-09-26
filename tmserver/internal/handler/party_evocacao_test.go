package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O BANDO NÃO É GRUPO. Decisão do Marco em 26/09/2026: "evocações não devem
// entrar em grupo, mas quando o BM evoca ela simula um grupo". Os pets moram no
// bando do dono (world.Entity.Evocacoes), não na PartyList de ninguém. Estes
// testes prendem as quatro consequências que o jogador vê:
//
//   - o BM com o bando em campo é convidado e entra em grupo (antes: "já tem grupo");
//   - o BM com o bando em campo lidera um grupo, e o painel abre direito;
//   - o bando não ocupa vaga de jogador e não sai quando o BM entra ou sai do grupo;
//   - o bando sai quando o dono sai do jogo.

// startServerGrupo é o startServerClock dos testes de grupo com o mundo exposto
// e o tique ligado, sem o qual o noLaco nunca roda.
func startServerGrupo(t *testing.T) (string, *world.World) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: world.DefaultGridDim, Now: clock.Load}, log, partyDB(), d.Handle)
	w.SetTickHandler(10*time.Millisecond, d.Tick)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return ln.Addr().String(), w
}

// evocar põe no bando de dono um pet como o generateSummon deixa (Summoner e
// Leader no dono, a vaga em Evocacoes), ao lado dele.
func evocar(t *testing.T, w *world.World, dono int) int {
	t.Helper()
	petID := -1
	noLaco(t, w, func(w *world.World) {
		de := w.Entity(dono)
		slot := vagaNoBando(de)
		if slot < 0 {
			return
		}
		petID = w.SpawnMobAt(world.MobSpawn{Template: summonTemplate("Tigre"), X: de.X + 1, Y: de.Y, RouteType: 5, GenIndex: -1})
		if petID < 0 {
			return
		}
		pet := w.Entity(petID)
		pet.Clan = summonClan
		pet.Summoner = dono
		pet.Leader = dono
		pet.NonCombatNPC = false
		de.Evocacoes[slot] = petID
	})
	if petID < 0 {
		t.Fatal("não consegui evocar")
	}
	return petID
}

// ateAddParty lê c até a próxima linha de grupo, pulando o que o mundo manda
// no meio (o pet anda, aparece, some).
func ateAddParty(t *testing.T, c net.Conn) protocol.MsgCNFAddPartyBody {
	t.Helper()
	p, _ := readUntil(t, c, protocol.MsgCNFAddParty)
	return protocol.MsgCNFAddPartyBody{LeaderConn: protocolLe16(p[0:2]), PartyID: protocolLe16(p[8:10])}
}

// noBando diz se petID está no bando de dono.
func noBando(dono *world.Entity, petID int) bool {
	for _, id := range dono.Evocacoes {
		if id == petID {
			return true
		}
	}
	return false
}

// Relatado em jogo: "se o BM evoca, não conseguimos grupar, pois diz que ele já
// tem grupo". O BM (conn 2) é convidado com o bando em campo e entra; o bando
// fica com ele e não ocupa vaga no grupo.
func TestBMComEvocacaoEntraNoGrupo(t *testing.T) {
	addr, w := startServerGrupo(t)
	a := enterWorldAs(t, addr, "tester") // conn 1, o líder
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb") // conn 2, o BM
	defer b.Close()
	petID := evocar(t, w, 2)

	reqPartyFrame(t, a, 1, 2)
	readUntil(t, b, protocol.MsgSendReqParty) // sem ele, o convite foi recusado como "já tem grupo"
	acceptPartyFrame(t, b, 1, "Hero")
	if slot := ateAddParty(t, a); slot.LeaderConn != 1 {
		t.Fatalf("o líder não recebeu a própria linha do painel: %+v", slot)
	}

	noLaco(t, w, func(w *world.World) {
		lider, bm, pet := w.Entity(1), w.Entity(2), w.Entity(petID)
		if bm.Leader != 1 || !hasMember(lider, 2) {
			t.Errorf("o BM não entrou no grupo: Leader=%d", bm.Leader)
		}
		if pet == nil || !noBando(bm, petID) || pet.Leader != 2 {
			t.Error("o bando tinha de continuar com o BM, e com ele de líder")
		}
		if hasMember(lider, petID) || partyMemberCount(lider) != 1 {
			t.Errorf("o pet ocupou vaga no grupo: %v", lider.PartyList)
		}
	})
}

// O outro lado: o BM com o bando em campo é o LÍDER, e o primeiro jogador entra.
// O pet não pode ocupar vaga (a contagem por vaga dava 2 e o líder ficava sem a
// própria linha do painel) nem aparecer no painel do membro.
func TestBMComEvocacaoLideraOGrupo(t *testing.T) {
	addr, w := startServerGrupo(t)
	a := enterWorldAs(t, addr, "tester") // conn 1, o BM líder
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb") // conn 2
	defer b.Close()
	petID := evocar(t, w, 1)

	reqPartyFrame(t, a, 1, 2)
	readUntil(t, b, protocol.MsgSendReqParty)
	acceptPartyFrame(t, b, 1, "Hero")
	if slot := ateAddParty(t, a); slot.LeaderConn != 1 || slot.PartyID != 1 {
		t.Fatalf("o líder com pets não recebeu a própria linha do painel: %+v", slot)
	}
	for {
		ty, p, ok := readMaybe(t, b)
		if !ok {
			break
		}
		if ty == protocol.MsgCNFAddParty && int(protocolLe16(p[8:10])) == petID {
			t.Errorf("o membro recebeu a linha do pet %d no painel de grupo", petID)
		}
	}
	noLaco(t, w, func(w *world.World) {
		if lider := w.Entity(1); hasMember(lider, petID) || partyMemberCount(lider) != 1 {
			t.Errorf("o pet ocupou vaga no grupo: %v", lider.PartyList)
		}
	})
}

// Sair do grupo não mexe no bando (no legado os pets de um membro moravam na
// lista do líder e saíam com o grupo, #234); sair do JOGO, sim.
func TestBandoFicaNoGrupoESaiComODono(t *testing.T) {
	addr, w := startServerGrupo(t)
	a := enterWorldAs(t, addr, "tester") // conn 1, o líder
	defer a.Close()
	b := enterWorldAs(t, addr, "tradeb") // conn 2, o BM
	petID := evocar(t, w, 2)

	reqPartyFrame(t, a, 1, 2)
	readUntil(t, b, protocol.MsgSendReqParty)
	acceptPartyFrame(t, b, 1, "Hero")
	ateAddParty(t, a)
	removePartyFrame(t, b, 2) // o BM sai do grupo
	readUntil(t, a, protocol.MsgRemoveParty)

	noLaco(t, w, func(w *world.World) {
		bm, pet := w.Entity(2), w.Entity(petID)
		if bm.Leader != 0 {
			t.Fatalf("o BM não saiu do grupo: Leader=%d", bm.Leader)
		}
		if pet == nil || pet.Summoner != 2 || !noBando(bm, petID) {
			t.Error("sair do grupo levou o bando junto")
		}
	})

	b.Close() // o dono sai do jogo
	for fim := time.Now().Add(2 * time.Second); ; {
		var vivo bool
		noLaco(t, w, func(w *world.World) {
			pet := w.Entity(petID)
			vivo = pet != nil && pet.Mode != world.MobEmpty && pet.Summoner == 2
		})
		if !vivo {
			return
		}
		if time.Now().After(fim) {
			t.Fatal("o bando continuou no mundo depois que o dono saiu do jogo")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Pet que morre libera a vaga no bando (world.DespawnMob). Sem isso as vagas
// ficam com ids velhos, e depois de doze mortes o BM não evoca mais nada. O
// "dono" é um mob porque o mundo não deixa um teste fabricar jogador, e
// DespawnMob só lê o bando de quem está no Summoner.
func TestPetQueMorreLiberaAVagaNoBando(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 32}, log, nil, d.Handle)
	donoID := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Dono"), X: 5, Y: 5, GenIndex: -1})
	petID := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Tigre"), X: 6, Y: 5, GenIndex: -1})
	if donoID < 0 || petID < 0 {
		t.Fatal("não consegui criar dono e pet")
	}
	dono, pet := w.Entity(donoID), w.Entity(petID)
	pet.Clan = summonClan
	pet.Summoner = donoID
	pet.Leader = donoID
	dono.Evocacoes[0] = petID

	w.DespawnMob(petID, 1) // morto por um monstro

	if dono.Evocacoes[0] != 0 {
		t.Errorf("a vaga do pet morto continuou ocupada: %v", dono.Evocacoes)
	}
}
