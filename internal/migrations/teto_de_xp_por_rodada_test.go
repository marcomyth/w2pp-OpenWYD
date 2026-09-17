package migrations_test

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// TestTetoDeXPPorRodadaBateComOCodigo: os DEFAULT da 0073 têm de bater com
// domain.DefaultRoundXPCap e DefaultRoundXPCapDouble, e as colunas têm de seguir
// as faixas de domain.RoundXPCapTopLevels. Um banco novo e um tmServer sem a
// linha rodariam tetos diferentes, e a prova do teto mediria o número errado.
func TestTetoDeXPPorRodadaBateComOCodigo(t *testing.T) {
	b, err := migrations.FS.ReadFile("0073_teto_de_xp_por_rodada.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(b)
	confere := func(prefixo string, quer [5]int64) {
		for i, topo := range domain.RoundXPCapTopLevels {
			col := fmt.Sprintf("%s_%d", prefixo, topo)
			re := regexp.MustCompile(`ADD COLUMN\s+` + col + `\s+BIGINT\s+NOT NULL\s+DEFAULT\s+(\d+)\s+CHECK\s*\(\s*` + col + `\s*>=\s*0\s*\)`)
			m := re.FindStringSubmatch(sql)
			if m == nil {
				t.Errorf("não achei %s como BIGINT NOT NULL DEFAULT <n> CHECK (>= 0)", col)
				continue
			}
			if v, _ := strconv.ParseInt(m[1], 10, 64); v != quer[i] {
				t.Errorf("DEFAULT de %s é %d, mas o domain diz %d", col, v, quer[i])
			}
		}
	}
	confere("round_xp_cap", domain.DefaultRoundXPCap)
	confere("round_xp_cap_double", domain.DefaultRoundXPCapDouble)
}
