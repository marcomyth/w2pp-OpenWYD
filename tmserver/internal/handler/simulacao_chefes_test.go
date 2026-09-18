//go:build simulacao

package handler

// Calibração dos chefes (18/09/2026): quanto tempo uma tropa de Huntress iguais
// ao Xorimpas leva para derrubar cada chefe, agora que o divisor de dano do slot
// 13 está no servidor (divisor_de_dano.go).
//
//	go test -tags simulacao -run TestSimulacaoChefes -v ./tmserver/internal/handler/
//
// O golpe do jogador e o do monstro saem das mesmas funções do servidor que a
// simulação da HT usa (acaoHT, danoDoGolpeDeMonstro, cadenciaDoGolpe), e o dano
// passa pelo divisor no mesmo ponto que em jogo.

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Quem morre volta depois disto: tempo de correr da cidade até o chefe. É o
// número mais incerto da simulação, e TestSimulacaoChefeVolta mede o quanto ele
// muda o resultado.
var simVoltaMs int64 = 60_000

// simDebug liga o relatório de 10 em 10 minutos dentro da luta.
var simDebug bool

// alvoDeRaide é o chefe do teste: o template, e o que se quer mudar nele.
type alvoDeRaide struct {
	arquivo string
	vida    int32 // 0 = a vida do template
	dano    int32 // 0 = o dano do template
	valor   int   // 0 = o refino do template no slot 13; senão o valor do divisor
	item    int16 // 0 = o item do template no slot 13; senão troca o divisor de item
	defesa  int32 // 0 = a defesa do template
}

// chefe sobe o monstro do template com os ajustes pedidos.
func (sm *simulador) chefe(a alvoDeRaide) *world.Entity {
	tmpl, _, err := npctemplate.Load(filepath.Join("..", "..", "..", "Release"), a.arquivo)
	if err != nil {
		sm.t.Skipf("%s: %v", a.arquivo, err)
	}
	m := sm.w.Entity(sm.w.SpawnMob(tmpl, 30, 30))
	if a.vida > 0 {
		// BaseMaxHP junto: refreshScore refaz MaxHP a partir dele quando um afeto
		// expira no monstro, e sem isto o chefe encolhia de volta para o número do
		// template no meio da luta (é o mesmo cuidado da Torre, towerwar.go).
		m.MaxHP, m.HP, m.BaseMaxHP = a.vida, a.vida, a.vida
	}
	if a.dano > 0 {
		m.Damage, m.BaseDamage = a.dano, a.dano
	}
	if a.defesa > 0 {
		m.AC, m.BaseAC = a.defesa, a.defesa
	}
	if a.item > 0 {
		m.Equip[world.DividerEquipSlot].Index = a.item
	}
	if a.valor > 0 {
		m.Equip[world.DividerEquipSlot].Effects[0].Value = uint8(a.valor)
	}
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
	divisor      int32
	efetiva      int64 // vida × divisor: a vida que a tropa precisa vencer
}

// raide: n Huntress batendo no chefe até um dos lados cair. O chefe bate em um
// jogador por vez, na cadência do template; quem morre volta simVoltaMs depois,
// com a vida cheia, como quem corre da cidade.
func (sm *simulador) raide(a alvoDeRaide, n int, limiteMs int64, semRevide bool) resultadoRaide {
	chefe := sm.chefe(a)
	lados := make([]*lado, n)
	volta := make([]int64, n)
	for i := range lados {
		lados[i] = &lado{e: sm.montar(xorimpas, i+1, buffsHT()), cd: map[int]int64{}}
	}
	r := resultadoRaide{divisor: divisorDeGolpe(chefe)}
	r.efetiva = int64(chefe.HP) * int64(r.divisor)
	var agora, proxMob int64
	for agora < limiteMs && chefe.HP > 0 {
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
			g := sm.acaoHT(l, chefe, agora)
			r.golpes++
			if g.dano > 0 {
				r.danoTotal += int64(g.dano)
			}
			if chefe.HP <= 0 {
				break
			}
		}
		// O chefe bate em quem estiver vivo, um por golpe.
		for proxMob <= agora && chefe.HP > 0 && !semRevide {
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
			if dmg := sm.d.danoDoGolpeDeMonstro(sm.w, chefe, alvo.e); dmg > 0 {
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
			proxMob += int64(cadenciaDoGolpe(chefe))
		}
		agora += simPasso
		if agora%(simTickAfetoS*1000) < simPasso {
			r.veneno += int64(sm.tickAfetos(chefe))
		}
		if simDebug && agora%600000 < simPasso {
			sm.t.Logf("  [%3d min] faltam %.1f mi efetivos; tropa somou %.1f mi; mortes %d",
				agora/60000, float64(chefe.HP)*float64(r.divisor)/1e6, float64(r.danoTotal)/1e6, r.mortes)
		}
		if vivos == 0 {
			// Todos mortos: o relógio corre até o primeiro voltar.
			continue
		}
	}
	r.ms, r.vidaRestant = agora, chefe.HP
	sm.w.DespawnMob(chefe.ID, 1)
	return r
}

// TestSimulacaoChefes: o tempo de luta de cada chefe, por valor do divisor e por
// tamanho da tropa. Os alvos do desenho (18/09/2026) são: Kefra 2 h com 20
// jogadores, Cav. Lugefer 10-20 min, Sombra Negra e o Lich 40-50 min.
func TestSimulacaoChefes(t *testing.T) {
	const limite = 4 * 60 * 60 * 1000
	simDebug = false

	sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
	saco := sm.raide(alvoDeRaide{arquivo: "Rainha_Rubra", vida: 1 << 30}, 1, 10*60*1000, true)
	t.Logf("uma HT sem revide: %d golpes em 10 min, média %.0f de dano, %.2f milhões por minuto",
		saco.golpes, float64(saco.danoTotal)/float64(saco.golpes), float64(saco.danoTotal)/(float64(saco.ms)/60000)/1e6)

	casos := []struct {
		rotulo  string
		alvo    alvoDeRaide
		valores []int
		grupos  []int
	}{
		{"Kefra 1936 ÷20, vida 52 mi", alvoDeRaide{arquivo: "Kefra", item: 1936, valor: 2, vida: 52_000_000}, []int{2}, []int{20}},
		{"Kefra 1936 ÷20, vida 56 mi", alvoDeRaide{arquivo: "Kefra", item: 1936, valor: 2, vida: 56_000_000}, []int{2}, []int{16, 20, 25, 30}},
		{"Cav. Lugefer valor 25", alvoDeRaide{arquivo: "Cav._Lugefer"}, []int{25}, []int{1, 2, 4, 6, 8, 12}},
		{"Sombra Negra valor 30", alvoDeRaide{arquivo: "Sombra_Negra"}, []int{30}, []int{1, 2, 4, 6, 8, 12}},
		{"Lich Crunt original, valor 2", alvoDeRaide{arquivo: "Lich_Crunt", vida: 800_000, dano: 5000, defesa: 6000}, []int{2}, []int{1, 2, 4, 6, 8, 12}},
		// A tropa comum das duas áreas, um jogador sozinho: é o custo de farmar LE.
		{"Templario Amald hoje (vida ×5)", alvoDeRaide{arquivo: "Templario_Amald"}, []int{0}, []int{1}},
		{"Templario Amald sem o ×5", alvoDeRaide{arquivo: "Templario_Amald", vida: 10_000}, []int{0}, []int{1}},
		{"Funer Seamer hoje", alvoDeRaide{arquivo: "Funer_Seamer"}, []int{0}, []int{1}},
		{"Funer Seamer sem o ×5", alvoDeRaide{arquivo: "Funer_Seamer", vida: 32_000}, []int{0}, []int{1}},
		{"Verid hoje", alvoDeRaide{arquivo: "Verid"}, []int{0}, []int{1, 6}},
		{"Verid sem o ×5", alvoDeRaide{arquivo: "Verid", vida: 32_000}, []int{0}, []int{1, 6}},
	}
	for _, c := range casos {
		t.Log("— " + c.rotulo + " —")
		for _, valor := range c.valores {
			for _, n := range c.grupos {
				sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
				a := c.alvo
				a.valor = valor
				r := sm.raide(a, n, limite, false)
				status := fmt.Sprintf("%d min", r.ms/60000)
				if r.vidaRestant > 0 {
					status = fmt.Sprintf("NÃO caiu em %d h", limite/3600000)
				}
				t.Logf("  valor %3d (÷%-6d %8.1f mi efetivos), %2d jogadores: %-22s %3d mortes, tropa %6.2f mi/min",
					valor, r.divisor, float64(r.efetiva)/1e6, n, status, r.mortes,
					float64(r.danoTotal)/(float64(r.ms)/60000)/1e6)
			}
		}
	}
}

// Sensibilidade ao tempo que o morto leva para voltar da cidade, que é a
// suposição mais frágil da simulação.
func TestSimulacaoChefeVolta(t *testing.T) {
	defer func() { simVoltaMs = 60_000 }()
	for _, volta := range []int64{30_000, 60_000, 120_000} {
		for _, n := range []int{13, 20, 30} {
			simVoltaMs = volta
			sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
			r := sm.raide(alvoDeRaide{arquivo: "Kefra", vida: 84_000, valor: 6}, n, 4*60*60*1000, false)
			status := fmt.Sprintf("%d min", r.ms/60000)
			if r.vidaRestant > 0 {
				status = fmt.Sprintf("NÃO caiu em 4 h (faltavam %.0f mi)", float64(r.vidaRestant)*float64(r.divisor)/1e6)
			}
			t.Logf("volta em %3d s, %2d jogadores, %.0f mi efetivos: %s", volta/1000, n, float64(r.efetiva)/1e6, status)
		}
	}
}

// TestSimulacaoTropaDosLE procura, para cada monstro de tropa da Vila Amald e do
// Kefra, a vida de template que faz UM jogador levar o tempo pedido (5-10 min por
// bicho, decisão de 18/09/2026). A vida efetiva é vida × divisor do slot 13, então
// o número procurado é bem menor do que o de antes do divisor.
func TestSimulacaoTropaDosLE(t *testing.T) {
	const limite = 30 * 60 * 1000
	alvoMin, alvoMax := int64(5*60_000), int64(10*60_000)
	tropa := []string{
		"Templario_Amald", "Mago_Amald", "Shama_Amald", "Ranger_Amald",
		"Batorero", "Batorero__", "FunerSickler", "Funer_Scyther", "Funer_Seamer",
		"Horizon_Cropper", "Simio", "Simio_Bleg", "Xeno_Cropper", "FunerSeamer",
		"Funer_Momenter", "Funer_Sickler", "GrubSwarm", "HorizonCropper", "Simio_Inf",
		"WriggleSwarm", "Aranha_Dourada", "Aranha_Rubra", "LiggleSwarm", "Serva_Rubra",
	}
	for _, arquivo := range tropa {
		// Tempo com os números de hoje, e depois a busca pela vida do alvo.
		sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
		hoje := sm.raide(alvoDeRaide{arquivo: arquivo}, 1, limite, false)
		vidaHoje := hoje.efetiva / int64(max(hoje.divisor, 1))

		baixo, alto := int32(1_000), int32(200_000)
		melhor, melhorMs := int32(0), int64(0)
		for i := 0; i < 18 && baixo <= alto; i++ {
			meio := baixo + (alto-baixo)/2
			sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
			r := sm.raide(alvoDeRaide{arquivo: arquivo, vida: meio}, 1, limite, false)
			if r.vidaRestant > 0 { // não caiu: vida demais
				alto = meio - 1
				continue
			}
			melhor, melhorMs = meio, r.ms
			switch {
			case r.ms < alvoMin:
				baixo = meio + 1
			case r.ms > alvoMax:
				alto = meio - 1
			default:
				baixo, alto = meio, meio-1 // dentro da faixa: para
			}
		}
		status := "hoje NÃO cai em 30 min"
		if hoje.vidaRestant == 0 {
			status = fmt.Sprintf("hoje %d min", hoje.ms/60000)
		}
		t.Logf("%-16s ÷%-4d vida %6d (%5.2f mi efetivos) %-22s => vida %6d (%.2f mi efetivos) para %d min",
			arquivo, hoje.divisor, vidaHoje, float64(hoje.efetiva)/1e6, status,
			melhor, float64(int64(melhor)*int64(hoje.divisor))/1e6, melhorMs/60000)
	}
}

// tropaDosLE é a vida proposta para cada monstro de tropa que hoje passa de 10
// minutos para um jogador sozinho. Quem já cai em menos disso não é mexido.
var tropaDosLE = []struct {
	arquivo string
	vida    int32
}{
	{"Templario_Amald", 20_000},
	{"Mago_Amald", 20_000},
	{"Shama_Amald", 20_000},
	{"Ranger_Amald", 20_000},
	{"Batorero", 16_000},
	{"Batorero__", 30_000},
	{"FunerSickler", 75_000},
	{"Funer_Scyther", 100_000},
	{"Funer_Seamer", 100_000},
	{"Simio", 50_000},
	{"Simio_Bleg", 20_000},
	{"FunerSeamer", 75_000},
	{"Funer_Momenter", 40_000},
	{"Funer_Sickler", 75_000},
	{"Simio_Inf", 40_000},
}

func TestSimulacaoTropaConfirma(t *testing.T) {
	const limite = 30 * 60 * 1000
	for _, c := range tropaDosLE {
		sm := novoSimulador(t, filepath.Join("..", "..", "..", "Release"))
		r := sm.raide(alvoDeRaide{arquivo: c.arquivo, vida: c.vida}, 1, limite, false)
		status := fmt.Sprintf("%d min", r.ms/60000)
		if r.vidaRestant > 0 {
			status = "NÃO cai em 30 min"
		}
		t.Logf("%-16s vida %6d ÷%-3d = %5.2f mi efetivos: %s (%d mortes)",
			c.arquivo, c.vida, r.divisor, float64(r.efetiva)/1e6, status, r.mortes)
	}
}
