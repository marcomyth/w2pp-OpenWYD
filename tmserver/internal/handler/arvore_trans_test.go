package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// tkDoTrans monta um TK Mortal de Força com a maestria da Trans.
func tkDoTrans(str, dex int16, learned int32, maestria int16) *world.Entity {
	e := &world.Entity{ID: 1, Class: 0, ClassMaster: classMasterMortal, Str: str, Dex: dex, LearnedSkill: learned}
	e.Special[2] = maestria
	return e
}

func TestTkTransExigeArmaduraCritica(t *testing.T) {
	tests := []struct {
		name string
		e    *world.Entity
		want bool
	}{
		{"TK com a Armadura Crítica", tkDoTrans(2800, 712, learnedArmaduraCritica, 255), true},
		{"TK sem a Armadura Crítica", tkDoTrans(2800, 712, learnedNocaoDeCombate, 255), false},
		{"Huntress com o bit 15", &world.Entity{ID: 1, Class: 3, LearnedSkill: learnedArmaduraCritica}, false},
		{"monstro com o bit 15", &world.Entity{ID: world.MaxUser, Class: 0, LearnedSkill: learnedArmaduraCritica}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tkTrans(tt.e); got != tt.want {
				t.Fatalf("tkTrans = %v, want %v", got, tt.want)
			}
		})
	}
}

// O crítico da Armadura sai da Força e da maestria, não mais da Destreza.
func TestCriticoDaArmadura(t *testing.T) {
	tests := []struct {
		name     string
		str, dex int16
		maestria int16
		want     uint8
	}{
		{"Força pura, maestria cheia", 2800, 712, 255, 25},
		{"Força pura, maestria 102", 2800, 712, 102, 10},
		{"meio a meio", 1000, 1000, 255, 12},
		{"Destreza pura", 500, 2000, 255, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tkDoTrans(tt.str, tt.dex, learnedArmaduraCritica, tt.maestria)
			if got := effectiveCritical(e); got != tt.want {
				t.Fatalf("effectiveCritical = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDefesaDaArmadura(t *testing.T) {
	// Os números saem dos botões, não de constantes repetidas aqui: o que o teste
	// prova é a REGRA — base do legado sempre, maestria por cima em proporção — e
	// não o valor do dia, que o torneio move.
	cheio := armaduraDefesaBase + armaduraDefesaMaestri
	if got := skillDerivedACBonus(tkDoTrans(2800, 712, learnedArmaduraCritica, 255), 1000); got != int32(cheio) {
		t.Errorf("maestria cheia: bônus = %d, want %d (base + maestria sobre 1000 de AC)", got, cheio)
	}
	if got := skillDerivedACBonus(tkDoTrans(2800, 712, learnedArmaduraCritica, transMaestriaMax/2), 1000); got <= int32(armaduraDefesaBase) || got >= int32(cheio) {
		t.Errorf("meia maestria: bônus = %d, queria entre %d e %d", got, armaduraDefesaBase, cheio)
	}
	if got := skillDerivedACBonus(tkDoTrans(2800, 712, learnedArmaduraCritica, 0), 1000); got != int32(armaduraDefesaBase) {
		t.Errorf("sem maestria: bônus = %d, want %d (só o do legado)", got, armaduraDefesaBase)
	}
	if got := skillDerivedACBonus(tkDoTrans(2800, 712, 0, 255), 1000); got != 0 {
		t.Errorf("sem a Armadura: bônus = %d, want 0", got)
	}
}

// O TK Trans atravessa a esquiva da HT, e só da HT.
func TestTransIgnoraEsquivaDaHT(t *testing.T) {
	d := New(Config{})
	ht := &world.Entity{ID: 2, Class: 3, Dex: 2000}
	fm := &world.Entity{ID: 3, Class: 1, Dex: 2000}
	trans := tkDoTrans(2800, 712, learnedArmaduraCritica, 255)
	outroTK := tkDoTrans(2800, 712, 0, 255)

	if got := d.parryRate(trans, ht); got != 0 {
		t.Errorf("TK Trans na HT: esquiva = %d, want 0", got)
	}
	if got := d.skillParryRate(trans, ht); got != 0 {
		t.Errorf("skill do TK Trans na HT: esquiva = %d, want 0", got)
	}
	if got := d.parryRate(outroTK, ht); got != 608 {
		t.Errorf("TK sem a Armadura na HT: esquiva = %d, want 608", got)
	}
	if got := d.parryRate(trans, fm); got != 608 {
		t.Errorf("TK Trans na FM: esquiva = %d, want 608 (só a HT perde a esquiva)", got)
	}
}

// O acerto da Noção tira uma parte da esquiva do alvo: quanto mais ele esquiva,
// mais o TK ganha.
func TestAcertoDaNocaoDeCombate(t *testing.T) {
	d := New(Config{})
	fm := &world.Entity{ID: 3, Class: 1, Dex: 2000}
	if got := d.parryRate(tkDoTrans(2800, 712, learnedNocaoDeCombate, 255), fm); got != 425 {
		t.Errorf("maestria cheia: esquiva da FM = %d, want 425 (608 − 30%%)", got)
	}
	if got := d.parryRate(tkDoTrans(2800, 712, learnedNocaoDeCombate, 0), fm); got != 608 {
		t.Errorf("sem maestria: esquiva da FM = %d, want 608", got)
	}
	if got := d.parryRate(tkDoTrans(2800, 712, 0, 255), fm); got != 608 {
		t.Errorf("sem a Noção: esquiva da FM = %d, want 608", got)
	}
}

func TestPisoDaNocaoNoGolpe(t *testing.T) {
	e := tkDoTrans(2800, 712, learnedNocaoDeCombate, 420)
	if got := masterDoGolpe(e); got != 15 {
		t.Errorf("com a Noção: combat do golpe = %d, want 15 (teto)", got)
	}
	sem := tkDoTrans(2800, 712, 0, 420)
	sem.Master = 3
	if got := masterDoGolpe(sem); got != 3 {
		t.Errorf("sem a Noção: combat do golpe = %d, want 3 (o do legado)", got)
	}
}

func TestPassivasDoTrans(t *testing.T) {
	const espada, martelo, lanca = 3761, 3781, 855
	ability := armasNasMaos(map[int16]int{espada: wtypeEspadaDuasMaos, martelo: wtypeMarteloDuasMaos, lanca: wtypeLanca})
	tests := []struct {
		name     string
		arma     int16
		learned  int32
		multi    int32
		critico  int16
		hp       int32
		esquivaP int32
	}{
		// HP: 2 × 5000 = 10000; +15% pela Força e +15% com o martelo.
		//
		// O multiplicador é 100 (a base) + o dano da Armadura Crítica, que sai da
		// Força, + os 20 da espada de 2 mãos. Vem do botão porque o torneio o move.
		{"espada de 2 mãos", espada, learnedArmaduraCritica, 100 + int32(armaduraDanoForca) + espadaDanoPct, 0, 1500, 0},
		{"martelo de 2 mãos", martelo, learnedArmaduraCritica, 100 + int32(armaduraDanoForca), 25, 3000, 0},
		{"lança", lanca, learnedArmaduraCritica, 100 + int32(armaduraDanoForca), 0, 1500, 0},
		{"espada com a Noção", espada, learnedArmaduraCritica | learnedNocaoDeCombate, 100 + int32(armaduraDanoForca) + espadaDanoPct, 0, 1500, 10},
		// Sem a Armadura Crítica não há dano da árvore: a Noção só dá esquiva.
		{"só a Noção, sem a Armadura", espada, learnedNocaoDeCombate, 100, 0, 0, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tkDoTrans(2800, 712, tt.learned, 255)
			e.MaxHP = 5000
			e.Equip[weaponSlotR] = world.Item{Index: tt.arma}
			for range 2 { // refazer o score não acumula
				applyAffectScoreWithItemAbility(e, ability)
			}
			if e.AffDamageMultiPct != tt.multi || e.AffCritical != tt.critico || e.AffMaxHP != tt.hp || e.AffEsquivaPct != tt.esquivaP {
				t.Fatalf("multi %d crítico %d HP %d esquiva %d, want %d %d %d %d",
					e.AffDamageMultiPct, e.AffCritical, e.AffMaxHP, e.AffEsquivaPct, tt.multi, tt.critico, tt.hp, tt.esquivaP)
			}
		})
	}
}

// O dano extra da Armadura Crítica vale só contra a Huntress jogadora.
func TestDanoDoTransContraHT(t *testing.T) {
	trans := tkDoTrans(2800, 712, learnedArmaduraCritica, 255)
	ht := &world.Entity{ID: 2, Class: 3}
	fm := &world.Entity{ID: 3, Class: 1}
	mobHT := &world.Entity{ID: world.MaxUser + 1, Class: 3}
	semArmadura := tkDoTrans(2800, 712, learnedNocaoDeCombate, 255)

	// Físico: extra plano. Skill: percentual.
	if got, want := danoDoTransContraHT(trans, ht, 1000, false), 1000+transContraHTPlano; got != want {
		t.Errorf("Trans na HT, físico: dano = %d, want %d", got, want)
	}
	if got, want := danoDoTransContraHT(trans, ht, 1000, true), 1000*(100+transContraHTSkillPct)/100; got != want {
		t.Errorf("Trans na HT, skill: dano = %d, want %d", got, want)
	}
	for _, c := range []struct {
		nome     string
		atk, alv *world.Entity
	}{
		{"Trans na FM", trans, fm},
		{"Trans em monstro de classe 3", trans, mobHT},
		{"TK sem a Armadura na HT", semArmadura, ht},
	} {
		for _, skill := range []bool{false, true} {
			if got := danoDoTransContraHT(c.atk, c.alv, 1000, skill); got != 1000 {
				t.Errorf("%s (skill=%v): dano = %d, want 1000 (sem o bônus)", c.nome, skill, got)
			}
		}
	}
	for _, skill := range []bool{false, true} {
		if got := danoDoTransContraHT(trans, ht, -3, skill); got != -3 {
			t.Errorf("esquiva (−3, skill=%v) = %d, want −3 intacta", skill, got)
		}
	}
}

// A Espada da Fênix bate metade em jogador e o mesmo em monstro; as outras skills
// não mudam.
func TestDanoDaFenixEmJogador(t *testing.T) {
	for _, c := range []struct {
		nome           string
		skill, id, dmg int
		want           int
	}{
		{"Fênix em jogador", skillEspadaDaFenix, 2, 1650, 825},
		{"Fênix em monstro", skillEspadaDaFenix, world.MaxUser + 1, 1650, 1650},
		{"Carga em jogador", 8, 2, 1650, 1650},
		{"Golpe Mortal em jogador", 10, 2, 1650, 1650},
		{"Fênix de 1 não vira 0", skillEspadaDaFenix, 2, 1, 1},
	} {
		if got := danoDaFenixEmJogador(c.skill, c.id, c.dmg); got != c.want {
			t.Errorf("%s: dano = %d, want %d", c.nome, got, c.want)
		}
	}
}
