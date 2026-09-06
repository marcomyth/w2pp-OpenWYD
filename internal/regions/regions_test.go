package regions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLine(t *testing.T) {
	casos := []struct {
		nome  string
		linha string
		want  Region
		ok    bool
	}{
		{"comum", "1161, 3595, 1267, 3695 = Agua_M",
			Region{X1: 1161, Y1: 3595, X2: 1267, Y2: 3695, Name: "Agua_M"}, true},
		// The file really is spaced like this on the Big_Cubo row.
		{"espaçamento irregular", "1272, 1427,1403, 1532 = Big_Cubo",
			Region{X1: 1272, Y1: 1427, X2: 1403, Y2: 1532, Name: "Big_Cubo"}, true},
		// Reino_Blue ships with the X corners swapped; left as written it would
		// contain no position at all.
		{"cantos invertidos", "1814, 1815, 1785, 1913 = Reino_Blue",
			Region{X1: 1785, Y1: 1815, X2: 1814, Y2: 1913, Name: "Reino_Blue"}, true},
		{"sem nome", "1, 2, 3, 4 =", Region{}, false},
		{"numeros de menos", "1, 2, 3 = X", Region{}, false},
		{"lixo", "isto nao e uma regiao", Region{}, false},
		{"comentario", "// nota", Region{}, false},
		{"vazia", "   ", Region{}, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, ok := parseLine(c.linha)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// Overlapping rows must resolve to the tighter one, which is the row that comes
// first. Armia and Erion are each written twice, the second a superset.
func TestPrimeiraLinhaVence(t *testing.T) {
	tab := Table{
		{X1: 0, Y1: 0, X2: 10, Y2: 10, Name: "Pequena"},
		{X1: 0, Y1: 0, X2: 100, Y2: 100, Name: "Grande"},
	}
	if got := tab.At(5, 5); got != "Pequena" {
		t.Errorf("At(5,5) = %q, want Pequena", got)
	}
	if got := tab.At(50, 50); got != "Grande" {
		t.Errorf("At(50,50) = %q, want Grande", got)
	}
	if got := tab.At(500, 500); got != "" {
		t.Errorf("At(500,500) = %q, want vazio", got)
	}
}

func TestLabel(t *testing.T) {
	casos := map[string]string{
		"Agua_M":               "Água Místico",
		"Dugeon_1_Andar_Hidra": "Dungeon 1º Andar (Hidra)",
		"Coliseu":              "Coliseu",
		// Not in the table: the fallback still has to produce words.
		"Regiao_Nova_Qualquer": "Regiao Nova Qualquer",
	}
	for in, want := range casos {
		if got := Label(in); got != want {
			t.Errorf("Label(%q) = %q, want %q", in, got, want)
		}
	}
}

// The parser is checked against the shipped file, not only against hand-written
// lines: this is content nobody in the project controls, and a format
// assumption that holds for a fixture but not for Regions.txt would be worth
// nothing.
func TestArquivoDeVerdade(t *testing.T) {
	path := filepath.Join("..", "..", "Release", "TMsrv", "run", "Regions.txt")
	if _, err := os.Stat(path); err != nil {
		t.Skip("Release/ não está montado nesta máquina")
	}
	tab, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tab) < 45 {
		t.Fatalf("só %d regiões — o arquivo tem 51 linhas, o parser está descartando demais", len(tab))
	}
	// Gargula_ spawns at 1250,3608 (NPCGener). If this stops resolving to the
	// Água Místico instance the whole "where is this monster from" feature is
	// silently answering "field" for every dungeon mob.
	if got := tab.At(1250, 3608); got != "Agua_M" {
		t.Errorf("o ponto do Gargula_ deu %q, want Agua_M", got)
	}
	if got := tab.At(2200, 2100); got != "Armia" {
		t.Errorf("At(2200,2100) = %q, want Armia", got)
	}
	// Regions.txt does NOT cover the towns the way mapzones does: Armia's
	// rectangles start at x=2164, well east of the 2086,2093 center mapzones
	// uses. The two tables are complementary, and a caller that expects this
	// one to name every position will get "" over most of the map.
	if got := tab.At(2086, 2093); got != "" {
		t.Errorf("o centro de Armia deu %q — se o arquivo passou a cobrir as cidades, "+
			"a ordem de consulta com mapzones precisa ser revista", got)
	}
}
