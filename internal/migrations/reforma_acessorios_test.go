package migrations_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// Reforma dos acessórios (16/09/2026): planetas, Amuleto dos Amantes e Arcano
// saem de toda venda; o Arcano sai também do drop; o Aki troca cinco vagas por
// anéis. A outra metade são os templates (dbserver/cmd/dbserver).
func TestReformaDosAcessoriosNoBanco(t *testing.T) {
	b, err := migrations.FS.ReadFile("0070_reforma_acessorios.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	comandos := comandosSQL(string(b))
	tudo := strings.Join(comandos, " ; ")

	const foraDeVenda = "(762, 763, 764, 765, 766, 767, 768, 1738, 567, 568, 569, 570)"
	const arcanos = "(567, 568, 569, 570)"
	quer := []string{
		"DELETE FROM npc_shop_item WHERE item_index IN " + foraDeVenda,
		"WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Aki') AND slot IN (18, 19, 20, 21, 22) AND item_index IN (646, 647, 693, 694, 695)",
		"DELETE FROM drop_rule WHERE item IN " + arcanos + " AND mob <> '*'",
		"UPDATE drop_rule_meta SET version = version + 1",
		"UPDATE donate_shop_item SET enabled = FALSE",
		"UPDATE daily_reward_item SET enabled = FALSE",
	}
	for _, q := range quer {
		if !strings.Contains(tudo, q) {
			t.Errorf("a migração não tem %q", q)
		}
	}

	// Drop: '*' a 0% só para os quatro Arcanos. Planetas e Amantes continuam
	// caindo — a raridade deles é ajuste da Mesa de Drops.
	re := regexp.MustCompile(`\('\*',\s*(\d+),\s*(\d+)\)`)
	zeradas := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		zeradas[m[1]] = m[2]
	}
	if len(zeradas) != 4 {
		t.Errorf("regras '*' = %v, queria só os quatro Arcanos", zeradas)
	}
	for _, item := range []string{"567", "568", "569", "570"} {
		if zeradas[item] != "0" {
			t.Errorf("falta ('*', %s, 0); achadas: %v", item, zeradas)
		}
	}

	// Nada fora dos itens da reforma: todo UPDATE ou DELETE filtra por eles, a
	// não ser os bumps de versão e o INSERT já conferido.
	for _, c := range comandos {
		if !strings.Contains(c, "UPDATE ") && !strings.Contains(c, "DELETE FROM ") {
			continue
		}
		switch {
		case strings.Contains(c, foraDeVenda), strings.Contains(c, arcanos),
			strings.Contains(c, "template_name = 'Aki'"),
			strings.HasPrefix(c, "UPDATE drop_rule_meta SET version = version + 1"),
			strings.HasPrefix(c, "INSERT INTO drop_rule "):
			continue
		}
		t.Errorf("comando mexe em linha sem filtrar pelos itens da reforma: %q", c)
	}
}
