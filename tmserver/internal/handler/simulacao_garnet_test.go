//go:build simulacao

package handler

// Garnet contra Esmeralda (17/09/2026): o quanto a absorção da Garnet e a
// perfuração da Esmeralda mudam a luta, em PvP e contra monstro.
//
//	go test -tags simulacao -run TestSimulacaoGarnetEsmeralda -v ./tmserver/internal/handler/
//
// SIM_OUT=<arquivo> grava o relatório em Markdown.
//
// A Esmeralda é a do servidor (EquipForceDamage, somada ao golpe depois do ÷4 e
// da regra PvP). A Garnet ainda não existe no servidor: aqui ela é a regra do
// legado (CMob.cpp:873) — 40 por passo de refino acima de +9, por peça (80 em
// grade 8) —, tirada do golpe que chega num jogador, com mínimo 1:
//   - de jogador, depois da Esmeralda e dos stats PvP (_MSG_Attack.cpp:1496);
//   - de monstro, depois da defesa (GetFunc.cpp:1639).
// O teto, quando há, limita a soma das peças.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// joiaPorPeca +15 fora de grade 8: 40 × 6 passos.
const joiaPorPeca = 240

// montagem de joias de um personagem: quantas peças +15 com cada uma.
type montagem struct {
	nome              string
	esmeraldas, garns int
}

var montagens = []montagem{
	{"sem joia", 0, 0},
	{"4 acessórios Esmeralda", 4, 0},
	{"4 acessórios Garnet", 0, 4},
	{"full Esmeralda (11 peças)", 11, 0},
	{"full Garnet (11 peças)", 0, 11},
}

// garnetPct, quando maior que zero, é o teto percentual: a Garnet nunca tira
// mais que esse tanto de cada golpe (opção A de 17/09).
var garnetPct int

// garnetAnulaEsmeralda: com teto percentual, a Garnet primeiro anula por inteiro
// a Esmeralda de quem bate e só o que sobra dela obedece ao teto, aplicado ao
// resto do golpe (variante A2).
var garnetAnulaEsmeralda bool

func (sm *simulador) absorveGarnet(atk, alvo *world.Entity, dmg int) int {
	g := sm.garnet[alvo.ID]
	if g <= 0 || dmg <= 0 {
		return dmg
	}
	if garnetPct <= 0 {
		return max(dmg-g, 1)
	}
	anula := 0
	if garnetAnulaEsmeralda && atk != nil {
		anula = min(g, int(atk.EquipForceDamage), dmg)
	}
	tira := anula + min(g-anula, (dmg-anula)*garnetPct/100)
	return max(dmg-tira, 1)
}

func (sm *simulador) vestir(e *world.Entity, m montagem, teto int) {
	e.EquipForceDamage = int32(m.esmeraldas * joiaPorPeca)
	g := m.garns * joiaPorPeca
	if teto > 0 {
		g = min(g, teto)
	}
	sm.garnet[e.ID] = g
}

type resultadoLuta struct {
	vitoriasA          int
	tempos             []float64
	golpesA, golpesB   []golpe
	danoMobMedio, nMob int
	mortesDoJogador    int
}

// pvp: HT (A) contra TK (B), alternando quem abre, 10 lutas.
func (sm *simulador) rodadaPvP(mHT, mTK montagem, teto int) resultadoLuta {
	var r resultadoLuta
	for i := range simRodadas {
		ht := &lado{e: sm.montar(xorimpas, 1, buffsHT()), cd: map[int]int64{}}
		tk := &lado{e: sm.montar(porradeiro, 2, nil), cd: map[int]int64{}}
		sm.vestir(ht.e, mHT, teto)
		sm.vestir(tk.e, mTK, teto)
		var agora int64
		for agora < simLimiteMs && ht.e.HP > 0 && tk.e.HP > 0 {
			if i%2 == 0 {
				ht.golpes = append(ht.golpes, sm.acaoHT(ht, tk.e, agora))
				if tk.e.HP > 0 {
					tk.golpes = append(tk.golpes, sm.fisico(tk, ht.e))
				}
			} else {
				tk.golpes = append(tk.golpes, sm.fisico(tk, ht.e))
				if ht.e.HP > 0 {
					ht.golpes = append(ht.golpes, sm.acaoHT(ht, tk.e, agora))
				}
			}
			agora += simPasso
			if agora%(simTickAfetoS*1000) < simPasso {
				sm.tickAfetos(tk.e)
			}
		}
		if tk.e.HP <= 0 {
			r.vitoriasA++
		}
		r.tempos = append(r.tempos, float64(agora)/1000)
		r.golpesA, r.golpesB = append(r.golpesA, ht.golpes...), append(r.golpesB, tk.golpes...)
	}
	return r
}

// espelhoTK: Porradeiro contra Porradeiro, alternando quem abre. mortesDoJogador
// conta as vitórias de B.
func (sm *simulador) espelhoTK(mA, mB montagem, teto int) resultadoLuta {
	var r resultadoLuta
	for i := range simRodadas {
		a := &lado{e: sm.montar(porradeiro, 1, nil), cd: map[int]int64{}}
		b := &lado{e: sm.montar(porradeiro, 2, nil), cd: map[int]int64{}}
		sm.vestir(a.e, mA, teto)
		sm.vestir(b.e, mB, teto)
		var agora int64
		for agora < simLimiteMs && a.e.HP > 0 && b.e.HP > 0 {
			x, y := a, b
			if i%2 == 1 {
				x, y = b, a
			}
			x.golpes = append(x.golpes, sm.fisico(x, y.e))
			if y.e.HP > 0 {
				y.golpes = append(y.golpes, sm.fisico(y, x.e))
			}
			agora += simPasso
		}
		if b.e.HP <= 0 {
			r.vitoriasA++
		}
		if a.e.HP <= 0 {
			r.mortesDoJogador++
		}
		r.tempos = append(r.tempos, float64(agora)/1000)
		r.golpesA, r.golpesB = append(r.golpesA, a.golpes...), append(r.golpesB, b.golpes...)
	}
	return r
}

// pve: a HT (com a montagem) contra o Cav. Lugefer, 10 lutas.
func (sm *simulador) rodadaPvE(m montagem, teto int, vida int32, danoX10, defesaX10 int) resultadoLuta {
	var r resultadoLuta
	somaMob := 0
	for range simRodadas {
		ht := &lado{e: sm.montar(xorimpas, 1, buffsHT()), cd: map[int]int64{}}
		sm.vestir(ht.e, m, teto)
		mob := sm.lugefer(2000, vida, danoX10, defesaX10)
		var agora, proxMob int64
		for agora < simLimiteMs && ht.e.HP > 0 && mob.HP > 0 {
			ht.golpes = append(ht.golpes, sm.acaoHT(ht, mob, agora))
			if mob.HP <= 0 {
				break
			}
			for proxMob <= agora && ht.e.HP > 0 {
				sm.d.tickCount = int(agora / 1000)
				dmg := sm.d.danoDoGolpeDeMonstro(sm.w, mob, ht.e)
				if dmg > 0 {
					dmg = sm.absorveGarnet(mob, ht.e, dmg)
					ht.e.HP = max(0, ht.e.HP-int32(dmg))
					somaMob += dmg
					r.nMob++
				}
				proxMob += int64(cadenciaDoGolpe(mob))
			}
			agora += simPasso
			if agora%(simTickAfetoS*1000) < simPasso {
				sm.tickAfetos(mob)
			}
		}
		if mob.HP <= 0 {
			r.vitoriasA++
		}
		if ht.e.HP <= 0 {
			r.mortesDoJogador++
		}
		r.tempos = append(r.tempos, float64(agora)/1000)
		r.golpesA = append(r.golpesA, ht.golpes...)
		sm.w.DespawnMob(mob.ID, 1)
	}
	r.danoMobMedio = div(somaMob, r.nMob)
	return r
}

func danoMedio(gs []golpe) int {
	r := resumir(gs, "")
	return div(r.soma, r.acertos)
}

func TestSimulacaoGarnetEsmeralda(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	var b strings.Builder
	fmt.Fprintf(&b, "# Garnet contra Esmeralda — %d lutas por cenário\n\n", simRodadas)
	fmt.Fprintf(&b, "Peça +15 fora de grade 8: %d de perfuração (Esmeralda) ou de absorção (Garnet). "+
		"4 acessórios = %d; full (11 peças) = %d. Sem poção.\n\n", joiaPorPeca, 4*joiaPorPeca, 11*joiaPorPeca)

	fmt.Fprintf(&b, "## PvP — Xorimpas (HT) contra Porradeiro (TK), sem teto\n\n"+
		"| HT | TK | HT venceu | Tempo médio (s) | Dano médio HT → TK | Dano médio TK → HT |\n|---|---|---|---|---|---|\n")
	for _, mHT := range montagens {
		for _, mTK := range montagens {
			if mHT.nome != "sem joia" && mTK.nome != "sem joia" && (mHT.esmeraldas > 0) == (mTK.esmeraldas > 0) {
				continue // Esmeralda × Esmeralda e Garnet × Garnet não respondem à pergunta
			}
			r := sm.rodadaPvP(mHT, mTK, 0)
			mt, _ := media(r.tempos)
			fmt.Fprintf(&b, "| %s | %s | %d de %d | %.1f | %d | %d |\n", mHT.nome, mTK.nome, r.vitoriasA, simRodadas, mt, danoMedio(r.golpesA), danoMedio(r.golpesB))
		}
	}

	fmt.Fprintf(&b, "\n## PvP espelho — Porradeiro (TK) contra Porradeiro (TK)\n\n"+
		"Mesmo personagem dos dois lados: isola o efeito das joias. Luta que passa de 15 min é empate.\n\n"+
		"| TK A | TK B | A venceu | B venceu | Tempo médio (s) | Dano médio A → B | Dano médio B → A |\n|---|---|---|---|---|---|---|\n")
	for _, par := range [][2]int{{0, 0}, {3, 0}, {4, 0}, {3, 4}, {1, 2}, {3, 2}, {1, 4}} {
		mA, mB := montagens[par[0]], montagens[par[1]]
		r := sm.espelhoTK(mA, mB, 0)
		mt, _ := media(r.tempos)
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %.1f | %d | %d |\n", mA.nome, mB.nome, r.vitoriasA, r.mortesDoJogador, mt, danoMedio(r.golpesA), danoMedio(r.golpesB))
	}
	for _, teto := range []int{1500, 1000, 500} {
		r := sm.espelhoTK(montagens[3], montagens[4], teto)
		mt, _ := media(r.tempos)
		fmt.Fprintf(&b, "| %s | %s, teto %d | %d | %d | %.1f | %d | %d |\n", montagens[3].nome, montagens[4].nome, teto, r.vitoriasA, r.mortesDoJogador, mt, danoMedio(r.golpesA), danoMedio(r.golpesB))
	}

	full := montagens[4]
	esm := montagens[3]
	fmt.Fprintf(&b, "\n## PvP — tetos da Garnet (full Esmeralda contra full Garnet)\n\n"+
		"| Teto | Garnet vale | HT Esmeralda × TK Garnet: tempo / dano HT→TK | HT Garnet × TK Esmeralda: tempo / dano TK→HT |\n|---|---|---|---|\n")
	for _, teto := range []int{0, 2000, 1500, 1000, 500} {
		a := sm.rodadaPvP(esm, full, teto)
		c := sm.rodadaPvP(full, esm, teto)
		ma, _ := media(a.tempos)
		mc, _ := media(c.tempos)
		vale := 11 * joiaPorPeca
		nome := "sem teto"
		if teto > 0 {
			vale, nome = min(vale, teto), fmt.Sprint(teto)
		}
		fmt.Fprintf(&b, "| %s | %d | %.1f s / %d | %.1f s / %d |\n", nome, vale, ma, danoMedio(a.golpesA), mc, danoMedio(c.golpesB))
	}

	cenariosPvE := []struct {
		nome               string
		vida               int32
		danoX10, defesaX10 int
	}{
		{"Cav. Lugefer como está", 0, 10, 10},
		{"Cav. Lugefer proposto (1 mi, dano ×2, defesa ×1,5)", 1_000_000, 20, 15},
	}
	for _, c := range cenariosPvE {
		fmt.Fprintf(&b, "\n## PvE — HT contra %s\n\n"+
			"| HT | Teto | HT venceu | HT morreu | Tempo médio (s) | Dano médio da HT | Golpes do monstro | Dano médio do monstro |\n|---|---|---|---|---|---|---|---|\n", c.nome)
		for _, m := range montagens {
			tetos := []int{0}
			if m.garns == 11 {
				tetos = []int{0, 1500, 1000, 500}
			}
			for _, teto := range tetos {
				r := sm.rodadaPvE(m, teto, c.vida, c.danoX10, c.defesaX10)
				mt, _ := media(r.tempos)
				nome := "—"
				if teto > 0 {
					nome = fmt.Sprint(teto)
				}
				fmt.Fprintf(&b, "| %s | %s | %d | %d | %.1f | %d | %d | %d |\n", m.nome, nome, r.vitoriasA, r.mortesDoJogador, mt, danoMedio(r.golpesA), r.nMob, r.danoMobMedio)
			}
		}
	}

	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSimulacaoGarnetPct: a opção A — a Garnet tira no máximo X% de cada golpe.
func TestSimulacaoGarnetPct(t *testing.T) {
	root := filepath.Join("..", "..", "..", "Release")
	sm := novoSimulador(t, root)
	defer func() { garnetPct, garnetAnulaEsmeralda = 0, false }()
	var b strings.Builder
	fmt.Fprintf(&b, "# Garnet com teto percentual — %d lutas por cenário\n\n", simRodadas)
	fmt.Fprintf(&b, "A Garnet tira min(soma das peças, X%% do golpe). Peça +15 = %d; full = %d. Sem poção.\n\n", joiaPorPeca, 11*joiaPorPeca)
	pcts := []struct {
		p     int
		anula bool
	}{{0, false}, {30, false}, {50, false}, {70, false}, {20, true}, {30, true}, {50, true}}
	nomePct := func(c struct {
		p     int
		anula bool
	}) string {
		if c.p == 0 {
			return "sem teto"
		}
		if c.anula {
			return fmt.Sprintf("A2 %d%%", c.p)
		}
		return fmt.Sprintf("A %d%%", c.p)
	}

	fmt.Fprintf(&b, "## Espelho TK\n\n| Teto | TK A | TK B | A venceu | B venceu | Tempo médio (s) | Dano A → B | Dano B → A |\n|---|---|---|---|---|---|---|---|\n")
	for _, p := range pcts {
		garnetAnulaEsmeralda = p.anula
		garnetPct = p.p
		for _, par := range [][2]int{{0, 4}, {3, 4}, {1, 4}, {1, 2}} {
			mA, mB := montagens[par[0]], montagens[par[1]]
			r := sm.espelhoTK(mA, mB, 0)
			mt, _ := media(r.tempos)
			fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %.1f | %d | %d |\n", nomePct(p), mA.nome, mB.nome, r.vitoriasA, r.mortesDoJogador, mt, danoMedio(r.golpesA), danoMedio(r.golpesB))
		}
	}

	fmt.Fprintf(&b, "\n## HT contra TK\n\n| Teto | HT | TK | HT venceu | Tempo médio (s) | Dano HT → TK | Dano TK → HT |\n|---|---|---|---|---|---|---|\n")
	for _, p := range pcts {
		garnetAnulaEsmeralda = p.anula
		garnetPct = p.p
		for _, par := range [][2]int{{0, 4}, {4, 0}, {3, 4}, {4, 3}} {
			mHT, mTK := montagens[par[0]], montagens[par[1]]
			r := sm.rodadaPvP(mHT, mTK, 0)
			mt, _ := media(r.tempos)
			fmt.Fprintf(&b, "| %s | %s | %s | %d de %d | %.1f | %d | %d |\n", nomePct(p), mHT.nome, mTK.nome, r.vitoriasA, simRodadas, mt, danoMedio(r.golpesA), danoMedio(r.golpesB))
		}
	}

	fmt.Fprintf(&b, "\n## PvE — HT contra o Cav. Lugefer\n\n| Teto | Lugefer | HT | HT venceu | HT morreu | Tempo médio (s) | Golpes do monstro | Dano médio do monstro |\n|---|---|---|---|---|---|---|---|\n")
	for _, p := range pcts {
		garnetAnulaEsmeralda = p.anula
		garnetPct = p.p
		for _, c := range []struct {
			nome               string
			vida               int32
			danoX10, defesaX10 int
		}{
			{"como está", 0, 10, 10},
			{"proposto", 1_000_000, 20, 15},
		} {
			for _, m := range []montagem{montagens[0], montagens[2], montagens[4]} {
				r := sm.rodadaPvE(m, 0, c.vida, c.danoX10, c.defesaX10)
				mt, _ := media(r.tempos)
				fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %.1f | %d | %d |\n", nomePct(p), c.nome, m.nome, r.vitoriasA, r.mortesDoJogador, mt, r.nMob, r.danoMobMedio)
			}
		}
	}

	t.Log("\n" + b.String())
	if out := os.Getenv("SIM_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
