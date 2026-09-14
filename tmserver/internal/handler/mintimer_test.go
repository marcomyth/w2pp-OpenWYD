package handler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O relógio "de minuto" do legado dispara a cada 12 s (TIMER_MIN,
// Server.cpp:4087), não a cada 60. Quem CONTA voltas dele e usa um minuto de
// parede fica cinco vezes mais lento — o mesmo defeito que os geradores já
// tinham e que TestMinuteGenerateContaPassagensDe12s prende.
//
// Estes testes prendem os três que faltavam, pelo efeito e não pela constante:
// em dez minutos de tiques, quantas vezes cada um roda.

// quandoDispara roda a função a cada tique de 1 s e devolve em que segundos ela
// agiu, olhando a mudança que ela mesma provoca.
func quandoDispara(d *Dispatcher, ate int, agiu func() bool, passo func()) []int {
	var segundos []int
	for tique := 1; tique <= ate; tique++ {
		d.tickCount = tique
		passo()
		if agiu() {
			segundos = append(segundos, tique)
		}
	}
	return segundos
}

// TestSalaDoTronoEsvaziaEmVinteEQuatroSegundos: a contagem 1→2→0 do legado corre
// no ProcessMinTimer, então são duas voltas de 12 s. Em um minuto de parede a
// sala ficava ocupada cinco vezes mais tempo do que o jogo original permite.
func TestSalaDoTronoEsvaziaEmVinteEQuatroSegundos(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 64}, log, nil, d.Handle)

	d.events.kingdom1 = 1
	limpou := 0
	segundos := quandoDispara(d, 120,
		func() bool {
			if d.events.kingdom1 == 0 && limpou == 0 {
				limpou = 1
				return true
			}
			return false
		},
		func() { d.tickKingdomRvR(w) })

	if len(segundos) != 1 || segundos[0] != 24 {
		t.Errorf("a sala do trono esvaziou em %v, queria [24] (duas voltas de 12 s)", segundos)
	}
}

// TestCasteloLimpaEmVinteEQuatroSegundos: mesma contagem de duas voltas, no
// CCastleZakum::ProcessMinTimer.
func TestCasteloLimpaEmVinteEQuatroSegundos(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 64}, log, nil, d.Handle)

	d.events.castle.MarkClear()
	var quando []int
	for tique := 1; tique <= 120; tique++ {
		d.tickCount = tique
		_, _, fase := d.events.castle.State()
		d.tickCastle(w)
		_, _, depois := d.events.castle.State()
		if fase != 0 && depois == 0 {
			quando = append(quando, tique)
		}
	}
	if len(quando) != 1 || quando[0] != 24 {
		t.Errorf("o castelo limpou em %v, queria [24] (duas voltas de 12 s)", quando)
	}
}

// TestTorreEKefraContinuamNoMinutoDeParede: os dois LEEM o relógio de parede em
// vez de contar voltas, então o tique é só cadência de consulta e não entra na
// conta. Trocar os dois para 12 s não mudaria nada além de consultar cinco vezes
// mais — este teste existe para que ninguém "conserte" o que não está quebrado.
func TestTorreEKefraContinuamNoMinutoDeParede(t *testing.T) {
	if minutoTicks != 60 {
		t.Errorf("minutoTicks = %d, queria 60: é um minuto de parede", minutoTicks)
	}
	// as janelas da torre são de minuto (aviso 0-5, abre no 6, acaba no 30), então
	// consultar uma vez por minuto alcança todas
	if 60%minutoTicks != 0 {
		t.Error("a consulta da torre precisa cair em minuto cheio")
	}
}
