//go:build simulacao

package handler

// AS EVOCAÇÕES CONTRA O ELENCO (19/09/2026).
//
// A regra em prova é a de arvore_evocacao.go: o golpe da evocação em jogador
// não passa pela conta do servidor, é o número da criatura curvado pela defesa
// do alvo. O que estes diagnósticos medem é se isso entrega os números pedidos
// SEM desabar contra alvo blindado, e onde o bando inteiro cai no elenco.
//
//	go test -tags simulacao -run TestDiagnosticoEvocacao -v ./tmserver/internal/handler/

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

var nomeDaCriatura = [9]string{
	"Condor", "Javali", "Lobo", "Urso", "Tigre", "Gorila", "Dragão Negro", "Succubus", "Invocação Final",
}

// O golpe de cada criatura contra a defesa real de cada personagem do elenco.
func TestDiagnosticoEvocacaoContraOElenco(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	fmt.Printf("golpe de UMA cabeça, Evocação %d, defesa de referência %d\n\n", evocacaoMaestriaCheia, evocacaoDefesaRef)
	fmt.Printf("%-28s %6s", "alvo", "defesa")
	for _, c := range []int{4, 6, 7} {
		fmt.Printf(" %10s", nomeDaCriatura[c])
	}
	fmt.Println()
	for _, c := range sm.elenco() {
		l := comVidaExtra(c.novo(1), simVidaExtra)
		def := int(effectiveAC(l.e))
		fmt.Printf("%-28s %6d", c.nome, def)
		for _, cr := range []int{4, 6, 7} {
			fmt.Printf(" %10d", danoDaEvocacaoEmJogador(cr, evocacaoMaestriaCheia, def))
		}
		fmt.Println()
	}
}

// O bando inteiro: dano por segundo de cada criatura na cadência de 3 s, contra
// o alvo mais blindado e contra o menos blindado do elenco.
func TestDiagnosticoEvocacaoBandoInteiro(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	menor, maior := 1<<30, 0
	nomeMenor, nomeMaior := "", ""
	for _, c := range sm.elenco() {
		def := int(effectiveAC(comVidaExtra(c.novo(1), simVidaExtra).e))
		if def < menor {
			menor, nomeMenor = def, c.nome
		}
		if def > maior {
			maior, nomeMaior = def, c.nome
		}
	}

	fmt.Printf("cadência %d ms; alvo leve = %s (defesa %d), alvo pesado = %s (defesa %d)\n\n",
		evocacaoCadenciaMs, nomeMenor, menor, nomeMaior, maior)
	fmt.Printf("%-14s %7s %8s %8s %9s %9s %7s\n",
		"criatura", "cabeças", "golpe/lv", "golpe/pd", "dps/leve", "dps/pesado", "razão")
	for cr := range 8 {
		n := summonCount(cr+1, evocacaoMaestriaCheia)
		lv := danoDaEvocacaoEmJogador(cr, evocacaoMaestriaCheia, menor)
		pd := danoDaEvocacaoEmJogador(cr, evocacaoMaestriaCheia, maior)
		porSeg := func(golpe int) float64 {
			return float64(golpe*n) * 1000 / float64(evocacaoCadenciaMs)
		}
		fmt.Printf("%-14s %7d %8d %8d %9.0f %9.0f %6.2f×\n",
			nomeDaCriatura[cr], n, lv, pd, porSeg(lv), porSeg(pd), float64(lv)/float64(pd))
	}
}

// A comparação que decide: o bando das três criaturas de topo contra o dano por
// segundo que cada personagem do elenco tira do Porradeiro Trans.
func TestDiagnosticoEvocacaoContraODanoDoElenco(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	trans := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
	defTrans := int(effectiveAC(trans.e))
	fmt.Printf("todos contra o Porradeiro Trans (defesa %d, HP %d)\n\n", defTrans, trans.e.HP)

	tipo := []struct {
		nome string
		dps  float64
	}{}
	for _, c := range sm.elenco() {
		a := comVidaExtra(c.novo(1), simVidaExtra)
		b := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
		_, dps := danoPorSegundo(sm, a, b, 400)
		tipo = append(tipo, struct {
			nome string
			dps  float64
		}{c.nome, dps})
	}
	for cr := range 8 {
		n := summonCount(cr+1, evocacaoMaestriaCheia)
		g := danoDaEvocacaoEmJogador(cr, evocacaoMaestriaCheia, defTrans)
		tipo = append(tipo, struct {
			nome string
			dps  float64
		}{fmt.Sprintf("bando: %d × %s", n, nomeDaCriatura[cr]),
			float64(g*n) * 1000 / float64(evocacaoCadenciaMs)})
	}
	for i := range tipo {
		for j := i + 1; j < len(tipo); j++ {
			if tipo[j].dps > tipo[i].dps {
				tipo[i], tipo[j] = tipo[j], tipo[i]
			}
		}
	}
	for _, x := range tipo {
		marca := ""
		if x.dps > float64(simPocaoPorSegundo) {
			marca = "passa da poção"
		}
		fmt.Printf("%-32s %8.0f /s   %s\n", x.nome, x.dps, marca)
	}
	fmt.Printf("\n(a Ultra Poção de Cura levanta a barra em %d/s)\n", 500)
}

// Quanto a Succubus precisa por golpe para o BANDO dela valer N vezes o bando
// de Tigres, e onde cada N a põe no elenco.
func TestDiagnosticoEvocacaoSuccubusForteComoOTigre(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37

	trans := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
	def := int(effectiveAC(trans.e))
	cabecasT := summonCount(5, evocacaoMaestriaCheia) // Tigre
	cabecasS := summonCount(8, evocacaoMaestriaCheia) // Succubus
	dps := func(golpe, cabecas int) float64 {
		return float64(golpe*cabecas) * 1000 / float64(evocacaoCadenciaMs)
	}
	golpeT := danoDaEvocacaoEmJogador(4, evocacaoMaestriaCheia, def)
	baseT := dps(golpeT, cabecasT)
	fmt.Printf("alvo: Porradeiro Trans, defesa %d\n", def)
	fmt.Printf("Tigre: %d cabeças × %d = %.0f/s (a régua)\n\n", cabecasT, golpeT, baseT)

	fmt.Printf("%-6s %10s %10s %10s %9s\n", "alvo", "tabela", "golpe/ref", "golpe/Trans", "dps")
	for _, vezes := range []float64{1.0, 1.5, 2.0, 2.5, 3.0} {
		alvoDps := baseT * vezes
		golpe := int(alvoDps * float64(evocacaoCadenciaMs) / 1000 / float64(cabecasS))
		// desfaz a curva para achar o número que vai na tabela
		tabela := golpe * (evocacaoDefesaRef + def) / (2 * evocacaoDefesaRef)
		fmt.Printf("%5.1f× %10d %10d %11d %9.0f\n", vezes, tabela,
			danoDaEvocacaoEmJogador2(tabela, evocacaoDefesaRef), golpe, alvoDps)
	}

	fmt.Printf("\npara comparar, contra o mesmo Trans:\n")
	for _, c := range sm.elenco() {
		a := comVidaExtra(c.novo(1), simVidaExtra)
		b := comVidaExtra(sm.transLutador(2, simEdenAnct), simVidaExtra)
		_, d := danoPorSegundo(sm, a, b, 400)
		fmt.Printf("  %-28s %8.0f /s\n", c.nome, d)
	}
}

// danoDaEvocacaoEmJogador2 é a curva com a base dada direto, para a varredura
// acima não depender da tabela do servidor.
func danoDaEvocacaoEmJogador2(base, defesa int) int {
	return base * 2 * evocacaoDefesaRef / (evocacaoDefesaRef + defesa)
}

// Quantos golpes cada um do elenco leva para derrubar cada evocação, com a AC e
// o HP que estão no ar (summonBonus). A régua pedida é "o TK-MAGO mata em no
// máximo 2 skills".
func TestDiagnosticoQuantosGolpesMatamAEvocacao(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	const vidaDeTeste = int32(1 << 30)
	const evo = 320

	// AC e HP por criatura, como summonBonus os entrega na Evocação cheia.
	type bicho struct {
		nome   string
		ac, hp int32
	}
	bichos := []bicho{}
	for i := range 8 {
		b := summonBonus[i]
		bichos = append(bichos, bicho{nomeDaCriatura[i], evo * b.acEvo / 100, evo * b.hpEvo / 100})
	}

	fmt.Printf("%-14s %6s %7s", "criatura", "AC", "HP")
	for _, c := range sm.elenco() {
		fmt.Printf(" %10.10s", c.nome)
	}
	fmt.Println()
	for _, b := range bichos {
		fmt.Printf("%-14s %6d %7d", b.nome, b.ac, b.hp)
		for _, c := range sm.elenco() {
			a := comVidaExtra(c.novo(1), simVidaExtra)
			alvo := petAlvo(2, b.ac, vidaDeTeste)
			total, n := 0, 0
			var agora int64
			for range 600 {
				alvo.HP = vidaDeTeste
				g := a.acao(a.lado, alvo, agora)
				total += g.dano
				n++
				agora += simPasso
			}
			medio := float64(total) / float64(n)
			if medio <= 0 {
				fmt.Printf(" %10s", "nunca")
				continue
			}
			fmt.Printf(" %10.1f", float64(b.hp)/medio)
		}
		fmt.Println()
	}
	fmt.Println("\n(golpes médios para matar; cada golpe é uma ação de 800 ms)")
}

// petAlvo é uma evocação como alvo: mob (ID >= MaxUser) do Clan 4, que é o que
// liga o ÷4 da perfuração no golpe recebido.
func petAlvo(id int, ac, hp int32) *world.Entity {
	// Summoner preenchido: sem dono a criatura não é uma evocação para as regras
	// de PvP (danoEmEvocacao, donoDaEvocacao).
	return &world.Entity{ID: world.MaxUser + id, Clan: summonClan, Summoner: 1, Level: 399, AC: ac, HP: hp, MaxHP: hp}
}

// Varredura do botão de dano recebido: quantas SKILLS cada mago precisa para
// derrubar cada evocação, em cada ajuste. A régua pedida é "o TK-MAGO mata em no
// máximo 2 skills".
func TestDiagnosticoBotaoDoDanoNaEvocacao(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	original := evocacaoDanoRecebidoPct
	defer func() { evocacaoDanoRecebidoPct = original }()
	const vidaDeTeste = int32(1 << 30)
	const evo = 320

	magos := []struct {
		nome string
		novo func(int) *lutador
	}{
		{"TK-MAGO", sm.tkMagoLutador},
		{"Black", sm.blackLutador},
	}
	for _, pct := range []int{100, 130, 150, 200} {
		evocacaoDanoRecebidoPct = pct
		fmt.Printf("\n=== dano recebido %d%% ===\n", pct)
		fmt.Printf("%-14s %7s", "criatura", "HP")
		for _, m := range magos {
			fmt.Printf(" %12s", m.nome)
		}
		fmt.Println()
		for i := range 8 {
			b := summonBonus[i]
			ac, hp := evo*b.acEvo/100, evo*b.hpEvo/100
			fmt.Printf("%-14s %7d", nomeDaCriatura[i], hp)
			for _, m := range magos {
				a := comVidaExtra(m.novo(1), simVidaExtra)
				alvo := petAlvo(2, ac, vidaDeTeste)
				// Só SKILLS: o golpe físico entre elas não é o que a régua mede.
				soma, n := 0, 0
				var agora int64
				for range 900 {
					alvo.HP = vidaDeTeste
					g := a.acao(a.lado, alvo, agora)
					if g.tipo != "físico" && g.dano > 0 {
						soma += g.dano
						n++
					}
					agora += simPasso
				}
				if n == 0 {
					fmt.Printf(" %12s", "-")
					continue
				}
				fmt.Printf(" %12.1f", float64(hp)/(float64(soma)/float64(n)))
			}
			fmt.Println()
		}
	}
	fmt.Println("\n(skills médias para matar; a régua é o TK-MAGO em no máximo 2)")
}
