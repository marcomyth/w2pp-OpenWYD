package world

import "testing"

// TestPisarNoNPCNaoOApagaDoGrid prende o defeito da Patrulha do Kaizen
// (26/09/2026): um jogador que parava na casa de um NPC tomava a casa dele no
// grid, e ao sair a deixava vazia. O NPC não anda, então nunca voltava ao grid, e
// quem chegava depois não o via mais (a varredura de visão só lê o grid) — até o
// reinício.
func TestPisarNoNPCNaoOApagaDoGrid(t *testing.T) {
	w := New(Config{GridDim: 64}, slogDiscard(), nil, nil)
	npc := w.SpawnMob(genMerchantTemplate(1), 20, 20)
	if npc < 0 {
		t.Fatal("o NPC não nasceu")
	}
	const conn = 1
	w.entities[conn] = &Entity{ID: conn, Mode: MobUser, HP: 100}
	w.SetEntityPos(conn, 18, 20)

	w.SetEntityPos(conn, 20, 20) // pisa na casa do NPC
	if id, ok := w.EntityAt(20, 20); !ok || id != npc {
		t.Fatalf("com o jogador em cima, o grid dá %d (ok=%v), want o NPC %d", id, ok, npc)
	}
	w.SetEntityPos(conn, 22, 22) // e sai
	if id, ok := w.EntityAt(20, 20); !ok || id != npc {
		t.Fatalf("depois que o jogador saiu, o grid dá %d (ok=%v), want o NPC %d", id, ok, npc)
	}
	if id, ok := w.EntityAt(22, 22); !ok || id != conn {
		t.Errorf("o jogador não voltou ao grid na casa livre: %d (ok=%v)", id, ok)
	}
	visto := false
	w.ForEachMobInViewAt(22, 22, func(e *Entity) { visto = visto || e.ID == npc })
	if !visto {
		t.Error("quem chega perto não vê o NPC")
	}
}

// Uma entrada velha do grid — o dono dela já está em outra casa ou nem existe
// mais — não segura a casa: quem chega fica com ela.
func TestEntradaVelhaDoGridNaoSeguraACasa(t *testing.T) {
	w := New(Config{GridDim: 64}, slogDiscard(), nil, nil)
	const a, b = 1, 2
	w.entities[a] = &Entity{ID: a, Mode: MobUser, HP: 100}
	w.entities[b] = &Entity{ID: b, Mode: MobUser, HP: 100}
	w.SetEntityPos(a, 10, 10)
	w.entities[a].X = 11 // saiu sem passar pelo grid
	w.SetEntityPos(b, 10, 10)
	if id, _ := w.EntityAt(10, 10); id != b {
		t.Errorf("a casa ficou com %d, want %d", id, b)
	}
}

func TestFreeCellFor(t *testing.T) {
	w := New(Config{GridDim: 64}, slogDiscard(), nil, nil)
	npc := w.SpawnMob(genMerchantTemplate(1), 20, 20)
	const conn = 1
	w.entities[conn] = &Entity{ID: conn, Mode: MobUser, HP: 100}
	w.SetEntityPos(conn, 5, 5)

	if x, y, ok := w.FreeCellFor(conn, 20, 20); !ok || (x == 20 && y == 20) || chebyshev(x, y, 20, 20) > 1 {
		t.Errorf("casa do NPC %d: FreeCellFor = (%d,%d,%v), want uma vizinha livre", npc, x, y, ok)
	}
	if x, y, ok := w.FreeCellFor(conn, 5, 5); !ok || x != 5 || y != 5 {
		t.Errorf("a própria casa: FreeCellFor = (%d,%d,%v), want (5,5)", x, y, ok)
	}
	if x, y, ok := w.FreeCellFor(conn, 30, 30); !ok || x != 30 || y != 30 {
		t.Errorf("casa livre: FreeCellFor = (%d,%d,%v), want (30,30)", x, y, ok)
	}
}

// Quem entrou numa casa ocupada fica fora do grid; quando o dono sai, a casa
// passa a ele. Sem o repasse ele seguiria invisível à varredura e um terceiro
// pousaria em cima dele achando a casa vazia (TestGMPosDoesNotEvictTheOccupantOfATile).
func TestCasaPassaParaQuemFicou(t *testing.T) {
	w := New(Config{GridDim: 64}, slogDiscard(), nil, nil)
	const a, b = 1, 2
	w.entities[a] = &Entity{ID: a, Mode: MobUser, HP: 100}
	w.entities[b] = &Entity{ID: b, Mode: MobUser, HP: 100}
	w.SetEntityPos(a, 10, 10)
	w.SetEntityPos(b, 10, 10) // b chega na casa de a
	if id, _ := w.EntityAt(10, 10); id != a {
		t.Fatalf("a casa foi para %d, want o dono %d", id, a)
	}
	w.SetEntityPos(a, 12, 12) // a sai
	if id, ok := w.EntityAt(10, 10); !ok || id != b {
		t.Fatalf("depois que o dono saiu a casa ficou com %d (ok=%v), want %d", id, ok, b)
	}
	if x, y, _ := w.FreeCellFor(3, 10, 10); x == 10 && y == 10 {
		t.Error("um terceiro ainda pousaria em cima de b")
	}
	if len(w.foraDoGrid) != 0 {
		t.Errorf("sobrou anotação fora do grid: %v", w.foraDoGrid)
	}
}
