package migrations_test

import (
	"regexp"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// O Emblema do Dragão (751) é a chave do Portão dos 2 Castelos e não pode cair
// de monstro nenhum (pedido do Marco em 25/09/2026). A regra tem de ser a '*' a
// 0%: uma regra por monstro deixaria os outros nove templates soltando a chave.
func TestEmblemaDoDragaoForaDoDrop(t *testing.T) {
	b, err := migrations.FS.ReadFile("0146_emblema_do_dragao_fora.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(b)
	if !regexp.MustCompile(`\('\*',\s*751,\s*0\)`).MatchString(sql) {
		t.Error("falta a regra ('*', 751, 0)")
	}
	if !regexp.MustCompile(`DELETE FROM drop_rule WHERE item = 751 AND mob <> '\*'`).MatchString(sql) {
		t.Error("as regras por monstro do emblema continuariam valendo")
	}
	if !regexp.MustCompile(`UPDATE drop_rule_meta SET version = version \+ 1`).MatchString(sql) {
		t.Error("sem subir a versão, o tmServer não relê a mesa")
	}
}
