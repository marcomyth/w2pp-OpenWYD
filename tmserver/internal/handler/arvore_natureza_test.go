package handler

import (
	"log/slog"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// bmDaNatureza monta um Beast Master com uma build do eixo e a maestria dada.
func bmDaNatureza(forca, destreza, maestria int, learned int32) *world.Entity {
	e := &world.Entity{ID: 1, Class: 2, Level: 399, LearnedSkill: learned,
		MaxHP: 10_000, HP: 10_000}
	e.Str, e.Dex = int16(forca), int16(destreza)
	e.Special[naturezaKind] = int16(maestria)
	// A Caliburn sozinha é a empunhadura NEUTRA — o caso base da classe, nem
	// bônus nem castigo. Quem testa empunhadura sobrescreve com comArmas.
	e.Equip[weaponSlotR] = world.Item{Index: testCaliburn}
	return e
}

// O eixo abre entre a Destreza pura e a Força pura, e as builds reais do elenco
// caem dentro da faixa — não espremidas num canto dela.
func TestEixoForcaDestreza(t *testing.T) {
	for _, c := range []struct {
		nome         string
		forca, destr int
		want         int
	}{
		{"Força pura (2800/700)", 2800, 700, 1000},
		{"Força (2100/1400)", 2100, 1400, 666},
		{"meio a meio", 1750, 1750, 500},
		{"Destreza (1400/2100)", 1400, 2100, 333},
		{"Destreza pura (700/2800)", 700, 2800, 0},
		// Fora da faixa útil o eixo satura, nunca extrapola.
		{"só Força", 3500, 0, 1000},
		{"só Destreza", 0, 3500, 0},
		{"personagem zerado", 0, 0, 0},
	} {
		if got := eixoForcaDestreza(bmDaNatureza(c.forca, c.destr, 320, 0)); got != c.want {
			t.Errorf("%s: eixo = %d, want %d", c.nome, got, c.want)
		}
	}
	if got := eixoForcaDestreza(nil); got != 0 {
		t.Errorf("nil: eixo = %d, want 0", got)
	}
}

// O eixo é MONOTÔNICO: cada ponto de Força a mais nunca baixa o resultado. É a
// propriedade que o jogador sente — trocar Destreza por Força tem de andar
// sempre para o mesmo lado, sem degrau para trás.
func TestEixoEMonotonico(t *testing.T) {
	anterior := -1
	for forca := 0; forca <= 3500; forca += 50 {
		got := eixoForcaDestreza(bmDaNatureza(forca, 3500-forca, 320, 0))
		if got < anterior {
			t.Fatalf("FOR %d: eixo caiu de %d para %d", forca, anterior, got)
		}
		anterior = got
	}
	if anterior != 1000 {
		t.Errorf("Força pura tinha de chegar a 1000, chegou a %d", anterior)
	}
}

// A absorção segue o eixo: Força pura leva o teto da faixa, Destreza pura o piso.
func TestAbsorcaoDaArmaduraElementalSegueOEixo(t *testing.T) {
	for _, c := range []struct {
		nome         string
		forca, destr int
		want         int
	}{
		{"Força pura", 2800, 700, armaduraAbsForca},
		{"Força", 2100, 1400, 216},
		{"meio a meio", 1750, 1750, 175},
		{"Destreza pura", 700, 2800, armaduraAbsDestreza},
	} {
		e := bmDaNatureza(c.forca, c.destr, naturezaMaestriaCheia, learnedArmaduraElemental)
		if got := dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(e); got != c.want {
			t.Errorf("%s: %d décimos, want %d", c.nome, got, c.want)
		}
	}
}

// A maestria da árvore escala tudo: quem não investiu não leva nada, quem
// investiu metade leva metade.
func TestAbsorcaoEscalaPelaMaestria(t *testing.T) {
	const forca, destr = 2800, 700
	for _, c := range []struct {
		maestria, want int
	}{
		{0, 0},
		{160, armaduraAbsForca / 2},
		{320, armaduraAbsForca},
		// Acima do teto da árvore não cresce mais.
		{400, armaduraAbsForca},
	} {
		e := bmDaNatureza(forca, destr, c.maestria, learnedArmaduraElemental)
		if got := dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(e); got != c.want {
			t.Errorf("maestria %d: %d décimos, want %d", c.maestria, got, c.want)
		}
	}
}

// Sem a passiva comprada não há absorção nenhuma, e nenhuma outra classe entra.
func TestArmaduraElementalExigeAPassiva(t *testing.T) {
	const forca, destr = 2800, 700
	if dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(bmDaNatureza(forca, destr, 320, 0)) != 0 {
		t.Error("sem a Armadura Elemental não pode haver absorção")
	}
	outra := bmDaNatureza(forca, destr, 320, learnedArmaduraElemental)
	outra.Class = 0
	if dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(outra) != 0 {
		t.Error("TK não pode entrar na árvore do BM")
	}
	mob := bmDaNatureza(forca, destr, 320, learnedArmaduraElemental)
	mob.ID = world.MaxUser + 1
	if dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(mob) != 0 {
		t.Error("monstro não pode entrar na árvore")
	}
	if dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(nil) != 0 {
		t.Error("nil não pode entrar na árvore")
	}
}

// O golpe: a fatia sai do dano, e o piso de 1 vale.
func TestArmaduraElementalTiraAFatiaDoGolpe(t *testing.T) {
	forte := bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental)
	agil := bmDaNatureza(700, 2800, naturezaMaestriaCheia, learnedArmaduraElemental)

	if got := dispatcherDaNatureza().absorcaoDaArmaduraElemental(forte, 1000); got != 700 {
		t.Errorf("Força pura em golpe de 1000: %d, want 700", got)
	}
	if got := dispatcherDaNatureza().absorcaoDaArmaduraElemental(agil, 1000); got != 950 {
		t.Errorf("Destreza pura em golpe de 1000: %d, want 950", got)
	}
	// Sem a passiva o golpe passa inteiro.
	if got := dispatcherDaNatureza().absorcaoDaArmaduraElemental(bmDaNatureza(2800, 700, 320, 0), 1000); got != 1000 {
		t.Errorf("sem a passiva: %d, want 1000", got)
	}
	// Golpe que não entrou continua não entrando, e o piso é 1.
	if got := dispatcherDaNatureza().absorcaoDaArmaduraElemental(forte, 0); got != 0 {
		t.Errorf("golpe 0: %d, want 0", got)
	}
	if got := dispatcherDaNatureza().absorcaoDaArmaduraElemental(forte, 1); got != 1 {
		t.Errorf("golpe 1: %d, want o piso 1", got)
	}
}

// A absorção tem de estar LIGADA no absorbBlow, o ponto único por onde todo
// golpe passa. A função certa existindo e ninguém a chamando é o modo mais
// comum de uma regra nova não valer nada em jogo.
func TestArmaduraElementalEntraNoAbsorbBlow(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	d := dispatcherDaNatureza()
	w := world.New(world.Config{GridDim: 16}, log, nil, nil)

	forte := bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental)
	// Golpe de JOGADOR e golpe de MONSTRO: é defesa do personagem, não regra de
	// arena, e o legado também não pergunta quem bateu (CMob.cpp:776-780).
	if got := d.absorbBlow(w, forte, 1000, true); got != 700 {
		t.Errorf("golpe de jogador: absorbBlow = %d, want 700", got)
	}
	if got := d.absorbBlow(w, forte, 1000, false); got != 700 {
		t.Errorf("golpe de monstro: absorbBlow = %d, want 700", got)
	}
	// Sem a passiva o absorbBlow devolve o golpe inteiro.
	if got := d.absorbBlow(w, bmDaNatureza(2800, 700, 320, 0), 1000, true); got != 1000 {
		t.Errorf("sem a passiva: absorbBlow = %d, want 1000", got)
	}
}

// O teto da árvore existe para a absorção não virar imunidade quando as etapas
// seguintes (empunhadura e forma) somarem por cima.
func TestAbsorcaoRespeitaOTeto(t *testing.T) {
	if armaduraAbsForca > naturezaAbsTeto {
		t.Fatalf("a faixa base (%d) já passa do teto (%d)", armaduraAbsForca, naturezaAbsTeto)
	}
	e := bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental)
	if got := dispatcherDaNatureza().absorcaoDaArmaduraElementalDecimos(e); got > naturezaAbsTeto {
		t.Errorf("absorção %d passou do teto %d", got, naturezaAbsTeto)
	}
}

// As armas reais do catálogo que o desenho da árvore nomeia.
const (
	testCaliburn = 871  // espada de 1 mão (wtype 1), nPos 192
	testBalmung  = 811  // machado/martelo de 1 mão (wtype 11), nPos 192
	testHermai   = 886  // arma de arremesso (wtype 103), nPos 64
	testEscudo   = 1713 // Escudo Svalin — SEM EF_WTYPE, como todo escudo
	testGarra    = 2595
	testCajado2  = 903
	testLanca    = 1008
)

// dispatcherDaNatureza conhece o wtype de cada arma do teste. O escudo fica de
// fora do mapa de propósito: escudo não tem EF_WTYPE no catálogo, e é
// exatamente por isso que ele é reconhecível.
func dispatcherDaNatureza() *Dispatcher {
	return New(Config{Log: slog.New(slog.DiscardHandler), CombatRules: regraSemEscala(), ItemEffects: map[int][]content.BaseEffect{
		testCaliburn: {{Eff: efWType, Val: wtypeEspadaUmaMao}},
		testBalmung:  {{Eff: efWType, Val: wtypeMachadoUmaMao}},
		testHermai:   {{Eff: efWType, Val: wtypeArremesso}},
		testGarra:    {{Eff: efWType, Val: wtypeGarra}},
		testCajado2:  {{Eff: efWType, Val: wtypeCajadoDuasMaos}},
		testLanca:    {{Eff: efWType, Val: wtypeLanca}},
	}})
}

func comArmas(e *world.Entity, direita, esquerda int16) *world.Entity {
	e.Equip[weaponSlotR] = world.Item{Index: direita}
	e.Equip[weaponSlotL] = world.Item{Index: esquerda}
	return e
}

func TestEmpunhaduraDaNatureza(t *testing.T) {
	d := dispatcherDaNatureza()
	for _, c := range []struct {
		nome        string
		dir, esquer int16
		want        empunhadura
	}{
		{"Caliburn + escudo", testCaliburn, testEscudo, empunhaduraEscudo},
		{"Balmung + escudo", testBalmung, testEscudo, empunhaduraEscudo},
		{"Hermai + escudo (a assinatura)", testHermai, testEscudo, empunhaduraEscudo},
		{"duas Caliburn", testCaliburn, testCaliburn, empunhaduraDuasArmas},
		{"Caliburn + Balmung", testCaliburn, testBalmung, empunhaduraDuasArmas},
		{"Caliburn sozinha", testCaliburn, 0, empunhaduraNeutra},
		{"Hermai sozinho", testHermai, 0, empunhaduraNeutra},
		{"garra", testGarra, 0, empunhaduraPenalizada},
		{"cajado de 2 mãos", testCajado2, 0, empunhaduraPenalizada},
		{"lança", testLanca, 0, empunhaduraPenalizada},
		// Arma de fora da árvore COM escudo continua de fora: o escudo não
		// redime a empunhadura errada.
		{"garra + escudo", testGarra, testEscudo, empunhaduraPenalizada},
		{"sem arma", 0, 0, empunhaduraPenalizada},
	} {
		e := comArmas(bmDaNatureza(2800, 700, 320, learnedEden), c.dir, c.esquer)
		if got := empunhaduraDaNatureza(e, d.itemAbility); got != c.want {
			t.Errorf("%s: empunhadura %d, want %d", c.nome, got, c.want)
		}
	}
	// Sem itemAbility o servidor não sabe o que está na mão: neutro, nunca um
	// bônus e nunca um castigo.
	e := comArmas(bmDaNatureza(2800, 700, 320, learnedEden), testCaliburn, testCaliburn)
	if got := empunhaduraDaNatureza(e, nil); got != empunhaduraNeutra {
		t.Errorf("sem itemAbility: %d, want neutra", got)
	}
	if got := empunhaduraDaNatureza(nil, d.itemAbility); got != empunhaduraNeutra {
		t.Errorf("nil: %d, want neutra", got)
	}
}

// A empunhadura muda a absorção: o escudo soma, a arma de fora da árvore corta.
func TestAbsorcaoSegueAEmpunhadura(t *testing.T) {
	d := dispatcherDaNatureza()
	base := armaduraAbsForca // 300, Força pura com a maestria cheia
	for _, c := range []struct {
		nome        string
		dir, esquer int16
		want        int
	}{
		{"Caliburn sozinha", testCaliburn, 0, base},
		{"duas Caliburn", testCaliburn, testCaliburn, base},
		{"Hermai + escudo", testHermai, testEscudo, min(base+naturezaAbsEscudo, naturezaAbsTeto)},
		{"garra", testGarra, 0, penalidadeDaEmpunhadura(base)},
	} {
		e := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental|learnedEden), c.dir, c.esquer)
		if got := d.absorcaoDaArmaduraElementalDecimos(e); got != c.want {
			t.Errorf("%s: %d décimos, want %d", c.nome, got, c.want)
		}
	}
	// O teto vale mesmo com escudo: nenhuma combinação passa de 40%.
	e := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental|learnedEden), testHermai, testEscudo)
	if got := d.absorcaoDaArmaduraElementalDecimos(e); got > naturezaAbsTeto {
		t.Errorf("com escudo a absorção passou do teto: %d > %d", got, naturezaAbsTeto)
	}
}

// O crítico da assinatura exige as QUATRO coisas: Éden, escudo, arma da árvore
// e mais Força que Destreza. Cada uma faltando zera o bônus.
func TestCriticoDaAssinatura(t *testing.T) {
	d := dispatcherDaNatureza()
	completo := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedEden), testHermai, testEscudo)
	if got := criticoDaAssinatura(completo, d.itemAbility); got != naturezaCriticoEscudo {
		t.Fatalf("a build completa: crítico %d, want %d", got, naturezaCriticoEscudo)
	}
	for _, c := range []struct {
		nome string
		e    *world.Entity
	}{
		{"sem o Éden", comArmas(bmDaNatureza(2800, 700, 320, 0), testHermai, testEscudo)},
		{"sem escudo", comArmas(bmDaNatureza(2800, 700, 320, learnedEden), testHermai, 0)},
		{"com duas armas em vez de escudo", comArmas(bmDaNatureza(2800, 700, 320, learnedEden), testCaliburn, testCaliburn)},
		{"arma de fora da árvore", comArmas(bmDaNatureza(2800, 700, 320, learnedEden), testGarra, testEscudo)},
		{"mais Destreza que Força", comArmas(bmDaNatureza(700, 2800, 320, learnedEden), testHermai, testEscudo)},
		{"meio a meio não basta", comArmas(bmDaNatureza(1750, 1750, 320, learnedEden), testHermai, testEscudo)},
		{"sem maestria na árvore", comArmas(bmDaNatureza(2800, 700, 0, learnedEden), testHermai, testEscudo)},
	} {
		if got := criticoDaAssinatura(c.e, d.itemAbility); got != 0 {
			t.Errorf("%s: crítico %d, want 0", c.nome, got)
		}
	}
}

// O dano por empunhadura entra no multiplicador do personagem, pelo caminho
// normal do score — não basta a função existir.
func TestDanoSegueAEmpunhadura(t *testing.T) {
	d := dispatcherDaNatureza()
	for _, c := range []struct {
		nome        string
		dir, esquer int16
		want        int32
	}{
		{"duas Caliburn", testCaliburn, testCaliburn, 100 + naturezaDanoDuasArmas},
		{"Caliburn + escudo", testCaliburn, testEscudo, 100},
		{"Caliburn sozinha", testCaliburn, 0, 100},
		{"garra", testGarra, 0, 100 - naturezaPenalidadePct},
		{"cajado de 2 mãos", testCajado2, 0, 100 - naturezaPenalidadePct},
	} {
		e := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedEden), c.dir, c.esquer)
		applyAffectScoreWithItemAbility(e, d.itemAbility)
		if e.AffDamageMultiPct != c.want {
			t.Errorf("%s: multiplicador %d%%, want %d%%", c.nome, e.AffDamageMultiPct, c.want)
		}
	}
	// Outra classe com as mesmas armas não recebe nada da árvore do BM.
	outra := comArmas(bmDaNatureza(2800, 700, 320, learnedEden), testCaliburn, testCaliburn)
	outra.Class = 0
	applyAffectScoreWithItemAbility(outra, d.itemAbility)
	if outra.AffDamageMultiPct != 100 {
		t.Errorf("TK com duas armas: %d%%, want 100%%", outra.AffDamageMultiPct)
	}
	// Sem maestria na árvore não há bônus nem castigo.
	sem := comArmas(bmDaNatureza(2800, 700, 0, learnedEden), testGarra, 0)
	applyAffectScoreWithItemAbility(sem, d.itemAbility)
	if sem.AffDamageMultiPct != 100 {
		t.Errorf("sem maestria: %d%%, want 100%%", sem.AffDamageMultiPct)
	}
}

// transformado põe a forma de pé em e (afeto 16, Value = forma+1).
func transformado(e *world.Entity, forma int) *world.Entity {
	e.Affect[1] = world.Affect{Type: affectTransform, Value: uint8(forma + 1),
		Level: uint16(maestriaDaNatureza(e)), Time: 5000}
	return e
}

const (
	formaLobo = iota
	formaUrso
	formaAstaroth
	formaTita
	formaEden
)

// O ÉDEN recebe a camada inteira; as outras formas, a fatia do peso delas. É o
// que faz a 8ª valer os 212 pontos sem precisar ser a melhor em nenhuma
// estatística isolada.
func TestMetamorfoseDaCamadaCheiaSoAoEden(t *testing.T) {
	learned := int32(learnedMetamorfoseSuperior | learnedArmaduraElemental)
	var anterior int
	for _, forma := range []int{formaLobo, formaUrso, formaAstaroth, formaTita, formaEden} {
		e := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), forma)
		got := camadaDaForma(e, forma, naturezaCamada.dano)
		if got <= 0 {
			t.Errorf("forma %d: camada de dano %d, tinha de ser positiva", forma, got)
		}
		if forma == formaEden && got <= anterior {
			t.Errorf("o Éden (%d) tinha de receber mais que a forma anterior (%d)", got, anterior)
		}
		anterior = got
	}
	// O Éden com o eixo cheio recebe exatamente o topo da faixa.
	eden := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), formaEden)
	if got := camadaDaForma(eden, formaEden, naturezaCamada.dano); got != naturezaCamada.dano[1] {
		t.Errorf("Éden com Força pura: dano %d, want %d", got, naturezaCamada.dano[1])
	}
}

// A AFINIDADE desloca o eixo: o Lobo puxa para a Destreza, o Urso e o Titã para
// a Força. Uma build no meio sente isso.
func TestAfinidadeDaFormaDeslocaOEixo(t *testing.T) {
	e := bmDaNatureza(1757, 1757, naturezaMaestriaCheia, learnedMetamorfoseSuperior)
	meio := eixoForcaDestreza(e)
	if got := eixoDaForma(e, formaLobo); got >= meio {
		t.Errorf("Lobo: eixo %d, tinha de ficar abaixo do neutro %d", got, meio)
	}
	if got := eixoDaForma(e, formaUrso); got <= meio {
		t.Errorf("Urso: eixo %d, tinha de ficar acima do neutro %d", got, meio)
	}
	if got := eixoDaForma(e, formaTita); got <= meio {
		t.Errorf("Titã: eixo %d, tinha de ficar acima do neutro %d", got, meio)
	}
	for _, forma := range []int{formaAstaroth, formaEden} {
		if got := eixoDaForma(e, forma); got != meio {
			t.Errorf("forma %d é neutra: eixo %d, want %d", forma, got, meio)
		}
	}
	// A afinidade nunca estoura os limites do eixo.
	for _, forma := range []int{formaLobo, formaUrso, formaAstaroth, formaTita, formaEden} {
		for _, b := range [][2]int{{2800, 700}, {700, 2800}} {
			e := bmDaNatureza(b[0], b[1], naturezaMaestriaCheia, learnedMetamorfoseSuperior)
			if got := eixoDaForma(e, forma); got < 0 || got > 1000 {
				t.Errorf("forma %d com FOR %d: eixo %d fora de [0,1000]", forma, b[0], got)
			}
		}
	}
}

// O eixo decide o QUE cada build recebe: a Força leva dano e vida, a Destreza
// leva velocidade e crítico.
func TestCamadaSegueOEixo(t *testing.T) {
	learned := int32(learnedMetamorfoseSuperior | learnedArmaduraElemental)
	forte := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), formaEden)
	agil := transformado(bmDaNatureza(700, 2800, naturezaMaestriaCheia, learned), formaEden)

	for _, c := range []struct {
		nome       string
		faixa      faixaDoEixo
		forcaMaior bool
	}{
		{"dano", naturezaCamada.dano, true},
		// A VIDA anda para o OUTRO lado desde 20/09/2026: a camada tira dos dois,
		// e tira MAIS da Força. Ver o bloco do topo de arvore_natureza.go.
		{"HP", naturezaCamada.hp, false},
		{"defesa", naturezaCamada.ac, true},
		{"absorção", naturezaCamada.abs, true},
		{"velocidade", naturezaCamada.vel, false},
		{"crítico", naturezaCamada.crit, false},
	} {
		f := camadaDaForma(forte, formaEden, c.faixa)
		a := camadaDaForma(agil, formaEden, c.faixa)
		if c.forcaMaior && f <= a {
			t.Errorf("%s: Força %d, Destreza %d — a Força tinha de levar mais", c.nome, f, a)
		}
		if !c.forcaMaior && a <= f {
			t.Errorf("%s: Destreza %d, Força %d — a Destreza tinha de levar mais", c.nome, a, f)
		}
	}
}

// Sem a Metamorfose comprada, transformar não traz camada nenhuma — e sem
// transformação a camada também não existe, mesmo com a passiva na mão.
func TestMetamorfoseExigePassivaETransformacao(t *testing.T) {
	d := dispatcherDaNatureza()
	comTudo := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia,
		int32(learnedMetamorfoseSuperior|learnedArmaduraElemental)), formaEden)
	base := d.absorcaoDaArmaduraElementalDecimos(comTudo)

	semPassiva := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental), formaEden)
	if got := d.absorcaoDaArmaduraElementalDecimos(semPassiva); got >= base {
		t.Errorf("sem a Metamorfose: %d, tinha de ser menor que %d", got, base)
	}
	semForma := bmDaNatureza(2800, 700, naturezaMaestriaCheia,
		int32(learnedMetamorfoseSuperior|learnedArmaduraElemental))
	if got := d.absorcaoDaArmaduraElementalDecimos(semForma); got >= base {
		t.Errorf("sem transformação: %d, tinha de ser menor que %d", got, base)
	}
	if _, ok := metamorfoseAtiva(semPassiva); ok {
		t.Error("sem a passiva a Metamorfose não pode estar ativa")
	}
	if _, ok := metamorfoseAtiva(semForma); ok {
		t.Error("sem transformação a Metamorfose não pode estar ativa")
	}
	// Sem maestria na árvore não há camada.
	semMaestria := transformado(bmDaNatureza(2800, 700, 0,
		int32(learnedMetamorfoseSuperior|learnedArmaduraElemental)), formaEden)
	if _, ok := metamorfoseAtiva(semMaestria); ok {
		t.Error("sem maestria a Metamorfose não pode estar ativa")
	}
}

// A camada entra no SCORE pelo caminho normal da transformação, não basta a
// função existir.
func TestMetamorfoseEntraNoScore(t *testing.T) {
	d := dispatcherDaNatureza()
	learned := int32(learnedMetamorfoseSuperior | learnedArmaduraElemental)

	com := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), formaEden)
	com.AC = 2000
	applyAffectScoreWithItemAbility(com, d.itemAbility)

	sem := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learnedArmaduraElemental), formaEden)
	sem.AC = 2000
	applyAffectScoreWithItemAbility(sem, d.itemAbility)

	if com.AffDamageMultiPct <= sem.AffDamageMultiPct {
		t.Errorf("dano: com %d%%, sem %d%% — a Metamorfose tinha de somar",
			com.AffDamageMultiPct, sem.AffDamageMultiPct)
	}
	// A vida DESCE com a Metamorfose, de propósito (a camada é negativa nos dois
	// lados do eixo). Provar que ela desce é tão importante quanto provar que o
	// dano sobe: é a metade do preço que a passiva cobra.
	if com.AffMaxHP >= sem.AffMaxHP {
		t.Errorf("HP: com %d, sem %d — a camada tinha de TIRAR vida", com.AffMaxHP, sem.AffMaxHP)
	}
	if com.AffAC <= sem.AffAC {
		t.Errorf("defesa: com %d, sem %d", com.AffAC, sem.AffAC)
	}
}

// O teto vale mesmo somando as três parcelas: eixo, escudo e forma.
// O teto e as três parcelas da absorção (eixo, escudo, forma).
//
// Hoje a soma das três no extremo dá EXATAMENTE o teto, então o min() não corta
// nada — ele é uma trava para o futuro, não um limite ativo. Este teste existe
// para avisar quando isso deixar de ser verdade: quem subir qualquer faixa vai
// ver a soma bruta passar do teto aqui, e aí decidir se sobe o teto junto ou se
// aceita o corte. Um clamp que começa a cortar calado é como uma build vira
// outra sem ninguém perceber.
func TestAbsorcaoTotalEAsTresParcelas(t *testing.T) {
	d := dispatcherDaNatureza()
	// A build mais defensiva que o jogo permite: Força pura, maestria cheia,
	// Hermai + escudo, Éden de pé, as três passivas na mão.
	learned := int32(learnedArmaduraElemental | learnedMetamorfoseSuperior | learnedEden)
	e := comArmas(transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), formaEden),
		testHermai, testEscudo)

	eixo := naturezaInterpola(e, armaduraAbsDestreza, armaduraAbsForca)
	escudo := naturezaAbsEscudo * maestriaDaNatureza(e) / naturezaMaestriaCheia
	forma := absorcaoDaMetamorfose(e)
	bruta := eixo + escudo + forma

	if got := d.absorcaoDaArmaduraElementalDecimos(e); got != min(bruta, naturezaAbsTeto) {
		t.Errorf("absorção %d, want %d (eixo %d + escudo %d + forma %d, teto %d)",
			got, min(bruta, naturezaAbsTeto), eixo, escudo, forma, naturezaAbsTeto)
	}
	if bruta > naturezaAbsTeto {
		t.Errorf("a soma bruta (%d = %d+%d+%d) passou do teto %d: o clamp começou a cortar, "+
			"e isso precisa ser uma decisão e não um acidente", bruta, eixo, escudo, forma, naturezaAbsTeto)
	}
	// As três parcelas existem de verdade — nenhuma pode ser zero nesta build.
	if eixo <= 0 || escudo <= 0 || forma <= 0 {
		t.Errorf("parcela zerada: eixo %d, escudo %d, forma %d", eixo, escudo, forma)
	}
}

// A camada da Metamorfose escala pela maestria da árvore, como todo o resto:
// quem não investiu nela não ganha nada por transformar.
func TestCamadaEscalaPelaMaestria(t *testing.T) {
	learned := int32(learnedMetamorfoseSuperior | learnedArmaduraElemental)
	cheio := camadaDaForma(transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), formaEden),
		formaEden, naturezaCamada.dano)
	metade := camadaDaForma(transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia/2, learned), formaEden),
		formaEden, naturezaCamada.dano)
	zero := camadaDaForma(transformado(bmDaNatureza(2800, 700, 0, learned), formaEden),
		formaEden, naturezaCamada.dano)

	if cheio != naturezaCamada.dano[1] {
		t.Errorf("maestria cheia: %d, want %d", cheio, naturezaCamada.dano[1])
	}
	if metade != cheio/2 {
		t.Errorf("meia maestria: %d, want %d", metade, cheio/2)
	}
	if zero != 0 {
		t.Errorf("sem maestria: %d, want 0", zero)
	}
}

// O PESO da forma corta a camada: só o Éden leva tudo, e as outras levam a
// fatia declarada em naturezaPorForma.
func TestPesoDaFormaCortaACamada(t *testing.T) {
	learned := int32(learnedMetamorfoseSuperior | learnedArmaduraElemental)
	for forma, p := range naturezaPorForma {
		e := transformado(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), forma)
		// Com Força pura e afinidade positiva ou neutra o eixo satura no topo,
		// então a camada é exatamente o topo da faixa cortado pelo peso.
		if p.afinidade < 0 {
			continue // o Lobo puxa o eixo para baixo; ele tem teste próprio
		}
		want := naturezaCamada.dano[1] * p.peso / 100
		if got := camadaDaForma(e, forma, naturezaCamada.dano); got != want {
			t.Errorf("forma %d (peso %d%%): camada %d, want %d", forma, p.peso, got, want)
		}
	}
}

// O escudo de teste precisa de EF_AC para a defesa do Tormento ter o que
// dobrar; o Svalin do catálogo tem 323.
const testEscudoAC = 323

func dispatcherComEscudoAC() *Dispatcher {
	d := dispatcherDaNatureza()
	d.itemEffects[testEscudo] = []content.BaseEffect{{Eff: efAc, Val: testEscudoAC}}
	return d
}

// A defesa do Escudo do Tormento faz o AC do escudo contar em dobro, e exige a
// passiva, o escudo e maestria na árvore.
func TestDefesaDoEscudoDoTormento(t *testing.T) {
	d := dispatcherComEscudoAC()
	learned := int32(learnedEscudoDoTormento)
	com := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, learned), testHermai, testEscudo)
	if got := defesaDoEscudoDoTormento(com, d.itemAbility); got != testEscudoAC {
		t.Errorf("com escudo e passiva: +%d de AC, want +%d", got, testEscudoAC)
	}
	// Meia maestria, meia defesa.
	meio := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia/2, learned), testHermai, testEscudo)
	if got := defesaDoEscudoDoTormento(meio, d.itemAbility); got != testEscudoAC/2 {
		t.Errorf("meia maestria: +%d, want +%d", got, testEscudoAC/2)
	}
	for _, c := range []struct {
		nome string
		e    *world.Entity
	}{
		{"sem a passiva", comArmas(bmDaNatureza(2800, 700, 320, 0), testHermai, testEscudo)},
		{"sem escudo", comArmas(bmDaNatureza(2800, 700, 320, learned), testHermai, 0)},
		{"com duas armas", comArmas(bmDaNatureza(2800, 700, 320, learned), testCaliburn, testBalmung)},
		{"arma fora da árvore", comArmas(bmDaNatureza(2800, 700, 320, learned), testGarra, testEscudo)},
		{"sem maestria", comArmas(bmDaNatureza(2800, 700, 0, learned), testHermai, testEscudo)},
	} {
		if got := defesaDoEscudoDoTormento(c.e, d.itemAbility); got != 0 {
			t.Errorf("%s: +%d de AC, want 0", c.nome, got)
		}
	}
	// Sem itemAbility o servidor não sabe o que está na mão: nada.
	if got := defesaDoEscudoDoTormento(com, nil); got != 0 {
		t.Errorf("sem itemAbility: +%d, want 0", got)
	}
}

// A defesa tem de estar LIGADA no score.
func TestDefesaDoTormentoEntraNoScore(t *testing.T) {
	d := dispatcherComEscudoAC()
	com := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia,
		int32(learnedEscudoDoTormento)), testHermai, testEscudo)
	applyAffectScoreWithItemAbility(com, d.itemAbility)
	sem := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, 0), testHermai, testEscudo)
	applyAffectScoreWithItemAbility(sem, d.itemAbility)
	if com.AffAC-sem.AffAC != testEscudoAC {
		t.Errorf("no score: diferença de AC %d, want %d", com.AffAC-sem.AffAC, testEscudoAC)
	}
}

// A REFLEXÃO exige o Éden por cima de tudo que a defesa já exige, e respeita o
// teto por golpe.
func TestReflexaoDoEscudoDoTormento(t *testing.T) {
	d := dispatcherComEscudoAC()
	comEden := int32(learnedEscudoDoTormento | learnedEden)
	com := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia, comEden), testHermai, testEscudo)

	if got := d.reflexaoDoEscudoDoTormento(com, 1000); got != 1000*tormentoReflexaoPct/100 {
		t.Errorf("golpe de 1000: reflete %d, want %d", got, 1000*tormentoReflexaoPct/100)
	}
	// O teto por golpe vale: 10% de 100.000 seria 10.000.
	if got := d.reflexaoDoEscudoDoTormento(com, 100_000); got != tormentoReflexaoTeto {
		t.Errorf("golpe enorme: reflete %d, want o teto %d", got, tormentoReflexaoTeto)
	}
	// Sem o Éden a passiva só defende.
	semEden := comArmas(bmDaNatureza(2800, 700, 320, int32(learnedEscudoDoTormento)), testHermai, testEscudo)
	if got := d.reflexaoDoEscudoDoTormento(semEden, 1000); got != 0 {
		t.Errorf("sem o Éden: reflete %d, want 0", got)
	}
	// Sem escudo, nada.
	semEscudo := comArmas(bmDaNatureza(2800, 700, 320, comEden), testHermai, 0)
	if got := d.reflexaoDoEscudoDoTormento(semEscudo, 1000); got != 0 {
		t.Errorf("sem escudo: reflete %d, want 0", got)
	}
	if got := d.reflexaoDoEscudoDoTormento(com, 0); got != 0 {
		t.Errorf("golpe de 0: reflete %d, want 0", got)
	}
}

// A reflexão NUNCA mata um jogador — para em 1 de vida. É o que mantém a classe
// batível: uma reflexão que mata faz de "bater no BM" um suicídio.
func TestReflexaoNaoMataJogador(t *testing.T) {
	d := dispatcherComEscudoAC()
	vitima := comArmas(bmDaNatureza(2800, 700, naturezaMaestriaCheia,
		int32(learnedEscudoDoTormento|learnedEden)), testHermai, testEscudo)
	vitima.ID = 1

	atacante := &world.Entity{ID: 2, Class: 0, HP: 50, MaxHP: 10_000}
	if got := d.aplicarReflexaoDoTormento(vitima, atacante, 10_000); got != 49 {
		t.Errorf("atacante com 50 de vida: refletiu %d, want 49", got)
	}
	if atacante.HP != 1 {
		t.Errorf("o atacante ficou com %d de vida, want 1", atacante.HP)
	}
	// Com 1 de vida não tira mais nada.
	if got := d.aplicarReflexaoDoTormento(vitima, atacante, 10_000); got != 0 {
		t.Errorf("atacante com 1 de vida: refletiu %d, want 0", got)
	}
	if atacante.HP != 1 {
		t.Errorf("o atacante ficou com %d, want 1", atacante.HP)
	}
	// Contra MONSTRO ela mata: ali não há ninguém para frustrar.
	mob := &world.Entity{ID: world.MaxUser + 1, HP: 50, MaxHP: 10_000}
	if got := d.aplicarReflexaoDoTormento(vitima, mob, 10_000); got != tormentoReflexaoTeto {
		t.Errorf("monstro: refletiu %d, want %d", got, tormentoReflexaoTeto)
	}
	if mob.HP != 0 {
		t.Errorf("o monstro ficou com %d de vida, want 0", mob.HP)
	}
	// Um morto não reflete, e ninguém reflete em si mesmo.
	morto := &world.Entity{ID: 3, HP: 0, MaxHP: 10_000}
	if got := d.aplicarReflexaoDoTormento(vitima, morto, 10_000); got != 0 {
		t.Errorf("atacante morto: refletiu %d, want 0", got)
	}
	if got := d.aplicarReflexaoDoTormento(vitima, vitima, 10_000); got != 0 {
		t.Errorf("em si mesmo: refletiu %d, want 0", got)
	}
	if got := d.aplicarReflexaoDoTormento(vitima, nil, 10_000); got != 0 {
		t.Errorf("nil: refletiu %d, want 0", got)
	}
}
