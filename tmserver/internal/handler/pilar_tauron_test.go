package handler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// grupoDeTauron é um grupo de três Tauron sem período de minuto em (x, y), com a
// Exp que o painel deu ao molde — acima de 1 milhão, o que o poria na regra dos
// chefes sozinhos.
func grupoDeTauron(nome string, x, y int16) *world.Generator {
	return &world.Generator{
		MinuteGenerate: -1, MaxNumMob: 3, LeaderName: nome,
		LeaderTmpl:   moldeDeMonstro(nome, 1_200_000, 0),
		FollowerTmpl: moldeDeMonstro(nome, 1_200_000, 0),
		SegX:         [5]int16{x}, SegY: [5]int16{y},
	}
}

// Os Tauron do Pilar voltam em no máximo 10 s; o mesmo Tauron fora do Pilar, e
// outro monstro dentro dele, seguem a regra de sempre.
func TestTauronDoPilarVoltaEmDezSegundos(t *testing.T) {
	gens := make([]*world.Generator, world.KefraGuardLast+1)
	gens[30] = grupoDeTauron("Tauron", 1181, 1727) // Pilar 1
	gens[31] = grupoDeTauron("Tauron", 1196, 1730) // o grupo ao lado do Pilar 3
	gens[32] = grupoDeTauron("Tauron", 1210, 1702) // Tauron fora da caixa
	gens[33] = grupoDeTauron("Aranha_Inferno", 1185, 1720)
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators(gens)
	d := dispatcherQuieto()
	d.InstallRespawnDelay(w)

	for _, idx := range []int{30, 31} {
		if got := d.esperaDoRenascimento(w, idx); got > pilarRenasce {
			t.Errorf("Tauron do Pilar (bloco %d) espera %d ms, quer no máximo %d", idx, got, pilarRenasce)
		}
		if d.chefeSozinho(w, idx) {
			t.Errorf("Tauron do Pilar (bloco %d) entrou na lista de chefes", idx)
		}
	}
	// Fora do Pilar a regra dos chefes continua valendo: é o que ela é.
	if got := d.esperaDoRenascimento(w, 32); got != 24*msPorHora {
		t.Errorf("Tauron fora do Pilar espera %d ms, quer as 24 h dos chefes", got)
	}
	if geradorDoPilar(w, 33) {
		t.Error("outro monstro na caixa do Pilar foi tomado por Tauron do Pilar")
	}
}

// Os Anciões Ciclops voltam em no máximo 10 s, mesmo com a Exp do painel acima de
// 1 milhão; o grupo que o Ciclope Cruel lidera com um Ancião de acompanhante segue
// a regra de sempre.
func TestAnciaoCiclopsVoltaEmDezSegundos(t *testing.T) {
	gens := make([]*world.Generator, world.KefraGuardLast+1)
	gens[30] = grupoDeTauron("Anciao_Ciclops", 1914, 1718)
	gens[30].MaxNumMob = 1
	gens[31] = grupoDeTauron("Anciao_Ciclops_", 2248, 1390)
	gens[31].MaxNumMob = 1
	gens[32] = &world.Generator{ // Ancião com Orc Médico
		MinuteGenerate: -1, MaxNumMob: 2, LeaderName: "Anciao_Ciclops",
		LeaderTmpl:   moldeDeMonstro("Anciao_Ciclops", 1_200_000, 0),
		FollowerTmpl: moldeDeMonstro("Orc_Medico", 25_308, 0),
	}
	gens[33] = &world.Generator{ // Ciclope Cruel lidera, Ancião acompanha
		MinuteGenerate: -1, MaxNumMob: 2, LeaderName: "Ciclope_Cruel",
		LeaderTmpl:   moldeDeMonstro("Ciclope_Cruel", 700, 0),
		FollowerTmpl: moldeDeMonstro("Anciao_Ciclops", 1_200_000, 0),
	}
	w := world.New(world.Config{GridDim: 64}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.NopPersistence{}, nil)
	w.RegisterGenerators(gens)
	d := dispatcherQuieto()
	d.InstallRespawnDelay(w)

	for _, idx := range []int{30, 31, 32} {
		if got := d.esperaDoRenascimento(w, idx); got > anciaoRenasce {
			t.Errorf("Ancião (bloco %d) espera %d ms, quer no máximo %d", idx, got, anciaoRenasce)
		}
		if d.chefeSozinho(w, idx) {
			t.Errorf("Ancião (bloco %d) entrou na lista de chefes", idx)
		}
	}
	if geradorDoAnciao(w, 33) {
		t.Error("grupo do Ciclope Cruel foi tomado por bloco do Ancião")
	}
}
