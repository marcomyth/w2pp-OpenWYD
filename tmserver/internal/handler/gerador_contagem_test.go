package handler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TestRotaTipo3DescontaOGeradorEVoltaANascer: um monstro de rota tipo 3 que chega
// ao fim da rota sai com DeleteMob tipo 3, e o legado desconta a contagem do
// bloco em todo tipo diferente de 0 (Server.cpp:7821-7833). Aqui só o tipo 1
// descontava: o bloco ficava com a contagem no teto e o relógio de 12 s nunca
// mais o repunha. É o que esvaziava a Pista +2 (blocos 5790-5848, MaxNumMob 1)
// bloco a bloco até o próximo reinício.
//
// O mundo não está servindo, então não há laço rodando: o teste é o único dono do
// estado, como nos outros testes de rota deste arquivo de pacote.
func TestRotaTipo3DescontaOGeradorEVoltaANascer(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 32}, log, nil, d.Handle)
	gen := &world.Generator{
		MinuteGenerate: 1, MinGroup: 0, MaxGroup: 0, MaxNumMob: 1,
		RouteType:  3,
		SegX:       [5]int16{5, 0, 0, 0, 7},
		SegY:       [5]int16{5, 0, 0, 0, 5},
		LeaderTmpl: aggressiveMob(),
	}
	w.RegisterGenerators([]*world.Generator{gen})
	ids := w.GenerateMob(0)
	if len(ids) != 1 || gen.CurrentNumMob != 1 {
		t.Fatalf("preparo: nasceram %d, contagem %d; queria 1 e 1", len(ids), gen.CurrentNumMob)
	}

	id := ids[0]
	for i := 0; i < 10; i++ {
		e := w.Entity(id)
		if e == nil {
			break
		}
		d.mobRoam(w, id, e)
	}
	if e := w.Entity(id); e != nil {
		t.Fatalf("o monstro de rota 3 não saiu no fim da rota (está em %d,%d)", e.X, e.Y)
	}
	if gen.CurrentNumMob != 0 {
		t.Fatalf("contagem do bloco = %d depois de o monstro sair pelo fim da rota, queria 0: o bloco fica no teto e nunca mais nasce", gen.CurrentNumMob)
	}

	// Um tique antes da passagem de 12 s o relógio não mexe no bloco.
	d.tickCount = minTimerTicks - 1
	d.generateMobs(w)
	if gen.CurrentNumMob != 0 {
		t.Fatalf("o bloco nasceu fora da passagem do relógio (contagem %d)", gen.CurrentNumMob)
	}
	// Na passagem, o bloco volta a nascer.
	d.tickCount = minTimerTicks
	d.generateMobs(w)
	if gen.CurrentNumMob != 1 {
		t.Fatalf("contagem depois da passagem de 12 s = %d, queria 1: o bloco não voltou a nascer", gen.CurrentNumMob)
	}
}
