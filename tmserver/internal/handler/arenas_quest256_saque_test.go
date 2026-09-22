package handler

import (
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// mesaDasArenas monta a Mesa de Drops das arenas a partir das migrações, na
// ordem em que o banco as aplica: a 0065 e a 0075 escrevem, a 0098 corta os
// Restos pela metade. Ler o SQL em vez de copiar os números é o que impede este
// teste de aprovar uma arena que a migração já mudou.
func mesaDasArenas(t *testing.T) []droprule.Rule {
	t.Helper()
	re := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`)
	chances := map[string]map[int16]int32{}
	for _, nome := range []string{
		"0065_arenas_kaizen_hidra.up.sql",
		"0075_arena_elfos_e_chave_orc.up.sql",
		"0098_quest_mortal_restos_e_ouro.up.sql",
	} {
		b, err := migrations.FS.ReadFile(nome)
		if err != nil {
			t.Fatalf("migração %s: %v", nome, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			item, err := strconv.Atoi(m[2])
			if err != nil || item < droprule.MinItem || item > droprule.MaxItem {
				continue // a 0098 também cita tiers de quest_reward
			}
			ch, _ := strconv.Atoi(m[3])
			if chances[m[1]] == nil {
				chances[m[1]] = map[int16]int32{}
			}
			chances[m[1]][int16(item)] = int32(ch)
		}
	}
	var out []droprule.Rule
	for mob, itens := range chances {
		for item, ch := range itens {
			out = append(out, droprule.Rule{Mob: mob, Item: item, Chance: ch})
		}
	}
	if len(out) == 0 {
		t.Fatal("a Mesa das arenas saiu vazia das migrações")
	}
	return out
}

type papelNaArena struct {
	tmpl string
	qtd  float64 // quantos deste template numa arena cheia
}

// As populações são as de populacao_arenas.go (fixas, 17/09/2026) repartidas
// pela proporção líder:seguidor dos blocos do NPCGener.
var arenasDaQuest256 = []struct {
	nome   string
	trofeu int16
	papeis []papelNaArena
}{
	{"Cemitério (Coveiro)", 4117, []papelNaArena{{"Aparicao", 29}, {"Esqueleto", 61}}},
	{"Jardim dos Deuses", 4118, []papelNaArena{{"Grande_Carb", 27.5}, {"Servo_Carbuncle", 62.5}}},
	{"Coração do Kaizen", 4119, []papelNaArena{{"Cav._Kaizen", 12}, {"Cav._Servo", 30}, {"Hidra", 3}}},
	{"Hidras", 4120, []papelNaArena{{"Hidra_Dourada", 16.5}, {"Hidra_Imortal", 34.5}}},
	{"Elfos", 4121, []papelNaArena{{"Mestre_Elfo", 16.5}, {"Servo_Elfo", 28.5}}},
}

// TestTrofeuCaiMaisQueRestoNasArenas é a regra que o Marco pediu em 22/09/2026:
// numa arena da Quest 256, o troféu é a recompensa e o Resto é o troco, então o
// troféu tem de cair MAIS. Antes da 0098 o Kaizen dava 1,36 Resto por troféu, as
// Hidras 1,36 e os Elfos 1,45 — o inverso.
//
// Mede matando pelo caminho real (mobKilled: slots do template, Mesa e pacote do
// líder), porque a conta de papel erra: o sorteio da Mesa é rand()%10000 sobre
// um rand() que só vai até 32767, e isso infla toda chance abaixo de 27,68% em
// 22,1% (internal/droprule/vies_test.go).
func TestTrofeuCaiMaisQueRestoNasArenas(t *testing.T) {
	raiz := releaseDir(t)
	mesa := droprule.NewTable(mesaDasArenas(t))
	const mortes = 2000

	for _, ar := range arenasDaQuest256 {
		saque := map[int16]float64{}
		for _, p := range ar.papeis {
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
				saque[idx] += n / mortes * p.qtd
			}
		}
		trofeus := saque[ar.trofeu]
		restos := saque[itemRestoOri] + saque[itemRestoLac]
		if trofeus <= 0 {
			t.Errorf("%s: o troféu %d não caiu em %d mortes por template", ar.nome, ar.trofeu, mortes)
			continue
		}
		t.Logf("%-20s troféu %5.1f | Restos %5.1f (%.2f por troféu)", ar.nome, trofeus, restos, restos/trofeus)
		if restos >= trofeus {
			t.Errorf("%s: caem %.1f Restos para %.1f troféus (%.2f por troféu) — o troféu tem de cair mais",
				ar.nome, restos, trofeus, restos/trofeus)
		}
	}
}

// TestSaqueDasArenasNaoTrazItemNovo lista o que cai, para que uma mudança de
// template ou de Mesa apareça na revisão em vez de só no jogo.
func TestSaqueDasArenasNaoTrazItemNovo(t *testing.T) {
	if testing.Short() {
		t.Skip("medição longa")
	}
	raiz := releaseDir(t)
	mesa := droprule.NewTable(mesaDasArenas(t))
	const mortes = 500
	for _, ar := range arenasDaQuest256 {
		saque := map[int16]float64{}
		for _, p := range ar.papeis {
			tmpl, _, err := npctemplate.Load(raiz, p.tmpl)
			if err != nil {
				t.Fatalf("template %s: %v", p.tmpl, err)
			}
			d, w, killer := mobKilledWorld(t)
			d.dropRules = mesa
			killer.Level = 320
			for k := 0; k < mortes; k++ {
				for i := range killer.Carry {
					killer.Carry[i] = world.Item{}
				}
				d.mobKilled(w, killer, spawnNamed(t, w, tmpl, p.tmpl))
				for _, it := range killer.Carry {
					if it.Index != 0 {
						saque[it.Index] += float64(itemAmount(it)) / mortes * p.qtd
					}
				}
			}
		}
		idxs := make([]int16, 0, len(saque))
		for i := range saque {
			idxs = append(idxs, i)
		}
		sort.Slice(idxs, func(a, b int) bool { return saque[idxs[a]] > saque[idxs[b]] })
		t.Logf("=== %s", ar.nome)
		for _, i := range idxs {
			if saque[i] >= 0.05 {
				t.Logf("    item %5d  %6.2f por arena", i, saque[i])
			}
		}
	}
}
