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

// Os dois lados do revide, conectados de verdade, fora de cidade e em modo PK,
// com um Condor evocado pelo dono (conn 1). A vítima é a conn 2.
const donoID, vitimaID = 1, 2

func subirRevide(t *testing.T) (*Dispatcher, *world.World, int, map[int]net.Conn) {
	t.Helper()
	const condor = 0
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Beast", Class: 2, X: 40, Y: 40,
		HP: 5000, MaxHP: 5000, MP: 30_000, MaxMP: 30_000, Level: 399, Int: 100,
		Damage: 500, LearnedSkill: 1 << 8, BaseSpecial: [4]int16{0, 0, 320, 0},
	}
	// A segunda conta é uma FM com a Cura: é ela quem cura o BM em jogo.
	db.loads = map[int64]world.CharacterState{11: {
		Slot: 0, Name: "Branca", Class: 1, X: 40, Y: 40,
		HP: 5000, MaxHP: 5000, MP: 30_000, MaxMP: 30_000, Level: 399,
		LearnedSkill: 1 << (skillCura % 24), BaseSpecial: [4]int16{0, 255, 0, 0},
	}}
	spells := content.NewSkillData([]content.Spell{
		{Index: 56, ManaSpent: 10, InstanceType: 11, InstanceValue: 1, MaxTarget: 1, Name: "Evocar Condor"},
		{Index: skillCura, ManaSpent: 10, TargetType: 1, Range: 10, InstanceType: 6, InstanceValue: 100, MaxTarget: 1, Name: "Cura"},
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
	t.Cleanup(func() { cancel(); <-done })

	// Duas contas DIFERENTES: o fakeDB só conhece estas, e enterWorld sozinho
	// inventaria uma "tester2" que não existe.
	dono := enterWorldAs(t, ln.Addr().String(), "tester")
	t.Cleanup(func() { dono.Close() })
	vitima := enterWorldAs(t, ln.Addr().String(), "tradeb")
	t.Cleanup(func() { vitima.Close() })
	// Os dois sockets seguem recebendo o mundo inteiro; sem drenar, a fila enche,
	// o laço derruba a sessão e o teste passa sem provar nada
	// (teste-de-socket-e-corrida).
	esvaziar(dono, vitima)

	// O bando nasce com o dono, e os dois jogadores ficam longe de cidade.
	// As conexões começam em 1 (o índice 0 do pMob não é entregue a ninguém).
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
	return d, w, petID, map[int]net.Conn{donoID: dono, vitimaID: vitima}
}

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
	d, w, petID, _ := subirRevide(t)

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

// O BANDO NÃO VIRA CONTRA O GRUPO. Mesma montagem, mas a vítima está no grupo
// do dono: o revide não a põe na lista de nenhum pet, e se ela já estiver lá
// (entrou antes do grupo) a evocação continua sem enxergá-la. Relatado em jogo:
// os tigres do BM batendo num companheiro dentro da masmorra.
func TestEvocacaoNaoAtacaCompanheiroDeGrupo(t *testing.T) {
	d, w, petID, _ := subirRevide(t)
	noLaco(t, w, func(w *world.World) {
		dono, vitima := w.Entity(donoID), w.Entity(vitimaID)
		if _, ok := addMember(dono, vitimaID); !ok {
			t.Fatal("o grupo não tinha lugar")
		}
		vitima.Leader = donoID
		d.commandSummons(w, donoID, vitima)
		d.commandSummons(w, vitimaID, dono) // o golpe de volta, do companheiro no dono
	})
	noLaco(t, w, func(w *world.World) {
		pet := w.Entity(petID)
		if naEnemyList(pet, vitimaID) {
			t.Error("o revide pôs o companheiro de grupo na lista de inimigos do pet")
		}
		if naEnemyList(pet, donoID) {
			t.Error("o revide pôs o próprio dono na lista de inimigos do pet")
		}
		addEnemyList(pet, w.Entity(vitimaID))
		if validTarget(w, pet, w.Entity(vitimaID)) {
			t.Error("com o companheiro já na lista, a evocação ainda o mira")
		}
	})
}

// O PET RESPONDE PELO DONO NO GRUPO. O bando mora fora do grupo e o Leader do
// pet é o próprio BM; quando o BM é MEMBRO do grupo de outro, o grupo do pet
// tem de vir do dono, ou a magia agressiva do líder (applyCastAffect, a Força
// Elemental, o tique das áreas) passa a tratar o pet do companheiro como
// inimigo.
func TestPetDeMembroEhDoGrupoDoLider(t *testing.T) {
	_, w, petID, _ := subirRevide(t)
	noLaco(t, w, func(w *world.World) {
		lider, bm, pet := w.Entity(vitimaID), w.Entity(donoID), w.Entity(petID)
		if skillSameLeaderOrGuild(w, lider, pet) {
			t.Fatal("sem grupo nenhum o pet já contava como aliado: o teste não prova nada")
		}
		if _, ok := addMember(lider, donoID); !ok {
			t.Fatal("o grupo não tinha lugar")
		}
		bm.Leader = vitimaID // o BM entra no grupo do outro
		if !skillSameLeaderOrGuild(w, lider, pet) {
			t.Error("o líder do grupo do BM trata o pet do BM como inimigo")
		}
	})
}

// A GUILDA TAMBÉM, ATÉ BAIXAR A MEDALHA. "Só é permitido se baixar a medalha,
// senão não é permitido" (Marco, 26/09). Sem grupo: dono e vítima só dividem a
// guilda. Com a medalha levantada o bando não a toma por inimiga; com a medalha
// da vítima baixada (guildoff, Session.GuildDisable), toma.
func TestEvocacaoRespeitaAMedalhaDaGuilda(t *testing.T) {
	d, w, petID, _ := subirRevide(t)
	noLaco(t, w, func(w *world.World) {
		w.Entity(donoID).Guild = 5
		w.Entity(vitimaID).Guild = 5
		d.commandSummons(w, donoID, w.Entity(vitimaID))
	})
	noLaco(t, w, func(w *world.World) {
		pet, vitima := w.Entity(petID), w.Entity(vitimaID)
		if naEnemyList(pet, vitimaID) {
			t.Error("o bando tomou por inimigo alguém da guilda do dono, com a medalha levantada")
		}
		addEnemyList(pet, vitima)
		if validTarget(w, pet, vitima) {
			t.Error("com a medalha levantada, a evocação ainda mira o guildado")
		}
		clearEnemyList(pet)
	})
	noLaco(t, w, func(w *world.World) {
		w.Session(vitimaID).GuildDisable = true // a vítima baixa a medalha
		d.commandSummons(w, donoID, w.Entity(vitimaID))
	})
	noLaco(t, w, func(w *world.World) {
		pet, vitima := w.Entity(petID), w.Entity(vitimaID)
		if !naEnemyList(pet, vitimaID) || !validTarget(w, pet, vitima) {
			t.Error("com a medalha baixada, o guildado tinha de virar alvo do revide")
		}
	})
}

// CURAR NÃO É ATACAR. Relatado em jogo: "ao usar o curar, os tigres atacam os
// membros do próprio grupo". O revide de PvP rodava para todo alvo jogador do
// MsgAttack, e a Cura chega pelo mesmo pacote: a FM que curava o BM entrava na
// lista de inimigos do bando dele. O legado sai antes do SetBattle com
// `if (dam <= 0) continue;` (_MSG_Attack.cpp:1292), e a cura é dano negativo.
//
// Sem grupo de propósito: é a cura que não pode chamar o bando, de ninguém.
func TestCuraNaoChamaOBando(t *testing.T) {
	_, w, petID, conns := subirRevide(t)
	const tick = serverTime + 123 // marca este golpe em LastAttackTick
	noLaco(t, w, func(w *world.World) { w.Entity(donoID).HP = 1000 })
	skillAttackFrame(t, conns[vitimaID], tick, donoID, skillCura, damSkill)
	esperarGolpe(t, w, vitimaID, tick)
	noLaco(t, w, func(w *world.World) {
		if hp := w.Entity(donoID).HP; hp <= 1000 {
			t.Fatalf("a Cura não curou (vida %d): o teste não prova nada", hp)
		}
		if pet := w.Entity(petID); naEnemyList(pet, vitimaID) {
			t.Errorf("curar o BM pôs a FM na lista de inimigos do bando dele: %v", pet.EnemyList)
		}
	})
}

// esperarGolpe espera o servidor processar o MsgAttack que conn mandou com
// tick. O handler grava LastAttackTick no começo e roda inteiro numa volta do
// laço, então ver o tick pelo noLaco é ver o golpe terminado.
func esperarGolpe(t *testing.T, w *world.World, conn int, tick uint32) {
	t.Helper()
	for fim := time.Now().Add(2 * time.Second); time.Now().Before(fim); {
		var visto uint32
		noLaco(t, w, func(w *world.World) {
			if s := w.Session(conn); s != nil {
				visto = s.LastAttackTick
			}
		})
		if visto == tick {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("o golpe de %d com tick %d não foi processado", conn, tick)
}
