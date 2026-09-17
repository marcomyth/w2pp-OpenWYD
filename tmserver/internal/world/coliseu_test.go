package world

import "testing"

// Os 26 blocos de população do Coliseu são de evento, e só eles: os chefes
// sozinhos da mesma região, o Guarda_Carga, a Prona e os vizinhos de número
// continuam do mundo. O 4864 fica no meio da faixa dos Espectros e é o Barrack_.
func TestColiseuSoOs26Blocos(t *testing.T) {
	coliseu := []int{0, 1, 2, 5, 6, 7,
		4854, 4855, 4856, 4857, 4858, 4859, 4860, 4861, 4862, 4863,
		4865, 4866, 4867, 4868, 4869, 4870, 4871, 4872, 4873, 4874}
	if len(coliseu) != 26 || len(coliseuGenerators) != 26 {
		t.Fatalf("lista do teste %d, lista do código %d; want 26 e 26", len(coliseu), len(coliseuGenerators))
	}
	for _, idx := range coliseu {
		if !IsEventOwnedGenerator(idx) {
			t.Errorf("bloco %d do Coliseu não é de evento: nasceria no boot e voltaria em 15 s", idx)
		}
	}
	chefes := []int{103, 104, 105, 106, 4853, 4864, 4885, 4886, 4887, 4888}
	outros := []int{974, 4232, 3, 4, 8, 4875}
	for _, idx := range append(chefes, outros...) {
		if IsEventOwnedGenerator(idx) {
			t.Errorf("bloco %d virou de evento; só os 26 de população saem do mundo", idx)
		}
	}
}

// Um monstro do Coliseu que morre não entra na fila de 15 s. Um chefe sozinho
// da mesma região entra: é a fila que o leva ao prazo de horas (handler/chefes.go).
func TestColiseuMorreENaoVolta(t *testing.T) {
	now := uint32(1000)
	w := New(Config{GridDim: 16, Now: func() uint32 { return now }}, slogDiscard(), nil, nil)
	for _, idx := range []int{0, 7, 4854, 4874} {
		id := w.SpawnMobAt(MobSpawn{Template: make([]byte, structMobTemplateSize), X: 5, Y: 6, GenIndex: int16(idx)})
		if id < MaxUser {
			t.Fatalf("SpawnMobAt(bloco %d) = %d, want um id de monstro", idx, id)
		}
		w.DespawnMob(id, 1)
		if len(w.respawnQueue) != 0 {
			t.Fatalf("bloco %d: fila de respawn = %d, want 0: o Coliseu voltaria sem evento", idx, len(w.respawnQueue))
		}
	}
	for i, idx := range []int{4864, 103, 4853, 4885} {
		id := w.SpawnMobAt(MobSpawn{Template: make([]byte, structMobTemplateSize), X: 7, Y: 8, GenIndex: int16(idx)})
		w.DespawnMob(id, 1)
		if len(w.respawnQueue) != i+1 {
			t.Fatalf("chefe do bloco %d: fila de respawn = %d, want %d: o chefe sozinho tem de voltar", idx, len(w.respawnQueue), i+1)
		}
	}
}
