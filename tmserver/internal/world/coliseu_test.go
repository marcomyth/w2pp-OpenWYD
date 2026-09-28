package world

import "testing"

// coliseuTodos são os 36 blocos de monstro da região Coliseu: as ondas (0-2 e
// 5-7), os 20 de Espectro e os dez chefes sozinhos. O 4864 fica no meio da faixa
// dos Espectros e é o Barrack_.
var coliseuTodos = []int{0, 1, 2, 5, 6, 7,
	4854, 4855, 4856, 4857, 4858, 4859, 4860, 4861, 4862, 4863,
	4865, 4866, 4867, 4868, 4869, 4870, 4871, 4872, 4873, 4874,
	103, 104, 105, 106, 4853, 4864, 4885, 4886, 4887, 4888}

// Os 36 blocos são de evento, e só eles: o Guarda_Carga, a Prona e os vizinhos
// de número continuam do mundo.
func TestColiseuSoOs36Blocos(t *testing.T) {
	if len(coliseuTodos) != 36 || len(coliseuGenerators) != 36 {
		t.Fatalf("lista do teste %d, lista do código %d; want 36 e 36", len(coliseuTodos), len(coliseuGenerators))
	}
	for _, idx := range coliseuTodos {
		if !IsEventOwnedGenerator(idx) {
			t.Errorf("bloco %d do Coliseu não é de evento: nasceria no boot e voltaria sozinho", idx)
		}
	}
	for _, idx := range []int{974, 4232, 3, 4, 8, 102, 107, 4852, 4875, 4884, 4889} {
		if IsEventOwnedGenerator(idx) {
			t.Errorf("bloco %d virou de evento; só os 36 do Coliseu saem do mundo", idx)
		}
	}
}

// Um monstro do Coliseu que morre não entra na fila de 15 s — nem os chefes, que
// antes voltavam em horas por ela (handler/chefes.go).
func TestColiseuMorreENaoVolta(t *testing.T) {
	now := uint32(1000)
	w := New(Config{GridDim: 16, Now: func() uint32 { return now }}, slogDiscard(), nil, nil)
	for _, idx := range []int{0, 7, 4854, 4874, 103, 4853, 4864, 4885} {
		id := w.SpawnMobAt(MobSpawn{Template: make([]byte, structMobTemplateSize), X: 5, Y: 6, GenIndex: int16(idx)})
		if id < MaxUser {
			t.Fatalf("SpawnMobAt(bloco %d) = %d, want um id de monstro", idx, id)
		}
		w.DespawnMob(id, 1)
		if len(w.respawnQueue) != 0 {
			t.Fatalf("bloco %d: fila de respawn = %d, want 0: o Coliseu voltaria sem evento", idx, len(w.respawnQueue))
		}
	}
	// Um bloco vizinho, fora do Coliseu, segue voltando pela fila.
	id := w.SpawnMobAt(MobSpawn{Template: make([]byte, structMobTemplateSize), X: 7, Y: 8, GenIndex: 4875})
	w.DespawnMob(id, 1)
	if len(w.respawnQueue) != 1 {
		t.Fatalf("bloco 4875: fila de respawn = %d, want 1", len(w.respawnQueue))
	}
}
