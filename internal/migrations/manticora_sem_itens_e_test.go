package migrations_test

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0183 tira da Mantícora comum as três armas E e os quatro elmos do set E, a
// 0%, e só dela.
func TestManticoraSemItensE(t *testing.T) {
	b, err := migrations.FS.ReadFile("0183_manticora_sem_itens_e.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, item := range []string{"3551", "3571", "3591", "1225", "1360", "1510", "1660"} {
		if !strings.Contains(sql, "('Manticora', "+item+", 0)") {
			t.Errorf("a 0183 não zera o item %s na Manticora", item)
		}
	}
	if strings.Count(sql, "('") != 7 {
		t.Errorf("a 0183 tem %d regras, want 7 (só a Manticora)", strings.Count(sql, "('"))
	}
	for _, quer := range []string{
		"ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance",
		"UPDATE drop_rule_meta SET version = version + 1",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0183 não tem %q", quer)
		}
	}
}
