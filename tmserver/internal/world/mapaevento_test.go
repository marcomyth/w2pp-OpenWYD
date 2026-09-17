package world

import "testing"

// Um bloco que nasce num mapa guardado para evento não põe ninguém lá, seja
// qual for o período dele: o boot, o relógio de minuto e o "gerar <bloco>" passam
// todos por GenerateMob. Um bloco igual, fora do mapa, continua gerando.
func TestBlocoEmMapaDeEventoNaoGera(t *testing.T) {
	w := New(Config{GridDim: 4096}, slogDiscard(), nil, nil)
	novaGuerra := &Generator{ // bloco 4241, Ladrão Fantasma
		MinuteGenerate: -1, MinGroup: 1, MaxGroup: 1, MaxNumMob: 9,
		SegX: [5]int16{1085}, SegY: [5]int16{1467},
		LeaderTmpl: genMobTemplate(5), FollowerTmpl: genMobTemplate(5),
	}
	pistas := &Generator{ // bloco com período, que o relógio de minuto reabastece
		MinuteGenerate: 3, MinGroup: 0, MaxGroup: 0, MaxNumMob: 9,
		SegX: [5]int16{3426}, SegY: [5]int16{1430},
		LeaderTmpl: genMobTemplate(5),
	}
	armia := &Generator{
		MinuteGenerate: -1, MinGroup: 0, MaxGroup: 0, MaxNumMob: 9,
		SegX: [5]int16{2100}, SegY: [5]int16{2100},
		LeaderTmpl: genMobTemplate(5),
	}
	w.RegisterGenerators([]*Generator{novaGuerra, pistas, armia})

	for i, nome := range []string{"Nova Guerra de Noatun", "Pistas"} {
		if ids := w.GenerateMob(i); len(ids) != 0 {
			t.Errorf("%s: GenerateMob pôs %d mobs, quer 0", nome, len(ids))
		}
		if ids := w.GenerateMobUpTo(i, 5); len(ids) != 0 {
			t.Errorf("%s: GenerateMobUpTo pôs %d mobs, quer 0", nome, len(ids))
		}
	}
	if ids := w.GenerateMob(2); len(ids) != 1 {
		t.Fatalf("Armia: GenerateMob pôs %d mobs, quer 1 — a trava pegou bloco fora dos mapas", len(ids))
	}
}

// O GM monta o evento trazendo o grupo para onde está ("gerar <bloco> aqui"), e
// isso vale dentro do mapa. Quando esse mob morre, não volta pela fila de 15 s:
// senão o evento deixaria o mapa povoado depois de acabar.
func TestEventoNoMapaNaoRenasce(t *testing.T) {
	w := New(Config{GridDim: 4096}, slogDiscard(), nil, nil)
	g := &Generator{
		MinuteGenerate: -1, MinGroup: 0, MaxGroup: 0, MaxNumMob: 9,
		SegX: [5]int16{2100}, SegY: [5]int16{2100},
		LeaderTmpl: genMobTemplate(5),
	}
	w.RegisterGenerators([]*Generator{nil, nil, nil, nil, nil, nil, nil, nil, g})

	ids := w.GenerateMobNear(8, 321, 309) // Monster City
	if len(ids) != 1 {
		t.Fatalf("gerar aqui dentro do mapa pôs %d mobs, quer 1", len(ids))
	}
	w.DespawnMob(ids[0], 1)
	if got := len(w.SpawnDueRespawns(^uint32(0))); got != 0 {
		t.Fatalf("mob do evento voltou pela fila: %d, quer 0", got)
	}

	// O mesmo bloco, no lugar dele, segue a regra de sempre e volta.
	ids = w.GenerateMob(8)
	if len(ids) != 1 {
		t.Fatalf("setup fora do mapa: %d mobs", len(ids))
	}
	w.DespawnMob(ids[0], 1)
	if got := len(w.SpawnDueRespawns(^uint32(0))); got != 1 {
		t.Fatalf("fora do mapa a fila devolveu %d, quer 1", got)
	}
}
