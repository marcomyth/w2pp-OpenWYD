package migrations_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// Nenhum ON CONFLICT ... DO UPDATE pode calcular o valor novo a partir do valor
// que já está na coluna.
//
// A 0098 fazia isso — `SET chance = drop_rule.chance / 2` e
// `SET coin = quest_reward.coin * 3 / 10` —, com uma intenção boa: cortar a
// partir da linha que o painel já tivesse afinado, em vez de substituí-la. O
// problema é que uma transformação que lê a própria coluna COMPÕE. Rodar duas
// vezes leva 2500 a 1250 e depois a 625, e o banco não denuncia: os números
// continuam plausíveis, só menores a cada vez. O ouro é pior — `* 3 / 10` duas
// vezes deixa 9% do original.
//
// "Migração não roda duas vezes" é verdade por design e mentira na prática: roda
// quando alguém restaura um banco pela metade, quando um arquivo é renomeado (o
// controle é pelo nome, internal/store/migrate.go), e quando alguém copia o
// trecho para uma migração nova sem reparar no detalhe.
//
// O absoluto tem o custo de sobrescrever uma afinação do painel. É um custo
// visível na hora e desfazível com um clique, contra um erro invisível e
// irrecuperável. Este teste guarda a escolha.
func TestUpsertNaoCalculaSobreOValorAntigo(t *testing.T) {
	arquivos, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	blocoUpsert := regexp.MustCompile(`(?is)ON\s+CONFLICT\b.*?DO\s+UPDATE\s+SET(.*?);`)
	literal := regexp.MustCompile(`'[^']*'`)
	excluido := regexp.MustCompile(`(?i)\bEXCLUDED\.[a-z_][a-z0-9_]*`)
	funcao := regexp.MustCompile(`(?i)\b(now|current_timestamp)\s*\(\s*\)`)
	numero := regexp.MustCompile(`\b\d+\b`)
	palavra := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.]*`)
	reservadas := map[string]bool{
		"true": true, "false": true, "null": true, "default": true,
		"current_timestamp": true,
	}

	var vistos int
	for _, f := range arquivos {
		nome := f.Name()
		if !strings.HasSuffix(nome, ".up.sql") {
			continue
		}
		b, err := migrations.FS.ReadFile(nome)
		if err != nil {
			t.Fatal(err)
		}
		// Os comentários falam de `chance / 2` justamente para explicar por que
		// ele saiu; o teste olha só o SQL.
		var corpo strings.Builder
		for _, linha := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(linha), "--") {
				continue
			}
			corpo.WriteString(linha)
			corpo.WriteString("\n")
		}

		for _, m := range blocoUpsert.FindAllStringSubmatch(corpo.String(), -1) {
			vistos++
			limpo := m[1]
			for _, re := range []*regexp.Regexp{literal, excluido, funcao, numero} {
				limpo = re.ReplaceAllString(limpo, " ")
			}
			for _, atribuicao := range strings.Split(limpo, ",") {
				i := strings.Index(atribuicao, "=")
				if i < 0 {
					continue // continuação de uma expressão da vírgula anterior
				}
				for _, p := range palavra.FindAllString(atribuicao[i+1:], -1) {
					if reservadas[strings.ToLower(p)] {
						continue
					}
					t.Errorf("%s: o DO UPDATE calcula sobre o valor antigo (%q em %q). "+
						"Escreva o valor absoluto, com EXCLUDED — aplicar duas vezes tem de "+
						"deixar a linha no mesmo lugar.",
						nome, p, strings.Join(strings.Fields(atribuicao), " "))
				}
			}
		}
	}
	if vistos == 0 {
		t.Fatal("nenhum ON CONFLICT DO UPDATE encontrado; o teste não está lendo as migrações")
	}
}
