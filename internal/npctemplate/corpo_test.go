package npctemplate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
)

// moldeComCorpo is a canonical 816-byte template wearing body corpo.
func moldeComCorpo(corpo int16) []byte {
	var m savefmt.Mob
	m.Equip[0].Index = corpo
	return savefmt.EncodeMob(m)
}

func TestCorposConhece(t *testing.T) {
	c := Corpos{}
	c.Add("Lobo", moldeComCorpo(12))
	c.Add("LoboRaivoso", moldeComCorpo(12)) // same body: the first name stays
	c.Add("Quebrado", []byte{1, 2, 3})      // does not decode: proves nothing

	if c[12] != "Lobo" {
		t.Errorf("exemplo do corpo 12 = %q, quero Lobo", c[12])
	}
	if len(c) != 1 {
		t.Errorf("corpos = %v, quero só o 12", c)
	}

	cases := []struct {
		name    string
		raw     []byte
		corpo   int16
		ok      bool
		wantErr bool
	}{
		{"corpo que já nasce", moldeComCorpo(12), 12, true, false},
		{"corpo que nenhum monstro usa", moldeComCorpo(345), 345, false, false},
		{"molde ilegível", []byte{0}, 0, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			corpo, ok, err := c.Conhece(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, quero erro %v", err, tc.wantErr)
			}
			if corpo != tc.corpo || ok != tc.ok {
				t.Errorf("Conhece = (%d, %v), quero (%d, %v)", corpo, ok, tc.corpo, tc.ok)
			}
		})
	}
}

// CorposDoArquivo reads the templates the file names, once each, and skips a
// name with no file the way the boot does.
func TestCorposDoArquivo(t *testing.T) {
	dir := t.TempDir()
	npc := Dir(dir)
	if err := os.MkdirAll(npc, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, corpo := range map[string]int16{"Lobo": 12, "Urso": 30} {
		if err := os.WriteFile(filepath.Join(npc, name), moldeComCorpo(corpo), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	c := CorposDoArquivo(dir, []string{"Lobo", "", "Urso", "Lobo", "NaoExiste"})

	if len(c) != 2 || c[12] != "Lobo" || c[30] != "Urso" {
		t.Errorf("corpos = %v, quero 12→Lobo e 30→Urso", c)
	}
}
