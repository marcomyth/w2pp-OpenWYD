//go:build simulacao

package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// A COPA: fase de grupos (todos contra todos E contra si mesmos) + mata-mata.
//
//	SIM_OUT=copa.json go test -tags simulacao -run TestCopa -v ./tmserver/internal/handler/
//
// O espelho — a variação contra ela mesma — é o que nenhum confronto normal
// mede: vida, defesa e dano idênticos dos dois lados. Se a luta não acaba, a
// classe não vence a própria cura, e o que se vê é a parede da poção.
//
// A poção roda em copaPocaoPorSegundo, e não no teto de applyCasting: o PvP de
// verdade bebe 500/s, e 500 e 2.000 dão jogos completamente diferentes.
const copaPocaoPorSegundo = 500

type usoDeSkill struct {
	Nome  string `json:"nome"`
	Usos  int    `json:"usos"`
	Dano  int    `json:"dano"`
	Maior int    `json:"maior"`
}

type duelo struct {
	NomeA    string       `json:"a"`
	NomeB    string       `json:"b"`
	Vencedor string       `json:"vencedor"`
	Segundos float64      `json:"segundos"`
	VidaA    int32        `json:"vidaA"`
	VidaB    int32        `json:"vidaB"`
	MaxA     int32        `json:"maxA"`
	MaxB     int32        `json:"maxB"`
	CuraA    int64        `json:"curaA"`
	CuraB    int64        `json:"curaB"`
	DanoA    int          `json:"danoA"`
	DanoB    int          `json:"danoB"`
	SkillsA  []usoDeSkill `json:"skillsA"`
	SkillsB  []usoDeSkill `json:"skillsB"`
	AbriuA   bool         `json:"abriuA"`
}

func resumoDeSkills(gs []golpe) ([]usoDeSkill, int) {
	usos, dano, maior := map[string]int{}, map[string]int{}, map[string]int{}
	total := 0
	for _, g := range gs {
		if g.tipo == "" {
			continue
		}
		usos[g.tipo]++
		dano[g.tipo] += g.dano
		if g.dano > maior[g.tipo] {
			maior[g.tipo] = g.dano
		}
		total += g.dano
	}
	out := make([]usoDeSkill, 0, len(usos))
	for k := range usos {
		out = append(out, usoDeSkill{k, usos[k], dano[k], maior[k]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dano > out[j].Dano })
	return out, total
}

func (sm *simulador) duelar(a, b personagemDaSimulacao, aPrimeiro bool) duelo {
	la := comVidaExtra(a.novo(1), simVidaExtra)
	lb := comVidaExtra(b.novo(2), simVidaExtra)
	maxA, maxB := la.maxHP, lb.maxHP
	r := sm.lutaEntre(la, lb, aPrimeiro)
	sa, da := resumoDeSkills(r.golpesA)
	sb, db := resumoDeSkills(r.golpesB)
	d := duelo{NomeA: a.nome, NomeB: b.nome,
		Segundos: float64(r.ms) / 1000, VidaA: r.vidaA, VidaB: r.vidaB, MaxA: maxA, MaxB: maxB,
		CuraA: r.curaA, CuraB: r.curaB, DanoA: da, DanoB: db, SkillsA: sa, SkillsB: sb, AbriuA: aPrimeiro}
	switch {
	case r.vidaB <= 0:
		d.Vencedor = "A"
	case r.vidaA <= 0:
		d.Vencedor = "B"
	default:
		d.Vencedor = "empate"
	}
	return d
}

type linhaDaTabela struct {
	Nome         string  `json:"nome"`
	Vit          int     `json:"vit"`
	Der          int     `json:"der"`
	Emp          int     `json:"emp"`
	Saldo        int     `json:"saldo"`
	TempoMedio   float64 `json:"tempoMedio"`
	DanoMedio    int     `json:"danoMedio"`
	EspelhoVit   int     `json:"espelhoVit"`
	EspelhoEmp   int     `json:"espelhoEmp"`
	EspelhoTempo float64 `json:"espelhoTempo"`

	acabaram int
	somaS    float64
	somaDano int
	lutas    int
}

type chaveDuelo struct {
	Fase     string  `json:"fase"`
	A        string  `json:"a"`
	B        string  `json:"b"`
	PlacarA  int     `json:"placarA"`
	PlacarB  int     `json:"placarB"`
	Empates  int     `json:"empates"`
	Vencedor string  `json:"vencedor"`
	NoFio    bool    `json:"noFio"`
	Duelos   []duelo `json:"duelos"`
}

type fichaDaCopa struct {
	Nome      string  `json:"nome"`
	Ataque    int32   `json:"ataque"`
	Defesa    int32   `json:"defesa"`
	HP        int32   `json:"hp"`
	Critico   float64 `json:"critico"`
	Dex       int16   `json:"dex"`
	Esquivado float64 `json:"esquivado"`
	Tira      float64 `json:"tira"`
	Leva      float64 `json:"leva"`
	Pico      int     `json:"pico"`
	PicoNome  string  `json:"picoNome"`
}

type copa struct {
	Lutas   int             `json:"lutasPorPar"`
	Pocao   int             `json:"pocao"`
	PvPPct  int             `json:"pvpPct"`
	Tabela  []linhaDaTabela `json:"tabela"`
	Grade   [][]int         `json:"grade"`
	Ordem   []string        `json:"ordem"`
	Chaves  []chaveDuelo    `json:"chaves"`
	Campeao string          `json:"campeao"`
	Fichas  []fichaDaCopa   `json:"fichas"`
}

func TestCopa(t *testing.T) {
	const lutas = 10
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	sm.d.combatRules.PvPSkillPct, sm.d.combatRules.PvPMeleePct = 37, 37
	pocaoOrig := simPocaoPorSegundo
	simPocaoPorSegundo = copaPocaoPorSegundo
	defer func() { simPocaoPorSegundo = pocaoOrig }()

	el := sm.elencoDoTorneio()
	n := len(el)
	c := copa{Lutas: lutas, Pocao: int(simPocaoPorSegundo), PvPPct: 37}

	for i := range el {
		l := comVidaExtra(el[i].novo(1), simVidaExtra)
		var sofre, tira, leva float64
		pico, picoNome, k := 0, "", 0
		for j := range el {
			if i == j {
				continue
			}
			o := comVidaExtra(el[j].novo(2), simVidaExtra)
			sofre += float64(sm.d.skillParryRate(l.e, o.e)) / 10
			_, tr := danoPorSegundo(sm, comVidaExtra(el[i].novo(1), simVidaExtra), comVidaExtra(el[j].novo(2), simVidaExtra), 400)
			_, lv := danoPorSegundo(sm, comVidaExtra(el[j].novo(1), simVidaExtra), comVidaExtra(el[i].novo(2), simVidaExtra), 400)
			tira, leva = tira+tr, leva+lv
			// o maior golpe que ela arranca — o número que o operador cobra em ~2.000
			at := comVidaExtra(el[i].novo(1), simVidaExtra)
			al := comVidaExtra(el[j].novo(2), simVidaExtra)
			vida := al.e.HP
			var agora int64
			for range 300 {
				al.e.HP = vida
				g := at.acao(at.lado, al.e, agora)
				if g.dano > pico {
					pico, picoNome = g.dano, g.tipo
				}
				agora += simPasso
			}
			k++
		}
		c.Fichas = append(c.Fichas, fichaDaCopa{el[i].nome, sm.d.effectiveDamage(l.e), effectiveAC(l.e),
			l.maxHP, float64(effectiveCritical(l.e)) * 0.4, effectiveDex(l.e),
			sofre / float64(k), tira / float64(k), leva / float64(k), pico, picoNome})
	}

	tab := make([]linhaDaTabela, n)
	grade := make([][]int, n)
	for i := range tab {
		tab[i].Nome = el[i].nome
		grade[i] = make([]int, n)
	}
	for i := range el {
		for j := i; j < n; j++ {
			for k := range lutas {
				d := sm.duelar(el[i], el[j], k%2 == 0)
				if i == j {
					tab[i].EspelhoTempo += d.Segundos
					if d.Vencedor == "empate" {
						tab[i].EspelhoEmp++
					} else {
						tab[i].EspelhoVit++
					}
					continue
				}
				tab[i].lutas++
				tab[j].lutas++
				tab[i].somaDano += d.DanoA
				tab[j].somaDano += d.DanoB
				switch d.Vencedor {
				case "A":
					tab[i].Vit++
					tab[j].Der++
					grade[i][j]++
					tab[i].somaS += d.Segundos
					tab[i].acabaram++
				case "B":
					tab[j].Vit++
					tab[i].Der++
					grade[j][i]++
					tab[j].somaS += d.Segundos
					tab[j].acabaram++
				default:
					tab[i].Emp++
					tab[j].Emp++
				}
			}
		}
	}
	for i := range tab {
		tab[i].Saldo = tab[i].Vit - tab[i].Der
		if tab[i].acabaram > 0 {
			tab[i].TempoMedio = tab[i].somaS / float64(tab[i].acabaram)
		}
		if tab[i].lutas > 0 {
			tab[i].DanoMedio = tab[i].somaDano / tab[i].lutas
		}
		tab[i].EspelhoTempo /= float64(lutas)
	}

	ordem := make([]int, n)
	for i := range ordem {
		ordem[i] = i
	}
	sort.SliceStable(ordem, func(x, y int) bool {
		a, b := tab[ordem[x]], tab[ordem[y]]
		if a.Vit != b.Vit {
			return a.Vit > b.Vit
		}
		if a.Saldo != b.Saldo {
			return a.Saldo > b.Saldo
		}
		return a.Der < b.Der
	})
	for _, i := range ordem {
		c.Tabela = append(c.Tabela, tab[i])
		c.Ordem = append(c.Ordem, tab[i].Nome)
	}
	for _, i := range ordem {
		linha := make([]int, 0, n)
		for _, j := range ordem {
			linha = append(linha, grade[i][j])
		}
		c.Grade = append(c.Grade, linha)
	}

	// MATA-MATA: os 8 primeiros, 1×8, 2×7, 3×6, 4×5, melhor de sete. Empate não
	// conta para ninguém; série empatada é decidida por quem chegou mais perto de
	// matar — menos vida deixada no adversário, em fração do máximo dele.
	serie := func(fase string, a, b personagemDaSimulacao) (chaveDuelo, personagemDaSimulacao) {
		ch := chaveDuelo{Fase: fase, A: a.nome, B: b.nome}
		for k := 0; k < 7 && ch.PlacarA < 4 && ch.PlacarB < 4; k++ {
			d := sm.duelar(a, b, k%2 == 0)
			ch.Duelos = append(ch.Duelos, d)
			switch d.Vencedor {
			case "A":
				ch.PlacarA++
			case "B":
				ch.PlacarB++
			default:
				ch.Empates++
			}
		}
		if ch.PlacarA == ch.PlacarB {
			ch.NoFio = true
			var restaA, restaB float64
			for _, d := range ch.Duelos {
				restaA += float64(d.VidaB) / float64(d.MaxB)
				restaB += float64(d.VidaA) / float64(d.MaxA)
			}
			if restaA <= restaB {
				ch.PlacarA++
			} else {
				ch.PlacarB++
			}
		}
		if ch.PlacarA > ch.PlacarB {
			ch.Vencedor = a.nome
			return ch, a
		}
		ch.Vencedor = b.nome
		return ch, b
	}

	oito := make([]personagemDaSimulacao, 0, 8)
	for _, i := range ordem[:8] {
		oito = append(oito, el[i])
	}
	q := make([]personagemDaSimulacao, 4)
	for i := range 4 {
		ch, v := serie(fmt.Sprintf("Quartas %d", i+1), oito[i], oito[7-i])
		c.Chaves = append(c.Chaves, ch)
		q[i] = v
	}
	ch1, s1 := serie("Semifinal 1", q[0], q[3])
	ch2, s2 := serie("Semifinal 2", q[1], q[2])
	c.Chaves = append(c.Chaves, ch1, ch2)

	perdedor := func(ch chaveDuelo) personagemDaSimulacao {
		nome := ch.A
		if ch.Vencedor == ch.A {
			nome = ch.B
		}
		for _, p := range el {
			if p.nome == nome {
				return p
			}
		}
		return el[0]
	}
	ch3, _ := serie("Terceiro lugar", perdedor(ch1), perdedor(ch2))
	c.Chaves = append(c.Chaves, ch3)

	chf, camp := serie("FINAL", s1, s2)
	c.Chaves = append(c.Chaves, chf)
	c.Campeao = camp.nome

	fmt.Printf("\n=== A COPA — %d variações, %d lutas por par, poção %d/s ===\n\n", n, lutas, c.Pocao)
	fmt.Printf("%-28s %4s %4s %4s %6s %8s %9s %9s %14s\n", "variação", "V", "D", "E", "saldo", "tempo", "ataque", "pico", "espelho")
	fichaDe := map[string]fichaDaCopa{}
	for _, f := range c.Fichas {
		fichaDe[f.Nome] = f
	}
	for _, l := range c.Tabela {
		f := fichaDe[l.Nome]
		esp := fmt.Sprintf("%d/%d em %.0fs", l.EspelhoVit, lutas, l.EspelhoTempo)
		fmt.Printf("%-28s %4d %4d %4d %6d %7.0fs %9d %9d %14s\n",
			cortar(l.Nome, 28), l.Vit, l.Der, l.Emp, l.Saldo, l.TempoMedio, f.Ataque, f.Pico, esp)
	}
	fmt.Printf("\n=== MATA-MATA ===\n")
	for _, ch := range c.Chaves {
		fio := ""
		if ch.NoFio {
			fio = "  (decidida no fio)"
		}
		fmt.Printf("%-16s %-26s %d x %d  %-26s -> %s%s\n", ch.Fase, cortar(ch.A, 26), ch.PlacarA, ch.PlacarB, cortar(ch.B, 26), ch.Vencedor, fio)
	}
	fmt.Printf("\nCAMPEAO: %s\n", c.Campeao)

	if out := os.Getenv("SIM_OUT"); out != "" {
		b, err := json.MarshalIndent(c, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("escrito em %s (%d KB)", out, len(b)/1024)
	}
}
