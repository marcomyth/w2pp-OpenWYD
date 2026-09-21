package handler

import (
	"log/slog"
	"math/rand"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

func htSobrevivencia(str, dex int16, learned int32) *world.Entity {
	return &world.Entity{ID: 1, Class: 3, Str: str, Dex: dex, LearnedSkill: learned}
}

func TestAgressividadeArcoSobeComAOitava(t *testing.T) {
	tests := []struct {
		name    string
		learned int32
		nUnique int
		want    int32
	}{
		// 0,68 × 1000 + 0,72 × 1000
		{"arco sem a 8ª", 1 << 2, uniqueArco, 1400},
		// 1,02 × 1000 + 1,08 × 1000
		{"arco com a 8ª", 1<<2 | learnedTempestade, uniqueArco, 2100},
		// 0,49 × 1000 + 0,53 × 1000: a garra não sobe
		{"garra com a 8ª", 1<<2 | learnedTempestade, 43, 1020},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := htSobrevivencia(1000, 1000, tt.learned)
			if got := bonusAgressividade(e, tt.nUnique); got != tt.want {
				t.Fatalf("bonusAgressividade = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAgressividadeArcoComOitavaSoNaHuntress(t *testing.T) {
	e := htSobrevivencia(1000, 1000, 1<<2|learnedTempestade)
	e.Class = 2 // o bit 7 da BM é outra skill
	if temOitavaDaSobrevivencia(e) {
		t.Fatal("bit 7 de outra classe contou como a 8ª da Sobrevivência")
	}
}

func TestEncantarGeloArcoEGarra(t *testing.T) {
	for wtype, want := range map[int]bool{wtypeArco: true, wtypeGarra: true, 1: false, 11: false, 102: false} {
		if got := armaDoEncantarGelo(wtype); got != want {
			t.Errorf("armaDoEncantarGelo(%d) = %v, want %v", wtype, got, want)
		}
	}
}

func TestMeditacaoDano(t *testing.T) {
	tests := []struct {
		name     string
		str, dex int16
		learned  int32
		level    int32
		want     int32
	}{
		{"sem a 8ª, maestria 255: legado", 3000, 0, 0, 255, 40},
		{"sem a 8ª, maestria 80: legado", 3000, 0, 0, 80, 23},
		{"com a 8ª, Força pura, maestria 255", 1600, 500, learnedTempestade, 255, 55},
		{"com a 8ª, Destreza pura, maestria 255", 500, 1600, learnedTempestade, 255, 40},
		{"com a 8ª, meio a meio, maestria 255", 1000, 1000, learnedTempestade, 255, 47},
		{"com a 8ª, maestria 0", 1600, 500, learnedTempestade, 0, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := htSobrevivencia(tt.str, tt.dex, tt.learned)
			if got := danoMeditacaoPct(e, tt.level, 15); got != tt.want {
				t.Fatalf("danoMeditacaoPct = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMeditacaoNoScoreComAOitava(t *testing.T) {
	e := htSobrevivencia(1600, 500, learnedTempestade)
	e.AC = 100
	e.Affect[0] = world.Affect{Type: affectMeditacao, Value: 15, Level: 255}
	applyAffectScore(e)
	// 155 é a Meditação; o resto é o BOTÃO de dano físico da 8ª, que entra no
	// mesmo multiplicador (arvore_troca.go). Ler o botão em vez de um número
	// fixo deixa o teste sobreviver ao próximo ajuste de balanceamento.
	quer := int32(155 + sobrevivenciaDanoFisicoOitava)
	if e.AffAC != -(255/3+10) || e.AffDamageMultiPct != quer {
		t.Fatalf("AffAC/Multi = %d/%d, want %d/%d", e.AffAC, e.AffDamageMultiPct, -(255/3 + 10), quer)
	}
}

func TestLancaDeFerroPerfuracao(t *testing.T) {
	tests := []struct {
		name     string
		str, dex int16
		learned  int32
		want     int
	}{
		{"sem a skill", 500, 2000, 0, 0},
		{"Destreza", 500, 2000, learnedLancaDeFerro, 15},
		{"Força", 2000, 500, learnedLancaDeFerro, 10},
		{"teto", 3000, 3000, learnedLancaDeFerro, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := htSobrevivencia(tt.str, tt.dex, tt.learned)
			if got := perfuracaoLancaDeFerro(e); got != tt.want {
				t.Fatalf("perfuracaoLancaDeFerro = %d, want %d", got, tt.want)
			}
		})
	}
	d := New(Config{})
	e := htSobrevivencia(500, 2000, learnedLancaDeFerro)
	if got := d.defesaPerfurada(e, 1000); got != 850 {
		t.Fatalf("defesaPerfurada = %d, want 850", got)
	}
	e.Class = 0
	if got := d.defesaPerfurada(e, 1000); got != 1000 {
		t.Fatalf("defesaPerfurada de TK = %d, want 1000 (o bit 6 é outra skill)", got)
	}
}

// A Lança de Ferro entra no golpe de skill: a mesma skill contra a mesma defesa
// sai maior com a perfuração.
func TestLancaDeFerroNoGolpeDeSkill(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	cast := castInfo{isSkill: true, spell: content.Spell{InstanceType: 1, InstanceValue: 30}}
	target := &world.Entity{ID: 2, AC: 2000}
	golpe := func(learned int32) int {
		w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
		caster := &world.Entity{ID: 1, Class: 3, Level: 200, Str: 500, Dex: 3000, LearnedSkill: learned}
		return d.resolveSkillHit(w, caster, target, target.ID, 72, cast)
	}
	sem, com := golpe(0), golpe(learnedLancaDeFerro)
	if com <= sem {
		t.Fatalf("com Lança de Ferro = %d, sem = %d; a perfuração não entrou", com, sem)
	}
}

func TestTetoFlecha(t *testing.T) {
	tests := []struct {
		str, dex int
		want     int
	}{
		{500, 1600, 1},  // Destreza pura
		{1000, 1000, 3}, // meio a meio
		{1600, 500, 5},  // Força pura
	}
	for _, tt := range tests {
		if got := tetoFlecha(tt.str, tt.dex); got != tt.want {
			t.Errorf("tetoFlecha(%d, %d) = %d, want %d", tt.str, tt.dex, got, tt.want)
		}
	}
}

// Cada flecha respeita o teto, e os pesos saem na proporção 40/30/17/9/4.
func TestRolarFlechaDistribuicao(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	const n = 200_000
	var conta [6]int
	for range n {
		m := rolarFlecha(r, 5)
		if m < 1 || m > 5 {
			t.Fatalf("multiplicador %d fora de 1-5", m)
		}
		conta[m]++
	}
	for i, peso := range pesosFlecha {
		got := float64(conta[i+1]) * 100 / n
		if got < float64(peso)-1 || got > float64(peso)+1 {
			t.Errorf("%dx saiu em %.1f%%, want ~%d%%", i+1, got, peso)
		}
	}
	for range 10_000 {
		if m := rolarFlecha(r, 3); m > 3 {
			t.Fatalf("teto 3 sorteou %dx", m)
		}
	}
	if m := rolarFlecha(r, 1); m != 1 {
		t.Fatalf("teto 1 sorteou %dx", m)
	}
}

// Duas flechas 5x no mesmo cast têm de ser raras (~1,5% na Força pura).
func TestTempestadeDoisCincoXRaro(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	const casts = 100_000
	dois := 0
	for range casts {
		cinco := 0
		for range tempestadeFlechas {
			if rolarFlecha(r, 5) == 5 {
				cinco++
			}
		}
		if cinco >= 2 {
			dois++
		}
	}
	if pct := float64(dois) * 100 / casts; pct < 1 || pct > 2.5 {
		t.Fatalf("dois ou mais 5x em %.2f%% dos casts, want ~1,5%%", pct)
	}
}

func TestDanoBrutoTempestade(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	bruto, soma := danoBrutoTempestade(r, 1000, 500, 1600)
	if soma != 5 || bruto != 2000 {
		t.Fatalf("Destreza pura = %d (soma %d), want 2000 (soma 5)", bruto, soma)
	}
	for range 1000 {
		bruto, soma = danoBrutoTempestade(r, 1000, 1600, 500)
		if soma < 5 || soma > 25 || bruto != 400*soma {
			t.Fatalf("Força pura = %d (soma %d)", bruto, soma)
		}
	}
}

func TestTempestadeUmAlvo(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	s := &world.Session{}
	caster := &world.Entity{ID: 1, Class: 3}
	target := &world.Entity{ID: 2}
	cast := castInfo{isSkill: true, spell: content.Spell{Index: skillTempestadeDeFlechas, MaxTarget: 6, Range: 6}}
	if !d.validateSkillTarget(w, s, caster, target, 0, cast, 0) {
		t.Fatal("o primeiro alvo foi recusado")
	}
	if d.validateSkillTarget(w, s, caster, target, 1, cast, 0) {
		t.Fatal("o segundo alvo passou")
	}
}

func TestTempestadeRecarga(t *testing.T) {
	d := New(Config{})
	s := &world.Session{AccountID: 9, Slot: 1}
	if falta := d.tempestadeRecargaRestante(s, 1000); falta != 0 {
		t.Fatalf("sem cast, falta %d", falta)
	}
	d.marcarRecargaTempestade(s, 1000)
	if falta := d.tempestadeRecargaRestante(s, 11_000); falta != 30_000 {
		t.Fatalf("10 s depois falta %d, want 30000", falta)
	}
	if falta := d.tempestadeRecargaRestante(s, 41_000); falta != 0 {
		t.Fatalf("40 s depois falta %d, want 0", falta)
	}
	outro := &world.Session{AccountID: 9, Slot: 2}
	if falta := d.tempestadeRecargaRestante(outro, 11_000); falta != 0 {
		t.Fatalf("outro personagem da conta herdou a recarga: %d", falta)
	}
	if got := textoRecargaTempestade(29_001); got != "Tempestade de Flechas em recarga: faltam 30 s." {
		t.Fatalf("texto = %q", got)
	}
}

// O buff da Encantar Gelo liga a lentidão com Arco e com Garra na mão direita, e
// não com outra arma.
func TestEncantarGeloNoScorePorArma(t *testing.T) {
	for wtype, want := range map[int]bool{wtypeArco: true, wtypeGarra: true, 11: false} {
		e := htSobrevivencia(1000, 1000, 0)
		e.Equip[weaponSlotR] = world.Item{Index: 831}
		e.Affect[0] = world.Affect{Type: 27, Value: 1, Level: 100}
		applyAffectScoreWithItemAbility(e, func(_ world.Item, ef uint8) int {
			if ef == efWType {
				return wtype
			}
			return 0
		})
		if got := e.Rsv&world.RsvFrost != 0; got != want {
			t.Errorf("EF_WTYPE %d: RsvFrost = %v, want %v", wtype, got, want)
		}
	}
}

// O cast aceito arma os 40 s, e o seguinte dentro deles é recusado.
func TestTempestadeRecargaNoCast(t *testing.T) {
	d := New(Config{Spells: content.NewSkillData([]content.Spell{{Index: skillTempestadeDeFlechas, ManaSpent: 75}})})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	s := &world.Session{AccountID: 5, Slot: 0}
	e := &world.Entity{ID: 1, Class: 3, LearnedSkill: learnedTempestade}
	if _, ok := d.validateCast(w, s, e, skillTempestadeDeFlechas, 1); !ok {
		t.Fatal("primeiro cast recusado")
	}
	if _, ok := d.validateCast(w, s, e, skillTempestadeDeFlechas, 2); ok {
		t.Fatal("segundo cast dentro da recarga passou")
	}
}
