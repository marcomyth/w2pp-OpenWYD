package handler

import (
	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/spawnrate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os Anciões Ciclops (pedido do Marco em 28/09/2026, "estão lentos"): cada um que
// morre volta sozinho em no máximo 10 s, como os Tauron do Pilar.
//
// São os blocos que o Ancião lidera — sozinho ou com um Orc Médico — nas duas
// áreas dele: em volta de (1890,1720) e no campo ao sul do spot dos Ciclopes
// Cruéis (2195-2280 × 1348-1402). Todos ficam sem período de minuto no
// NPCGener.txt, e é a fila de renascimento que os traz de volta, com a espera
// daqui.
//
// Antes, parte deles voltava pelo relógio de minuto (um grupo a cada 12 ou 24 s)
// e o resto pela fila comum de 15 s. Os blocos sem período ainda corriam o risco
// da regra dos chefes sozinhos (chefes.go): é bloco de até três monstros, e uma
// Exp de 1 milhão ou mais dada no painel ao molde os prenderia em 24 horas, como
// aconteceu com o Tauron do Pilar.
//
// Os grupos que o Ciclope Cruel lidera e em que o Ancião só acompanha não mudam:
// o pedido foi o Ancião, e o spot dos Cruéis tem o desenho dele (ciclopes.go).
//
// Reconhecido pelo molde do líder, e não pelo índice do bloco: o índice anda quando
// o arquivo é editado.
const (
	// anciaoRenasce é o teto da espera de um Ancião Ciclops, em ms.
	anciaoRenasce = 10_000
)

// anciaoMoldes são os dois moldes do Ancião Ciclops que lideram bloco.
var anciaoMoldes = map[string]bool{
	droprule.Canonical("Anciao_Ciclops"):  true,
	droprule.Canonical("Anciao_Ciclops_"): true,
}

// geradorDoAnciao diz se o bloco idx é liderado por um Ancião Ciclops.
func geradorDoAnciao(w *world.World, idx int) bool {
	g := w.GeneratorAt(idx)
	return g != nil && anciaoMoldes[droprule.Canonical(g.LeaderName)]
}

// esperaDoAnciao é a espera de um Ancião Ciclops: no máximo 10 s, ou menos se o
// painel da área pedir menos.
func (d *Dispatcher) esperaDoAnciao(w *world.World, idx int) uint32 {
	return min(anciaoRenasce, spawnrate.ScaleMillis(world.DefaultRespawnDelay, d.spawnPercentFor(w, idx)))
}
