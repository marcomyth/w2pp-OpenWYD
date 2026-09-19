//go:build simulacao

package handler

// A FM CANCELAMENTO contra o elenco (18/09/2026).
//
// A ficha dela vem da janela do jogo JÁ com as regras da árvore no ar e com os
// buffs de pé, e o TestDiagnosticoFichaDaCancel confere campo a campo contra o
// print — é o teste da simulação contra a realidade, não o contrário.
//
// O resto mede as duas coisas que decidem a luta dela: o RITMO DA POÇÃO (a maior
// do catálogo é a Ultra Poção de Cura, de 500, e o teto do tique é 2.000/s) e a
// TRAVA do Cancelamento, que fecha a poção do alvo por 20 s.
//
//	go test -tags simulacao -run TestDiagnostico -v ./tmserver/internal/handler/

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"
)

// Confere a ficha da simulação contra a janela do jogo, campo a campo.
func TestDiagnosticoFichaDaCancel(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	e := sm.montarCancel(1)
	linha := func(nome string, jogo, sim int) {
		erro := 0.0
		if jogo != 0 {
			erro = float64(sim-jogo) * 100 / float64(jogo)
		}
		fmt.Printf("%-16s jogo %7d   simulação %7d   erro %+6.1f%%\n", nome, jogo, sim, erro)
	}
	linha("Ataque", cancelAtaque, int(sm.d.effectiveDamage(e)))
	linha("Defesa", cancelDefesa, int(effectiveAC(e)))
	linha("HP", cancelHP, int(effectiveMaxHP(e)))
	linha("MP", cancelMP, int(scoreMaxMP(e)))
	linha("Crítico (x10)", cancelCritico10, int(effectiveCritical(e))*4)
	linha("Maestria Esp.", cancelMaestria10, effectiveSpecial(e, 3))
	fmt.Printf("%-16s          -   simulação %7d   (Magia %d)\n", "Atq Mágico", 0, e.Magic)
	fmt.Printf("\ncomponentes do ataque: base %d + afeto %d, multiplicador %d%%, arma %d\n",
		e.Damage, e.AffDamage, e.AffDamageMultiPct, sm.d.weaponDamage(e))
}

// Com a ficha real: quanto cada um tira por segundo do outro, e quem mata.
func TestDiagnosticoCancelRealContraElenco(t *testing.T) {
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	fmt.Printf("FM Cancel: ataque %d, defesa %d, HP %d (+%d de acessório)\n\n",
		sm.d.effectiveDamage(sm.montarCancel(1)), effectiveAC(sm.montarCancel(1)),
		sm.montarCancel(1).HP, simVidaExtra)
	for _, o := range sm.elenco() {
		if o.nome == "FM Cancel" {
			continue
		}
		a := comVidaExtra(sm.cancelLutador(1), simVidaExtra)
		b := comVidaExtra(o.novo(2), simVidaExtra)
		hpDele := b.e.HP
		_, dela := danoPorSegundo(sm, a, b, 400)
		a2 := comVidaExtra(sm.cancelLutador(1), simVidaExtra)
		b2 := comVidaExtra(o.novo(2), simVidaExtra)
		hpDela := a2.e.HP
		_, dele := danoPorSegundo(sm, b2, a2, 400)
		mata := func(dps float64, hp int32) string {
			if dps <= applyCasting {
				return "nunca"
			}
			return fmt.Sprintf("%.0f s", float64(hp)/(dps-applyCasting))
		}
		fmt.Printf("%-28s ela %6.0f/s (mata em %-7s) | ele %6.0f/s (mata ela em %-7s)\n",
			o.nome, dela, mata(dela, hpDele), dele, mata(dele, hpDela))
	}
}

// Lutas de verdade com a ficha real e os valores que estão no ar.
func TestDiagnosticoCancelLutasReais(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	fmt.Printf("perfuração %d%%, dano com duas armas +%d%% (o que está no ar)\n\n",
		cancelPerfuracaoPct, cancelDanoDuasArmas)
	for _, o := range sm.elenco() {
		if o.nome == "FM Cancel" {
			continue
		}
		vit, der, emp := 0, 0, 0
		var soma int64
		for k := range lutas {
			r := sm.lutaEntre(comVidaExtra(sm.cancelLutador(1), simVidaExtra), comVidaExtra(o.novo(2), simVidaExtra), k%2 == 0)
			soma += r.ms
			switch {
			case r.vidaB <= 0:
				vit++
			case r.vidaA <= 0:
				der++
			default:
				emp++
			}
		}
		fmt.Printf("%-28s %d vitórias, %d derrotas, %d empates (média %.0f s)\n",
			o.nome, vit, der, emp, float64(soma)/float64(lutas)/1000)
	}
}

// O elenco inteiro contra si mesmo, por ritmo de poção: uma poção de 500 por
// segundo, duas, e o teto de 2.000/s de quem macra.
func TestDiagnosticoRitmoDaPocao(t *testing.T) {
	const lutas = 6
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	defer func(v int32) { simPocaoPorSegundo = v }(simPocaoPorSegundo)

	elenco := sm.elenco()
	for _, pocao := range []int32{500, 1000, 2000} {
		simPocaoPorSegundo = pocao
		acabaram, total := 0, 0
		var duracoes []int
		for i := range elenco {
			for j := range elenco {
				if i == j {
					continue
				}
				for k := range lutas {
					r := sm.lutaEntre(comVidaExtra(elenco[i].novo(1), simVidaExtra),
						comVidaExtra(elenco[j].novo(2), simVidaExtra), k%2 == 0)
					total++
					if r.vidaA <= 0 || r.vidaB <= 0 {
						acabaram++
						duracoes = append(duracoes, int(r.ms/1000))
					}
				}
			}
		}
		sort.Ints(duracoes)
		mediana := 0
		if len(duracoes) > 0 {
			mediana = duracoes[len(duracoes)/2]
		}
		dentro := 0
		for _, d := range duracoes {
			if d >= 60 && d <= 120 {
				dentro++
			}
		}
		fmt.Printf("poção %4d/s: %3d de %3d lutas terminam (%.0f%%), mediana %3d s, %d na meta de 1-2 min\n",
			pocao, acabaram, total, float64(acabaram)*100/float64(total), mediana, dentro)
	}
}

// A FM Cancel contra o elenco, cruzando o ritmo da poção com a trava do
// Cancelamento.
func TestDiagnosticoTrancaEPocao(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	defer func(v int32) { simPocaoPorSegundo = v }(simPocaoPorSegundo)

	for _, pocao := range []int32{500, 1000, 2000} {
		simPocaoPorSegundo = pocao
		fmt.Printf("\n=== poção %d/s ===\n", pocao)
		for _, o := range sm.elenco() {
			if o.nome == "FM Cancel" {
				continue
			}
			vit, der, emp := 0, 0, 0
			var soma int64
			for k := range lutas {
				r := sm.lutaEntre(comVidaExtra(sm.cancelLutador(1), simVidaExtra),
					comVidaExtra(o.novo(2), simVidaExtra), k%2 == 0)
				soma += r.ms
				switch {
				case r.vidaB <= 0:
					vit++
				case r.vidaA <= 0:
					der++
				default:
					emp++
				}
			}
			fmt.Printf("%-28s %2d vit, %2d der, %2d emp (média %3.0f s)\n",
				o.nome, vit, der, emp, float64(soma)/float64(lutas)/1000)
		}
	}
}
