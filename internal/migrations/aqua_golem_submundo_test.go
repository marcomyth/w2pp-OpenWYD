package migrations_test

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0169 é o saque do Aqua Golem do Submundo pedido em 26/09/2026: Restos,
// Emblema do Guarda, Repletion D e a Moeda de 1Mi entram, os dois ovos de Cavalo
// Fantasma saem. Só o template de campo aberto — o nome exato é o que impede a
// regra de vazar para os golems da Água (Aqua_Golem_ e Aqua_Golem___).
func TestAquaGolemSubmundo(t *testing.T) {
	b, err := migrations.FS.ReadFile("0169_aqua_golem_submundo.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"('Aqua_Golem', 419, 300)",
		"('Aqua_Golem', 420, 150)",
		"('Aqua_Golem', 4042, 100)",
		"('Aqua_Golem', 4019, 50)",
		"('Aqua_Golem', 4026, 20)",
		"('Aqua_Golem', 2307, 0)",
		"('Aqua_Golem', 2312, 0)",
		"ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance",
		"UPDATE drop_rule_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0169 não tem %q", quer)
		}
	}
	for _, proibido := range []string{"'Aqua_Golem_'", "'Aqua_Golem___'", "DELETE"} {
		if strings.Contains(sql, proibido) {
			t.Errorf("a 0169 tem %q", proibido)
		}
	}
}
