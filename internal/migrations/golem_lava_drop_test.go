package migrations_test

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// chancesDoGolemLava lê as tuplas ('Golem_Lava', item, chance) de uma migração.
func chancesDoGolemLava(t *testing.T, arquivo string) map[int]int {
	t.Helper()
	b, err := migrations.FS.ReadFile(arquivo)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\('Golem_Lava',\s*(\d+),\s*(\d+)\)`)
	out := map[int]int{}
	for _, m := range re.FindAllStringSubmatch(semComentariosSQL(string(b)), -1) {
		item, _ := strconv.Atoi(m[1])
		chance, _ := strconv.Atoi(m[2])
		out[item] = chance
	}
	return out
}

// A 0178 corta por pelo menos 3 toda chance acima de zero que a 0148 deu ao
// Golem de Pedra da sala de lava, sem zerar nenhuma e sem esquecer item.
func TestGolemLavaDropTresVezesMenos(t *testing.T) {
	antes := chancesDoGolemLava(t, "0148_lava_so_na_sala.up.sql")
	depois := chancesDoGolemLava(t, "0178_golem_lava_drop_tres_vezes_menos.up.sql")
	if len(antes) == 0 {
		t.Fatal("a 0148 não tem regra do Golem_Lava; o teste perdeu a referência")
	}
	cobertos := 0
	for item, era := range antes {
		if era == 0 {
			if _, ok := depois[item]; ok {
				t.Errorf("item %d estava a 0 e a 0178 mexeu nele", item)
			}
			continue
		}
		cobertos++
		agora, ok := depois[item]
		switch {
		case !ok:
			t.Errorf("item %d (era %d) ficou fora da 0178", item, era)
		case agora == 0:
			t.Errorf("item %d zerado; o pedido era cortar, não tirar", item)
		case agora*3 > era:
			t.Errorf("item %d: %d não é um terço de %d", item, agora, era)
		}
	}
	if len(depois) != cobertos {
		t.Errorf("a 0178 tem %d regras, a 0148 tinha %d chances acima de zero", len(depois), cobertos)
	}
	if baixo := chancesDoGolemLava(t, "0178_golem_lava_drop_tres_vezes_menos.down.sql"); len(baixo) != cobertos {
		t.Errorf("o down volta %d regras, esperava %d", len(baixo), cobertos)
	} else {
		for item, v := range baixo {
			if v != antes[item] {
				t.Errorf("down: item %d volta a %d, a 0148 dava %d", item, v, antes[item])
			}
		}
	}
}
