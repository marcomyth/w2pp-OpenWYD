package migrations_test

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0068 guarda as entradas do Hall do Kefra por personagem. Ela só ACRESCENTA:
// a coluna nasce com 0 para todo mundo, então nenhum personagem existente ganha
// ou perde entrada por causa da migração. O CHECK existe porque o gasto é um
// decremento — sem ele, um erro de conta deixaria entrada negativa no banco.
func TestKefraTicketMigracao(t *testing.T) {
	b, err := migrations.FS.ReadFile("0068_kefra_ticket.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"ALTER TABLE character",
		"ADD COLUMN kefra_ticket INTEGER NOT NULL DEFAULT 0 CHECK (kefra_ticket >= 0)",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0068 não tem %q", quer)
		}
	}
	// Migração de acréscimo não apaga nada: nem coluna, nem linha.
	for _, proibido := range []string{"DROP", "DELETE", "TRUNCATE", "UPDATE"} {
		if strings.Contains(strings.ToUpper(sql), proibido) {
			t.Errorf("a 0068 tem %q e ela só pode acrescentar a coluna", proibido)
		}
	}

	volta, err := migrations.FS.ReadFile("0068_kefra_ticket.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(semComentariosSQL(string(volta)), "DROP COLUMN kefra_ticket") {
		t.Error("a volta da 0068 não tira a coluna das entradas")
	}
}
