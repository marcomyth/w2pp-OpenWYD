package handler

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// nevascaCarregada is SkillData row 36 as the loader leaves it: raw AffectTime 1,
// divided by 4, is 0.
var nevascaCarregada = content.Spell{Index: 36, AffectType: 1, AffectValue: 2, AffectTime: 0,
	Aggressive: 1, AffectResist: 2, MaxTarget: 1, Name: "Nevasca"}

// politicaDeProducao is the duration policy tmServer boots with (main.go): the
// cut the Encantar Gelo slow must NOT go through.
var politicaDeProducao = world.AffectDuration{ScalePct: 15, MinTicks: 8, MaxTicks: 75}

// golpeComEncantarGelo swings until the 50% proc lands the slow on target, and
// reports whether it ever did.
func golpeComEncantarGelo(d *Dispatcher, w *world.World, ht, target *world.Entity) bool {
	for i := 0; i < 64; i++ {
		d.applyOnHitAffects(w, ht, target, target.ID)
		if target.HasAffect(1) {
			return true
		}
	}
	return false
}

func tempoDoAfeto(e *world.Entity, t uint8) uint32 {
	for i := range e.Affect {
		if e.Affect[i].Type == t {
			return e.Affect[i].Time
		}
	}
	return 0
}

// TestEncantarGeloLentidaoPegaEmMonstro: the slow lands on a monster, lasts the
// legacy (0+1)×(Special[1]+150)/100 ticks, and the monster AI reads it.
func TestEncantarGeloLentidaoPegaEmMonstro(t *testing.T) {
	cases := []struct {
		special   int16
		wantTicks uint32
	}{
		{0, 1},   // 150/100
		{100, 2}, // 250/100
		{250, 4}, // 400/100
	}
	for _, c := range cases {
		d := New(Config{Spells: content.NewSkillData([]content.Spell{nevascaCarregada}),
			AffectDuration: politicaDeProducao, CombatRules: regraSemEscala()})
		w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
		ht := &world.Entity{ID: 1, Class: 3, Rsv: world.RsvFrost}
		ht.Special[1] = c.special
		mob := w.Entity(w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Alvo"), X: 5, Y: 5, GenIndex: -1}))

		if !golpeComEncantarGelo(d, w, ht, mob) {
			t.Fatalf("maestria %d: a lentidão nunca pegou no monstro", c.special)
		}
		if got := tempoDoAfeto(mob, 1); got != c.wantTicks {
			t.Errorf("maestria %d: lentidão com %d ticks, want %d (duração do legado)", c.special, got, c.wantTicks)
		}
		if mob.AffRunSpeed >= 0 || mob.AffAttackSpeed >= 0 {
			t.Errorf("maestria %d: o monstro não ficou lento (run %d, attack %d)",
				c.special, mob.AffRunSpeed, mob.AffAttackSpeed)
		}
	}
}

// TestEncantarGeloLentidaoEmJogadorDuraOLegado: against a player the slow also
// keeps its legacy length instead of the production policy's one tick.
func TestEncantarGeloLentidaoEmJogadorDuraOLegado(t *testing.T) {
	d := New(Config{Spells: content.NewSkillData([]content.Spell{nevascaCarregada}),
		AffectDuration: politicaDeProducao, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	ht := &world.Entity{ID: 1, Class: 3, Rsv: world.RsvFrost}
	ht.Special[1] = 250
	alvo := &world.Entity{ID: 2}

	if !golpeComEncantarGelo(d, w, ht, alvo) {
		t.Fatal("a lentidão nunca pegou no jogador")
	}
	if got := tempoDoAfeto(alvo, 1); got != 4 {
		t.Errorf("lentidão com %d ticks, want 4", got)
	}
}

// TestSemEncantarGeloNaoHaLentidao: no RsvFrost (no Encantar Gelo buff, or no bow in the right
// hand — affect_score.go), no slow.
func TestSemEncantarGeloNaoHaLentidao(t *testing.T) {
	d := New(Config{Spells: content.NewSkillData([]content.Spell{nevascaCarregada}), CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	ht := &world.Entity{ID: 1, Class: 3}
	mob := w.Entity(w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Alvo"), X: 5, Y: 5, GenIndex: -1}))

	if golpeComEncantarGelo(d, w, ht, mob) {
		t.Fatal("lentidão sem o buff da Encantar Gelo")
	}
}
