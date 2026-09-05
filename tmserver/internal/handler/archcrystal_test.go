package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func archCrystalFixture(t *testing.T) (*Dispatcher, *world.World, *world.Session, *world.Entity) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	d := New(Config{
		Log:           log,
		ItemVolatiles: map[int]int{4106: volArchCrystal, 4107: volArchCrystal, 4108: volArchCrystal, 4109: volArchCrystal},
	})
	w := world.New(world.Config{GridDim: 16}, log, nil, d.Handle)
	e := &world.Entity{
		ID: 0, Mode: world.MobUser, Name: "Heroi",
		Level: 355, ClassMaster: classMasterArch,
		Exp:       500_000_000,
		BaseMaxHP: 1000, BaseMaxMP: 1000, MaxHP: 1000, MaxMP: 1000, HP: 1000, MP: 1000,
		Str: 8, Int: 4, Dex: 7, Con: 6,
		BaseStr: 8, BaseInt: 4, BaseDex: 7, BaseCon: 6,
	}
	return d, w, &world.Session{Conn: 0, Mode: world.UserPlay}, e
}

// The four crystals are a chain: each stage must be handed in in order, costs
// 100M experience, and raises one base attribute.
func TestArchCrystalChain(t *testing.T) {
	d, w, s, e := archCrystalFixture(t)
	startExp := e.Exp
	mp0, hp0 := e.BaseMaxMP, e.BaseMaxHP

	// Stage 3 before stage 1 is refused — the chain, not four loose items.
	e.Carry[0] = world.Item{Index: 4108}
	d.useArchCrystal(w, s, e, 0)
	if e.ArchCristal != 0 {
		t.Fatalf("out-of-order hand-in advanced the quest to %d, want 0", e.ArchCristal)
	}
	if e.Carry[0].Index != 4108 {
		t.Error("out-of-order hand-in consumed the crystal")
	}
	if e.Exp != startExp {
		t.Errorf("out-of-order hand-in charged %d exp", startExp-e.Exp)
	}

	// Stage 1: +80 MP.
	e.Carry[0] = world.Item{Index: 4106}
	d.useArchCrystal(w, s, e, 0)
	if e.ArchCristal != 1 {
		t.Fatalf("stage = %d, want 1", e.ArchCristal)
	}
	if e.BaseMaxMP != mp0+80 {
		t.Errorf("BaseMaxMP = %d, want %d", e.BaseMaxMP, mp0+80)
	}
	if want := startExp - archCrystalExpCost; e.Exp != want {
		t.Errorf("exp = %d, want %d (100M per stage)", e.Exp, want)
	}
	if e.Carry[0].Index != 0 {
		t.Error("the crystal was not consumed")
	}

	// Repeating stage 1 is refused.
	e.Carry[0] = world.Item{Index: 4106}
	d.useArchCrystal(w, s, e, 0)
	if e.BaseMaxMP != mp0+80 {
		t.Error("a repeated hand-in granted the bonus twice")
	}

	// Stage 2: +30 AC.
	e.Carry[0] = world.Item{Index: 4107}
	ac0 := e.BaseAC
	d.useArchCrystal(w, s, e, 0)
	if e.BaseAC != ac0+30 {
		t.Errorf("BaseAC = %d, want %d", e.BaseAC, ac0+30)
	}

	// Stage 3: +80 HP.
	e.Carry[0] = world.Item{Index: 4108}
	d.useArchCrystal(w, s, e, 0)
	if e.BaseMaxHP != hp0+80 {
		t.Errorf("BaseMaxHP = %d, want %d", e.BaseMaxHP, hp0+80)
	}

	// Stage 4: +60 HP, +60 MP, +20 AC.
	e.Carry[0] = world.Item{Index: 4109}
	d.useArchCrystal(w, s, e, 0)
	if e.ArchCristal != 4 {
		t.Fatalf("stage = %d, want 4", e.ArchCristal)
	}
	if e.BaseMaxHP != hp0+140 || e.BaseMaxMP != mp0+140 {
		t.Errorf("HP/MP = %d/%d, want %d/%d", e.BaseMaxHP, e.BaseMaxMP, hp0+140, mp0+140)
	}
	if e.BaseAC != ac0+50 {
		t.Errorf("BaseAC = %d, want %d", e.BaseAC, ac0+50)
	}
	if want := startExp - 4*archCrystalExpCost; e.Exp != want {
		t.Errorf("exp = %d, want %d (four stages)", e.Exp, want)
	}
}

// The AC the crystals grant has to survive a relog. BaseScore.Ac is derived, not
// stored, so it is rebuilt from the persisted stage counter — this is the whole
// reason arch_cristal is a column.
func TestArchCrystalACSurvivesRelog(t *testing.T) {
	tests := []struct {
		stage uint8
		want  int32
	}{
		{0, 0},
		{1, 0},  // stage 1 grants MP, not AC
		{2, 30}, // +30
		{3, 30}, // stage 3 grants HP
		{4, 50}, // +30 then +20
	}
	for _, tt := range tests {
		e := &world.Entity{ClassMaster: classMasterArch, Level: 355, ArchCristal: tt.stage}
		base := playerBaseAC(&world.Entity{ClassMaster: classMasterArch, Level: 355})
		if got := playerBaseAC(e) - base; got != tt.want {
			t.Errorf("stage %d → crystal AC %d, want %d", tt.stage, got, tt.want)
		}
	}
}

// The legacy subtracts the price without checking the balance, which would wrap
// a character below 100M negative. Clamping at zero is a deliberate divergence:
// a negative Exp feeds straight into the level curve.
func TestArchCrystalExpNeverGoesNegative(t *testing.T) {
	d, w, s, e := archCrystalFixture(t)
	e.Exp = 1000
	e.Carry[0] = world.Item{Index: 4106}
	d.useArchCrystal(w, s, e, 0)
	if e.Exp < 0 {
		t.Errorf("exp = %d, want >= 0", e.Exp)
	}
	if e.ArchCristal != 1 {
		t.Errorf("stage = %d, want 1 — the hand-in itself must still work", e.ArchCristal)
	}
}
