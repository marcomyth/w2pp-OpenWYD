package migrations_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

const migQuestMortal = "0098_quest_mortal_restos_e_ouro.up.sql"

// valores lê as tuplas de um VALUES ( ... ) da migração, uma linha por tupla.
func valores(t *testing.T, sql, depois string) [][]int {
	t.Helper()
	i := strings.Index(sql, depois)
	if i < 0 {
		t.Fatalf("não achei %q na migração", depois)
	}
	trecho := sql[i:]
	if j := strings.Index(trecho, "ON CONFLICT"); j > 0 {
		trecho = trecho[:j]
	}
	re := regexp.MustCompile(`\(\s*('[^']*'|-?\d+)\s*(?:,\s*(-?\d+)\s*)+\)`)
	campo := regexp.MustCompile(`'[^']*'|-?\d+`)
	var out [][]int
	for _, m := range re.FindAllString(trecho, -1) {
		var linha []int
		for k, c := range campo.FindAllString(m, -1) {
			if strings.HasPrefix(c, "'") {
				continue // o nome do monstro; a chave é (posição, valor)
			}
			n, err := strconv.Atoi(c)
			if err != nil {
				t.Fatalf("campo %d de %q: %v", k, m, err)
			}
			linha = append(linha, n)
		}
		out = append(out, linha)
	}
	if len(out) == 0 {
		t.Fatalf("nenhuma tupla depois de %q", depois)
	}
	return out
}

// O VALUES da 0098 tem de ser a METADE exata do que a 0065 e a 0075 escreveram.
// Ele só é usado quando a linha não existe, mas é nesse caso que um dígito
// errado passa despercebido: a arena passaria a pagar outra coisa e ninguém
// veria, porque o caminho comum (ON CONFLICT) está certo.
func TestRestosDaQuestSaoMetadeDaMesaAnterior(t *testing.T) {
	b, err := migrations.FS.ReadFile(migQuestMortal)
	if err != nil {
		t.Fatal(err)
	}
	novo := map[string]int{}
	sql := string(b)
	re := regexp.MustCompile(`\('([^']+)',\s*(\d+),\s*(\d+)\)`)
	for _, m := range re.FindAllStringSubmatch(sql, -1) {
		ch, _ := strconv.Atoi(m[3])
		novo[m[1]+"/"+m[2]] = ch
	}
	for _, origem := range []string{"0065_arenas_kaizen_hidra.up.sql", "0075_arena_elfos_e_chave_orc.up.sql"} {
		ob, err := migrations.FS.ReadFile(origem)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(ob), -1) {
			if m[2] != "419" && m[2] != "420" {
				continue // Âmagos e Chave do Rei Orc não foram pedidos
			}
			velha, _ := strconv.Atoi(m[3])
			chave := m[1] + "/" + m[2]
			nova, ok := novo[chave]
			if !ok {
				t.Errorf("%s: a 0098 não corta %s", origem, chave)
				continue
			}
			if nova != velha/2 {
				t.Errorf("%s: a 0098 põe %d, e metade de %d é %d", chave, nova, velha, velha/2)
			}
		}
	}
	if len(novo) != 12 {
		t.Errorf("a 0098 mexe em %d pares (mob,item), e as três arenas são 12", len(novo))
	}
}

// O ouro dos troféus tem de ser 30% do que Common/Settings/QuestsRate.txt paga
// hoje, e a XP e as faixas têm de sair IGUAIS ao arquivo: a tabela sobrepõe o
// conteúdo inteiro, então uma XP digitada a menos aqui cortaria a quest sem que
// o pedido tivesse sido esse.
func TestOuroDaQuestE30PorCentoDoConteudo(t *testing.T) {
	arquivo := filepath.Join("..", "..", "Release", "Common", "Settings", "QuestsRate.txt")
	raw, err := os.ReadFile(arquivo)
	if err != nil {
		t.Skipf("sem o conteúdo em Release/: %v", err)
	}
	type rate struct{ exp, arch, coin, min, max int }
	conteudo := map[int]*rate{}
	for _, linha := range strings.Split(string(raw), "\n") {
		campos := strings.Fields(linha)
		if len(campos) < 3 {
			continue
		}
		tier, err := strconv.Atoi(campos[1])
		if err != nil || tier < 0 || tier > 4 {
			continue
		}
		if conteudo[tier] == nil {
			conteudo[tier] = &rate{}
		}
		n := func(i int) int { v, _ := strconv.Atoi(campos[i]); return v }
		switch strings.ToUpper(campos[0]) {
		case "EXP":
			conteudo[tier].exp, conteudo[tier].arch = n(2), n(3)
		case "COIN":
			conteudo[tier].coin = n(2)
		case "LEVEL":
			conteudo[tier].min, conteudo[tier].max = n(2), n(3)
		}
	}
	b, err := migrations.FS.ReadFile(migQuestMortal)
	if err != nil {
		t.Fatal(err)
	}
	linhas := valores(t, string(b), "INSERT INTO quest_reward")
	if len(linhas) != 5 {
		t.Fatalf("a migração grava %d tiers, e são 5", len(linhas))
	}
	for _, l := range linhas {
		if len(l) != 8 {
			t.Fatalf("tier com %d colunas: %v", len(l), l)
		}
		tier, exp, arch, coin, nivelMin, nivelMax := l[0], l[1], l[2], l[3], l[4], l[5]
		c := conteudo[tier]
		if c == nil {
			t.Errorf("tier %d não está em QuestsRate.txt", tier)
			continue
		}
		if want := c.coin * 3 / 10; coin != want {
			t.Errorf("tier %d: ouro %d, e 30%% de %d é %d", tier, coin, c.coin, want)
		}
		if exp != c.exp || arch != c.arch {
			t.Errorf("tier %d: XP %d/%d, e o conteúdo diz %d/%d", tier, exp, arch, c.exp, c.arch)
		}
		if nivelMin != c.min || nivelMax != c.max {
			t.Errorf("tier %d: faixa %d-%d, e o conteúdo diz %d-%d", tier, nivelMin, nivelMax, c.min, c.max)
		}
	}
}
