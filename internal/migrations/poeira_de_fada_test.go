package migrations_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// A Poeira de Fada (414, 4142 e 5600) sai de toda venda e todo drop que o jogador
// alcança sozinho (decisão de 15/09/2026). As que já estão com jogadores continuam
// valendo, e a equipe ainda pode dar pelo painel e pelo /gm. A 0066 é a parte do
// banco; a outra metade são as casas zeradas nos templates, porque o dbServer
// ressemeia a loja a partir deles em todo boot (dbserver/cmd/dbserver, TestTemplates).
func TestPoeiraDeFadaSaiDaVendaEDoDrop(t *testing.T) {
	b, err := migrations.FS.ReadFile("0066_poeira_de_fada_fora.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	comandos := comandosSQL(string(b))

	// Drop: uma regra '*' a 0% para cada poeira, e só elas.
	re := regexp.MustCompile(`\('\*',\s*(\d+),\s*(\d+)\)`)
	zeradas := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		zeradas[m[1]] = m[2]
	}
	for _, item := range []string{"414", "4142", "5600"} {
		if chance, ok := zeradas[item]; !ok || chance != "0" {
			t.Errorf("falta a regra ('*', %s, 0); regras '*' achadas: %v", item, zeradas)
		}
	}
	if len(zeradas) != 3 {
		t.Errorf("regras '*' = %v, queria só as três poeiras", zeradas)
	}

	const poeiras = "(414, 4142, 5600)"
	quer := []string{
		"DELETE FROM npc_shop_item WHERE item_index IN " + poeiras,
		"UPDATE npc_config_meta SET version = version + 1",
		"DELETE FROM drop_rule WHERE item IN " + poeiras + " AND mob <> '*'",
		"UPDATE drop_rule_meta SET version = version + 1",
		"UPDATE world_event_config SET enabled = FALSE",
		"UPDATE donate_shop_item SET enabled = FALSE",
		"UPDATE daily_reward_item SET enabled = FALSE",
	}
	tudo := strings.Join(comandos, " ; ")
	for _, q := range quer {
		if !strings.Contains(tudo, q) {
			t.Errorf("a migração não tem %q", q)
		}
	}

	// Nada fora das poeiras: todo UPDATE ou DELETE que não seja o bump de uma
	// tabela de versão tem de filtrar pelos três índices.
	for _, c := range comandos {
		mexe := strings.Contains(c, "UPDATE ") || strings.Contains(c, "DELETE FROM ")
		if !mexe {
			continue
		}
		if strings.Contains(c, poeiras) {
			continue
		}
		if strings.HasPrefix(c, "UPDATE drop_rule_meta SET version = version + 1") {
			continue
		}
		// O INSERT das regras '*' tem ON CONFLICT DO UPDATE; o conteúdo dele já foi
		// conferido acima, item por item.
		if strings.HasPrefix(c, "INSERT INTO drop_rule ") {
			continue
		}
		t.Errorf("comando mexe em linha sem filtrar pelas poeiras: %q", c)
	}
}

// comandosSQL tira os comentários, junta os espaços e separa por ';'.
func comandosSQL(sql string) []string {
	var linhas []string
	for _, l := range strings.Split(sql, "\n") {
		if i := strings.Index(l, "--"); i >= 0 {
			l = l[:i]
		}
		linhas = append(linhas, l)
	}
	var out []string
	for _, c := range strings.Split(strings.Join(linhas, " "), ";") {
		c = strings.Join(strings.Fields(c), " ")
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}
