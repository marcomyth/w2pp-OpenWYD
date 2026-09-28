package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTeleportDest(t *testing.T) {
	// Armia teleport tile → Noatum, cost 700 (rounds the position to the tile and
	// spreads the destination by +rand%3).
	for i := 0; i < 30; i++ {
		dx, dy, cost, ok := TeleportDest(2116+int16(i%4), 2100+int16(i%4))
		if !ok || cost != 700 {
			t.Fatalf("Armia tile: ok=%v cost=%d, want ok cost=700", ok, cost)
		}
		if dx < 1044 || dx >= 1044+3 || dy < 1724 || dy >= 1724+3 {
			t.Fatalf("Armia→Noatum dest = %d,%d out of (1044,1724)+3", dx, dy)
		}
		if Village(dx, dy) != 4 { // Noatum
			t.Fatalf("Armia teleport landed in village %d, want Noatum(4)", Village(dx, dy))
		}
	}
	// Free hub route Noatum → Armia.
	if _, _, cost, ok := TeleportDest(1044, 1724); !ok || cost != 0 {
		t.Errorf("Noatum→Armia: ok=%v cost=%d, want ok cost=0", ok, cost)
	}
	// Non-teleport position.
	if _, _, _, ok := TeleportDest(2096, 2096); ok {
		t.Errorf("non-tile position reported a teleport")
	}
}

// TestNoatumParaODesertoForaDaGuerra: o tile (1056,1724) de Noatum leva ao Deserto
// em (1164,1720)+rand%3 fora da guerra RvR (GetFunc.cpp:987-991). A tabela tinha
// (3250,1703), que é o destino da rota com condição "Deserto - Kefra"
// (GetFunc.cpp:1007-1011, só com KefraLive != 0): um passo em Noatum levava de
// graça à cidade do Kefra.
func TestNoatumParaODesertoForaDaGuerra(t *testing.T) {
	for i := 0; i < 30; i++ {
		dx, dy, cost, ok := TeleportDest(1056+int16(i%4), 1724+int16(i%4))
		if !ok || cost != 0 {
			t.Fatalf("tile de Noatum → Deserto: ok=%v custo=%d, queria ok e custo 0", ok, cost)
		}
		if dx < 1164 || dx > 1166 || dy < 1720 || dy > 1722 {
			t.Fatalf("tile de Noatum → Deserto levou a (%d,%d), queria (1164..1166, 1720..1722)", dx, dy)
		}
	}
}

// The floors past the first were unreachable: every stair between dungeon levels
// was missing from the table, so the tile answered with silence. These are the
// exact pairs of GetFunc.cpp:876-944.
func TestDungeonStairsResolve(t *testing.T) {
	cases := []struct {
		name         string
		x, y         int16
		wantX, wantY int16
	}{
		{"1º → 2º andar", 148, 3780, 1004, 4028},
		{"1º → 2º andar, casa vizinha", 144, 3780, 1004, 4028},
		{"2º → 1º andar", 1004, 4028, 148, 3780},
		{"1º → 2º andar, outra escada", 408, 4072, 1004, 4064},
		{"2º → 1º andar, outra escada", 1004, 4064, 408, 4072},
		{"1º → 3º andar", 744, 3816, 1004, 3992},
		{"3º → 1º andar", 1004, 3992, 744, 3820},
		{"arco sem destino no original → 3º andar", 744, 3804, 912, 3808},
		{"3º andar → arco sem destino no original", 912, 3808, 744, 3804},
		{"2º → 3º andar", 680, 4076, 916, 3820},
		{"3º → 2º andar", 916, 3820, 680, 4076},
		{"2º → 3º andar, outra escada", 876, 3872, 932, 3820},
		{"3º → 2º andar, outra escada", 932, 3820, 876, 3872},
		{"Submundo 1º → 2º", 1516, 3996, 1304, 3816},
		{"Submundo 2º → 1º", 1304, 3816, 1516, 3996},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y, cost, ok := TeleportDest(c.x, c.y)
			if !ok {
				t.Fatalf("(%d,%d) não é um tile de teleporte", c.x, c.y)
			}
			// The destination is spread by rand%3, so the tile is the floor of it.
			if x < c.wantX || x > c.wantX+2 || y < c.wantY || y > c.wantY+2 {
				t.Errorf("destino (%d,%d), esperado (%d..%d, %d..%d)",
					x, y, c.wantX, c.wantX+2, c.wantY, c.wantY+2)
			}
			if cost != 0 {
				t.Errorf("escada de masmorra cobrando %d de ouro", cost)
			}
		})
	}
}

// TeleportDest rounds down to a multiple of 4, so every tile in the 4×4 block
// has to resolve — landing one step off must not be the difference between a
// working stair and a dead one. The blocks are the two arches the players stood
// on and got nothing: (746,3806) and (746,3817).
func TestTheWholeStairBlockResolves(t *testing.T) {
	for _, bloco := range [][2]int16{{744, 3804}, {744, 3816}} {
		for dx := int16(0); dx < 4; dx++ {
			for dy := int16(0); dy < 4; dy++ {
				x, y := bloco[0]+dx, bloco[1]+dy
				if _, _, _, ok := TeleportDest(x, y); !ok {
					t.Errorf("(%d,%d) não resolve, mas está no mesmo bloco de (%d,%d)", x, y, bloco[0], bloco[1])
				}
			}
		}
	}
}

// O cliente só manda _MSG_ReqTeleport nas casas com o bit 0x10 do
// AttributeMap.dat (uma casa do mapa = 4×4 posições). Uma rota em qualquer outra
// casa é muda: a do arco da Dungeon estava em (744,3820), uma casa abaixo da de
// teleporte, e o jogador parado no arco não ia a lugar nenhum. E, na Dungeon,
// toda casa de teleporte tem rota — as duas que o original deixou sem destino
// também, desde 25/09/2026.
func TestRotasCaemNasCasasDeTeleporteDoMapa(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "AttributeMap.dat"))
	if err != nil {
		t.Skipf("AttributeMap.dat indisponível: %v", err)
	}
	const dim, bitTeleporte = 1024, 0x10
	if len(b) != dim*dim {
		t.Fatalf("AttributeMap.dat com %d bytes, want %d", len(b), dim*dim)
	}
	teleporte := func(x, y int) bool { return b[(y/4)*dim+x/4]&bitTeleporte != 0 }
	for origem := range teleportTable {
		if !teleporte(int(origem[0]), int(origem[1])) {
			t.Errorf("rota em (%d,%d), casa sem o bit 0x10: o cliente nunca pede teleporte ali", origem[0], origem[1])
		}
	}
	// A Dungeon e o Submundo: x < 1400, y >= 3700. As rotas condicionais ficam
	// fora desta caixa (handler/{kefra_hall,kefra,vale}.go).
	for cy := 3700 / 4; cy < dim; cy++ {
		for cx := 0; cx < 1400/4; cx++ {
			x, y := int16(cx*4), int16(cy*4)
			if !teleporte(int(x), int(y)) {
				continue
			}
			if _, achou := teleportTable[[2]int16{x, y}]; !achou {
				t.Errorf("casa de teleporte (%d,%d) na Dungeon sem rota: o jogador pisa e nada acontece", x, y)
			}
		}
	}
}

// Noatum is the hub and the only paid leg: the three cities pay 700 to reach it
// and leaving is free. A stair that started charging would be a silent tax.
func TestOnlyTheCityLegsCharge(t *testing.T) {
	for origin, route := range teleportTable {
		paid := route.cost != 0
		isCityToNoatum := route.dx == 1044 && route.cost == 700
		if paid && !isCityToNoatum {
			t.Errorf("(%d,%d) cobra %d e não é uma perna de cidade → Noatum",
				origin[0], origin[1], route.cost)
		}
	}
}

// The table is GetTeleportPosition minus its conditional routes. The legacy
// tests 39 positions (GetFunc.cpp:782-1026) and three of them carry a condition
// a pure lookup cannot express, so they live in the handler: the two Kefra floors
// and the Azran floor, which only reaches the Hidden Valley for a player wearing
// the Fada do Vale. A count guards against the next subset — the table once
// carried 18 of them, and the ones missing were every dungeon stair.
//
// The 37 this test used to assert was never the legacy count: it was the size of
// the table itself, so a route dropped from the table could pass unnoticed.
func TestTableIsComplete(t *testing.T) {
	const (
		legacyRoutes = 39 // GetFunc.cpp:782-1026
		conditional  = 3  // handler/{kefra_hall,kefra,vale}.go
		// O par (744,3804) ↔ (912,3808): casas de teleporte do mapa que o
		// original não roteava (25/09/2026).
		foraDoLegado = 2
	)
	if got, want := len(teleportTable), legacyRoutes-conditional+foraDoLegado; got != want {
		t.Errorf("tabela com %d rotas, want %d (%d do legado menos %d condicionais mais %d fora do legado)",
			got, want, legacyRoutes, conditional, foraDoLegado)
	}
	// Uma rota condicional NA tabela é o bug que fechamos: a consulta é pura e
	// responde antes de qualquer condição, então o piso de Azran levava ao Vale
	// quem não tinha fada nenhuma.
	for _, origem := range [][2]int16{{2364, 3892}, {2364, 3924}, {2548, 1740}} {
		if _, achou := teleportTable[origem]; achou {
			t.Errorf("a rota condicional (%d,%d) está na tabela", origem[0], origem[1])
		}
	}
}
