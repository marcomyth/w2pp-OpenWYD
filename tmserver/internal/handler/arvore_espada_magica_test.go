package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// tkDaEspadaMagica monta um TK com a INT e a maestria da Espada Mágica.
func tkDaEspadaMagica(intel int16, learned int32, maestria int16) *world.Entity {
	e := &world.Entity{ID: 1, Class: 0, ClassMaster: classMasterMortal, Int: intel, LearnedSkill: learned}
	e.Special[3] = maestria
	return e
}

func TestTkEspadaMagicaExigeTempestadeDeGelo(t *testing.T) {
	tests := []struct {
		name string
		e    *world.Entity
		want bool
	}{
		{"TK com a Tempestade de Gelo", tkDaEspadaMagica(2848, learnedTempestadeDeGelo, 255), true},
		{"TK sem a Tempestade de Gelo", tkDaEspadaMagica(2848, 1<<22, 255), false},
		{"Huntress com o bit 23", &world.Entity{ID: 1, Class: 3, LearnedSkill: learnedTempestadeDeGelo}, false},
		{"monstro com o bit 23", &world.Entity{ID: world.MaxUser, Class: 0, LearnedSkill: learnedTempestadeDeGelo}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tkEspadaMagica(tt.e); got != tt.want {
				t.Fatalf("tkEspadaMagica = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReguaDeInt(t *testing.T) {
	for _, c := range []struct {
		intel int16
		want  int
	}{{0, 0}, {1250, 500}, {2500, 1000}, {2848, 1000}} {
		if got := reguaDeInt(tkDaEspadaMagica(c.intel, learnedTempestadeDeGelo, 255)); got != c.want {
			t.Errorf("INT %d: régua = %d, want %d", c.intel, got, c.want)
		}
	}
}

func TestSkillDeDanoDaEspadaMagica(t *testing.T) {
	for skill := 15; skill <= 24; skill++ {
		want := skill >= 17 && skill <= 23
		if got := skillDeDanoDaEspadaMagica(skill); got != want {
			t.Errorf("skill %d: %v, want %v", skill, got, want)
		}
	}
}

func TestArmaDaEspadaMagica(t *testing.T) {
	const lanca, espada = 855, 3761
	ability := armasNasMaos(map[int16]int{lanca: wtypeLanca, espada: wtypeEspadaDuasMaos})
	for _, c := range []struct {
		nome string
		arma int16
		want int
		// O número da lança sai do botão: o torneio o move, e o que este teste
		// prova é que a árvore paga a lança e mais nada.
	}{{"lança", lanca, espadaMagicaLancaPct}, {"espada de 2 mãos", espada, 100}, {"sem arma", 0, 100}} {
		e := tkDaEspadaMagica(2848, learnedTempestadeDeGelo, 255)
		e.Equip[weaponSlotR] = world.Item{Index: c.arma}
		if got := armaPctEspadaMagica(e, ability); got != c.want {
			t.Errorf("%s: %d%%, want %d%%", c.nome, got, c.want)
		}
	}
}

// Chance 10% + 15% × i; multiplicador de ×2,0 até ×(2 + 2 × i).
func TestCriticoDaEspadaMagica(t *testing.T) {
	cases := []struct {
		name  string
		intel int16
		rolls rolagens
		want  int
	}{
		{"INT cheia, pega (24 < 25), sorte máxima", 2848, rolagens{24, 20}, 40},
		{"INT cheia, pega, sorte mínima", 2848, rolagens{0, 0}, 20},
		{"INT cheia, não pega (25)", 2848, rolagens{25}, 0},
		{"INT 1250, pega (16 < 17), ×3,0", 1250, rolagens{16, 10}, 30},
		{"INT 1250, não pega (17)", 1250, rolagens{17}, 0},
		{"sem INT, pega (9 < 10), só ×2,0", 0, rolagens{9, 0}, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.rolls
			if got := rolarCriticoDeMago(&r, tkDaEspadaMagica(c.intel, learnedTempestadeDeGelo, 255)); got != c.want {
				t.Errorf("multiplicador = %d, want %d", got, c.want)
			}
		})
	}
}

func TestExterminarTudoOuNada(t *testing.T) {
	if r := (rolagens{9}); !exterminarAcerta(&r) {
		t.Error("rolagem 9: want acerto (10%)")
	}
	if r := (rolagens{10}); exterminarAcerta(&r) {
		t.Error("rolagem 10: want erro")
	}
}

// Chance 30% × i em milésimos; cura 25% do dano, até 10% do HP máximo.
func TestRouboDeVida(t *testing.T) {
	cases := []struct {
		name  string
		intel int16
		dano  int
		rolls rolagens
		want  int32
	}{
		{"INT cheia, pega (299)", 2848, 2000, rolagens{299}, 500},
		{"INT cheia, teto de 10% do HP", 2848, 8000, rolagens{0}, 1000},
		{"INT cheia, não pega (300)", 2848, 2000, rolagens{300}, 0},
		{"INT 1250, pega (149)", 1250, 2000, rolagens{149}, 500},
		{"INT 1250, não pega (150)", 1250, 2000, rolagens{150}, 0},
		{"golpe que errou", 2848, -3, rolagens{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := tkDaEspadaMagica(c.intel, learnedTempestadeDeGelo, 255)
			e.MaxHP = 5000 // HP máximo efetivo 10.000
			r := c.rolls
			if got := rouboDeVida(&r, e, c.dano); got != c.want {
				t.Errorf("cura = %d, want %d", got, c.want)
			}
		})
	}
}

// Possuído (14) e Samaritano (24) dão +500 de CON cada no TK Espada Mágica, e somam.
func TestConDaEspadaMagica(t *testing.T) {
	buffs := func(learned int32) *world.Entity {
		e := tkDaEspadaMagica(2848, learned, 255)
		e.MaxHP = 5000
		e.Affect[0] = world.Affect{Type: 14, Level: 100, Time: 50}
		e.Affect[1] = world.Affect{Type: affectSamaritano, Level: 100, Time: 50}
		applyAffectScoreWithItemAbility(e, nil)
		return e
	}
	com, sem := buffs(learnedTempestadeDeGelo), buffs(1<<22)
	if got := com.AffCon - sem.AffCon; got != 1000 {
		t.Errorf("CON a mais = %d, want 1000 (500 de cada buff)", got)
	}
	if got := com.AffMaxHP - sem.AffMaxHP; got != 2000 {
		t.Errorf("HP a mais = %d, want 2000", got)
	}
	metade := tkDaEspadaMagica(2848, learnedTempestadeDeGelo, 102)
	if got := conDaEspadaMagica(metade); got != 200 {
		t.Errorf("maestria 102: CON = %d, want 200", got)
	}
}

// espadaMagicaNoFio sobe um servidor com um TK Espada Mágica full INT e um
// monstro de muita vida ao lado.
func espadaMagicaNoFio(t *testing.T, skill int, hp, maxHP int32) (net.Conn, *world.World, int, *atomic.Uint32, func()) {
	t.Helper()
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "TKMago", Class: 0, X: 5, Y: 5,
		HP: hp, MaxHP: maxHP, MP: 20_000, MaxMP: 20_000, Level: 100, Int: 2500,
		LearnedSkill: 1<<skill | learnedTempestadeDeGelo, BaseSpecial: [4]int16{0, 0, 0, 255},
	}
	spells := content.NewSkillData([]content.Spell{{
		Index: skill, TargetType: 1, Range: 5, InstanceType: 1, InstanceValue: 65, Aggressive: 1, MaxTarget: 1,
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
	// The Exterminar knocks its target a cell away; face 219 stays put (applyExterminarMotion).
	mob.Equip[0].Index = 219
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	c := enterWorld(t, ln.Addr().String())
	return c, w, mid, clock, func() { c.Close(); cancel(); <-done }
}

// golpesNoFio lança a skill n vezes e devolve o dano e o DoubleCritical de cada eco.
func golpesNoFio(t *testing.T, c net.Conn, clock *atomic.Uint32, n, alvo, skill int) (danos []int32, criticos int) {
	t.Helper()
	for i := range n {
		clock.Store(serverTime + uint32(i)*1000)
		skillAttackFrame(t, c, serverTime+uint32(i)*1000, alvo, skill, -1)
		for {
			ty, p, ok := readMaybe(t, c)
			if !ok {
				t.Fatal("sem eco do ataque")
			}
			if ty != protocol.MsgAttack {
				continue
			}
			var body protocol.MsgAttackBody
			if err := body.Decode(p); err != nil {
				t.Fatal(err)
			}
			if len(body.Dam) == 1 {
				danos = append(danos, body.Dam[0].Damage)
			}
			if body.DoubleCritical&2 != 0 {
				criticos++
			}
			break
		}
	}
	return danos, criticos
}

// Pelo golpe real: as skills da árvore critam e roubam vida.
func TestEspadaMagicaCritaERoubaVidaNoGolpe(t *testing.T) {
	const skillAtaqueDaAlma = 20
	c, w, mid, clock, fechar := espadaMagicaNoFio(t, skillAtaqueDaAlma, 1000, 50_000)
	defer fechar()
	danos, criticos := golpesNoFio(t, c, clock, 30, mid, skillAtaqueDaAlma)
	if len(danos) != 30 {
		t.Fatalf("ecos = %d, want 30", len(danos))
	}
	if criticos == 0 {
		t.Error("nenhum crítico em 30 golpes (25% de chance cada)")
	}
	// Dentro do laço: o mundo é de dono único e ler a ficha da goroutine do teste
	// corre com o desmonte da sessão. A asserção é a mesma.
	var hp int32
	noLaco(t, w, func(w *world.World) { hp = w.Entity(1).HP })
	if hp < 4000 {
		t.Errorf("HP do TK = %d, want o roubo de vida acima de 4.000 (começou em 1.000)", hp)
	}
}

// Pelo golpe real: o Exterminar só acerta 10% das vezes, contra um alvo que não
// esquiva.
func TestExterminarErraNoGolpe(t *testing.T) {
	c, _, mid, clock, fechar := espadaMagicaNoFio(t, skillExterminar, 50_000, 50_000)
	defer fechar()
	danos, _ := golpesNoFio(t, c, clock, 40, mid, skillExterminar)
	acertos, lancados := 0, 0
	for _, d := range danos {
		if d != 0 {
			lancados++
		}
		if d > 0 {
			acertos++
		}
	}
	if lancados < 35 {
		t.Fatalf("só %d de 40 Exterminar chegaram ao alvo", lancados)
	}
	if acertos > 15 {
		t.Errorf("acertos = %d de %d, want perto de 10%%", acertos, len(danos))
	}
	if acertos == len(danos) {
		t.Error("todo Exterminar acertou")
	}
}

// Pelo golpe real: com lança a skill da árvore sai 40% maior que com outra arma.
func TestLancaDaEspadaMagicaNoGolpe(t *testing.T) {
	const lanca, espada = 900, 901
	d := New(Config{CombatRules: regraSemEscala(), ItemEffects: map[int][]content.BaseEffect{
		lanca:  {{Eff: efWType, Val: wtypeLanca}},
		espada: {{Eff: efWType, Val: wtypeEspadaDuasMaos}},
	}})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	cast := castInfo{isSkill: true, special: 255, spell: content.Spell{Index: 20, InstanceType: 1, InstanceValue: 65}}
	soma := func(arma int16) int {
		total := 0
		for range 200 {
			tk := tkDaEspadaMagica(2500, learnedTempestadeDeGelo, 255)
			tk.Equip[weaponSlotR] = world.Item{Index: arma}
			mob := &world.Entity{ID: world.MaxUser + 1, HP: 100_000, MaxHP: 100_000}
			total += d.resolveSkillHit(w, tk, mob, mob.ID, 20, cast)
		}
		return total
	}
	com, sem := soma(lanca), soma(espada)
	// A razão esperada sai do botão, que o torneio move; a margem é do sorteio de
	// dano, que é o mesmo dos dois lados.
	quer := float64(espadaMagicaLancaPct) / float64(espadaMagicaArmaPct)
	if razao := float64(com) / float64(sem); razao < quer*0.95 || razao > quer*1.05 {
		t.Fatalf("lança/espada = %.3f (%d/%d), want perto de %.2f", razao, com, sem, quer)
	}
}
