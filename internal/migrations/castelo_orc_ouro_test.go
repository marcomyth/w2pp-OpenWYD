package migrations_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// moedaDoCasteloOrc é uma linha (monstro da quest, Moeda de Prata) da Mesa.
type moedaDoCasteloOrc struct {
	mob  string
	item int
}

// moedasDoCasteloOrc lê as tuplas ('COrc_*', 4026 ou 4027, chance) de uma
// migração: as Moedas de Prata de 1Mi e de 5Mi, o único ouro da quest.
func moedasDoCasteloOrc(t *testing.T, arquivo string) map[moedaDoCasteloOrc]int {
	t.Helper()
	b, err := migrations.FS.ReadFile(arquivo)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\('(COrc_[A-Za-z]+)',\s*(402[67]),\s*(\d+)\)`)
	out := map[moedaDoCasteloOrc]int{}
	for _, m := range re.FindAllStringSubmatch(semComentariosSQL(string(b)), -1) {
		item, _ := strconv.Atoi(m[2])
		chance, _ := strconv.Atoi(m[3])
		out[moedaDoCasteloOrc{m[1], item}] = chance
	}
	return out
}

// A 0190 corta 40% de toda Moeda de Prata que a 0053 e a 0063 deram ao Castelo
// Orc, sem esquecer monstro: uma linha que ficasse de fora continuaria pagando o
// valor antigo, e a tropa renasce a corrida inteira.
func TestCasteloOrcOuroMenos40(t *testing.T) {
	antes := moedasDoCasteloOrc(t, "0053_castelo_orc_drops.up.sql")
	for k, v := range moedasDoCasteloOrc(t, "0063_castelo_orc_guardioes.up.sql") {
		antes[k] = v
	}
	if len(antes) == 0 {
		t.Fatal("a 0053 e a 0063 não têm Moeda de Prata no Castelo Orc; o teste perdeu a referência")
	}
	// O Mago Orc não tem tupla própria: a 0063 copia as linhas do Meio Orc por
	// INSERT…SELECT, então a moeda dele é a do Meio Orc.
	b, err := migrations.FS.ReadFile("0063_castelo_orc_guardioes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(semComentariosSQL(string(b)), "SELECT 'COrc_Mago', item, chance FROM drop_rule WHERE mob = 'COrc_MeioOrc'") {
		t.Fatal("a 0063 não copia mais o Meio Orc para o Mago; revise de onde vem a moeda dele")
	}
	meioOrc, ok := antes[moedaDoCasteloOrc{"COrc_MeioOrc", 4026}]
	if !ok {
		t.Fatal("o Meio Orc não tem Moeda de Prata (1Mi) na 0053")
	}
	antes[moedaDoCasteloOrc{"COrc_Mago", 4026}] = meioOrc

	depois := moedasDoCasteloOrc(t, "0190_castelo_orc_ouro_menos_40.up.sql")
	for k, era := range antes {
		agora, ok := depois[k]
		switch {
		case !ok:
			t.Errorf("%s item %d (era %d) ficou fora da 0190", k.mob, k.item, era)
		case agora*10 != era*6:
			t.Errorf("%s item %d: %d não é 60%% de %d", k.mob, k.item, agora, era)
		}
	}
	if len(depois) != len(antes) {
		t.Errorf("a 0190 tem %d linhas de moeda, o castelo tinha %d", len(depois), len(antes))
	}

	baixo := moedasDoCasteloOrc(t, "0190_castelo_orc_ouro_menos_40.down.sql")
	if len(baixo) != len(antes) {
		t.Errorf("o down volta %d linhas, esperava %d", len(baixo), len(antes))
	}
	for k, v := range baixo {
		if v != antes[k] {
			t.Errorf("down: %s item %d volta a %d, antes era %d", k.mob, k.item, v, antes[k])
		}
	}
}
