package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O REVIDE NO FIO. É o teste que sustenta a árvore de Evocação inteira em PvP,
// porque as duas metades da regra só existem com sessão viva dos dois lados: o
// gate de `SessionMode == UserPlay` em validTarget e a troca de identidade do
// dono em donoDaEvocacao.
//
// O que se prova aqui, com dois jogadores de verdade conectados:
//
//  1. ANTES de qualquer golpe, a evocação NÃO enxerga o outro jogador. É a
//     garantia de que um bando solto não sai caçando gente pelo mapa.
//  2. DEPOIS de o dono bater nele, a evocação passa a enxergá-lo — o revide do
//     legado (_MSG_Attack.cpp:1699).
//  3. Bater na evocação responde pelo DONO para ponto de PK e guerra
//     (_MSG_Attack.cpp:1342).
func TestEvocacaoAtacaJogadorDepoisDoRevideNoFio(t *testing.T) {
	const condor = 0
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Beast", Class: 2, X: 40, Y: 40,
		HP: 5000, MaxHP: 5000, MP: 30_000, MaxMP: 30_000, Level: 399, Int: 100,
		Damage: 500, LearnedSkill: 1 << 8, BaseSpecial: [4]int16{0, 0, 320, 0},
	}
	spells := content.NewSkillData([]content.Spell{
		{Index: 56, ManaSpent: 10, InstanceType: 11, InstanceValue: 1, MaxTarget: 1, Name: "Evocar Condor"},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log, Spells: spells, SummonMobs: [][]byte{summonTemplate("Condor")}, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 64, Now: clock.Load}, log, db, d.Handle)
	// Sem tique o laço fica parado entre pacotes e o GoDetached do noLaco nunca
	// chega a rodar.
	w.SetTickHandler(10*time.Millisecond, d.Tick)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()

	// Duas contas DIFERENTES: o fakeDB só conhece estas, e enterWorld sozinho
	// inventaria uma "tester2" que não existe.
	dono := enterWorldAs(t, ln.Addr().String(), "tester")
	defer dono.Close()
	vitima := enterWorldAs(t, ln.Addr().String(), "tradeb")
	defer vitima.Close()
	// Os dois sockets seguem recebendo o mundo inteiro; sem drenar, a fila enche,
	// o laço derruba a sessão e o teste passa sem provar nada
	// (teste-de-socket-e-corrida).
	esvaziar(dono, vitima)

	// O bando nasce com o dono, e os dois jogadores ficam longe de cidade.
	// As conexões começam em 1 (o índice 0 do pMob não é entregue a ninguém).
	const donoID, vitimaID = 1, 2
	var petID int
	noLaco(t, w, func(w *world.World) {
		for _, conn := range []int{donoID, vitimaID} {
			e := w.Entity(conn)
			if e == nil {
				t.Errorf("conn %d entrou sem entidade", conn)
				return
			}
			e.X, e.Y = int16(40+conn), 40
			e.PKMode = true
		}
		d.generateSummon(w, w.Session(donoID), w.Entity(donoID), condor, 1)
		for _, id := range petsDoLider(w.Entity(donoID)) {
			if pet := w.Entity(id); pet != nil && pet.Summoner == donoID {
				petID = id
			}
		}
	})
	if petID == 0 {
		t.Fatal("a evocação não nasceu")
	}

	// [1] Antes do revide: o bicho não enxerga o outro jogador.
	noLaco(t, w, func(w *world.World) {
		pet, alvo := w.Entity(petID), w.Entity(vitimaID)
		if validTarget(w, pet, alvo) {
			t.Error("sem revide a evocação NÃO pode mirar um jogador")
		}
	})

	// [2] O dono entra na briga: commandSummons leva o bando junto, que é o
	// revide do legado.
	noLaco(t, w, func(w *world.World) {
		d.commandSummons(w, donoID, w.Entity(vitimaID))
	})
	noLaco(t, w, func(w *world.World) {
		pet, alvo := w.Entity(petID), w.Entity(vitimaID)
		if !naEnemyList(pet, vitimaID) {
			t.Fatal("o revide tinha de pôr a vítima na lista de inimigos do bicho")
		}
		if !validTarget(w, pet, alvo) {
			t.Error("depois do revide a evocação tem de enxergar o jogador")
		}
	})

	// [3] Bater no bicho responde pelo dono, e o dono batendo no próprio bicho
	// não vira PvP nenhum.
	noLaco(t, w, func(w *world.World) {
		pet := w.Entity(petID)
		if got := donoDaEvocacao(w, pet); got == nil || got.ID != donoID {
			t.Errorf("o responsável pela evocação tem de ser o dono, veio %v", got)
		}
		if !golpeContaComoPvP(w, vitimaID, petID, pet) {
			t.Error("a vítima batendo na evocação conta como PvP")
		}
		if golpeContaComoPvP(w, donoID, petID, pet) {
			t.Error("o dono batendo na própria evocação NÃO conta como PvP")
		}
	})

	// [4] O dono SAI de jogo (volta ao char-select) sem que a entidade dele deixe
	// o mundo: a evocação para de responder por ele na hora. Sem este gate, um
	// bicho de alguém que não está mais jogando continuaria marcando criminoso
	// quem encostasse nele.
	noLaco(t, w, func(w *world.World) {
		s := w.Session(donoID)
		if s == nil {
			t.Fatal("o dono tinha de ter sessão")
		}
		anterior := s.Mode
		s.Mode = world.UserSelChar
		defer func() { s.Mode = anterior }()

		pet := w.Entity(petID)
		if got := donoDaEvocacao(w, pet); got != pet {
			t.Error("com o dono fora de jogo, a evocação responde por si mesma")
		}
		if golpeContaComoPvP(w, vitimaID, petID, pet) {
			t.Error("evocação de quem não está em jogo não entra na contagem de PvP")
		}
	})
}
