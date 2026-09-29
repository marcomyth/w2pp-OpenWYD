package migrations_test

import (
	"bufio"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

const desertoEViraD = "0186_deserto_e_vira_d.up.sql"

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

// regrasDaMesa junta as regras das migrações dadas: mob → item → chance.
func regrasDaMesa(t *testing.T, nomes ...string) map[string]map[int]int {
	t.Helper()
	re := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`)
	out := map[string]map[int]int{}
	for _, n := range nomes {
		b, err := migrations.FS.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(semComentariosSQL(string(b)), -1) {
			item, _ := strconv.Atoi(m[2])
			chance, _ := strconv.Atoi(m[3])
			if out[m[1]] == nil {
				out[m[1]] = map[int]int{}
			}
			out[m[1]][item] = chance
		}
	}
	return out
}

// itemDoCatalogo é o que o teste precisa de uma linha do ItemList.csv.
type itemDoCatalogo struct {
	pos, grade, nivel, classe int
	arma                      bool
}

func catalogo(t *testing.T) map[int]itemDoCatalogo {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "Release", "Common", "ItemList.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	efeito := func(c []string, nome string) int {
		for i := 9; i+1 < len(c); i += 2 {
			if strings.TrimSpace(c[i]) == nome {
				v, _ := strconv.Atoi(strings.TrimSpace(c[i+1]))
				return v
			}
		}
		return -1
	}
	out := map[int]itemDoCatalogo{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		c := strings.Split(sc.Text(), ",")
		if len(c) < 9 {
			continue
		}
		idx, err := strconv.Atoi(c[0])
		if err != nil {
			continue
		}
		pos, _ := strconv.Atoi(c[6])
		grade, _ := strconv.Atoi(c[8])
		out[idx] = itemDoCatalogo{
			pos: pos, grade: grade,
			nivel: efeito(c, "EF_ITEMLEVEL"), classe: efeito(c, "EF_CLASS"),
			arma: efeito(c, "EF_WTYPE") >= 0,
		}
	}
	return out
}

// carryDoTemplate lê o Carry de um template do Release: índice → vagas.
func carryDoTemplate(t *testing.T, mob string) map[int]int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "Release", "TMsrv", "run", "npc", mob))
	if err != nil {
		t.Fatal(err)
	}
	const carry, itemSize, maxCarry = 268, 8, 64 // STRUCT_MOB.Carry (internal/savefmt)
	out := map[int]int{}
	for vaga := range maxCarry {
		if idx := int(int16(binary.LittleEndian.Uint16(b[carry+vaga*itemSize:]))); idx > 0 {
			out[idx]++
		}
	}
	return out
}

var desertoTemplates = []string{
	"Tauron", "Ladrao_Tauron", "Aranha_Inferno", "Taron_Assassino", "Arqueiro_Tauron",
	"Verme_", "Manticora", "Aeon_Tauron", "Treant", "Lugefer", "Adamant_Tauron",
}

// Nenhum dos onze templates do Deserto (0109) pode soltar arma E ou peça do set
// E: cada uma que o template carrega tem de ter a regra a 0% da 0183 (Mantícora)
// ou da 0186. Lê os templates de verdade, então um item E novo num template, ou
// um template esquecido, quebra o teste.
func TestDesertoSemItensE(t *testing.T) {
	mesa := regrasDaMesa(t, "0183_manticora_sem_itens_e.up.sql", desertoEViraD)
	achados := 0
	for _, mob := range desertoTemplates {
		for idx := range carryDoTemplate(t, mob) {
			if !itemE(idx) {
				continue
			}
			achados++
			if c, ok := mesa[mob][idx]; !ok || c != 0 {
				t.Errorf("%s solta o item E %d e não tem regra a 0%% (tem=%v, chance %d)", mob, idx, ok, c)
			}
		}
	}
	// Sem isto, um template que mudasse de formato passaria calado, sem achar nada.
	if achados == 0 {
		t.Fatal("nenhum item E achado nos templates do Deserto: o teste não está lendo o Carry")
	}
}

// Cada monstro que perdeu item E ganha o equivalente D pela Mesa: o mesmo
// número de Armas D, e para cada posição de peça E, as quatro peças do Set D (A)
// daquela posição, uma por classe. Tudo nível D, e com as chances combinadas.
func TestDesertoEViraD(t *testing.T) {
	cat := catalogo(t)
	mesa := regrasDaMesa(t, "0183_manticora_sem_itens_e.up.sql", desertoEViraD)
	novas := regrasDaMesa(t, desertoEViraD)
	for _, mob := range []string{"Verme_", "Manticora", "Aeon_Tauron", "Adamant_Tauron"} {
		carry := carryDoTemplate(t, mob)
		armasE, posicoesE := 0, map[int]bool{}
		vagasArmaE := 0
		for idx, vagas := range carry {
			if !itemE(idx) {
				continue
			}
			if cat[idx].arma {
				armasE++
				vagasArmaE = vagas
			} else {
				posicoesE[cat[idx].pos] = true
			}
		}
		if armasE == 0 || len(posicoesE) == 0 {
			t.Fatalf("%s não tem arma e peça E no template: o teste não prova nada", mob)
		}
		armasD, pecas := 0, map[int]map[int]bool{}
		for idx, chance := range novas[mob] {
			if chance == 0 {
				continue
			}
			it, ok := cat[idx]
			if !ok || it.nivel != 4 {
				t.Errorf("%s: o item %d da 0186 não é nível D (EF_ITEMLEVEL %d)", mob, idx, it.nivel)
				continue
			}
			if it.arma {
				armasD++
				// Uma vaga de 1 em 900 vira 9; duas vagas (o Verme) viram 18.
				if want := 9 * vagasArmaE; chance != want {
					t.Errorf("%s: Arma D %d com chance %d, want %d", mob, idx, chance, want)
				}
				continue
			}
			if it.grade != 3 {
				t.Errorf("%s: a peça %d não é (A) (Grade %d)", mob, idx, it.grade)
			}
			if chance != 8 {
				t.Errorf("%s: peça %d com chance %d, want 8", mob, idx, chance)
			}
			if pecas[it.pos] == nil {
				pecas[it.pos] = map[int]bool{}
			}
			pecas[it.pos][it.classe] = true
		}
		if armasD != armasE {
			t.Errorf("%s: %d armas E saíram e %d Armas D entraram", mob, armasE, armasD)
		}
		for pos := range posicoesE {
			for _, classe := range []int{1, 2, 4, 8} {
				if !pecas[pos][classe] {
					t.Errorf("%s: falta a peça D (A) da posição %d para a classe %d", mob, pos, classe)
				}
			}
		}
		if len(pecas) != len(posicoesE) {
			t.Errorf("%s: peças D em %d posições, e as E estavam em %d", mob, len(pecas), len(posicoesE))
		}
		// Os itens E do template continuam fora (o primeiro teste confere todos).
		for idx := range carry {
			if itemE(idx) && mesa[mob][idx] != 0 {
				t.Errorf("%s: item E %d com chance %d", mob, idx, mesa[mob][idx])
			}
		}
	}
}

// A 0186 só mexe nos quatro monstros, e sobe a versão da Mesa para o jogo
// recarregar.
func TestDesertoEViraDSoNosQuatro(t *testing.T) {
	for mob := range regrasDaMesa(t, desertoEViraD) {
		switch mob {
		case "Verme_", "Manticora", "Aeon_Tauron", "Adamant_Tauron":
		default:
			t.Errorf("a 0186 mexe em %s", mob)
		}
	}
	b, err := migrations.FS.ReadFile(desertoEViraD)
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance",
		"UPDATE drop_rule_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0186 não tem %q", quer)
		}
	}
}
