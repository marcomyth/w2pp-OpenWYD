package migrations_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

const migBrincos = "0175_brincos_so_no_golem_de_fogo.up.sql"

// regrasDaMigracao lê as tuplas (mob, item, chance) de uma migração da Mesa.
func regrasDaMigracao(t *testing.T, nome string) map[string]map[int]int {
	t.Helper()
	b, err := migrations.FS.ReadFile(nome)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[int]int{}
	re := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`)
	for _, m := range re.FindAllStringSubmatch(semComentariosSQL(string(b)), -1) {
		item, _ := strconv.Atoi(m[2])
		chance, _ := strconv.Atoi(m[3])
		if out[m[1]] == nil {
			out[m[1]] = map[int]int{}
		}
		out[m[1]][item] = chance
	}
	return out
}

func eBrinco(item int) bool { return item >= 591 && item <= 595 }

// A 0175 deixa os brincos só no Golem de Fogo da sala, a 1 (10× menos que os 10
// da 0152), e troca os brincos das Gárgulas do Molar pelos seis braceletes.
func TestBrincosSoNoGolemDeFogo(t *testing.T) {
	regras := regrasDaMigracao(t, migBrincos)
	for item := 591; item <= 595; item++ {
		if got := regras["Golem_Fogo_Lava"][item]; got != 1 {
			t.Errorf("Golem_Fogo_Lava/%d = %d, quer 1", item, got)
		}
		for _, mob := range []string{"Gargula_Lava", "Gargula_Inf", "Gargula_Servo"} {
			got, ok := regras[mob][item]
			if !ok || got != 0 {
				t.Errorf("%s/%d = %d (tem regra: %v), quer 0", mob, item, got, ok)
			}
		}
	}
	for _, mob := range []string{"Gargula_Inf", "Gargula_Servo"} {
		for _, item := range []int{507, 510, 511, 512, 513, 514} {
			if got := regras[mob][item]; got != 10 {
				t.Errorf("%s/bracelete %d = %d, quer 10", mob, item, got)
			}
		}
		if _, ok := regras[mob][515]; ok {
			t.Errorf("%s: o 515 é o Bracelete genérico, sem efeito", mob)
		}
	}
	if _, ok := regras["Gargula_Lava"][507]; ok {
		t.Error("os braceletes são das Gárgulas do Molar, não da sala do Golem")
	}
}

// Os brincos das Gárgulas do Molar vêm do Carry do template, fora da Mesa: a
// regra a 0% só os tira se cobrir cada brinco que o arquivo carrega. Se alguém
// puser outro brinco no template, este teste cobra a regra.
func TestTemplatesDoMolarNaoSoltamBrincoForaDaMesa(t *testing.T) {
	const (
		tamanhoMob = 816
		offCarry   = 268
		itemSize   = 8
		maxCarry   = 64
	)
	regras := regrasDaMigracao(t, migBrincos)
	for _, mob := range []string{"Gargula_Inf", "Gargula_Servo"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", mob))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) != tamanhoMob {
			t.Fatalf("%s: %d bytes, quer %d", mob, len(b), tamanhoMob)
		}
		achou := 0
		for i := 0; i < maxCarry; i++ {
			item := int(binary.LittleEndian.Uint16(b[offCarry+i*itemSize:]))
			if !eBrinco(item) {
				continue
			}
			achou++
			if chance, ok := regras[mob][item]; !ok || chance != 0 {
				t.Errorf("%s: slot %d carrega o brinco %d sem regra a 0%%", mob, i, item)
			}
		}
		if achou == 0 {
			t.Errorf("%s: nenhum brinco no Carry — o template mudou e a 0175 pode sobrar", mob)
		}
	}
}
