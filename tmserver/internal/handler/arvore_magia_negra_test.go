package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// fmBlack monta uma Foema com a INT e os bits dados, maestria 255 na Magia Negra.
func fmBlack(intel int16, learned int32) *world.Entity {
	e := &world.Entity{ID: 1, Class: 1, ClassMaster: classMasterMortal, Int: intel, BaseInt: intel, LearnedSkill: learned}
	e.Special[2] = 255
	return e
}

func TestFmMagiaNegraExigeInferno(t *testing.T) {
	tests := []struct {
		name string
		e    *world.Entity
		want bool
	}{
		{"FM com o Inferno", fmBlack(3148, learnedInferno), true},
		{"FM sem o Inferno", fmBlack(3148, 1<<14), false},
		{"TK com o bit 15 (Armadura Crítica)", &world.Entity{ID: 1, Class: 0, LearnedSkill: learnedInferno}, false},
		{"monstro com o bit 15", &world.Entity{ID: world.MaxUser, Class: 1, LearnedSkill: learnedInferno}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmMagiaNegra(tt.e); got != tt.want {
				t.Fatalf("fmMagiaNegra = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkillDeDanoDaMagiaNegra(t *testing.T) {
	for skill := 31; skill <= 40; skill++ {
		want := skill >= 32 && skill <= 39 && skill != 37
		if got := skillDeDanoDaMagiaNegra(skill); got != want {
			t.Errorf("skill %d: %v, want %v", skill, got, want)
		}
	}
}

func TestCajadoDaMagiaNegra(t *testing.T) {
	const cajado2, cajado1, escudo, lanca = 910, 904, 920, 855
	ability := armasNasMaos(map[int16]int{cajado2: wtypeCajadoDuasMaos, cajado1: wtypeCajadoUmaMao, escudo: 51, lanca: wtypeLanca})
	for _, c := range []struct {
		nome        string
		dir, esquer int16
		want        int
	}{
		{"cajado de 2 mãos", cajado2, 0, 140},
		{"cajado de 1 mão e escudo", cajado1, escudo, 120},
		{"escudo e cajado de 1 mão na esquerda", escudo, cajado1, 120},
		{"cajado de 1 mão sozinho", cajado1, 0, 120},
		{"lança", lanca, 0, 100},
		{"sem arma", 0, 0, 100},
	} {
		e := fmBlack(3148, learnedInferno)
		e.Equip[weaponSlotR], e.Equip[weaponSlotL] = world.Item{Index: c.dir}, world.Item{Index: c.esquer}
		if got := armaPctMagiaNegra(e, ability); got != c.want {
			t.Errorf("%s: %d%%, want %d%%", c.nome, got, c.want)
		}
	}
}

func TestMagoCritico(t *testing.T) {
	black, tk := fmBlack(3148, learnedInferno), tkDaEspadaMagica(2848, learnedTempestadeDeGelo, 255)
	if !magoCritico(black, 39) || magoCritico(black, 37) || magoCritico(black, 20) {
		t.Error("FM Magia Negra: crita nas 32-39 menos o Trovão, e só nelas")
	}
	if !magoCritico(tk, 22) || magoCritico(tk, 39) {
		t.Error("TK Espada Mágica: crita nas 17-23, e só nelas")
	}
	if magoCritico(fmBlack(3148, 0), 39) {
		t.Error("FM sem o Inferno não crita")
	}
}

// Chance 30% × i; repõe 25% do dano; o lançamento para no teto que sobrou.
func TestRouboDeMana(t *testing.T) {
	cases := []struct {
		name     string
		intel    int16
		dano     int
		restante int32
		rolls    rolagens
		want     int32
	}{
		{"INT cheia, pega (299)", 3148, 2000, 4000, rolagens{299}, 500},
		{"INT cheia, não pega (300)", 3148, 2000, 4000, rolagens{300}, 0},
		{"INT 1250, pega (149)", 1250, 2000, 4000, rolagens{149}, 500},
		{"só cabe o que sobrou do teto", 3148, 8000, 700, rolagens{0}, 700},
		{"teto já cheio", 3148, 8000, 0, rolagens{}, 0},
		{"golpe que errou", 3148, -3, 4000, rolagens{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.rolls
			if got := rouboDeMana(&r, fmBlack(c.intel, learnedInferno), c.dano, c.restante); got != c.want {
				t.Errorf("mana = %d, want %d", got, c.want)
			}
		})
	}
	e := fmBlack(3148, learnedInferno)
	e.MaxMP = 20_000 // mana máxima efetiva 40.000
	if got := tetoDoRouboDeMana(e); got != 4000 {
		t.Errorf("teto = %d, want 4000 (10%% da mana máxima)", got)
	}
}

// Pelo golpe real: o cajado de 2 mãos sai 40% maior que outra arma.
func TestCajadoDaMagiaNegraNoGolpe(t *testing.T) {
	const cajado, lanca = 900, 901
	d := New(Config{CombatRules: regraSemEscala(), ItemEffects: map[int][]content.BaseEffect{
		cajado: {{Eff: efWType, Val: wtypeCajadoDuasMaos}},
		lanca:  {{Eff: efWType, Val: wtypeLanca}},
	}})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	cast := castInfo{isSkill: true, special: 255, spell: content.Spell{Index: 36, InstanceType: 3, InstanceValue: 200}}
	soma := func(arma int16) int {
		total := 0
		for range 200 {
			fm := fmBlack(3148, learnedInferno)
			fm.Equip[weaponSlotR] = world.Item{Index: arma}
			mob := &world.Entity{ID: world.MaxUser + 1, HP: 100_000, MaxHP: 100_000}
			total += d.resolveSkillHit(w, fm, mob, mob.ID, 36, cast)
		}
		return total
	}
	com, sem := soma(cajado), soma(lanca)
	if razao := float64(com) / float64(sem); razao < 1.35 || razao > 1.45 {
		t.Fatalf("cajado/lança = %.3f (%d/%d), want perto de 1,40", razao, com, sem)
	}
}

// Pelo golpe real: a Nevasca da black crita e devolve mana.
func TestMagiaNegraCritaERoubaManaNoGolpe(t *testing.T) {
	const nevasca = 36
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Black", Class: 1, X: 5, Y: 5,
		HP: 50_000, MaxHP: 50_000, MP: 1000, MaxMP: 20_000, Level: 100, Int: 3148,
		LearnedSkill: 1<<(nevasca%24) | learnedInferno, BaseSpecial: [4]int16{0, 0, 255, 0},
	}
	spells := content.NewSkillData([]content.Spell{{
		Index: nevasca, TargetType: 1, Range: 5, InstanceType: 3, InstanceValue: 200, Aggressive: 1, MaxTarget: 1,
	}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	clock := new(atomic.Uint32)
	clock.Store(serverTime)
	d := New(Config{Log: log, Spells: spells})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, db, d.Handle)
	mid := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Alvo"), X: 6, Y: 5, GenIndex: -1})
	mob := w.Entity(mid)
	mob.HP, mob.MaxHP, mob.Damage = 50_000_000, 50_000_000, 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	c := enterWorld(t, ln.Addr().String())
	defer func() { c.Close(); cancel(); <-done }()

	danos, criticos := golpesNoFio(t, c, clock, 30, mid, nevasca)
	if len(danos) != 30 {
		t.Fatalf("ecos = %d, want 30", len(danos))
	}
	if criticos == 0 {
		t.Error("nenhum crítico em 30 golpes (25% de chance cada)")
	}
	if mp := w.Entity(1).MP; mp < 3000 {
		t.Errorf("MP da black = %d, want o roubo de mana acima de 3.000 (começou em 1.000)", mp)
	}
}

// O Relâmpago automático do Trovão também crita e rouba mana na black.
func TestTrovaoDaMagiaNegra(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala(), Spells: content.NewSkillData([]content.Spell{
		{Index: 33, InstanceType: 5, InstanceValue: 65, MaxTarget: 1, Aggressive: 1},
	})})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	mid := w.SpawnMobAt(world.MobSpawn{Template: plainMobTemplate("Alvo"), X: 4, Y: 4, GenIndex: -1})
	mob := w.Entity(mid)
	rodar := func(learned int32) (dano, mana int32) {
		s := &world.Session{Conn: 1, ReqMp: 1000}
		fm := fmBlack(3148, learned)
		fm.X, fm.Y, fm.HP, fm.MP, fm.MaxMP, fm.Level = 5, 5, 1000, 1000, 20_000, 100
		for range 200 {
			mob.HP, mob.MaxHP = 50_000_000, 50_000_000
			d.applyThunderTick(w, s, fm, 100)
			dano += 50_000_000 - mob.HP
		}
		return dano, fm.MP
	}
	danoSem, manaSem := rodar(0)
	danoCom, manaCom := rodar(learnedInferno)
	if float64(danoCom) < 1.2*float64(danoSem) {
		t.Errorf("dano do Trovão com o Inferno = %d, sem = %d; want o crítico somando", danoCom, danoSem)
	}
	if manaCom <= manaSem {
		t.Errorf("mana no fim com o Inferno = %d, sem = %d; want o roubo de mana", manaCom, manaSem)
	}
}
