package handler

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// rolagens is a combat.Rand that hands out the given rolls in order.
type rolagens []int

func (r *rolagens) Intn(int) int {
	v := (*r)[0]
	*r = (*r)[1:]
	return v
}

// ht builds a Huntress with the given attributes and learned bits.
func ht(str, dex int16, learned int32) *world.Entity {
	return &world.Entity{ID: 1, Class: 3, Str: str, Dex: dex, LearnedSkill: learned}
}

// ---- 80 · Golpe Felino ---------------------------------------------------------

func TestChanceCriticoGolpeFelino(t *testing.T) {
	cases := []struct {
		str, dex int
		want     int
	}{
		{1600, 500, 30}, // Força pura
		{1000, 1000, 20},
		{500, 1600, 10}, // Destreza pura
	}
	for _, c := range cases {
		if got := chanceCriticoGolpeFelino(c.str, c.dex); got != c.want {
			t.Errorf("chance(%d, %d) = %d, want %d", c.str, c.dex, got, c.want)
		}
	}
}

func TestRolarCriticoGolpeFelino(t *testing.T) {
	cases := []struct {
		name  string
		rolls rolagens
		want  int
	}{
		{"acerta o crítico no menor multiplicador", rolagens{29, 0}, 20},
		{"acerta o crítico no maior multiplicador", rolagens{0, 10}, 30},
		{"fora da chance não há crítico", rolagens{30}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.rolls
			if got := rolarCriticoGolpeFelino(&r, 1600, 500); got != c.want {
				t.Errorf("multiplicador = %d, want %d", got, c.want)
			}
		})
	}
}

// ---- 81 · Ligação Espectral ----------------------------------------------------

func TestLigacaoEspectralMultiplicaODano(t *testing.T) {
	cases := []struct {
		level int32
		want  int32
	}{
		{0, 101}, {100, 102}, {200, 103}, {255, 104},
	}
	for _, c := range cases {
		e := ht(1000, 1000, 0)
		e.Special[2] = 150
		e.Affect[0] = world.Affect{Type: 37, Level: uint16(c.level)}
		applyAffectScore(e)
		if e.AffDamageMultiPct != c.want {
			t.Errorf("maestria %d: multiplicador = %d, want %d", c.level, e.AffDamageMultiPct, c.want)
		}
		if e.AffForceDamage != 150 {
			t.Errorf("maestria %d: dano somado = %d, want 150 (a regra antiga fica)", c.level, e.AffForceDamage)
		}
	}
}

// ---- 83 · Extração → Melhoria na Esquiva ---------------------------------------

func TestMelhoriaNaEsquiva(t *testing.T) {
	cases := []struct {
		name string
		e    *world.Entity
		want int
	}{
		{"sem a skill", ht(1000, 1000, 0), 0},
		{"destreza pura", ht(500, 1600, learnedExtracao), 40},
		{"meio a meio", ht(1000, 1000, learnedExtracao), 60},
		{"força pura", ht(1600, 500, learnedExtracao), 80},
		{"força pura com a 8ª", ht(1600, 500, learnedExtracao|learnedTrocaDeEspirito), 110},
		{"destreza pura com a 8ª", ht(500, 1600, learnedExtracao|learnedTrocaDeEspirito), 70},
		{"o mesmo bit em outra classe é outra skill", &world.Entity{Class: 0, Str: 1600, Dex: 500, LearnedSkill: learnedExtracao}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := melhoriaNaEsquiva(c.e); got != c.want {
				t.Errorf("esquiva = %d, want %d", got, c.want)
			}
		})
	}
	if got := esquivaComMelhoria(600, ht(1600, 500, learnedExtracao|learnedTrocaDeEspirito)); got != esquivaTeto {
		t.Errorf("com o teto: %d, want %d", got, esquivaTeto)
	}
}

// The bonus reaches the roll every attack path reads, not only a helper.
func TestMelhoriaNaEsquivaEntraNoSorteio(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	atacante := &world.Entity{ID: 2, Dex: 100}
	sem := ht(1600, 500, 0)
	com := ht(1600, 500, learnedExtracao)

	if got, base := d.parryRate(atacante, com), d.parryRate(atacante, sem); got != base+80 {
		t.Errorf("golpe físico: esquiva %d, want %d (+80)", got, base+80)
	}
	if got, base := d.skillParryRate(atacante, com), d.skillParryRate(atacante, sem); got != base+80 {
		t.Errorf("skill: esquiva %d, want %d (+80)", got, base+80)
	}
	if got, base := d.monsterParryRate(atacante, com), d.monsterParryRate(atacante, sem); got != base+80 {
		t.Errorf("golpe de monstro: esquiva %d, want %d (+80)", got, base+80)
	}
}

func TestExtracaoEAlquimiaNaoSeLancam(t *testing.T) {
	d := New(Config{Spells: content.NewSkillData([]content.Spell{
		{Index: skillExtracao, ManaSpent: 30}, {Index: skillAlquimia, ManaSpent: 30},
	})})
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	s := &world.Session{Conn: 1}
	e := ht(1000, 1000, 1<<(skillExtracao%content.MaxSkill)|1<<(skillAlquimia%content.MaxSkill))
	for _, skill := range []int{skillExtracao, skillAlquimia} {
		if _, ok := d.validateCast(w, s, e, skill, 1000); ok {
			t.Errorf("skill %d foi lançada; virou passiva", skill)
		}
	}
}

// ---- 85 · Escudo Dourado -------------------------------------------------------

func TestEscudoDouradoNaoCobraOuro(t *testing.T) {
	d := New(Config{Spells: content.NewSkillData([]content.Spell{
		{Index: skillEscudoDourado, ManaSpent: 120, AffectType: affectEscudoDourado, AffectValue: 150, AffectTime: 7},
	})})
	w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	s := &world.Session{Conn: 1}
	e := ht(1000, 1000, 1<<(skillEscudoDourado%content.MaxSkill))
	e.Special[2] = 200

	if _, ok := d.validateCast(w, s, e, skillEscudoDourado, 1000); !ok {
		t.Fatal("Escudo Dourado recusado sem ouro nenhum")
	}
	if e.Coin != 0 {
		t.Errorf("ouro = %d, want 0", e.Coin)
	}
}

func TestDefesaEscudoDourado(t *testing.T) {
	cases := []struct {
		name  string
		e     *world.Entity
		level int32
		want  int32
	}{
		{"destreza pura, maestria 200", ht(500, 1600, 0), 200, 228},
		{"meio a meio, maestria 200", ht(1000, 1000, 0), 200, 303},
		{"força pura, maestria 200", ht(1600, 500, 0), 200, 378},
		{"força pura, maestria 255, com a 8ª", ht(1600, 500, learnedTrocaDeEspirito), 255, 500},
		{"maestria acima de 255 conta 255", ht(1600, 500, learnedTrocaDeEspirito), 400, 500},
		{"destreza pura, maestria 255, com a 8ª", ht(500, 1600, learnedTrocaDeEspirito), 255, 350},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.e.Affect[0] = world.Affect{Type: affectEscudoDourado, Value: 150, Level: uint16(c.level)}
			applyAffectScore(c.e)
			if c.e.AffAC != c.want {
				t.Errorf("defesa = %d, want %d", c.e.AffAC, c.want)
			}
		})
	}
}

// ---- 86 · Explosão Etérea ------------------------------------------------------

func startServerExplosao(t *testing.T, mobs [][2]int16) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Huntress", Class: 3, X: 5, Y: 5,
		HP: 1000, MaxHP: 1000, MP: 500, MaxMP: 500,
		Level: 50, Str: 100, Damage: 200, LearnedSkill: 1 << (skillExplosaoEterea % content.MaxSkill),
	}
	spells := content.NewSkillData([]content.Spell{{Index: skillExplosaoEterea, TargetType: 5, ManaSpent: 25,
		Range: 6, InstanceType: 1, InstanceValue: 35, Aggressive: 1, MaxTarget: 10, Name: "Explosao_Eterea"}})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Spells: spells, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 32, Now: relogioEmServerTime()}, log, db, d.Handle)
	for _, m := range mobs {
		w.SpawnMob(punchingBagMob(), m[0], m[1])
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	}
}

// TestExplosaoEtereaCompletaOsAlvos: the client names one monster; the server
// adds the three others within 3 cells of it, leaves out the one 5 cells away,
// and every added monster takes damage.
func TestExplosaoEtereaCompletaOsAlvos(t *testing.T) {
	principal := world.MaxUser // first spawn
	addr, stop := startServerExplosao(t, [][2]int16{
		{8, 5},   // principal, 3 casas da HT
		{9, 5},   // extra
		{10, 7},  // extra (raio 2)
		{11, 8},  // extra (raio 3)
		{13, 10}, // fora do raio (5 casas do principal)
	})
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	skillAttackFrame(t, c, serverTime, principal, skillExplosaoEterea, damSkill)
	payload, _ := readUntil(t, c, protocol.MsgAttack)
	var got protocol.MsgAttackBody
	if err := got.Decode(payload); err != nil {
		t.Fatal(err)
	}
	if len(got.Dam) != protocol.MaxTarget {
		t.Fatalf("o eco trouxe %d entradas, want %d", len(got.Dam), protocol.MaxTarget)
	}
	feridos := map[int32]int32{}
	for _, dam := range got.Dam {
		if dam.TargetID > 0 {
			feridos[dam.TargetID] = dam.Damage
		}
	}
	for i := 0; i < 4; i++ {
		id := int32(world.MaxUser + i)
		if feridos[id] <= 0 {
			t.Errorf("monstro %d: dano %d, want > 0 (entradas %v)", id, feridos[id], feridos)
		}
	}
	if _, ok := feridos[int32(world.MaxUser+4)]; ok {
		t.Errorf("o monstro fora do raio entrou na explosão: %v", feridos)
	}
}

// TestExplosaoEtereaAlcanceDez: a main target 9 cells away is in reach now (the
// CSV range is 6).
func TestExplosaoEtereaAlcanceDez(t *testing.T) {
	addr, stop := startServerExplosao(t, [][2]int16{{14, 5}})
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	if dmg := attackEchoDamage(t, c, serverTime, world.MaxUser, skillExplosaoEterea, damSkill); dmg <= 0 {
		t.Fatalf("alvo a 9 casas: dano %d, want > 0", dmg)
	}
}

var _ combat.Rand = (*rolagens)(nil)

// TestGolpeFelinoCriticoNoAtaque: through the real attack handler a Força-pure
// Huntress (160 / 50 is past 3× Destreza) lands Golpe Felino criticals — flagged on the wire and at least twice
// the plain blow.
func TestGolpeFelinoCriticoNoAtaque(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Huntress", Class: 3, X: 5, Y: 5,
		HP: 1000, MaxHP: 1000, MP: 5000, MaxMP: 5000,
		Level: 50, Str: 160, Dex: 50, Damage: 200, LearnedSkill: 1 << (skillGolpeFelino % content.MaxSkill),
	}
	spells := content.NewSkillData([]content.Spell{{Index: skillGolpeFelino, TargetType: 1, ManaSpent: 10,
		Range: 2, InstanceType: 1, InstanceValue: 30, Aggressive: 1, MaxTarget: 1, Name: "Golpe_Felino"}})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Spells: spells, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16, Now: relogioEmServerTime()}, log, db, d.Handle)
	saco := punchingBagMob()
	binary.LittleEndian.PutUint32(saco[92+16:], 2_000_000) // MaxHp: the blows must not kill it
	binary.LittleEndian.PutUint32(saco[92+24:], 2_000_000) // Hp
	w.SpawnMob(saco, 6, 5)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	defer func() { cancel(); <-done }()
	c := enterWorld(t, ln.Addr().String())
	defer c.Close()

	menorNormal, maiorCritico, criticos := int32(1<<30), int32(0), 0
	for i := 0; i < 18; i++ {
		// 800 ms apart: the attack cadence, and 18 of them stay inside the 15 s the
		// server lets a ClientTick run ahead of its clock.
		skillAttackFrame(t, c, serverTime+uint32(i)*attackCadence, world.MaxUser, skillGolpeFelino, damSkill)
		payload, _ := readUntil(t, c, protocol.MsgAttack)
		var got protocol.MsgAttackBody
		if err := got.Decode(payload); err != nil {
			t.Fatal(err)
		}
		dmg := got.Dam[0].Damage
		if dmg <= 0 {
			continue
		}
		if got.DoubleCritical&2 != 0 {
			criticos++
			maiorCritico = max(maiorCritico, dmg)
		} else {
			menorNormal = min(menorNormal, dmg)
		}
	}
	if criticos == 0 {
		t.Fatal("18 golpes de Força pura (30% de chance) sem nenhum crítico")
	}
	if maiorCritico < 2*menorNormal*9/10 {
		t.Errorf("maior crítico %d, want pelo menos ~2× o golpe normal (%d)", maiorCritico, menorNormal)
	}
}
