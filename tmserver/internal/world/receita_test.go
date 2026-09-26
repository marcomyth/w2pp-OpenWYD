package world

import (
	"encoding/binary"
	"testing"
)

// mobComVida is genMobTemplate with a chosen MaxHp, so a test can tell two
// sheets of the same monster apart by the entity they produce.
func mobComVida(hp uint32) []byte {
	b := genMobTemplate(0)
	const cs = 92
	binary.LittleEndian.PutUint32(b[cs+16:], hp) // MaxHp
	binary.LittleEndian.PutUint32(b[cs+24:], hp) // Hp
	return b
}

// blocoDeUm is a block that holds one lone monster and respawns it through the
// 15 s queue (MinuteGenerate 0) — the kind of block most of the world is.
func blocoDeUm(tmpl []byte, nome string, x, y int16) *Generator {
	return &Generator{
		Name: nome, MinGroup: 0, MaxGroup: 0, MaxNumMob: 1,
		SegX: [5]int16{x}, SegY: [5]int16{y},
		LeaderTmpl: tmpl, LeaderName: nome,
	}
}

// blocoLivre is an index no event or dungeon claims: block 0 is one of the
// event-owned blocks, whose deaths never reach the queue.
const blocoLivre = 20000

// registrar installs g at blocoLivre and returns the index.
func registrar(w *World, g *Generator) int {
	w.SetGenerator(blocoLivre, g)
	return blocoLivre
}

// matarEEsperar kills the block's only monster in combat and runs the queue past
// its delay, returning what came back.
func matarEEsperar(t *testing.T, w *World, now *uint32, id int) []int {
	t.Helper()
	w.DespawnMob(id, 1)
	if len(w.respawnQueue) != 1 {
		t.Fatalf("a morte não entrou na fila (%d entradas)", len(w.respawnQueue))
	}
	*now += DefaultRespawnDelay
	return w.SpawnDueRespawns(*now)
}

// A sheet edited on /monstros swaps the block's bytes in place. The monster that
// was already dead in the queue has to come back with the NEW sheet: the queue
// used to bring back the copy it died with, so in a zone where every block
// respawns through it the edit never reached anyone.
func TestRespawnVoltaComAFichaAtualDoBloco(t *testing.T) {
	now := uint32(1000)
	w := New(Config{GridDim: 64, Now: func() uint32 { return now }}, slogDiscard(), nil, nil)
	g := blocoDeUm(mobComVida(100), "Lobo", 20, 20)
	ids := w.GenerateMob(registrar(w, g))
	if len(ids) != 1 {
		t.Fatalf("GenerateMob = %v, quero 1", ids)
	}

	g.LeaderTmpl = mobComVida(900) // what the sheet reload does

	voltou := matarEEsperar(t, w, &now, ids[0])
	if len(voltou) != 1 {
		t.Fatalf("renasceram %v, quero 1", voltou)
	}
	if hp := w.Entity(voltou[0]).MaxHP; hp != 900 {
		t.Errorf("renasceu com %d de vida, quero 900 (a ficha nova)", hp)
	}
}

// A recipe replaced by the panel moves the block's revision. A monster born
// under the old recipe does not come back as itself: the block raises from the
// recipe it has now — another monster, somewhere else.
func TestRespawnDeReceitaTrocadaNasceDaReceitaNova(t *testing.T) {
	now := uint32(1000)
	w := New(Config{GridDim: 64, Now: func() uint32 { return now }}, slogDiscard(), nil, nil)
	g := blocoDeUm(mobComVida(100), "Lobo", 20, 20)
	ids := w.GenerateMob(registrar(w, g))

	// The panel's new recipe: a bear, twenty tiles away.
	g.LeaderTmpl, g.LeaderName, g.Name = mobComVida(700), "Urso", "Urso"
	g.SegX[0], g.SegY[0] = 40, 40
	g.Rev++

	voltou := matarEEsperar(t, w, &now, ids[0])
	if len(voltou) != 1 {
		t.Fatalf("renasceram %v, quero 1 (o bloco tem teto 1)", voltou)
	}
	e := w.Entity(voltou[0])
	if e.TemplateName != "Urso" || e.MaxHP != 700 {
		t.Errorf("renasceu %q com %d de vida, quero o Urso da receita nova", e.TemplateName, e.MaxHP)
	}
	if chebyshevWorld(e.X, e.Y, 40, 40) > 3 {
		t.Errorf("renasceu em (%d,%d), quero perto de (40,40), o ponto novo", e.X, e.Y)
	}
	if e.GenRev != g.Rev {
		t.Errorf("GenRev = %d, quero %d: o próximo que morrer voltaria de novo pelo bloco", e.GenRev, g.Rev)
	}
	if g.CurrentNumMob != 1 {
		t.Errorf("CurrentNumMob = %d, quero 1", g.CurrentNumMob)
	}
}

// Nothing edited: the queue behaves as before — the same monster, at the spot it
// was born, without spending a roll of the parity RNG on regenerating.
func TestRespawnSemMudancaVoltaComoEra(t *testing.T) {
	now := uint32(1000)
	w := New(Config{GridDim: 64, Now: func() uint32 { return now }}, slogDiscard(), nil, nil)
	g := blocoDeUm(mobComVida(100), "Lobo", 20, 20)
	ids := w.GenerateMob(registrar(w, g))
	x, y := w.Entity(ids[0]).X, w.Entity(ids[0]).Y

	voltou := matarEEsperar(t, w, &now, ids[0])
	if len(voltou) != 1 {
		t.Fatalf("renasceram %v, quero 1", voltou)
	}
	if e := w.Entity(voltou[0]); e.X != x || e.Y != y || e.TemplateName != "Lobo" {
		t.Errorf("voltou %q em (%d,%d), quero Lobo em (%d,%d)", e.TemplateName, e.X, e.Y, x, y)
	}
}

// A block the panel created lives at 20000 and up: the table grows to reach it,
// the gap reads as empty slots, and its mobs carry the index.
func TestSetGeneratorCresceATabela(t *testing.T) {
	w := New(Config{GridDim: 64}, slogDiscard(), nil, nil)
	w.RegisterGenerators([]*Generator{blocoDeUm(mobComVida(100), "Lobo", 20, 20)})

	novo := blocoDeUm(mobComVida(100), "Urso", 30, 30)
	w.SetGenerator(20000, novo)
	if w.GeneratorCount() != 20001 || w.GeneratorAt(20000) != novo || w.GeneratorAt(0) == nil {
		t.Fatalf("tabela = %d slots, 20000=%p, 0=%p", w.GeneratorCount(), w.GeneratorAt(20000), w.GeneratorAt(0))
	}
	if w.GeneratorAt(1000) != nil {
		t.Error("a lacuna devia ser vazia")
	}
	ids := w.GenerateMob(20000)
	if len(ids) != 1 || w.Entity(ids[0]).GenIndex != 20000 {
		t.Fatalf("GenerateMob(20000) = %v", ids)
	}

	w.SetGenerator(1<<15, novo) // past what a mob's int16 can name
	if w.GeneratorCount() != 20001 {
		t.Errorf("um índice fora do int16 cresceu a tabela para %d", w.GeneratorCount())
	}
}

func chebyshevWorld(ax, ay, bx, by int16) int {
	dx, dy := int(ax)-int(bx), int(ay)-int(by)
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return max(dx, dy)
}
