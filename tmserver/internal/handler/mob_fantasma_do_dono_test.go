package handler

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O BateNeles (27/09, Submundo) via um Morlock de vida zerada que só ele via, em
// que batia por horas sem dano nem XP, e que só sumia com teleporte ou relog. A
// coleira do pet (summonLeash) é maior que o alcance de vista: o BM anda, o
// monstro sai da vista dele (RemoveMob) e os tigres continuam a luta atrás. O
// cliente guarda a entidade depois do RemoveMob; um quadro que volte a nomear
// aquele id a desenha de novo, e quando o monstro morre o RemoveMob vai só a
// quem está em vista do corpo — o dono não está, e o fantasma fica.
//
// Os testes abaixo prendem a regra do lado do servidor: depois do RemoveMob,
// nada que nomeie o monstro chega ao dono enquanto ele continua fora de vista.

// donoLongeDoMonstro monta o BM em (5,5) com um Condor evocado, um monstro de 10
// de vida em (8,5), e o leva a (25,5), a 17 casas do monstro, conferindo o
// RemoveMob. Devolve o mundo, a conexão, o id do monstro e o do pet.
func donoLongeDoMonstro(t *testing.T) (*Dispatcher, *world.World, net.Conn, int, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Spells: evokeSpell(), SummonMobs: [][]byte{summonTemplate("Condor")}})
	w := world.New(world.Config{GridDim: 64, Now: clock.Load}, log, summonDB(60), d.Handle)

	alvo := plainMobTemplate("Morlock")
	binary.LittleEndian.PutUint64(alvo[32:], 2000)  // STRUCT_MOB.Exp
	binary.LittleEndian.PutUint32(alvo[92+0:], 50)  // Level
	binary.LittleEndian.PutUint32(alvo[92+16:], 10) // MaxHp
	binary.LittleEndian.PutUint32(alvo[92+24:], 10) // Hp
	mobID := w.SpawnMobAt(world.MobSpawn{Template: alvo, X: 8, Y: 5, GenIndex: -1})
	if mobID < 0 {
		t.Fatal("não consegui criar o monstro")
	}
	w.SetTickHandler(time.Hour, d.Tick) // sem IA: a luta é conduzida pelo teste
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	c := enterWorld(t, ln.Addr().String()) // o BM, em (5,5); o monstro está a 3 casas
	t.Cleanup(func() { c.Close() })

	skillAttackFrame(t, c, serverTime, 1, 56, damSkill) // evoca o Condor
	pets := collectPets(t, c, 400*time.Millisecond)
	if len(pets) == 0 {
		t.Fatal("o Condor não nasceu")
	}
	var pet int
	for id := range pets {
		pet = id
	}
	noLaco(t, w, func(w *world.World) {
		w.ForEachPlaying(-1, func(s *world.Session, _ *world.Entity) {
			if !w.Seen(s, mobID) {
				t.Error("o dono não recebeu o monstro ao entrar; o teste não parte do estado do jogo")
			}
		})
	})

	actionFrameXY(t, c, protocol.MsgAction, serverTime, 5, 5, 25, 5)
	for {
		h, _, ok := readMaybeHeaderRaw(t, c)
		if !ok {
			t.Fatal("o dono se afastou e não recebeu o RemoveMob do monstro")
		}
		if h.Type == protocol.MsgRemoveMob && int(h.ID) == mobID {
			return d, w, c, mobID, pet
		}
	}
}

// exigeSilencioSobre lê o que resta para o dono e reprova todo quadro que nomeie id.
func exigeSilencioSobre(t *testing.T, c net.Conn, id int, oQue string) {
	t.Helper()
	for {
		h, payload, ok := readMaybeHeaderRaw(t, c)
		if !ok {
			return
		}
		for _, n := range nomeia(h, payload) {
			if n == id {
				t.Errorf("%s: depois do RemoveMob o dono recebeu %#x com o id %d do monstro fora de vista; "+
					"é o que o cliente desenha como mob fantasma", oQue, h.Type, id)
			}
		}
	}
}

// TestMorteLongeDoDonoNaoRessuscitaOMob: o pet mata o monstro que o dono já não
// vê. A confirmação do abate vai só a quem vê o corpo, como o GridMulticast do
// legado (MobKilled.cpp:346, 1435, 3129).
func TestMorteLongeDoDonoNaoRessuscitaOMob(t *testing.T) {
	d, w, c, mobID, pet := donoLongeDoMonstro(t)
	noLaco(t, w, func(w *world.World) {
		m := w.Entity(mobID)
		m.HP = 0
		d.mobKilled(w, w.Entity(pet), m)
		if w.Entity(mobID) != nil {
			t.Error("o monstro não saiu do mundo")
		}
	})
	exigeSilencioSobre(t, c, mobID, "abate")
}

// TestGolpeDoPetNaoRessuscitaOMob: o pet, ainda em vista do dono, bate no
// monstro que já saiu da vista dele. O golpe vai a quem vê o pet, e leva o id do
// alvo em Dam[].
func TestGolpeDoPetNaoRessuscitaOMob(t *testing.T) {
	d, w, c, mobID, pet := donoLongeDoMonstro(t)
	noLaco(t, w, func(w *world.World) {
		w.SetEntityPos(pet, 9, 5) // a 16 casas do dono, colado no monstro
		p, m := w.Entity(pet), w.Entity(mobID)
		m.HP, m.MaxHP = 1_000_000, 1_000_000 // o golpe não mata: só o quadro do golpe está em prova
		p.AtkTick = 1                        // golpe anterior há muito: sem o escalonamento do primeiro
		d.mobAttack(w, pet, p, m)
		if m.HP == m.MaxHP {
			t.Error("o pet não golpeou; o teste não exercitou o quadro do golpe")
		}
	})
	exigeSilencioSobre(t, c, mobID, "golpe do pet")
}

// nomeia devolve os ids de entidade de que o quadro fala.
func nomeia(h protocol.Header, payload []byte) []int {
	var ids []int
	switch h.Type {
	case protocol.MsgCNFMobKill, protocol.MsgCreateMob, protocol.MsgCreateMobTrade:
		if len(payload) >= 6 {
			ids = append(ids, int(binary.LittleEndian.Uint16(payload[4:6])))
		}
	case protocol.MsgAttack, protocol.MsgAttackOne, protocol.MsgAttackTwo:
		var b protocol.MsgAttackBody
		if b.Decode(payload) == nil {
			for _, dam := range b.Dam {
				ids = append(ids, int(dam.TargetID))
			}
		}
	}
	if int(h.ID) >= world.MaxUser && h.ID != protocol.IDScene {
		ids = append(ids, int(h.ID))
	}
	return ids
}
