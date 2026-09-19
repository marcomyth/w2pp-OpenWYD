//go:build simulacao

package handler

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Exploração: quanto da defesa da HT o TK Trans precisaria atravessar.
func TestSimulacaoTransPerfuracao(t *testing.T) {
	const lutas = 30
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "| %% PvP | Arma | Perfuração na HT | Vitórias | Mediana | Dano/s TK | Dano/s HT |\n|---|---|---|---|---|---|---|\n")
	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, arma := range []int16{simEdenAnct, simDemolidorAnct} {
			for _, perf := range []int32{0, 25, 50, 75} {
				var tempos []float64
				vit := map[string]int{}
				var tkSoma, htSoma, total float64
				for i := range lutas {
					ht := sm.htLutador(1)
					ht.e.AC = ht.e.AC * (100 - perf) / 100
					l := sm.lutaEntre(ht, sm.transLutador(2, arma), i%2 == 0)
					s := float64(l.ms) / 1000
					tempos = append(tempos, s)
					total += s
					vit[l.vencedor]++
					tkSoma += float64(resumir(l.golpesB, "").soma)
					htSoma += float64(resumir(l.golpesA, "").soma)
				}
				sort.Float64s(tempos)
				fmt.Fprintf(&b, "| %d%% | %d | %d%% | HT %d, TK %d, empate %d | %.0f s | %.0f | %.0f |\n", pct, arma, perf,
					vit["HT"], vit["TK"], vit["empate (15 min)"], tempos[lutas/2], tkSoma/total, htSoma/total)
			}
		}
	}
	t.Log("\n" + b.String())
}

// Exploração: com quanto de Ataque na janela o TK Trans vira a luta.
func TestSimulacaoTransAtaque(t *testing.T) {
	const lutas = 30
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "| %% PvP | Arma | Ataque na janela | Vitórias | Mediana | 1-2 min | Dano/s TK | Dano/s HT |\n|---|---|---|---|---|---|---|---|\n")
	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, arma := range []int16{simEdenAnct, simDemolidorAnct} {
			for _, atq := range []int32{5515, 8111, 10000, 12000, 14000} {
				j := porradeiro
				j.ataque = atq
				var tempos []float64
				vit := map[string]int{}
				var tkSoma, htSoma, total float64
				meta := 0
				for i := range lutas {
					e, hp := sm.montarTrans(j, 2, arma)
					tk := &lutador{lado: &lado{e: e, cd: map[int]int64{}}, nome: "TK", maxHP: hp}
					tk.acao = func(ld *lado, alvo *world.Entity, _ int64) golpe { return sm.fisico(ld, alvo) }
					l := sm.lutaEntre(sm.htLutador(1), tk, i%2 == 0)
					s := float64(l.ms) / 1000
					tempos = append(tempos, s)
					total += s
					if s >= 60 && s <= 120 {
						meta++
					}
					vit[l.vencedor]++
					tkSoma += float64(resumir(l.golpesB, "").soma)
					htSoma += float64(resumir(l.golpesA, "").soma)
				}
				sort.Float64s(tempos)
				fmt.Fprintf(&b, "| %d%% | %d | %d | HT %d, TK %d, empate %d | %.0f s | %d | %.0f | %.0f |\n", pct, arma, atq,
					vit["HT"], vit["TK"], vit["empate (15 min)"], tempos[lutas/2], meta, tkSoma/total, htSoma/total)
			}
		}
	}
	t.Log("\n" + b.String())
}

// A Xorimpas com UMA 8ª só (um personagem não aprende as três): a Tempestade
// só sai com a 8ª da Sobrevivência.
func (sm *simulador) htUmaOitava(id int, oitava int32) *lutador {
	j := xorimpas
	j.learned = 0xFFFFFF&^(learnedTempestade|1<<15|learnedInvisibilidade) | oitava
	l := &lutador{lado: &lado{e: sm.montar(j, id, buffsHT()), cd: map[int]int64{}}, nome: "HT", maxHP: j.hp}
	l.acao = func(ld *lado, alvo *world.Entity, agora int64) golpe {
		for _, sk := range []int{skillTempestadeDeFlechas, skillGolpeFelino, skillLaminaDasSombras} {
			if sk == skillTempestadeDeFlechas && ld.e.LearnedSkill&learnedTempestade == 0 {
				continue
			}
			if agora < ld.cd[sk] {
				continue
			}
			sp, _ := sm.d.spells.Get(sk)
			espera := int64(sp.Delay) * 1000
			if sk == skillTempestadeDeFlechas {
				espera = tempestadeRecargaMs
			}
			ld.cd[sk] = agora + max(espera, simPasso)
			return sm.skill(ld, alvo, sk)
		}
		return sm.fisico(ld, alvo)
	}
	return l
}

func TestSimulacaoTransUmaOitava(t *testing.T) {
	const lutas = 30
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	var b strings.Builder
	fmt.Fprintf(&b, "| %% PvP | Xorimpas com a 8ª | TK | Vitórias | Mínimo | Mediana | 1-2 min | Dano/s TK | Dano/s HT | Maior golpe da HT | Maior golpe do TK |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	oitavas := []struct {
		nome string
		bit  int32
	}{
		{"todas (simulação anterior)", learnedTempestade | 1<<15 | learnedInvisibilidade},
		{"Sobrevivência (Tempestade)", learnedTempestade},
		{"Troca", 1 << 15},
		{"Captura (Invisibilidade)", learnedInvisibilidade},
	}
	tks := []struct {
		nome string
		novo func(int) *lutador
	}{
		{"hoje", sm.tkLutador},
		{"Trans Éden", func(id int) *lutador { return sm.transLutador(id, simEdenAnct) }},
		{"Trans Demolidor", func(id int) *lutador { return sm.transLutador(id, simDemolidorAnct) }},
	}
	for _, pct := range []int32{100, 37} {
		sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = pct, pct
		for _, o := range oitavas {
			for _, c := range tks {
				var tempos []float64
				vit := map[string]int{}
				var tkSoma, htSoma, total float64
				meta, maiorHT, maiorTK := 0, 0, 0
				for i := range lutas {
					l := sm.lutaEntre(sm.htUmaOitava(1, o.bit), c.novo(2), i%2 == 0)
					s := float64(l.ms) / 1000
					tempos = append(tempos, s)
					total += s
					if s >= 60 && s <= 120 {
						meta++
					}
					vit[l.vencedor]++
					rt, rh := resumir(l.golpesB, ""), resumir(l.golpesA, "")
					tkSoma, htSoma = tkSoma+float64(rt.soma), htSoma+float64(rh.soma)
					maiorHT, maiorTK = max(maiorHT, rh.max), max(maiorTK, rt.max)
				}
				sort.Float64s(tempos)
				fmt.Fprintf(&b, "| %d%% | %s | %s | HT %d, TK %d, empate %d | %.0f s | %.0f s | %d | %.0f | %.0f | %d | %d |\n", pct, o.nome, c.nome,
					vit["HT"], vit["TK"], vit["empate (15 min)"], tempos[0], tempos[lutas/2], meta, tkSoma/total, htSoma/total, maiorHT, maiorTK)
			}
		}
	}
	t.Log("\n" + b.String())
}

// Calibração do dano extra do TK Trans contra HT (Armadura Crítica).
func TestSimulacaoTransContraHT(t *testing.T) {
	const lutas = 30
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	defer func(v int) { transContraHTPct = v }(transContraHTPct)
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	var b strings.Builder
	fmt.Fprintf(&b, "| Bônus contra HT | Xorimpas com a 8ª | TK | Vitórias | Mínimo | Mediana | Máximo | 1-2 min | Dano/s TK | Dano/s HT |\n|---|---|---|---|---|---|---|---|---|---|\n")
	oitavas := []struct {
		nome string
		bit  int32
	}{
		{"Sobrevivência", learnedTempestade},
		{"Troca", 1 << 15},
		{"Captura", learnedInvisibilidade},
	}
	armas := []struct {
		nome string
		arma int16
	}{{"Éden", simEdenAnct}, {"Demolidor", simDemolidorAnct}}
	for _, bonus := range []int{100, 150, 200, 250, 300} {
		transContraHTPct = bonus
		for _, o := range oitavas {
			for _, a := range armas {
				var tempos []float64
				vit := map[string]int{}
				var tkSoma, htSoma, total float64
				meta := 0
				for i := range lutas {
					l := sm.lutaEntre(sm.htUmaOitava(1, o.bit), sm.transLutador(2, a.arma), i%2 == 0)
					s := float64(l.ms) / 1000
					tempos = append(tempos, s)
					total += s
					if s >= 60 && s <= 120 {
						meta++
					}
					vit[l.vencedor]++
					tkSoma += float64(resumir(l.golpesB, "").soma)
					htSoma += float64(resumir(l.golpesA, "").soma)
				}
				sort.Float64s(tempos)
				fmt.Fprintf(&b, "| +%d%% | %s | %s | HT %d, TK %d, empate %d | %.0f s | %.0f s | %.0f s | %d | %.0f | %.0f |\n", bonus, o.nome, a.nome,
					vit["HT"], vit["TK"], vit["empate (15 min)"], tempos[0], tempos[lutas/2], tempos[lutas-1], meta, tkSoma/total, htSoma/total)
			}
		}
	}
	t.Log("\n" + b.String())
}
