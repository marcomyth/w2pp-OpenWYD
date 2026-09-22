package handler

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
	"github.com/jeanluca/w2pp-openwyd/internal/npctemplate"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	itemMoeda1Mi int16 = 4026
	itemMoeda5Mi int16 = 4027
)

// mesaDaAparicao lê as duas regras da 0099 do próprio SQL, para o teste não
// aprovar uma frequência que a migração já mudou.
func mesaDaAparicao(t *testing.T) []droprule.Rule {
	t.Helper()
	b, err := migrations.FS.ReadFile("0099_aparicao_moeda_de_1kk.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(b)
	if i := strings.Index(sql, "INSERT INTO drop_rule"); i >= 0 {
		sql = sql[i:] // o comentário acima cita as mesmas linhas
	}
	re := regexp.MustCompile(`\('(Aparicao)',\s*(\d+),\s*(\d+)\)`)
	var out []droprule.Rule
	for _, m := range re.FindAllStringSubmatch(sql, -1) {
		item, _ := strconv.Atoi(m[2])
		ch, _ := strconv.Atoi(m[3])
		out = append(out, droprule.Rule{Mob: m[1], Item: int16(item), Chance: int32(ch)})
	}
	if len(out) != 2 {
		t.Fatalf("a 0099 escreve %d regras para a Aparição, e são 2", len(out))
	}
	return out
}

// TestAparicaoTrocaAMoedaMantendoAFrequencia é o pedido de 22/09/2026: a moeda de
// 5 milhões sai, a de 1 milhão entra, e a frequência fica a MESMA.
//
// O "mesma" é o que precisa de medição: o slot 57 do template entrega 2,857%
// (droprate 35), mas a Mesa sorteia rand()%10000 sobre um rand() que para em
// 32767 e infla toda chance baixa em 22%. Escrever 286 daria 3,49%. Este teste
// mede as duas pelo caminho real de morte e exige que fiquem a menos de 0,2
// ponto percentual uma da outra — sem ele, "mesma frequência" seria uma
// afirmação que ninguém conferiu.
func TestAparicaoTrocaAMoedaMantendoAFrequencia(t *testing.T) {
	raiz := releaseDir(t)
	tmpl, _, err := npctemplate.Load(raiz, "Aparicao")
	if err != nil {
		t.Fatalf("template da Aparição: %v", err)
	}
	const mortes = 20000

	conta := func(regras []droprule.Rule) (cinco, um float64) {
		d, w, killer := mobKilledWorld(t)
		d.dropRules = droprule.NewTable(regras)
		killer.Level = 115
		var c5, c1 int
		for k := 0; k < mortes; k++ {
			for i := range killer.Carry {
				killer.Carry[i] = world.Item{}
			}
			d.mobKilled(w, killer, spawnNamed(t, w, tmpl, "Aparicao"))
			for _, it := range killer.Carry {
				switch it.Index {
				case itemMoeda5Mi:
					c5++
				case itemMoeda1Mi:
					c1++
				}
			}
		}
		return float64(c5) / mortes * 100, float64(c1) / mortes * 100
	}

	antes5, antes1 := conta(nil) // sem Mesa: só o template
	depois5, depois1 := conta(mesaDaAparicao(t))
	t.Logf("antes:  5Mi %.3f%%  1Mi %.3f%%", antes5, antes1)
	t.Logf("depois: 5Mi %.3f%%  1Mi %.3f%%", depois5, depois1)

	if antes5 < 2 {
		t.Fatalf("a Aparição não estava soltando a moeda de 5Mi (%.3f%%): o teste perdeu o alvo", antes5)
	}
	if depois5 != 0 {
		t.Errorf("a moeda de 5 milhões ainda cai: %.3f%%", depois5)
	}
	if diff := depois1 - antes5; diff > 0.2 || diff < -0.2 {
		t.Errorf("a frequência mudou: era %.3f%% de 5Mi e virou %.3f%% de 1Mi (%.3f de diferença)",
			antes5, depois1, diff)
	}
}
