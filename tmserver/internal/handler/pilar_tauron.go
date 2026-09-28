package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/spawnrate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os Tauron do Pilar, perto de Noatun (pedido do Marco em 28/09/2026): cada um
// que morre volta sozinho em no máximo 10 s, como os bichos da sala da lava.
//
// São quatro grupos de três na caixa do Pilar: os blocos "Pilar 1" (3168),
// "Pilar 2" (3169) e "Pilar 3" (3170) do NPCGener.txt e o grupo ao lado (3171).
// Os quatro ficam sem período de minuto no arquivo (MinuteGenerate -1), e é a
// fila de renascimento que os traz de volta, com a espera daqui.
//
// Antes, o Pilar 3 e o 3171 voltavam pelo relógio de minuto, um grupo a cada 24 s;
// o Pilar 1 e o 2 não tinham período, e com a Exp do Tauron editada no painel
// acima de 1 milhão caíam na regra dos chefes sozinhos (chefes.go): morriam uma
// vez e só voltavam em 24 horas.
//
// Reconhecido pelo molde e pelo lugar, e não pelo índice do bloco: o índice anda
// quando o arquivo é editado.
const (
	pilarTemplate = "Tauron"
	// pilarRenasce é o teto da espera de um Tauron do Pilar, em ms.
	pilarRenasce = 10_000
	// A caixa do Pilar: os pontos de partida dos quatro grupos (1180-1196 ×
	// 1714-1730) com uma folga de 5 tiles.
	pilarX1, pilarY1 = 1175, 1709
	pilarX2, pilarY2 = 1201, 1735
)

// geradorDoPilar diz se o bloco idx é um dos grupos de Tauron do Pilar.
func geradorDoPilar(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	if g == nil || droprule.Canonical(g.LeaderName) != droprule.Canonical(pilarTemplate) {
		return false
	}
	x, y := g.SegX[0], g.SegY[0]
	return x >= pilarX1 && x <= pilarX2 && y >= pilarY1 && y <= pilarY2
}

// esperaDoPilar é a espera de um Tauron do Pilar: no máximo 10 s, ou menos se o
// painel da área pedir menos.
func (d *Dispatcher) esperaDoPilar(w *world.World, idx int) uint32 {
	return min(pilarRenasce, spawnrate.ScaleMillis(world.DefaultRespawnDelay, d.spawnPercentFor(w, idx)))
}
