//go:build simulacao

package handler

// O TORNEIO: cada variação do elenco contra cada outra, 10 duelos por par
// (20/09/2026, depois do eixo novo do BM Natureza).
//
// É a pergunta que nenhuma medida de "dano por segundo" responde: quem GANHA.
// Dano por segundo mede o soco; o duelo mede o soco contra a vida, a poção, a
// cura e a esquiva do outro ao mesmo tempo, e a ordem dos dois costuma ser
// diferente.
//
// Cada par joga 10 vezes alternando quem abre, porque abrir vale vantagem.
//
//	go test -tags simulacao -run TestTorneio -v ./tmserver/internal/handler/
import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// elencoDoTorneio é o elenco com as variações que ganharam regra nova: as duas
// pontas do eixo do BM Natureza e a FM Cancelamento de arco.
func (sm *simulador) elencoDoTorneio() []personagemDaSimulacao {
	extras := []personagemDaSimulacao{
		{"BM Natureza FORÇA", func(id int) *lutador {
			return sm.bmNaturezaEden(id, buildsDoEixo[0], simHermai, simEscudo)
		}},
		{"BM Natureza DESTREZA", func(id int) *lutador {
			return sm.bmNaturezaEden(id, buildsDoEixo[6], simCaliburn, simBalmung)
		}},
		{"FM Cancel de arco", func(id int) *lutador {
			e := sm.montarCancel(id)
			e.Equip[weaponSlotR] = world.Item{Index: 1006, // Arco Ásir
				Effects: [3]world.Effect{{Effect: simEfSanc, Value: simRefino11}}}
			e.Equip[weaponSlotL] = world.Item{}
			sm.d.applyAffectScore(e)
			deltaDoBotaoDaCancel(sm, e)
			l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "FM Cancel arco", maxHP: e.HP}
			// O mesmo turno da FM Cancelamento de duas armas: o arco troca a arma, não
			// a árvore. Medi-la só no golpe físico tirava dela o Cancelamento e a
			// Névoa, e era metade do porquê de ela não ganhar nenhuma luta.
			l.acao = sm.acaoDaCancel()
			return l
		}},
		// As duas 8ªs do BM que não estavam no torneio. Um personagem só aprende
		// UMA 8ª, então Elemental, Evocação e Natureza são três BMs diferentes e
		// têm de disputar separados. O Elemental vai de cajado de duas mãos, que é
		// o bônus de arma maior da árvore dele (140%, arvore_elemental.go).
		{"BM Elemental (cajado)", func(id int) *lutador {
			e := sm.montarBMCom(id, false, simBMTetoSemOitava, simCajado2Maos, learnedEspiritoVingador)
			l := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "BM Elemental", maxHP: effectiveMaxHP(e)}
			l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe { return sm.acaoDoBM(l, alvo, agora) }
			return l
		}},
		{"BM Evocador (Succubus)", func(id int) *lutador {
			// 7 é a Súcubo, o bando da 8ª dele (as lutasDoBM já usam 7, 4 e 6).
			return sm.bmLutador(id, false, 7)
		}},
	}
	return append(sm.elenco(), extras...)
}

type placar struct {
	nome                 string
	vitorias, derrotas   int
	empates              int
	somaMs               int64
	lutasQueAcabaram     int
	melhorContra, piorEm string
}

func TestTorneio(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	elenco := sm.elencoDoTorneio()
	n := len(elenco)
	placares := make([]placar, n)
	// vitorias[i][j] = quantas das 10 o i ganhou do j.
	vitorias := make([][]int, n)
	for i := range vitorias {
		vitorias[i] = make([]int, n)
		placares[i].nome = elenco[i].nome
	}

	for i := range elenco {
		for j := range elenco {
			if i >= j {
				continue // cada par joga uma vez só; o placar conta os dois lados
			}
			for k := range lutas {
				a := comVidaExtra(elenco[i].novo(1), simVidaExtra)
				b := comVidaExtra(elenco[j].novo(2), simVidaExtra)
				r := sm.lutaEntre(a, b, k%2 == 0)
				switch {
				case r.vidaB <= 0:
					vitorias[i][j]++
					placares[i].vitorias++
					placares[j].derrotas++
					placares[i].somaMs += r.ms
					placares[i].lutasQueAcabaram++
				case r.vidaA <= 0:
					vitorias[j][i]++
					placares[j].vitorias++
					placares[i].derrotas++
					placares[j].somaMs += r.ms
					placares[j].lutasQueAcabaram++
				default:
					placares[i].empates++
					placares[j].empates++
				}
			}
		}
	}

	ordem := make([]int, n)
	for i := range ordem {
		ordem[i] = i
	}
	sort.SliceStable(ordem, func(x, y int) bool {
		return placares[ordem[x]].vitorias > placares[ordem[y]].vitorias
	})

	fmt.Printf("\n=== O TORNEIO: %d duelos por par, %d variações, poção a %d/s ===\n\n",
		lutas, n, int(applyCasting))
	fmt.Printf("%-28s %6s %6s %8s %12s\n", "variação", "vit", "der", "empates", "tempo médio")
	for _, i := range ordem {
		p := placares[i]
		tempo := "—"
		if p.lutasQueAcabaram > 0 {
			tempo = fmt.Sprintf("%.0f s", float64(p.somaMs)/float64(p.lutasQueAcabaram)/1000)
		}
		fmt.Printf("%-28s %6d %6d %8d %12s\n", p.nome, p.vitorias, p.derrotas, p.empates, tempo)
	}

	fmt.Printf("\n=== A GRADE (linha ganhou da coluna, de %d) ===\n\n", lutas)
	fmt.Printf("%-24s", "")
	for _, j := range ordem {
		fmt.Printf("%5d", j)
	}
	fmt.Println()
	for _, i := range ordem {
		fmt.Printf("%2d %-21s", i, cortar(placares[i].nome, 21))
		for _, j := range ordem {
			if i == j {
				fmt.Printf("%5s", "—")
				continue
			}
			fmt.Printf("%5d", vitorias[i][j])
		}
		fmt.Println()
	}
}

func cortar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// TestDiagnosticoDoElenco mostra, para cada variação, o que ela TIRA e o que
// ela LEVA por segundo na média do elenco — os dois números que explicam o
// placar do torneio.
//
// Quem não passa da poção (applyCasting) não mata ninguém, por mais vida que
// tenha: é a diferença entre empatar 80 vezes e vencer.
func TestDiagnosticoDoElenco(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	elenco := sm.elencoDoTorneio()
	fmt.Printf("%-28s %10s %10s %12s\n", "variação", "tira/s", "leva/s", "passa a poção?")
	type linha struct {
		nome       string
		tira, leva float64
	}
	var linhas []linha
	for i := range elenco {
		var somaTira, somaLeva float64
		for j := range elenco {
			if i == j {
				continue
			}
			_, tira := danoPorSegundo(sm,
				comVidaExtra(elenco[i].novo(1), simVidaExtra),
				comVidaExtra(elenco[j].novo(2), simVidaExtra), 400)
			_, leva := danoPorSegundo(sm,
				comVidaExtra(elenco[j].novo(1), simVidaExtra),
				comVidaExtra(elenco[i].novo(2), simVidaExtra), 400)
			somaTira, somaLeva = somaTira+tira, somaLeva+leva
		}
		n := float64(len(elenco) - 1)
		linhas = append(linhas, linha{elenco[i].nome, somaTira / n, somaLeva / n})
	}
	sort.SliceStable(linhas, func(a, b int) bool { return linhas[a].tira > linhas[b].tira })
	for _, l := range linhas {
		passa := "NÃO"
		if l.tira > applyCasting {
			passa = fmt.Sprintf("sim (+%.0f)", l.tira-applyCasting)
		}
		fmt.Printf("%-28s %10.0f %10.0f %12s\n", l.nome, l.tira, l.leva, passa)
	}
}

// TestVarreduraDosBotoes mede cada botão de balanceamento isolado, contra a
// média do elenco. Um botão de cada vez, porque mexer em dois ao mesmo tempo
// esconde qual dos dois fez o quê.
func TestVarreduraDosBotoes(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	elenco := sm.elencoDoTorneio()
	// dps mede quanto a variação de nome `quem` tira, na média do elenco.
	dps := func(quem string, bonusDepois int32) float64 {
		var soma float64
		var eu personagemDaSimulacao
		for _, p := range elenco {
			if p.nome == quem {
				eu = p
			}
		}
		n := 0
		for _, o := range elenco {
			if o.nome == quem {
				continue
			}
			meu := comVidaExtra(eu.novo(1), simVidaExtra)
			// DEPOIS da calibragem: a montagem resolve o Damage base para o total
			// bater a janela, então um botão aplicado antes é reabsorvido e a
			// varredura inteira sai igual.
			meu.e.AffDamageMultiPct += bonusDepois
			_, tira := danoPorSegundo(sm, meu,
				comVidaExtra(o.novo(2), simVidaExtra), 400)
			soma += tira
			n++
		}
		return soma / float64(n)
	}

	fmt.Printf("a poção cura %d/s: quem não passa disso não mata ninguém\n\n", int(applyCasting))

	// 1) Xorimpas Sobrevivência: a recarga da Tempestade.
	original := sobrevivenciaDanoFisicoOitava
	fmt.Printf("%-34s %10s\n", "Sobrevivência — físico da 8ª (+%)", "tira/s")
	for _, ms := range []int{0, 40, 60, 80, 100} {
		sobrevivenciaDanoFisicoOitava = ms
		fmt.Printf("%-34d %10.0f\n", ms/1000, dps("Xorimpas 8ª Sobrevivência", 0))
	}
	sobrevivenciaDanoFisicoOitava = original

	// 2) Xorimpas Troca: o crítico do Golpe Felino com a 8ª.
	origCrit := trocaDanoFisicoOitava
	fmt.Printf("\n%-34s %10s\n", "Troca — físico da 8ª (+%)", "tira/s")
	for _, c := range []int{0, 40, 60, 80, 100} {
		trocaDanoFisicoOitava = c
		fmt.Printf("%-34d %10.0f\n", c, dps("Xorimpas 8ª Troca", 0))
	}
	trocaDanoFisicoOitava = origCrit

	// 3) A Black: o cajado de duas mãos.
	origCajado := magiaNegraCajado2MaosPct
	fmt.Printf("\n%-34s %10s\n", "Black — cajado de 2 mãos", "tira/s")
	for _, p := range []int{140, 130, 120, 110, 100} {
		magiaNegraCajado2MaosPct = p
		fmt.Printf("%-34d %10.0f\n", p, dps("Black (FM Magia Negra)", 0))
	}
	magiaNegraCajado2MaosPct = origCajado

	// 4) O BM Natureza de Destreza: a ponta de Destreza da faixa de dano.
	origCamada := naturezaCamada
	fmt.Printf("\n%-34s %10s\n", "BM Destreza — dano[DES] da camada", "tira/s")
	for _, d := range []int{93, 80, 70, 60, 50} {
		naturezaCamada = origCamada
		naturezaCamada.dano = faixaDoEixo{d, origCamada.dano[1]}
		fmt.Printf("%-34d %10.0f\n", d, dps("BM Natureza DESTREZA", 0))
	}
	naturezaCamada = origCamada
}

// TestComposicaoDoDanoDaHT abre o dano das três Xorimpas por AÇÃO. Dois botões
// seguidos (a recarga da Tempestade e o crítico do Golpe Felino) quase não
// moveram o total, e isso só pode significar uma coisa: o dano não está onde eu
// estava mexendo.
func TestComposicaoDoDanoDaHT(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	for _, nome := range []string{"Xorimpas 8ª Captura", "Xorimpas 8ª Troca", "Xorimpas 8ª Sobrevivência"} {
		var eu personagemDaSimulacao
		for _, p := range sm.elencoDoTorneio() {
			if p.nome == nome {
				eu = p
			}
		}
		a := comVidaExtra(eu.novo(1), simVidaExtra)
		b := comVidaExtra(sm.blackLutador(2), simVidaExtra)
		vida := b.e.HP
		porTipo := map[string]int{}
		vezes := map[string]int{}
		var agora int64
		const n = 600
		for range n {
			b.e.HP = vida
			g := a.acao(a.lado, b.e, agora)
			porTipo[g.tipo] += g.dano
			vezes[g.tipo]++
			agora += simPasso
		}
		fmt.Printf("\n=== %s (%d ações) ===\n", nome, n)
		tipos := make([]string, 0, len(porTipo))
		for k := range porTipo {
			tipos = append(tipos, k)
		}
		sort.SliceStable(tipos, func(i, j int) bool { return porTipo[tipos[i]] > porTipo[tipos[j]] })
		total := 0
		for _, k := range porTipo {
			total += k
		}
		for _, k := range tipos {
			fmt.Printf("  %-22s %5d vezes  %10d de dano  (%2d%% do total)\n",
				k, vezes[k], porTipo[k], porTipo[k]*100/max(total, 1))
		}
	}
}

// TestVarreduraDoBMDestrezaNoTorneio mede o nerf do BM de Destreza pelo que
// importa: VITÓRIAS, não dano por segundo. O dano médio dele já estava perto do
// da Black e ele ganhava mais duelos que ela — os dois números não andam juntos.
func TestVarreduraDoBMDestrezaNoTorneio(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	original := naturezaCamada
	defer func() { naturezaCamada = original }()

	fmt.Printf("%10s %8s %8s %10s %12s\n", "dano[DES]", "vit", "der", "empates", "tempo médio")
	for _, d := range []int{60, 59, 58, 57, 56, 55} {
		naturezaCamada = original
		naturezaCamada.dano = faixaDoEixo{d, original.dano[1]}
		elenco := sm.elencoDoTorneio()
		alvo := -1
		for i, p := range elenco {
			if p.nome == "BM Natureza DESTREZA" {
				alvo = i
			}
		}
		vit, der, emp := 0, 0, 0
		var soma int64
		var n int
		for j := range elenco {
			if j == alvo {
				continue
			}
			for k := range lutas {
				a := comVidaExtra(elenco[alvo].novo(1), simVidaExtra)
				b := comVidaExtra(elenco[j].novo(2), simVidaExtra)
				r := sm.lutaEntre(a, b, k%2 == 0)
				switch {
				case r.vidaB <= 0:
					vit++
					soma += r.ms
					n++
				case r.vidaA <= 0:
					der++
					soma += r.ms
					n++
				default:
					emp++
				}
			}
		}
		tempo := "—"
		if n > 0 {
			tempo = fmt.Sprintf("%.0f s", float64(soma)/float64(n)/1000)
		}
		fmt.Printf("%10d %8d %8d %10d %12s\n", d, vit, der, emp, tempo)
	}
}
