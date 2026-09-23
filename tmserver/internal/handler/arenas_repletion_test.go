package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	repletionClasseC int16 = 4018
	repletionClasseD int16 = 4019
)

var amagosDeEntrada = []int16{2392, 2393, 2394, 2395}

// arenaDeCima é uma das três arenas que a 0102 mexe, com a composição de uma
// arena cheia (populacao_arenas.go + a proporção do NPCGener).
var arenasDeCima = []struct {
	nome   string
	papeis []papelDaArena
}{
	{"Kaizen", []papelDaArena{{"Cav._Kaizen", 12}, {"Cav._Servo", 30}}},
	{"Hidras", []papelDaArena{{"Hidra_Dourada", 16.5}, {"Hidra_Imortal", 34.5}}},
	{"Elfos", []papelDaArena{{"Mestre_Elfo", 16.5}, {"Servo_Elfo", 28.5}}},
}

type papelDaArena struct {
	tmpl string
	qtd  float64
}

// saqueDaArena mede, por arena cheia limpa uma vez, quanto de cada item cai.
func saqueDaArena(t *testing.T, raiz string, papeis []papelDaArena, mesa droprule.Table, mortes int) map[int16]float64 {
	t.Helper()
	out := map[int16]float64{}
	for _, p := range papeis {
		tmpl, _, err := npctemplate.Load(raiz, p.tmpl)
		if err != nil {
			t.Fatalf("template %s: %v", p.tmpl, err)
		}
		d, w, killer := mobKilledWorld(t)
		d.dropRules = mesa
		killer.Level = 320
		conta := map[int16]float64{}
		for k := 0; k < mortes; k++ {
			for i := range killer.Carry {
				killer.Carry[i] = world.Item{}
			}
			d.mobKilled(w, killer, spawnNamed(t, w, tmpl, p.tmpl))
			for _, it := range killer.Carry {
				if it.Index != 0 {
					conta[it.Index] += float64(itemAmount(it))
				}
			}
		}
		for idx, n := range conta {
			out[idx] += n / float64(mortes) * p.qtd
		}
	}
	return out
}

// TestArenasPagamPoucoRepletion é o pedido de 22/09/2026: "só Classe C e D
// poucos". O "poucos" é o que este teste prende — entre meia e quatro unidades
// por arena cheia limpa. Sem um teto, a próxima mão que ajustar a Mesa não tem
// como saber que o pedido era esse.
func TestArenasPagamPoucoRepletion(t *testing.T) {
	raiz := releaseDir(t)
	mesa := droprule.NewTable(regrasDaMigracao(t, "0102_arenas_repletion_e_amagos.up.sql", ""))
	const mortes = 20000
	for _, a := range arenasDeCima {
		saque := saqueDaArena(t, raiz, a.papeis, mesa, mortes)
		rep := saque[repletionClasseC] + saque[repletionClasseD]
		t.Logf("%-8s Classe C %.2f + Classe D %.2f = %.2f Repletion por arena", a.nome, saque[repletionClasseC], saque[repletionClasseD], rep)
		if rep < 0.5 {
			t.Errorf("%s: %.2f Repletion por arena — o pedido era poucos, não nenhum", a.nome, rep)
		}
		if rep > 4 {
			t.Errorf("%s: %.2f Repletion por arena — o pedido era POUCOS", a.nome, rep)
		}
		if saque[repletionClasseC] <= saque[repletionClasseD] {
			t.Errorf("%s: a Classe D (%.2f) não pode cair mais que a C (%.2f)", a.nome, saque[repletionClasseD], saque[repletionClasseC])
		}
	}
}

// TestAmagosDobraramNasArenas prende a outra metade: os quatro Âmagos de entrada
// passaram a cair o dobro do que a 0065/0075 escreveu. É o drop que alimenta a
// curva de montaria, então o número fica medido e não estimado.
func TestAmagosDobraramNasArenas(t *testing.T) {
	raiz := releaseDir(t)
	antes := droprule.NewTable(append(
		regrasDaMigracao(t, "0065_arenas_kaizen_hidra.up.sql", ""),
		regrasDaMigracao(t, "0075_arena_elfos_e_chave_orc.up.sql", "")...))
	depois := droprule.NewTable(regrasDaMigracao(t, "0102_arenas_repletion_e_amagos.up.sql", ""))
	const mortes = 20000

	soma := func(s map[int16]float64) float64 {
		var v float64
		for _, a := range amagosDeEntrada {
			v += s[a]
		}
		return v
	}
	for _, a := range arenasDeCima {
		vAntes := soma(saqueDaArena(t, raiz, a.papeis, antes, mortes))
		vDepois := soma(saqueDaArena(t, raiz, a.papeis, depois, mortes))
		t.Logf("%-8s Âmagos por arena: %.2f -> %.2f", a.nome, vAntes, vDepois)
		if razao := vDepois / vAntes; razao < 1.8 || razao > 2.2 {
			t.Errorf("%s: os Âmagos foram a %.2fx, e o combinado era o dobro", a.nome, razao)
		}
	}
}
