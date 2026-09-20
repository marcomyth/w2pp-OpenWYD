//go:build simulacao

package handler

// O BM NATUREZA (20/09/2026) — o eixo Força↔Destreza e a Armadura Elemental.
//
// A ficha NÃO vem de janela do jogo (o BM Natureza ainda não foi montado em
// jogo). Vem do PORRADEIRO, o físico de referência do elenco: mesmos atributos
// somados, mesma vida, mesmo ataque, mesma defesa, mesmo refino. É de propósito
// — o que se quer medir é a ÁRVORE, não a build, e com as fichas iguais a
// diferença que sobra é só a classe.
//
// A soma FOR+DES é CONSTANTE em todas as linhas. Sem isso, uma build de Força
// pareceria melhor só por ter mais atributo, e não por estar no eixo.
//
// A empunhadura (Balmung/Caliburn/Hermai/escudo) é a etapa 2 e ainda não entra
// aqui: esta medida é só do eixo e da absorção.
//
//	go test -tags simulacao -run TestSimulacaoNatureza -v ./tmserver/internal/handler/

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// simBMNaturezaMaestria é a Natureza cheia do BM que escolheu esta árvore.
const simBMNaturezaMaestria = 320

// buildDoEixo é uma distribuição FOR/DES com a soma do Porradeiro.
type buildDoEixo struct {
	nome         string
	forca, destr int16
}

// buildsDoEixo varre o eixo de ponta a ponta, sempre com FOR+DES = 3514.
var buildsDoEixo = []buildDoEixo{
	{"Força pura", 2802, 712},
	{"Força", 2450, 1064},
	{"Força leve", 2100, 1414},
	{"meio a meio", 1757, 1757},
	{"Destreza leve", 1414, 2100},
	{"Destreza", 1064, 2450},
	{"Destreza pura", 712, 2802},
}

// janelaDoBMNatureza é a ficha do Porradeiro com classe 2 e o eixo variando.
func janelaDoBMNatureza(b buildDoEixo) janela {
	j := porradeiro
	j.nome = "BM Natureza"
	j.classe = 2
	j.str, j.dex = b.forca, b.destr
	// [1] Elemental, [2] Evocação, [3] Natureza. A árvore da 8ª vai cheia; as
	// outras duas param no teto de quem não tem oitava.
	j.special = [4]int16{0, simBMTetoSemOitava, simBMTetoSemOitava, simBMNaturezaMaestria}
	j.learned = simBMLearned | learnedEden
	return j
}

// bmNaturezaLutador monta o BM com a empunhadura NEUTRA (Caliburn sozinha), que
// é a linha de base da classe.
func (sm *simulador) bmNaturezaLutador(id int, b buildDoEixo) *lutador {
	return sm.bmNaturezaCom(id, b, simCaliburn, 0)
}

// bmNaturezaCom monta o BM com uma empunhadura escolhida.
//
// A arma entra DEPOIS da calibragem, de propósito: assim todas as empunhaduras
// partem do mesmo ataque, e o que sobra na diferença entre as linhas é só o que
// a árvore paga — não o dano de arma de cada item.
func (sm *simulador) bmNaturezaCom(id int, b buildDoEixo, dir, esq int16) *lutador {
	j := janelaDoBMNatureza(b)
	e := sm.montar(j, id, nil)
	refino := [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}
	e.Equip[weaponSlotR] = world.Item{Index: dir, Effects: refino}
	if esq != 0 {
		e.Equip[weaponSlotL] = world.Item{Index: esq, Effects: refino}
	}
	sm.d.applyAffectScore(e)
	l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "BM", maxHP: j.hp}
	l.acao = func(ld *lado, alvo *world.Entity, _ int64) golpe { return sm.fisico(ld, alvo) }
	return l
}

// As armas reais que o desenho da árvore nomeia.
const (
	simCaliburn = 871  // espada de 1 mão
	simBalmung  = 811  // machado/martelo de 1 mão
	simHermai   = 886  // arma de arremesso
	simEscudo   = 1713 // Escudo Svalin
	simGarraBM  = 2595
)

// TestSimulacaoNaturezaEmpunhadura mostra o que cada modo de empunhar entrega:
// a absorção, o multiplicador de dano e o crítico.
func TestSimulacaoNaturezaEmpunhadura(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	for _, b := range []buildDoEixo{buildsDoEixo[0], buildsDoEixo[6]} {
		fmt.Printf("\n=== build %s (FOR %d / DES %d) ===\n", b.nome, b.forca, b.destr)
		fmt.Printf("%-26s %9s %8s %8s %9s\n", "empunhadura", "absorção", "dano", "crítico", "ataque")
		for _, c := range []struct {
			nome     string
			dir, esq int16
		}{
			{"Caliburn sozinha", simCaliburn, 0},
			{"Caliburn + Balmung", simCaliburn, simBalmung},
			{"Caliburn + escudo", simCaliburn, simEscudo},
			{"Hermai + escudo", simHermai, simEscudo},
			{"garra (fora da árvore)", simGarraBM, 0},
		} {
			e := sm.bmNaturezaCom(1, b, c.dir, c.esq).e
			fmt.Printf("%-26s %8.1f%% %7d%% %8d %9d\n", c.nome,
				float64(sm.d.absorcaoDaArmaduraElementalDecimos(e))/10,
				e.AffDamageMultiPct, e.AffCritical, sm.d.effectiveDamage(e))
		}
	}
}

// TestSimulacaoNaturezaEixo mostra o que o eixo entrega em cada ponto: a
// absorção e o que ela vale em vida efetiva.
func TestSimulacaoNaturezaEixo(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	fmt.Printf("O EIXO FORÇA↔DESTREZA (soma constante %d, Natureza %d)\n\n",
		porradeiro.str+porradeiro.dex, simBMNaturezaMaestria)
	fmt.Printf("%-15s %6s %6s %7s %9s %12s\n", "build", "FOR", "DES", "eixo", "absorção", "vida efet.")
	for _, b := range buildsDoEixo {
		e := sm.bmNaturezaLutador(1, b).e
		abs := sm.d.absorcaoDaArmaduraElementalDecimos(e)
		efetiva := float64(e.HP) * 1000 / float64(1000-abs)
		fmt.Printf("%-15s %6d %6d %6d‰ %7.1f%% %10.0f (×%.2f)\n",
			b.nome, b.forca, b.destr, eixoForcaDestreza(e), float64(abs)/10, efetiva,
			efetiva/float64(e.HP))
	}
}

// TestSimulacaoNaturezaContraElenco é a pergunta que decide o número: quanto o
// elenco tira por segundo do BM, e em quanto tempo o mata. A meta é 1-2 minutos
// COM poção (applyCasting), que é o teto do tique de cura.
func TestSimulacaoNaturezaContraElenco(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	mata := func(dps float64, hp int32) string {
		if dps <= applyCasting {
			return "nunca"
		}
		return fmt.Sprintf("%.0f s", float64(hp)/(dps-applyCasting))
	}

	for _, b := range []buildDoEixo{buildsDoEixo[0], buildsDoEixo[3], buildsDoEixo[6]} {
		fmt.Printf("\n=== BM Natureza, build %s (FOR %d / DES %d) ===\n", b.nome, b.forca, b.destr)
		bm := comVidaExtra(sm.bmNaturezaLutador(1, b), simVidaExtra)
		fmt.Printf("absorção %.1f%%, HP %d, ataque %d, defesa %d\n\n",
			float64(sm.d.absorcaoDaArmaduraElementalDecimos(bm.e))/10, bm.e.HP,
			sm.d.effectiveDamage(bm.e), effectiveAC(bm.e))
		for _, o := range sm.elenco() {
			alvo := comVidaExtra(sm.bmNaturezaLutador(1, b), simVidaExtra)
			hpDoBM := alvo.e.HP
			_, contraOBM := danoPorSegundo(sm, comVidaExtra(o.novo(2), simVidaExtra), alvo, 400)

			oponente := comVidaExtra(o.novo(2), simVidaExtra)
			hpDele := oponente.e.HP
			_, doBM := danoPorSegundo(sm, comVidaExtra(sm.bmNaturezaLutador(1, b), simVidaExtra), oponente, 400)

			fmt.Printf("%-28s nele %6.0f/s (mata em %-7s) | no BM %6.0f/s (mata em %-7s)\n",
				o.nome, doBM, mata(doBM, hpDele), contraOBM, mata(contraOBM, hpDoBM))
		}
	}
}

// TestSimulacaoNaturezaEsquivaContraAbsorcao separa os dois efeitos que a build
// tem sobre o dano recebido: a ABSORÇÃO, que a árvore dá à Força, e a ESQUIVA,
// que a Destreza dá de graça pelo parry (attackerdex). Se a segunda for maior
// que a primeira, o eixo anda para o lado errado e o desenho não se sustenta.
func TestSimulacaoNaturezaEsquivaContraAbsorcao(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	semArmadura := func(id int, b buildDoEixo) *lutador {
		l := sm.bmNaturezaLutador(id, b)
		l.e.LearnedSkill &^= learnedArmaduraElemental
		return l
	}

	for _, nomeAtacante := range []string{"Black (FM Magia Negra)", "Porradeiro Trans (Éden)", "Xorimpas 8ª Captura"} {
		var atacante personagemDaSimulacao
		for _, o := range sm.elenco() {
			if o.nome == nomeAtacante {
				atacante = o
			}
		}
		fmt.Printf("\n=== quem bate: %s ===\n", nomeAtacante)
		fmt.Printf("%-15s %10s %10s %9s %9s\n", "build", "sem armad.", "com armad.", "absorção", "esquiva")
		var baseSemArmadura float64
		for i, b := range buildsDoEixo {
			alvoSem := comVidaExtra(semArmadura(1, b), simVidaExtra)
			_, sem := danoPorSegundo(sm, comVidaExtra(atacante.novo(2), simVidaExtra), alvoSem, 600)
			alvoCom := comVidaExtra(sm.bmNaturezaLutador(1, b), simVidaExtra)
			_, com := danoPorSegundo(sm, comVidaExtra(atacante.novo(2), simVidaExtra), alvoCom, 600)
			if i == 0 {
				baseSemArmadura = sem
			}
			// "absorção" é o que a árvore tirou; "esquiva" é o que a Destreza
			// tirou sozinha, medida contra a build de Força pura sem armadura.
			fmt.Printf("%-15s %10.0f %10.0f %8.1f%% %8.1f%%\n", b.nome, sem, com,
				(sem-com)/sem*100, (baseSemArmadura-sem)/baseSemArmadura*100)
		}
	}
}

// TestSimulacaoNaturezaTaxaDeErro conta quantos golpes simplesmente NÃO entram
// por build. É o que separa "esquiva" de "dano menor": se a taxa de zeros sobe
// com a Destreza, a vantagem dela é o parry, e o parry compõe igual à
// absorção — os dois disputam o mesmo espaço.
func TestSimulacaoNaturezaTaxaDeErro(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	fmt.Printf("%-15s %6s", "build", "DES")
	nomes := []string{"Black (FM Magia Negra)", "Porradeiro Trans (Éden)", "Xorimpas 8ª Captura"}
	for _, n := range nomes {
		fmt.Printf(" %22s", n)
	}
	fmt.Println()
	for _, b := range buildsDoEixo {
		fmt.Printf("%-15s %6d", b.nome, b.destr)
		for _, n := range nomes {
			var atacante personagemDaSimulacao
			for _, o := range sm.elenco() {
				if o.nome == n {
					atacante = o
				}
			}
			alvo := comVidaExtra(sm.bmNaturezaLutador(1, b), simVidaExtra)
			a := comVidaExtra(atacante.novo(2), simVidaExtra)
			vida := alvo.e.HP
			zeros, total := 0, 800
			var agora int64
			for i := 0; i < total; i++ {
				alvo.e.HP = vida
				g := a.acao(a.lado, alvo.e, agora)
				if g.dano <= 0 {
					zeros++
				}
				agora += simPasso
			}
			fmt.Printf(" %21.1f%%", float64(zeros)*100/float64(total))
		}
		fmt.Println()
	}
}

// TestSimulacaoNaturezaDuasBuilds é a pergunta da decisão A: as duas pontas do
// eixo são jogáveis, cada uma do seu jeito?
//
// FORÇA + escudo é o modo de aguentar (absorção e crítico); DESTREZA + duas
// armas é o modo de bater (esquiva e dano). Se uma delas não mata ninguém ou
// morre para todo mundo, o eixo não é uma escolha — é uma armadilha.
func TestSimulacaoNaturezaDuasBuilds(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	mata := func(dps float64, hp int32) string {
		if dps <= applyCasting {
			return "nunca"
		}
		return fmt.Sprintf("%.0f s", float64(hp)/(dps-applyCasting))
	}

	builds := []struct {
		nome     string
		b        buildDoEixo
		dir, esq int16
	}{
		{"FORÇA + escudo", buildsDoEixo[0], simHermai, simEscudo},
		{"DESTREZA + duas armas", buildsDoEixo[6], simCaliburn, simBalmung},
	}
	for _, bd := range builds {
		novo := func(id int) *lutador { return sm.bmNaturezaCom(id, bd.b, bd.dir, bd.esq) }
		e := novo(1).e
		fmt.Printf("\n=== %s: absorção %.1f%%, ataque %d, crítico %d ===\n", bd.nome,
			float64(sm.d.absorcaoDaArmaduraElementalDecimos(e))/10, sm.d.effectiveDamage(e), e.AffCritical)
		for _, o := range sm.elenco() {
			alvo := comVidaExtra(novo(1), simVidaExtra)
			hpDoBM := alvo.e.HP
			_, noBM := danoPorSegundo(sm, comVidaExtra(o.novo(2), simVidaExtra), alvo, 600)

			oponente := comVidaExtra(o.novo(2), simVidaExtra)
			hpDele := oponente.e.HP
			_, doBM := danoPorSegundo(sm, comVidaExtra(novo(1), simVidaExtra), oponente, 600)

			fmt.Printf("%-28s BM tira %6.0f/s (mata em %-7s) | leva %6.0f/s (morre em %-7s)\n",
				o.nome, doBM, mata(doBM, hpDele), noBM, mata(noBM, hpDoBM))
		}
	}
}

// bmNaturezaEden é o BM com o Éden de pé e a Metamorfose Superior na mão — a
// build que a árvore inteira serve.
func (sm *simulador) bmNaturezaEden(id int, b buildDoEixo, dir, esq int16) *lutador {
	l := sm.bmNaturezaCom(id, b, dir, esq)
	// Afeto 16, Value 5 = Éden (transMesh 32). O Level é a maestria da Natureza
	// na hora do lançamento (transform.go).
	l.e.Affect[1] = world.Affect{Type: affectTransform, Value: 5,
		Level: simBMNaturezaMaestria, Time: 5000}
	sm.d.applyAffectScore(l.e)
	l.e.HP = effectiveMaxHP(l.e)
	l.maxHP = l.e.HP
	return l
}

// TestSimulacaoNaturezaEixoEquilibrado é a pergunta da etapa 3: depois da
// Metamorfose, as duas pontas do eixo empatam?
//
// A Destreza ganha ~50% de esquiva de graça pela mecânica base do jogo
// (combat/critical.go), que é mais do que qualquer absorção alcança. A aposta
// da etapa 3 é que a Força recupere pelo ATAQUE e pela vida, não pela defesa.
func TestSimulacaoNaturezaEixoEquilibrado(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	builds := []struct {
		nome     string
		b        buildDoEixo
		dir, esq int16
	}{
		{"FORÇA + escudo", buildsDoEixo[0], simHermai, simEscudo},
		{"DESTREZA + duas armas", buildsDoEixo[6], simCaliburn, simBalmung},
	}
	fmt.Printf("%-24s %9s %8s %8s %9s %8s\n", "build (Éden de pé)", "absorção", "ataque", "HP", "crítico", "veloc.")
	for _, bd := range builds {
		e := sm.bmNaturezaEden(1, bd.b, bd.dir, bd.esq).e
		fmt.Printf("%-24s %8.1f%% %8d %8d %9d %8d\n", bd.nome,
			float64(sm.d.absorcaoDaArmaduraElementalDecimos(e))/10,
			sm.d.effectiveDamage(e), effectiveMaxHP(e), e.AffCritical, e.AffAttackSpeed)
	}

	mata := func(dps float64, hp int32) string {
		if dps <= applyCasting {
			return "nunca"
		}
		return fmt.Sprintf("%.0f s", float64(hp)/(dps-applyCasting))
	}
	for _, bd := range builds {
		novo := func(id int) *lutador { return sm.bmNaturezaEden(id, bd.b, bd.dir, bd.esq) }
		fmt.Printf("\n=== %s ===\n", bd.nome)
		var somaTira, somaLeva float64
		for _, o := range sm.elenco() {
			alvo := comVidaExtra(novo(1), simVidaExtra)
			hpDoBM := alvo.e.HP
			_, noBM := danoPorSegundo(sm, comVidaExtra(o.novo(2), simVidaExtra), alvo, 600)
			oponente := comVidaExtra(o.novo(2), simVidaExtra)
			hpDele := oponente.e.HP
			_, doBM := danoPorSegundo(sm, comVidaExtra(novo(1), simVidaExtra), oponente, 600)
			somaTira, somaLeva = somaTira+doBM, somaLeva+noBM
			fmt.Printf("%-28s tira %6.0f/s (mata em %-7s) | leva %6.0f/s (morre em %-7s)\n",
				o.nome, doBM, mata(doBM, hpDele), noBM, mata(noBM, hpDoBM))
		}
		n := float64(len(sm.elenco()))
		fmt.Printf("%-28s MÉDIA %6.0f/s tirando, %6.0f/s levando\n", "", somaTira/n, somaLeva/n)
	}
}

// TestSimulacaoNaturezaVarreduraDoEixo procura o par de números que empata as
// duas pontas no ATAQUE — a defesa já empatou com a camada da etapa 3.
//
// ATENÇÃO: a simulação NÃO modela velocidade de ataque — todo golpe sai a cada
// simPasso (800 ms) — então a coluna da velocidade não move nada aqui. Em jogo
// ela move, e sempre a favor da Destreza: o empate medido abaixo é um PISO.
//
// Os dois botões são a VELOCIDADE que a Destreza ganha e o DANO que a Força
// ganha. Varrer os dois juntos é o único jeito de ver o ponto de encontro: um
// sozinho sempre "melhora" a ponta que ele serve.
func TestSimulacaoNaturezaVarreduraDoEixo(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	original := naturezaCamada
	defer func() { naturezaCamada = original }()

	media := func(b buildDoEixo, dir, esq int16) (tira, leva float64) {
		novo := func(id int) *lutador { return sm.bmNaturezaEden(id, b, dir, esq) }
		for _, o := range sm.elenco() {
			_, noBM := danoPorSegundo(sm, comVidaExtra(o.novo(2), simVidaExtra),
				comVidaExtra(novo(1), simVidaExtra), 400)
			_, doBM := danoPorSegundo(sm, comVidaExtra(novo(1), simVidaExtra),
				comVidaExtra(o.novo(2), simVidaExtra), 400)
			tira, leva = tira+doBM, leva+noBM
		}
		n := float64(len(sm.elenco()))
		return tira / n, leva / n
	}

	fmt.Printf("%-10s %-10s | %-22s | %-22s | %s\n", "vel DES", "dano FOR",
		"FORÇA tira/leva", "DESTREZA tira/leva", "vantagem da Destreza")
	for _, vel := range []int{40} {
		for _, dano := range []int{35, 55, 70, 85, 100} {
			naturezaCamada = original
			naturezaCamada.vel = faixaDoEixo{vel, 5}
			naturezaCamada.dano = faixaDoEixo{10, dano}
			fTira, fLeva := media(buildsDoEixo[0], simHermai, simEscudo)
			dTira, dLeva := media(buildsDoEixo[6], simCaliburn, simBalmung)
			fmt.Printf("%-10d %-10d | %8.0f / %-11.0f | %8.0f / %-11.0f | %+.0f%%\n",
				vel, dano, fTira, fLeva, dTira, dLeva, (dTira/fTira-1)*100)
		}
	}
}
