package world

import "testing"

// DeferGenerator tira do mundo o que o bloco tem de pé e o devolve pela fila de
// renascimento depois da espera: nem um tique antes, o mesmo monstro no mesmo
// lugar, e a contagem do bloco volta a um só quando ele volta.
func TestDeferGeneratorVoltaDepoisDaEspera(t *testing.T) {
	var agora uint32 = 1_000
	w := New(Config{GridDim: 64, Now: func() uint32 { return agora }}, slogDiscard(), nil, nil)
	g := &Generator{
		MinuteGenerate: -1, MaxNumMob: 1, LeaderName: "Grunt",
		SegX: [5]int16{20}, SegY: [5]int16{20},
		LeaderTmpl: genMobTemplate(0),
	}
	outro := &Generator{
		MinuteGenerate: -1, MaxNumMob: 1, LeaderName: "Grunt",
		SegX: [5]int16{30}, SegY: [5]int16{30},
		LeaderTmpl: genMobTemplate(0),
	}
	w.RegisterGenerators([]*Generator{g, outro})
	ids := w.GenerateMob(0)
	vizinho := w.GenerateMob(1)
	if len(ids) != 1 || len(vizinho) != 1 {
		t.Fatalf("GenerateMob = %v e %v, want um monstro em cada bloco", ids, vizinho)
	}
	antes := *w.Entity(ids[0])

	const espera = 4 * 3_600_000
	if n := w.DeferGenerator(0, espera); n != 1 {
		t.Fatalf("DeferGenerator adiou %d, want 1", n)
	}
	if w.Entity(ids[0]) != nil {
		t.Fatal("o monstro adiado continua no mundo")
	}
	if g.CurrentNumMob != 0 {
		t.Fatalf("CurrentNumMob = %d depois de adiar, want 0", g.CurrentNumMob)
	}
	if w.Entity(vizinho[0]) == nil {
		t.Fatal("adiar o bloco 0 tirou o monstro do bloco 1")
	}

	agora += espera - 1
	if got := w.SpawnDueRespawns(agora); len(got) != 0 {
		t.Fatalf("voltou %v um tique antes da espera", got)
	}
	agora++
	got := w.SpawnDueRespawns(agora)
	if len(got) != 1 {
		t.Fatalf("SpawnDueRespawns na hora = %v, want um monstro", got)
	}
	e := w.Entity(got[0])
	if e.SpawnX != antes.SpawnX || e.SpawnY != antes.SpawnY || e.GenIndex != 0 || e.TemplateName != antes.TemplateName {
		t.Errorf("voltou %s em (%d,%d) do bloco %d, want %s em (%d,%d) do bloco 0",
			e.TemplateName, e.SpawnX, e.SpawnY, e.GenIndex, antes.TemplateName, antes.SpawnX, antes.SpawnY)
	}
	if g.CurrentNumMob != 1 {
		t.Errorf("CurrentNumMob = %d depois da volta, want 1", g.CurrentNumMob)
	}
	if n := w.DeferGenerator(99, espera); n != 0 {
		t.Errorf("bloco inexistente adiou %d", n)
	}
}
