package handler

import (
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/spawnrate"
)

// minTimerTicks is one pass of the legacy "minute" timer, in world ticks (our
// tick is 1 s, world/tick.go).
//
// FIDELIDADE AO LEGADO (restaurada): despite its name, TIMER_MIN fires every
// 12000 ms (Server.cpp:4087, next to TIMER_SEC's 500), and ProcessMinTimer
// counts ITS OWN passes (ProcessSecMinTimer.cpp:2523). So NPCGener.txt's
// MinuteGenerate was never minutes: a block with MinuteGenerate 10 refills every
// 10 passes, 120 s (ProcessSecMinTimer.cpp:2723-2733). The rewrite had read the
// name literally and run the generators and the weather on a 60-tick minute,
// which made every timed spawn block refill five times slower than the game it
// ports.
const minTimerTicks = int(spawnrate.MinTimerPass / time.Second)

// minutoTicks e um minuto de parede em tiques do mundo.
//
// A auditoria que este comentario pedia esta FEITA, e a distincao que ela achou
// vale mais que a lista: o que decide nao e o nome do relogio, e o que o codigo
// faz com ele.
//
//   - quem CONTA VOLTAS do relogio do legado tem que usar minTimerTicks, que
//     vale 12 s. Eram tres, e os tres estavam cinco vezes mais lentos: a sala do
//     trono do reino (kingdom.go, a contagem 1-2-0 de ProcessSecMinTimer.cpp:2621),
//     o portao do campo de treino (o "Close Gates" de :2650) e a limpeza do
//     Castelo Zakum (CCastleZakum.cpp:345, chamado de :2619).
//   - quem LE O RELOGIO DE PAREDE pode ficar aqui, porque o tique e so cadencia
//     de consulta e nao entra na conta: a Guerra de Torres compara now.Hour() e
//     now.Minute() (towerwar.go) e o Kefra semanal compara Weekday() e Hour()
//     (kefra.go). Consultar de minuto em minuto pega todas as janelas dos dois.
//
// Ou seja: antes de trocar um pelo outro, pergunte se o trecho CONTA ou CONSULTA.
const minutoTicks = 60
