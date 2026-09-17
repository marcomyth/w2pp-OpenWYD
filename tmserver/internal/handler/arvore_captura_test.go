package handler

import (
	"log/slog"
	"math/rand"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/combat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// htCaptura monta uma Huntress jogadora com a maestria de Captura dada.
func htCaptura(str, dex int16, learned int32, maestria int16) *world.Entity {
	e := &world.Entity{ID: 1, Class: 3, Str: str, Dex: dex, LearnedSkill: learned}
	e.Special[3] = maestria
	return e
}

// armaNaMao devolve um itemAbility que diz que a mão direita tem wtype.
func armaNaMao(wtype int) func(world.Item, uint8) int {
	return func(_ world.Item, ef uint8) int {
		if ef == efWType {
			return wtype
		}
		return 0
	}
}

func TestLaminaDasSombrasForca(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	if got, _ := danoLaminaDasSombras(r, htCaptura(500, 1600, 0, 0), 1000); got != 1000 {
		t.Fatalf("Destreza pura = %d, want 1000", got)
	}
	if got, crit := danoLaminaDasSombras(r, htCaptura(1600, 500, 0, 0), 1000); got != 1500 || crit {
		t.Fatalf("Força pura sem a 8ª = %d (crit %v), want 1500 sem crítico", got, crit)
	}
}

// Com a 8ª, a Força pura critica em ~30% dos golpes e sempre a 3x.
func TestLaminaDasSombrasCriticoComOitava(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	e := htCaptura(1600, 500, learnedInvisibilidade, 0)
	const n = 20_000
	crits := 0
	for range n {
		got, crit := danoLaminaDasSombras(r, e, 1000)
		if crit {
			crits++
			if got != 4500 {
				t.Fatalf("crítico = %d, want 4500 (1500 × 3)", got)
			}
		} else if got != 1500 {
			t.Fatalf("sem crítico = %d, want 1500", got)
		}
	}
	if pct := crits * 100 / n; pct < 28 || pct > 32 {
		t.Fatalf("crítico em %d%%, want ~30%%", pct)
	}
	// Destreza pura: 10% de chance e 2x.
	e = htCaptura(500, 1600, learnedInvisibilidade, 0)
	for range 2000 {
		if got, crit := danoLaminaDasSombras(r, e, 1000); crit && got != 2000 {
			t.Fatalf("crítico da Destreza pura = %d, want 2000", got)
		}
	}
}

func TestEvasaoAprimorada(t *testing.T) {
	tests := []struct {
		name     string
		str, dex int16
		learned  int32
		wtype    int
		want     int
	}{
		{"arco, Força pura", 1600, 500, 0, wtypeArco, 50},
		{"arco, Força pura, 8ª", 1600, 500, learnedInvisibilidade, wtypeArco, 75},
		{"garra, Força pura", 1600, 500, 0, wtypeGarra, 75},
		{"garra, Força pura, 8ª", 1600, 500, learnedInvisibilidade, wtypeGarra, 110},
		{"garra, meio a meio", 1000, 1000, learnedInvisibilidade, wtypeGarra, 0},
		{"garra, Destreza", 500, 1600, learnedInvisibilidade, wtypeGarra, 0},
		{"outra arma", 1600, 500, learnedInvisibilidade, 11, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bonusEvasaoAprimorada(htCaptura(tt.str, tt.dex, tt.learned, 0), tt.wtype); got != tt.want {
				t.Fatalf("bonusEvasaoAprimorada = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestVisaoDeCacadora(t *testing.T) {
	forca := htCaptura(1600, 500, learnedVisaoDeCacadora, 255)
	destreza := htCaptura(500, 1600, learnedVisaoDeCacadora, 255)
	if got := criticoVisaoDeCacadora(forca); got != visaoCriticoTeto {
		t.Errorf("crítico Força pura = %d, want %d (10%% na janela)", got, visaoCriticoTeto)
	}
	if got := criticoVisaoDeCacadora(destreza); got != 10 {
		t.Errorf("crítico Destreza pura = %d, want 10 (40%% do teto)", got)
	}
	if got := defesaVisaoDeCacadora(destreza); got != 300 {
		t.Errorf("defesa Destreza pura = %d, want 300", got)
	}
	if got := defesaVisaoDeCacadora(forca); got != 120 {
		t.Errorf("defesa Força pura = %d, want 120", got)
	}
	forca.Soul = 1
	if got := criticoVisaoDeCacadora(forca); got != visaoCriticoTetoSoul {
		t.Errorf("crítico com soul = %d, want %d (20%% na janela)", got, visaoCriticoTetoSoul)
	}
	// O teto vale mesmo com maestria acima de 255.
	forca.Special[3] = 400
	if got := criticoVisaoDeCacadora(forca); got != visaoCriticoTetoSoul {
		t.Errorf("crítico com maestria 400 = %d, want %d", got, visaoCriticoTetoSoul)
	}
	// A janela C mostra o byte × 0,4%.
	if visaoCriticoTeto*4/10 != 10 || visaoCriticoTetoSoul*4/10 != 20 {
		t.Error("tetos fora de 10% / 20% na janela C")
	}
}

func TestProtecaoDasSombrasNoScore(t *testing.T) {
	e := htCaptura(1600, 500, learnedProtecaoDasSombras, 255)
	applyAffectScoreWithItemAbility(e, armaNaMao(wtypeGarra))
	if e.AffAC != 600 || e.AffEsquivaPct != 50 {
		t.Fatalf("garra: AffAC/Esquiva = %d/%d, want 600/50", e.AffAC, e.AffEsquivaPct)
	}
	e.LearnedSkill |= learnedInvisibilidade
	applyAffectScoreWithItemAbility(e, armaNaMao(wtypeGarra))
	if e.AffAC != 800 || e.AffEsquivaPct != 65 {
		t.Fatalf("garra com a 8ª: AffAC/Esquiva = %d/%d, want 800/65", e.AffAC, e.AffEsquivaPct)
	}
	applyAffectScoreWithItemAbility(e, armaNaMao(wtypeArco))
	if e.AffAC != 0 || e.AffEsquivaPct != 0 {
		t.Fatalf("arco: AffAC/Esquiva = %d/%d, want 0/0 (só garra)", e.AffAC, e.AffEsquivaPct)
	}
	// O bit antigo (Invisibilidade) sozinho não liga mais a passiva.
	e = htCaptura(1600, 500, learnedInvisibilidade, 255)
	applyAffectScoreWithItemAbility(e, armaNaMao(wtypeGarra))
	if e.AffAC != 0 {
		t.Fatalf("só a Invisibilidade deu %d de defesa", e.AffAC)
	}
}

// Evasão com a 8ª (+110%) e Proteção com a 8ª (+65%) param em +125%, e o sorteio
// continua no teto de 650.
func TestEsquivaDaCapturaTetos(t *testing.T) {
	e := htCaptura(1600, 500, learnedProtecaoDasSombras|learnedInvisibilidade, 255)
	e.Affect[0] = world.Affect{Type: affectEvasao, Value: 1, Level: 100}
	applyAffectScoreWithItemAbility(e, armaNaMao(wtypeGarra))
	if e.AffEsquivaPct != esquivaCapturaTeto {
		t.Fatalf("AffEsquivaPct = %d, want %d", e.AffEsquivaPct, esquivaCapturaTeto)
	}
	if got := esquivaComMelhoria(200, e); got != 450 {
		t.Fatalf("esquiva 200 × 2,25 = %d, want 450", got)
	}
	if got := esquivaComMelhoria(400, e); got != esquivaTeto {
		t.Fatalf("esquiva 400 × 2,25 = %d, want o teto %d", got, esquivaTeto)
	}
}

// A esquiva da Captura chega ao sorteio de golpe de monstro também.
func TestEsquivaDaCapturaNoParryDeMonstro(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	alvo := htCaptura(1600, 500, 0, 0)
	mob := &world.Entity{ID: world.MaxUser, Dex: 0}
	sem := d.monsterParryRate(mob, alvo)
	alvo.AffEsquivaPct = 50
	if com := d.monsterParryRate(mob, alvo); com != min(sem*150/100, esquivaTeto) {
		t.Fatalf("parry com +50%% = %d, sem = %d", com, sem)
	}
}

func TestToxinaEmMonstro(t *testing.T) {
	pequeno := &world.Entity{ID: world.MaxUser, MaxHP: 50_000}
	grande := &world.Entity{ID: world.MaxUser, MaxHP: 2_000_000}
	medio := &world.Entity{ID: world.MaxUser, MaxHP: 350_000}
	tests := []struct {
		name     string
		mob      *world.Entity
		maestria int
		want     int32
	}{
		{"pouca vida, maestria cheia", pequeno, 255, 2000},
		{"muita vida, maestria cheia", grande, 255, 5000},
		{"vida média", medio, 255, 3500},
		{"maestria zero", grande, 0, 0},
		{"meia maestria", pequeno, 127, 996},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := danoToxinaEmMonstro(tt.mob, tt.maestria); got != tt.want {
				t.Fatalf("danoToxinaEmMonstro = %d, want %d", got, tt.want)
			}
		})
	}
}

// Todo acerto com a Toxina envenena o monstro, e o tick dele sai pela maestria.
func TestToxinaEnvenenaMonstroEmTodoAcerto(t *testing.T) {
	spells := content.NewSkillData([]content.Spell{{Index: 40, TickType: affectPoison, TickValue: 10, AffectTime: 600, Aggressive: 1}})
	d := New(Config{Spells: spells, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	ht := htCaptura(1600, 500, 0, 255)
	ht.Rsv = world.RsvDrain
	for range 20 {
		mob := &world.Entity{ID: world.MaxUser + 1, HP: 100_000, MaxHP: 2_000_000}
		d.applyOnHitAffects(w, ht, mob, mob.ID)
		slot := -1
		for i := range mob.Affect {
			if mob.Affect[i].Type == affectPoison {
				slot = i
			}
		}
		if slot < 0 {
			t.Fatal("acerto sem veneno")
		}
		if mob.Affect[slot].Value != toxinaMarca || mob.Affect[slot].Level != 255 {
			t.Fatalf("slot = %+v, want marca da Toxina e maestria 255", mob.Affect[slot])
		}
		// refreshScore reconstrói o score do mob de teste, que não tem template: a vida
		// é reposta aqui, e o tick esperado sai do MaxHP que sobrou.
		mob.HP = 100_000
		want := 100_000 - danoToxinaEmMonstro(mob, 255)
		d.processMobAffect(w, mob.ID, mob)
		if mob.HP != want || want == 100_000 {
			t.Fatalf("HP depois do tick = %d, want %d", mob.HP, want)
		}
	}
}

func TestChanceLaminaAerea(t *testing.T) {
	tests := []struct {
		name     string
		str, dex int16
		learned  int32
		maestria int16
		wtype    int
		want     int
	}{
		{"garra cheia", 1600, 500, 0, 255, wtypeGarra, 60},
		{"garra cheia, 8ª", 1600, 500, learnedInvisibilidade, 255, wtypeGarra, 75},
		{"arco cheio", 1600, 500, 0, 255, wtypeArco, 35},
		{"arco cheio, 8ª", 1600, 500, learnedInvisibilidade, 255, wtypeArco, 50},
		{"garra, Destreza pura", 500, 1600, learnedInvisibilidade, 255, wtypeGarra, 37},
		{"garra, sem maestria nem Força", 500, 1600, learnedInvisibilidade, 0, wtypeGarra, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := htCaptura(tt.str, tt.dex, tt.learned, tt.maestria)
			if got := chanceLaminaAerea(e, tt.wtype); got != tt.want {
				t.Fatalf("chanceLaminaAerea = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLimitarLaminaAerea(t *testing.T) {
	if got := limitarLaminaAerea(4000, 3000); got != 900 {
		t.Errorf("golpe 3K: extra = %d, want 900 (30%%)", got)
	}
	if got := limitarLaminaAerea(1000, 10_000); got != 500 {
		t.Errorf("golpe 10K: extra = %d, want 500 (÷2)", got)
	}
	if got := limitarLaminaAerea(50, 10_000); got != 60 {
		t.Errorf("mínimo = %d, want 60", got)
	}
}

// A Lâmina Aérea nunca passa de 30% do golpe, em nenhum sorteio.
func TestLaminaAereaNuncaPassaDoTeto(t *testing.T) {
	d := New(Config{CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, slog.Default(), nil, nil)
	ht := htCaptura(3000, 500, learnedLaminaAerea|learnedInvisibilidade, 255)
	alvo := &world.Entity{ID: world.MaxUser, AC: 0}
	procs := 0
	for range 2000 {
		body := &protocol.MsgAttackBody{}
		payload := make([]byte, protocol.MsgAttackDamOffset)
		got, proc := d.applyAirBladeProc(w, ht, alvo, protocol.MsgAttackTwo, body, payload, 3000)
		if proc > 900 || got != 3000+proc {
			t.Fatalf("golpe 3000 + extra %d = %d", proc, got)
		}
		if proc > 0 {
			procs++
		}
	}
	if procs == 0 {
		t.Fatal("a Lâmina Aérea nunca disparou")
	}
}

var _ combat.Rand = (*rand.Rand)(nil)
