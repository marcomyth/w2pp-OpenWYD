package regiao

import (
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/jeanluca/w2pp-openwyd/internal/regions"
)

// A transcrição bate com o Regions.txt do conteúdo: mesmos retângulos, na mesma
// ordem. Se alguém mexer no arquivo, este teste avisa que a tabela ficou para trás.
func TestTabelaBateComORegionsTxt(t *testing.T) {
	arq, err := regions.Load(filepath.Join("..", "..", "Release", "TMsrv", "run", "Regions.txt"))
	if err != nil {
		t.Skipf("sem o Regions.txt do conteúdo: %v", err)
	}
	var daqui []linha
	for _, l := range tabela {
		if l.regions != "" {
			daqui = append(daqui, l)
		}
	}
	if len(daqui) != len(arq) {
		t.Fatalf("a tabela tem %d linhas do Regions.txt e o arquivo tem %d", len(daqui), len(arq))
	}
	for i, r := range arq {
		l := daqui[i]
		if l.regions != r.Name || l.x1 != r.X1 || l.y1 != r.Y1 || l.x2 != r.X2 || l.y2 != r.Y2 {
			t.Errorf("linha %d: tabela %s (%d,%d,%d,%d), arquivo %s (%d,%d,%d,%d)", i,
				l.regions, l.x1, l.y1, l.x2, l.y2, r.Name, r.X1, r.Y1, r.X2, r.Y2)
		}
	}
}

// Todo nome cabe no campo do pacote e se escreve em CP1252, que é o que o
// cliente desenha.
func TestNomesCabemNoPacote(t *testing.T) {
	for _, l := range Todos() {
		if l.Nome == "" {
			t.Errorf("lugar sem nome: %+v", l)
		}
		if n := utf8.RuneCountInString(l.Nome); n > NomeMax {
			t.Errorf("%q tem %d letras, o campo leva %d", l.Nome, n, NomeMax)
		}
		for _, r := range l.Nome {
			if r > 0xFF {
				t.Errorf("%q tem a letra %q, que não existe em CP1252", l.Nome, r)
			}
		}
	}
}

// O mais específico ganha: a arena fica dentro de uma região maior.
func TestEm(t *testing.T) {
	casos := []struct {
		x, y int32
		want Lugar
	}{
		{2400, 2100, Lugar{Quest, "Coveiro"}},        // dentro de Armia
		{2240, 1710, Lugar{Quest, "Jardim"}},         // dentro de Azran
		{2300, 2100, Lugar{MapaAberto, "Armia"}},     // Armia fora da arena
		{1200, 1700, Lugar{MapaAberto, "Deserto"}},   // Deserto_Pilar
		{1100, 3500, Lugar{Masmorra, "Água Normal"}}, // Agua_N
		{1800, 1850, Lugar{MapaAberto, "Reinos"}},    // Reino_Blue, cantos invertidos no arquivo
		{5, 5, Padrao}, // nada cobre
	}
	for _, c := range casos {
		if got := Em(c.x, c.y); got != c.want {
			t.Errorf("Em(%d,%d) = %+v, quero %+v", c.x, c.y, got, c.want)
		}
	}
}
