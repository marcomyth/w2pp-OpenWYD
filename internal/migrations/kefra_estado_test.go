package migrations_test

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A 0067 prepara o estado do Kefra para ter uma fonte só, gravável pelo jogo e
// pelo painel: a guilda que matou vai para world_event_config, e a auditoria
// passa a aceitar uma gravação sem conta (a do jogo), com a fonte anotada. Ela
// não muda o estado em si: o Kefra continua como estava.
func TestKefraEstadoMigracao(t *testing.T) {
	b, err := migrations.FS.ReadFile("0067_kefra_estado.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := semComentariosSQL(string(b))
	for _, quer := range []string{
		"ALTER TABLE world_event_config ADD COLUMN kefra_guild_id INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE world_event_audit ALTER COLUMN account_id DROP NOT NULL",
		"ADD COLUMN fonte TEXT NOT NULL DEFAULT 'painel'",
	} {
		if !strings.Contains(sql, quer) {
			t.Errorf("a 0067 não tem %q", quer)
		}
	}
	if strings.Contains(sql, "kefra_live_enabled") {
		t.Error("a 0067 mexe no estado do Kefra; ela só pode preparar o caminho")
	}

	volta, err := migrations.FS.ReadFile("0067_kefra_estado.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(semComentariosSQL(string(volta)), "DROP COLUMN IF EXISTS kefra_guild_id") {
		t.Error("a volta da 0067 não tira a coluna da guilda")
	}
}

// semComentariosSQL tira os comentários de linha e junta os espaços.
func semComentariosSQL(sql string) string {
	var linhas []string
	for _, l := range strings.Split(sql, "\n") {
		if i := strings.Index(l, "--"); i >= 0 {
			l = l[:i]
		}
		linhas = append(linhas, l)
	}
	return strings.Join(strings.Fields(strings.Join(linhas, " ")), " ")
}
