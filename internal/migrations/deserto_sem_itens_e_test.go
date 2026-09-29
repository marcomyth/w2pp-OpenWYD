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

// itemE diz se o índice é uma arma E (e as Anct dela) ou uma peça dos quatro
// sets E, com as versões (Le).
func itemE(i int) bool {
	faixas := [][2]int{
		{3551, 3596}, {3601, 3784}, // armas E e as Anct
		{1225, 1229}, {1360, 1364}, {1510, 1514}, {1660, 1664}, // sets E
		{3801, 3805}, {3821, 3825}, {3841, 3845}, {3861, 3865}, // sets E (Le)
	}
	for _, f := range faixas {
		if i >= f[0] && i <= f[1] {
			return true
		}
	}
	return false
}

// regrasZeradas junta as regras a 0% das migrações dadas: mob → itens.
func regrasZeradas(t *testing.T, nomes ...string) map[string]map[int]bool {
	t.Helper()
	re := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*0\)`)
	out := map[string]map[int]bool{}
	for _, n := range nomes {
		b, err := migrations.FS.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(semComentariosSQL(string(b)), -1) {
			item, _ := strconv.Atoi(m[2])
			if out[m[1]] == nil {
				out[m[1]] = map[int]bool{}
			}
			out[m[1]][item] = true
		}
	}
	return out
}

// Nenhum dos onze templates do Deserto (0109) pode soltar arma E ou peça do set
// E: cada uma que o template carrega tem de ter a regra a 0% da 0183 (Mantícora)
// ou da 0186 (Verme, Aeon e Adamant). Lê os templates de verdade, então um item
// E novo num template, ou um template esquecido, quebra o teste.
func TestDesertoSemItensE(t *testing.T) {
	zeradas := regrasZeradas(t, "0183_manticora_sem_itens_e.up.sql", "0186_deserto_sem_itens_e.up.sql")
	deserto := []string{
		"Tauron", "Ladrao_Tauron", "Aranha_Inferno", "Taron_Assassino", "Arqueiro_Tauron",
		"Verme_", "Manticora", "Aeon_Tauron", "Treant", "Lugefer", "Adamant_Tauron",
	}
	const carry, itemSize, maxCarry = 268, 8, 64 // STRUCT_MOB.Carry (internal/savefmt)
	achados := 0
	for _, mob := range deserto {
		b, err := os.ReadFile(filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", mob))
		if err != nil {
			t.Fatal(err)
		}
		for vaga := range maxCarry {
			idx := int(int16(binary.LittleEndian.Uint16(b[carry+vaga*itemSize:])))
			if idx <= 0 || !itemE(idx) {
				continue
			}
			achados++
			if !zeradas[mob][idx] {
				t.Errorf("%s solta o item E %d (vaga %d) e não tem regra a 0%%", mob, idx, vaga)
			}
		}
	}
	// Sem isto, um template que mudasse de formato passaria calado, sem achar nada.
	if achados == 0 {
		t.Fatal("nenhum item E achado nos templates do Deserto: o teste não está lendo o Carry")
	}
}

// A 0186 só mexe nos três templates, só a 0%, e sobe a versão da Mesa para o
// jogo recarregar.
func TestDesertoSemItensESoNosTres(t *testing.T) {
	zeradas := regrasZeradas(t, "0186_deserto_sem_itens_e.up.sql")
	total := 0
	for mob, itens := range zeradas {
		switch mob {
		case "Verme_", "Aeon_Tauron", "Adamant_Tauron":
		default:
			t.Errorf("a 0186 mexe em %s", mob)
		}
		total += len(itens)
	}
	if total != 20 {
		t.Errorf("a 0186 tem %d regras a 0%%, want 20", total)
	}
	b, err := migrations.FS.ReadFile("0186_deserto_sem_itens_e.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance",
		"UPDATE drop_rule_meta SET version = version + 1",
	} {
		if !regexp.MustCompile(regexp.QuoteMeta(quer)).MatchString(sql) {
			t.Errorf("a 0186 não tem %q", quer)
		}
	}
}
