package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// invisSpell is the SkillData row of skill 95 as far as the rule reads it: a
// self buff installing affect 28.
var invisSpell = content.Spell{
	Index: skillInvisibilidade, ManaSpent: 10, AffectType: affectInvisibilidade,
	AffectValue: 1, AffectTime: 0, MaxTarget: 1, Name: "Invisibilidade",
}

func TestChanceGolpeFurtivoX4(t *testing.T) {
	cases := []struct {
		name     string
		str, dex int
		want     int
	}{
		{"força igual à destreza é build de destreza", 1000, 1000, 0},
		{"destreza maior", 500, 2000, 0},
		{"força 1200 / destreza 1000", 1200, 1000, 9},
		{"força 1500 / destreza 1000", 1500, 1000, 20},
		{"força 2000 / destreza 1000", 2000, 1000, 33},
		{"1600 / 500 já é força pura", 1600, 500, 50},
		{"força 3000 / destreza 300", 3000, 300, 50},
		{"destreza zero", 1000, 0, 50},
		{"destreza negativa não passa do teto", 1000, -500, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chanceGolpeFurtivoX4(c.str, c.dex); got != c.want {
				t.Errorf("chanceGolpeFurtivoX4(%d, %d) = %d, want %d", c.str, c.dex, got, c.want)
			}
		})
	}
}

// rolagemFixa is a combat.Rand that always rolls the same number.
type rolagemFixa int

func (r rolagemFixa) Intn(int) int { return int(r) }

// semRolagem fails the test if the sorteio is consulted at all.
type semRolagem struct{ t *testing.T }

func (r semRolagem) Intn(int) int {
	r.t.Fatal("a build de destreza não deveria sortear")
	return 0
}

func TestRolarGolpeFurtivo(t *testing.T) {
	// Força 2000 / Destreza 1000: X4 33%, X3 30%, X2 37%.
	cases := []struct {
		roll int
		want int
	}{
		{0, 4}, {32, 4}, {33, 3}, {62, 3}, {63, 2}, {99, 2},
	}
	for _, c := range cases {
		if got := rolarGolpeFurtivo(rolagemFixa(c.roll), 2000, 1000); got != c.want {
			t.Errorf("roll %d → X%d, want X%d", c.roll, got, c.want)
		}
	}
	if got := rolarGolpeFurtivo(semRolagem{t}, 800, 1200); got != 2 {
		t.Errorf("build de destreza → X%d, want X2", got)
	}
}

func TestInvisRecarga(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	s := &world.Session{AccountID: 7, Slot: 2}

	if falta := d.invisRecargaRestante(s, 5000); falta != 0 {
		t.Fatalf("sem cast nenhum, recarga = %d ms, want 0", falta)
	}
	d.marcarRecargaInvis(s, 1000)
	for _, c := range []struct {
		now  uint32
		want uint32
	}{
		{1000, 80_000}, {80_999, 1}, {81_000, 0}, {200_000, 0},
	} {
		if got := d.invisRecargaRestante(s, c.now); got != c.want {
			t.Errorf("now %d: recarga = %d ms, want %d", c.now, got, c.want)
		}
	}
	// Outro slot da mesma conta é outro personagem.
	if falta := d.invisRecargaRestante(&world.Session{AccountID: 7, Slot: 3}, 1000); falta != 0 {
		t.Errorf("outro personagem herdou a recarga: %d ms", falta)
	}
	// O relógio do servidor é uint32 em ms e dá a volta em 49 dias.
	d.marcarRecargaInvis(s, 0xFFFF_F000)
	if got := d.invisRecargaRestante(s, 0x0000_1000); got != 80_000-0x2000 {
		t.Errorf("na volta do relógio: recarga = %d ms, want %d", got, 80_000-0x2000)
	}
	if got := textoRecargaInvis(1); got != "Invisibilidade em recarga: faltam 1 s." {
		t.Errorf("texto = %q", got)
	}
}

// newInvisWorld is a world with a controllable clock and a Huntress (no socket)
// that has just cast Invisibilidade.
func newInvisWorld(t *testing.T) (*Dispatcher, *world.World, *atomic.Uint32, *world.Entity) {
	t.Helper()
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
	e := &world.Entity{ID: 1, Class: 3, Str: 3000, Dex: 1000}
	d.applyCastAffect(w, e, e, e.ID, castInfo{isSkill: true, spell: invisSpell})
	if !e.HasAffect(affectInvisibilidade) || !e.InvisivelAtiva || e.Rsv&world.RsvHide == 0 {
		t.Fatalf("cast não deixou a HT invisível: affect=%v ativa=%v rsv=%#x",
			e.HasAffect(affectInvisibilidade), e.InvisivelAtiva, e.Rsv)
	}
	return d, w, clock, e
}

func TestInvisibilidadeDuraDezSegundos(t *testing.T) {
	d, w, _, e := newInvisWorld(t)

	for i := range e.Affect {
		if e.Affect[i].Type == affectInvisibilidade && e.Affect[i].Time != invisIconTicks {
			t.Errorf("slot do afeto com Time %d, want %d", e.Affect[i].Time, invisIconTicks)
		}
	}
	d.expirarInvisibilidade(w, e, serverTime+invisDuracaoMs-1)
	if !e.HasAffect(affectInvisibilidade) {
		t.Fatal("a invisibilidade acabou antes dos 10 s")
	}
	d.expirarInvisibilidade(w, e, serverTime+invisDuracaoMs)
	if e.HasAffect(affectInvisibilidade) || e.InvisivelAtiva || e.Rsv&world.RsvHide != 0 {
		t.Fatalf("aos 10 s ainda invisível: affect=%v ativa=%v rsv=%#x",
			e.HasAffect(affectInvisibilidade), e.InvisivelAtiva, e.Rsv)
	}
}

// The affect can come back from a save without the clock that ends it.
func TestInvisibilidadeSemRelogioERevelada(t *testing.T) {
	d, w, _, e := newInvisWorld(t)
	e.InvisivelAtiva = false

	d.expirarInvisibilidade(w, e, serverTime)
	if e.HasAffect(affectInvisibilidade) {
		t.Fatal("afeto sem relógio continuou valendo")
	}
}

func TestSairDaInvisibilidadeAoAtacar(t *testing.T) {
	agressiva := castInfo{isSkill: true, spell: content.Spell{Index: 88, InstanceType: 1, Aggressive: 1}}
	buff := castInfo{isSkill: true, spell: content.Spell{Index: 77, AffectType: 21}}
	cases := []struct {
		name         string
		cast         castInfo
		dex          int16
		wantMult     int
		wantRevelada bool
	}{
		{"ataque físico de build de destreza sai X2 e revela", castInfo{}, 3000, 2, true},
		{"skill agressiva revela sem multiplicar", agressiva, 1000, 0, true},
		{"buff não revela", buff, 1000, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, w, _, e := newInvisWorld(t)
			e.Dex = c.dex

			if got := d.sairDaInvisibilidadeAoAtacar(w, e, c.cast); got != c.wantMult {
				t.Errorf("multiplicador = %d, want %d", got, c.wantMult)
			}
			if revelada := !e.HasAffect(affectInvisibilidade); revelada != c.wantRevelada {
				t.Errorf("revelada = %v, want %v", revelada, c.wantRevelada)
			}
		})
	}
	t.Run("quem não está invisível não sorteia", func(t *testing.T) {
		d := New(Config{CombatRules: regraSemEscala()})
		w := world.New(world.Config{GridDim: 16}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil)
		e := &world.Entity{ID: 1, Class: 3, Str: 3000}
		if got := d.sairDaInvisibilidadeAoAtacar(w, e, castInfo{}); got != 0 {
			t.Errorf("multiplicador = %d, want 0", got)
		}
	})
}

// startServerInvis serves one Huntress next to a punching bag, with skill 95 in
// the catalog. Str and Dex are both 0, so the sneak attack is always X2 and never
// touches the RNG — which is what lets two servers be compared blow for blow.
func startServerInvis(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	db := skillCombatDB(1 << (skillInvisibilidade % content.MaxSkill))
	db.loadResult.Class = 3
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Spells: content.NewSkillData([]content.Spell{invisSpell}), CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16, Now: relogioEmServerTime()}, log, db, d.Handle)
	w.SpawnMob(punchingBagMob(), 6, 5)
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

func meleeEcho(t *testing.T, c net.Conn, tick uint32) (int32, [][]byte) {
	t.Helper()
	skillAttackFrame(t, c, tick, world.MaxUser, -1, damMelee)
	payload, antes := readUntil(t, c, protocol.MsgAttack)
	var body protocol.MsgAttackBody
	if err := body.Decode(payload); err != nil {
		t.Fatal(err)
	}
	return body.Dam[0].Damage, antes
}

// TestGolpeFurtivoPeloSocket runs the whole attack handler: the same blow on two
// identical servers, once plain and once out of Invisibilidade, must differ by
// exactly the multiplier; and a second cast inside the 80 s is refused aloud.
func TestGolpeFurtivoPeloSocket(t *testing.T) {
	addrA, stopA := startServerInvis(t)
	defer stopA()
	a := enterWorld(t, addrA)
	defer a.Close()
	normal, _ := meleeEcho(t, a, serverTime)
	if normal <= 0 {
		t.Fatalf("golpe normal = %d, want > 0", normal)
	}

	addrB, stopB := startServerInvis(t)
	defer stopB()
	b := enterWorld(t, addrB)
	defer b.Close()

	skillAttackFrame(t, b, serverTime, 1, skillInvisibilidade, damSkill)
	readUntil(t, b, protocol.MsgAttack)

	furtivo, antes := meleeEcho(t, b, serverTime+1000)
	if furtivo != 2*normal {
		t.Fatalf("golpe saindo da invisibilidade = %d, want %d (2 × %d)", furtivo, 2*normal, normal)
	}
	if !algumTexto(antes, "Golpe furtivo: dano x2.") {
		t.Error("o jogador não foi avisado do multiplicador")
	}

	// The next blow is plain again: the bonus is spent.
	if depois, _ := meleeEcho(t, b, serverTime+2000); depois >= furtivo {
		t.Errorf("golpe seguinte = %d, want menor que o furtivo %d", depois, furtivo)
	}

	skillAttackFrame(t, b, serverTime+3000, 1, skillInvisibilidade, damSkill)
	for i := 0; i < 16; i++ {
		ty, p, ok := readMaybe(t, b)
		if !ok {
			t.Fatal("o cast em recarga não respondeu nada")
		}
		if ty == protocol.MsgAttack {
			t.Fatal("o cast em recarga foi aceito")
		}
		if ty == protocol.MsgMessagePanel && strings.Contains(string(p), "recarga") {
			return
		}
	}
	t.Fatal("nenhum aviso de recarga")
}

func algumTexto(frames [][]byte, texto string) bool {
	for _, f := range frames {
		if strings.Contains(string(f), texto) {
			return true
		}
	}
	return false
}
