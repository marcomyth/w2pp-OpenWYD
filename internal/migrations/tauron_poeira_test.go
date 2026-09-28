package migrations_test

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0177 corta a Poeira dos cinco Tauron em 10×: de 100 e 50 (0109) para 10 e 5.
func TestTauronPoeiraDezVezesMenos(t *testing.T) {
	b, err := migrations.FS.ReadFile("0177_tauron_poeira_dez_vezes_menos.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, mob := range []string{"Tauron", "Ladrao_Tauron", "Arqueiro_Tauron", "Aeon_Tauron", "Adamant_Tauron"} {
		for _, quer := range []string{"('" + mob + "',", "412, 10)", "413, 5)"} {
			if !strings.Contains(sql, quer) {
				t.Errorf("a 0177 não tem %q (%s)", quer, mob)
			}
		}
	}
	for _, quer := range []string{
		"ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance",
		"UPDATE drop_rule_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0177 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "DELETE") {
		t.Error("a 0177 apaga regra; ela só baixa as chances")
	}
}
