//go:build simulacao

package handler

// Simulação do Kefra contra uma guilda (18/09/2026): quantos minutos uma tropa de
// Huntress iguais ao Xorimpas leva para derrubar o chefe, por vida do chefe.
//
//	go test -tags simulacao -run TestSimulacaoKefra -v ./tmserver/internal/handler/
//
// O golpe do jogador e o do monstro saem das mesmas funções do servidor que a
// simulação da HT usa (acaoHT, danoDoGolpeDeMonstro, cadenciaDoGolpe).

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Quem morre volta depois disto: tempo de correr da cidade até o chefe. É o
// número mais incerto da simulação, e o relatório mostra a sensibilidade a ele.
var simVoltaMs int64 = 60_000

// simDebug liga o relatório de 10 em 10 minutos dentro da luta.
var simDebug bool

// kefra sobe o chefe do template, com a vida que se quer testar.
func (sm *simulador) kefra(id int, vida int32) *world.Entity {
	tmpl, _, err := npctemplate.Load(filepath.Join("..", "..", "..", "Release"), "Kefra")
	if err != nil {
		sm.t.Skipf("Kefra: %v", err)
	}
	mid := sm.w.SpawnMob(tmpl, 30, 30)
	m := sm.w.Entity(mid)
	// BaseMaxHP junto: refreshScore refaz MaxHP a partir dele quando um afeto
	// expira no monstro, e sem isto o chefe encolhia de volta para o número do
	// template no meio da luta (é o mesmo cuidado da Torre, towerwar.go).
	m.MaxHP, m.HP, m.BaseMaxHP = vida, vida, vida
	return m
}

type resultadoRaide struct {
	ms           int64
	mortes       int
	danoTotal    int64
	golpes       int
	golpesMob    int
	danoMobTotal int64
	vidaRestant  int32
	veneno       int64
}

// raide: n Huntress batendo no Kefra até um dos lados cair. O chefe bate em um
// jogador por vez, na cadência do template; quem morre volta simVoltaMs depois,
// com a vida cheia, como quem corre da cidade.
func (sm *simulador) raide(n int, vida, dano int32, limiteMs int64, semRevide bool) resultadoRaide {
	kefra := sm.kefra(2000, vida)
	if dano > 0 {
		kefra.Damage, kefra.BaseDamage = dano, dano
	}
	lados := make([]*lado, n)
	volta := make([]int64, n)
	for i := range lados {
		lados[i] = &lado{e: sm.montar(xorimpas, i+1, buffsHT()), cd: map[int]int64{}}
	}
	var r resultadoRaide
	var agora, proxMob int64
	for agora < limiteMs && kefra.HP > 0 {
		vivos := 0
		for i, l := range lados {
			if l.e.HP <= 0 {
				if agora >= volta[i] {
					l.e.HP = l.e.MaxHP
				} else {
					continue
				}
			}
			vivos++
			g := sm.acaoHT(l, kefra, agora)
			r.golpes++
			if g.dano > 0 {
				r.danoTotal += int64(g.dano)
			}
			if kefra.HP <= 0 {
				break
			}
		}
		// O chefe bate em quem estiver vivo, um por golpe.
		for proxMob <= agora && kefra.HP > 0 && !semRevide {
			alvo := (*lado)(nil)
			for _, l := range lados {
				if l.e.HP > 0 {
					alvo = l
					break
				}
			}
			if alvo == nil {
				break
			}
			sm.d.tickCount = int(agora / 1000)
			if dmg := sm.d.danoDoGolpeDeMonstro(sm.w, kefra, alvo.e); dmg > 0 {
				r.golpesMob++
				r.danoMobTotal += int64(dmg)
				alvo.e.HP = max(0, alvo.e.HP-int32(dmg))
				if alvo.e.HP == 0 {
					r.mortes++
					for i, l := range lados {
						if l == alvo {
							volta[i] = agora + simVoltaMs
						}
					}
				}
			}
			proxMob += int64(cadenciaDoGolpe(kefra))
		}
		agora += simPasso
		if agora%(simTickAfetoS*1000) < simPasso {
			r.veneno += int64(sm.tickAfetos(kefra))
		}
		if simDebug && agora%600000 < simPasso {
			sm.t.Logf("  [%3d min] chefe %.1f mi de vida; tropa somou %.1f mi; veneno %.1f mi; mortes %d", agora/60000, float64(kefra.HP)/1e6, float64(r.danoTotal)/1e6, float64(r.veneno)/1e6, r.mortes)
		}
		if vivos == 0 {
			// Todos mortos: o relógio corre até o primeiro voltar.
			continue
		}
	}
	r.ms, r.vidaRestant = agora, kefra.HP
	sm.w.DespawnMob(kefra.ID, 1)
	return r
}

// TestSimulacaoKefra mede o dano por segundo da tropa e o tempo de luta para as
// vidas candidatas do chefe.
func TestSimulacaoKefra(t *testing.T) {
	const limite = 6 * 60 * 60 * 1000 // 6 h: teto do relógio da simulação
	simDebug = false

	// 1. O golpe da Huntress no chefe (defesa 0, sem resistência), em 10 min de
	//    luta contra um saco de pancada que não revida.
	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	saco := sm.raide(1, 1<<30, 0, 10*60*1000, true)
	t.Logf("uma HT sem revide: %d golpes em 10 min, média %.0f de dano, %.2f milhões por minuto",
		saco.golpes, float64(saco.danoTotal)/float64(saco.golpes), float64(saco.danoTotal)/(float64(saco.ms)/60000)/1e6)

	for _, n := range []int{13, 20, 30} {
		for _, vida := range []int32{100_000_000, 300_000_000, 500_000_000} {
			for _, dano := range []int32{4800, 20000} {
				sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
				r := sm.raide(n, vida, dano, limite, false)
				status := fmt.Sprintf("%d min", r.ms/60000)
				if r.vidaRestant > 0 {
					status = fmt.Sprintf("NÃO caiu em %d h (faltavam %.0f mi)", limite/3600000, float64(r.vidaRestant)/1e6)
				}
				rotulo := fmt.Sprintf("dano %5d", dano)
				if dano == 0 {
					rotulo = "dano  4800 (template)"
				}
				t.Logf("%2d jogadores, %4.0f mi de vida, %s: %s — %d mortes; chefe deu %d golpes de %.0f; tropa %.2f mi/min",
					n, float64(vida)/1e6, rotulo, status, r.mortes, r.golpesMob,
					float64(r.danoMobTotal)/max(float64(r.golpesMob), 1), float64(r.danoTotal)/(float64(r.ms)/60000)/1e6)
			}
		}
	}
}

// Sensibilidade ao tempo que o morto leva para voltar da cidade, que é a
// suposição mais frágil da simulação.
func TestSimulacaoKefraVolta(t *testing.T) {
	defer func() { simVoltaMs = 60_000 }()
	for _, volta := range []int64{30_000, 60_000, 120_000} {
		for _, n := range []int{13, 20, 30} {
			simVoltaMs = volta
			sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
			r := sm.raide(n, 300_000_000, 20000, 6*60*60*1000, false)
			status := fmt.Sprintf("%d min", r.ms/60000)
			if r.vidaRestant > 0 {
				status = fmt.Sprintf("NÃO caiu em 6 h (faltavam %.0f mi)", float64(r.vidaRestant)/1e6)
			}
			t.Logf("volta em %3d s, %2d jogadores, 300 mi, mono: %s", volta/1000, n, status)
		}
	}
}
