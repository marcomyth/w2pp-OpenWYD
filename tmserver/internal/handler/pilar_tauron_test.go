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
