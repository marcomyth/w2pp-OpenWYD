package handler

import (
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/rng"
)

// brincos são os cinco que a sala do Golem de Fogo solta (0152/0175).
var brincos = map[int16]string{
	591: "Athena", 592: "Titã", 593: "Zeus", 594: "Hecate", 595: "Hercules",
}

// regrasDe lê as tuplas (mob, item, chance) de uma migração da Mesa.
func regrasDe(t *testing.T, nome string) []droprule.Rule {
	t.Helper()
	b, err := migrations.FS.ReadFile(nome)
	if err != nil {
		t.Fatal(err)
	}
	var out []droprule.Rule
	re := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		item, _ := strconv.Atoi(m[2])
		chance, _ := strconv.Atoi(m[3])
		out = append(out, droprule.Rule{Mob: m[1], Item: int16(item), Chance: int32(chance)})
	}
	return out
}

// matar sorteia a Mesa de um monstro como dropTableRolls faz: cada regra, na
// ordem do item, com rand()%10000 do MSVC. Devolve os brincos que caíram.
func matar(tab droprule.Table, mob string, r *rng.MSVC) []int16 {
	var caiu []int16
	for _, regra := range tab.Rolls(mob) {
		if droprule.Roll(regra.Chance, r.Intn) {
			if _, ok := brincos[regra.Item]; ok {
				caiu = append(caiu, regra.Item)
			}
		}
	}
	return caiu
}

// A cada quantos Golens de Fogo cai um brinco, e quais — pelo sorteio real da
// Mesa sobre as regras da 0175. Rode com -v para ver a tabela.
//
// Conta exata: rand() vai de 0 a 32767 e a chance 3 acerta com rand()%10000 em
// {0,1,2}, que aparecem 4 vezes cada no intervalo: 12/32768 = 0,0366% por brinco,
// 0,1831% somando os cinco — 1 brinco a cada ~546 Golens.
func TestBrincoACadaQuantosGolens(t *testing.T) {
	tab := droprule.NewTable(regrasDe(t, "0175_brincos_so_no_golem_de_fogo.up.sql"))
	const mortes = 2_000_000
	r := rng.NewSeeded(20260927)
	porBrinco := map[int16]int{}
	total := 0
	for range mortes {
		for _, it := range matar(tab, "Golem_Fogo_Lava", r) {
			porBrinco[it]++
			total++
		}
	}
	if total == 0 {
		t.Fatal("nenhum brinco em 2 milhões de Golens")
	}
	aCada := float64(mortes) / float64(total)
	t.Logf("Golem de Fogo: %d brincos em %d mortes — 1 a cada %.0f Golens (%.4f%% por morte)",
		total, mortes, aCada, 100*float64(total)/mortes)
	itens := make([]int, 0, len(brincos))
	for it := range brincos {
		itens = append(itens, int(it))
	}
	sort.Ints(itens)
	for _, it := range itens {
		n := porBrinco[int16(it)]
		t.Logf("  %d Brinco de %-8s %6d  (%.1f%% dos brincos, 1 a cada %.0f Golens)",
			it, brincos[int16(it)], n, 100*float64(n)/float64(total), float64(mortes)/float64(n))
		// Cada brinco tem a mesma regra: a parte dele fica perto de 20%.
		if parte := float64(n) / float64(total); parte < 0.18 || parte > 0.22 {
			t.Errorf("brinco %d saiu em %.1f%% dos drops, quer ~20%%", it, 100*parte)
		}
	}
	const exato = 32768.0 / (5 * 12) // 546,1 Golens por brinco
	if aCada < exato*0.95 || aCada > exato*1.05 {
		t.Errorf("1 brinco a cada %.0f Golens, a conta exata dá %.0f", aCada, exato)
	}
	for _, mob := range []string{"Gargula_Lava", "Gargula_Inf", "Gargula_Servo"} {
		for range 200_000 {
			if caiu := matar(tab, mob, r); len(caiu) > 0 {
				t.Fatalf("%s soltou o brinco %d; depois da 0175 só o Golem solta", mob, caiu[0])
			}
		}
	}
}

// A noite do Marco (27/09/2026): 5 horas na sala com 4 personagens, mais de 40
// brincos. Com a 0152 a sala inteira pagava ~0,61% de brinco por morte, então 40
// brincos são ~6.550 mortes; a sala tem 24 Golens e 12 Gárgulas, renascendo 15 s
// depois de morrer. A mesma caçada é repetida mil vezes com a regra antiga e com
// a nova, e o teste mostra quantos brincos cada uma rende.
func TestNoiteDoMarcoAntesEDepois(t *testing.T) {
	const (
		mortesNaNoite = 6550
		noites        = 1000
	)
	sessao := func(tab droprule.Table, r *rng.MSVC) int {
		n := 0
		for i := range mortesNaNoite {
			mob := "Golem_Fogo_Lava" // 24 de cada 36 são Golens, 12 são Gárgulas
			if i%3 == 2 {
				mob = "Gargula_Lava"
			}
			n += len(matar(tab, mob, r))
		}
		return n
	}
	resumo := func(nome string, tab droprule.Table) (mediana int) {
		r := rng.NewSeeded(27092026)
		v := make([]int, noites)
		for i := range v {
			v[i] = sessao(tab, r)
		}
		sort.Ints(v)
		t.Logf("%-5s brincos numa noite de %d mortes: mediana %d, 9 de cada 10 noites entre %d e %d",
			nome, mortesNaNoite, v[noites/2], v[noites/20], v[noites-noites/20])
		return v[noites/2]
	}
	antes := resumo("0152", droprule.NewTable(regrasDe(t, "0152_sala_golem_de_fogo.up.sql")))
	depois := resumo("0175", droprule.NewTable(regrasDe(t, "0175_brincos_so_no_golem_de_fogo.up.sql")))
	if antes < 32 || antes > 48 {
		t.Errorf("a regra antiga deu mediana %d na noite; a calibragem era ~40", antes)
	}
	if depois*4 > antes {
		t.Errorf("a 0175 deu mediana %d contra %d antes: cortou menos de 4×", depois, antes)
	}
}
