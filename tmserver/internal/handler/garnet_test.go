package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/combatrule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func TestEquipGarnet(t *testing.T) {
	// Grade 8 pays 80 per step, everything else 40 (CMob.cpp:873) — Grade 8, not
	// the Grade 6 of the Esmeralda.
	d := &Dispatcher{itemGrades: map[int]int{1000: 0, 2000: 6, 3000: 8}}
	cases := []struct {
		name  string
		equip map[int]world.Item
		want  int32
	}{
		{"sem garnet", map[int]world.Item{0: gemItem(1000, 15, 1)}, 0},
		{"garnet abaixo de +10", map[int]world.Item{0: gemItem(1000, 9, 3)}, 0},
		{"garnet +10 comum", map[int]world.Item{0: gemItem(1000, 10, 3)}, 40},
		{"garnet +15 comum", map[int]world.Item{0: gemItem(1000, 15, 3)}, 240},
		{"garnet +15 grade 6 não dobra", map[int]world.Item{0: gemItem(2000, 15, 3)}, 240},
		{"garnet +15 grade 8", map[int]world.Item{0: gemItem(3000, 15, 3)}, 480},
		{"soma as peças, acessório incluído", map[int]world.Item{
			1: gemItem(1000, 15, 3), 8: gemItem(1000, 15, 3), 11: gemItem(1000, 12, 3),
		}, 240 + 240 + 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &world.Entity{}
			for slot, it := range tc.equip {
				e.Equip[slot] = it
			}
			if got := d.equipGarnet(e); got != tc.want {
				t.Errorf("equipGarnet = %d, quero %d", got, tc.want)
			}
		})
	}
}

// TestAbsorverGarnet é a regra A2 decidida em 17/09: anula a Esmeralda de quem
// bate, e o resto da Garnet tira no máximo GarnetPct% do resto do golpe.
func TestAbsorverGarnet(t *testing.T) {
	const full = 2640 // 11 peças +15
	jogador := func(garnet, esmeralda int32) *world.Entity {
		return &world.Entity{ID: 1, EquipGarnet: garnet, EquipForceDamage: esmeralda}
	}
	monstro := &world.Entity{ID: world.MaxUser + 1}
	cases := []struct {
		name      string
		pct       int32
		atk, alvo *world.Entity
		dmg, want int
	}{
		{"sem esmeralda: tira só 20%", 20, jogador(0, 0), jogador(full, 0), 800, 640},
		{"full contra full: a garnet inteira vai na esmeralda, sobra a luta sem joia", 20, jogador(0, full), jogador(full, 0), 800 + full, 800},
		{"garnet menor que a esmeralda: anula só o que tem", 20, jogador(0, full), jogador(960, 0), 800 + full, 800 + full - 960},
		{"sobra pouca garnet: o teto é o que sobra", 20, jogador(0, 900), jogador(960, 0), 5000, 5000 - 960},
		{"monstro: só o percentual, até o total", 30, monstro, jogador(full, 0), 9300, 9300 - full},
		{"monstro com golpe menor", 20, monstro, jogador(full, 0), 3000, 2400},
		{"100% é o legado: golpe pequeno vira 1", 100, jogador(0, 0), jogador(full, 0), 800, 1},
		{"0% só anula a esmeralda", 0, jogador(0, 480), jogador(full, 0), 1280, 800},
		{"0% sem esmeralda não tira nada", 0, monstro, jogador(full, 0), 800, 800},
		{"nunca abaixo de 1", 100, jogador(0, full), jogador(full, 0), full, 1},
		{"alvo sem garnet", 20, jogador(0, full), jogador(0, 0), 3000, 3000},
		{"alvo monstro não usa garnet", 20, jogador(0, 0), &world.Entity{ID: world.MaxUser + 2, EquipGarnet: full}, 800, 800},
		{"erro continua erro", 20, monstro, jogador(full, 0), 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &Dispatcher{combatRules: combatrule.Default()}
			d.combatRules.GarnetPct = tc.pct
			if got := d.absorverGarnet(tc.atk, tc.alvo, tc.dmg); got != tc.want {
				t.Errorf("absorverGarnet(%d) = %d, quero %d", tc.dmg, got, tc.want)
			}
		})
	}
}

// TestGolpeDeMonstroPassaPelaGarnet: o golpe de monstro de verdade, com o mesmo
// sorteio nos dois mundos, sai menor exatamente pela regra da Garnet.
func TestGolpeDeMonstroPassaPelaGarnet(t *testing.T) {
	golpe := func(garnet int32) int {
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		d := New(Config{Log: log})
		w := world.New(world.Config{GridDim: 64}, log, nil, d.Handle)
		mob := &world.Entity{ID: world.MaxUser + 7, Damage: 6000}
		alvo := &world.Entity{ID: 3, HP: 100000, EquipGarnet: garnet}
		return d.danoDoGolpeDeMonstro(w, mob, alvo)
	}
	sem, com := golpe(0), golpe(2640)
	if sem <= 1 {
		t.Fatalf("o golpe sem garnet saiu %d: o teste não mede nada", sem)
	}
	quer := sem - min(2640, sem*int(combatrule.Default().GarnetPct)/100)
	if com != quer {
		t.Errorf("golpe com garnet = %d, quero %d (sem garnet: %d)", com, quer, sem)
	}
}

// TestGolpePvPPassaPelaGarnet passa pelo servidor inteiro: um golpe físico de
// jogador num alvo com uma Garnet +15 (240). Cada servidor começa com o mesmo
// sorteio, então o golpe é o mesmo nos três: com a Garnet em 0% e sem Esmeralda
// ela não tira nada; com 20% tira um quinto, até 240; com 100% (o legado), 240.
func TestGolpePvPPassaPelaGarnet(t *testing.T) {
	golpe := func(pct int32) int32 {
		db := newDB()
		db.loadResult = world.CharacterState{
			Slot: 0, Name: "Hero", X: 5, Y: 5,
			HP: 100000, MaxHP: 100000, Level: 200, Str: 3000, AC: 0,
		}
		db.loadResult.Equip[1] = gemItem(1000, 15, 3)
		regra := *regraSemEscala()
		regra.GarnetPct = pct
		addr, stop := startServerClockRegra(t, db, regra)
		defer stop()
		atacante := enterWorld(t, addr)
		defer atacante.Close()
		alvo := enterWorld(t, addr)
		defer alvo.Close()

		send(t, atacante, protocol.MsgPKMode, protocol.EncodeStandardParm(1))
		attackFrame(t, atacante, serverTime, 2, 0)
		ty, payload, ok := readMaybe(t, alvo)
		if !ok || ty != protocol.MsgAttack {
			t.Fatalf("alvo recebeu %#x ok=%v, want o MsgAttack", ty, ok)
		}
		var got protocol.MsgAttackBody
		if err := got.Decode(payload); err != nil {
			t.Fatal(err)
		}
		if len(got.Dam) != 1 || got.Dam[0].TargetID != 2 {
			t.Fatalf("Dam = %+v", got.Dam)
		}
		return got.Dam[0].Damage
	}
	controle := golpe(0)
	if controle <= 300 {
		t.Fatalf("golpe com a Garnet em 0%% = %d: pequeno demais para medir a Garnet", controle)
	}
	if d, quer := golpe(20), controle-min(240, controle*20/100); d != quer {
		t.Errorf("golpe com a Garnet em 20%% = %d, quero %d (sem ela: %d)", d, quer, controle)
	}
	if d, quer := golpe(100), controle-240; d != quer {
		t.Errorf("golpe com a Garnet em 100%% = %d, quero %d (sem ela: %d)", d, quer, controle)
	}
}

func startServerClockRegra(t *testing.T, persist world.Persistence, regra combatrule.Rules) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) }, CombatRules: &regra})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, persist, d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	}
}
