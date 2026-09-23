//go:build integration

// A prova de que uma migração de dados pode rodar duas vezes sem mover nada.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/migrations"
)

// comandos quebra o .sql em instruções, jogando fora os comentários de linha.
// Serve para reaplicar uma migração pelo caminho de fora do controle de versão,
// que é exatamente o que este teste quer fazer.
func comandos(sql string) []string {
	var limpo strings.Builder
	for _, linha := range strings.Split(sql, "\n") {
		if strings.HasPrefix(strings.TrimSpace(linha), "--") {
			continue
		}
		limpo.WriteString(linha)
		limpo.WriteString("\n")
	}
	var out []string
	for _, c := range strings.Split(limpo.String(), ";") {
		if strings.TrimSpace(c) != "" {
			out = append(out, c)
		}
	}
	return out
}

// A 0098 aplicada duas vezes tem de deixar as linhas no mesmo lugar.
//
// A primeira versão dela não deixava: cortava a chance pela metade e o ouro a
// 30% LENDO a própria coluna, então a segunda passada cortava de novo — 2500
// virava 1250, e o ouro ficava em 9% do original. O erro é silencioso, porque os
// números continuam plausíveis; só o teste que roda duas vezes vê.
//
// Este teste não é só sobre a 0098. Ele é o lugar onde a próxima migração que
// mexer em valores pode ser posta para provar o mesmo, e é o par de dentro do
// banco do guarda estático em internal/migrations/upsert_absoluto_test.go, que
// lê o SQL sem executá-lo.
func TestMigracao0098PodeRodarDuasVezes(t *testing.T) {
	s, ctx := freshStore(t)

	antes := retratoDa0098(ctx, t, s)
	if len(antes) == 0 {
		t.Fatal("o retrato veio vazio; a migração não escreveu nada")
	}

	b, err := migrations.FS.ReadFile("0098_quest_mortal_restos_e_ouro.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range comandos(string(b)) {
		if _, err := s.pool.Exec(ctx, c); err != nil {
			t.Fatalf("reaplicando: %v\n%s", err, strings.TrimSpace(c))
		}
	}

	depois := retratoDa0098(ctx, t, s)
	for chave, v := range antes {
		if depois[chave] != v {
			t.Errorf("%s: era %d e virou %d depois da segunda aplicação", chave, v, depois[chave])
		}
	}
	if len(depois) != len(antes) {
		t.Errorf("o retrato mudou de tamanho: %d -> %d", len(antes), len(depois))
	}
}

// E o outro lado da escolha, escrito para não virar surpresa: com o corte
// absoluto, uma linha afinada pelo painel VOLTA para o valor da migração se ela
// rodar de novo. É o preço do absoluto, e é o barato dos dois — visível na hora e
// desfazível pelo painel, contra um corte composto que ninguém enxerga.
func TestReaplicar0098DesfazAAfinacaoDoPainel(t *testing.T) {
	s, ctx := freshStore(t)

	if _, err := s.pool.Exec(ctx,
		`UPDATE quest_reward SET coin = 450 WHERE tier = 0`); err != nil {
		t.Fatalf("simulando a afinação do painel: %v", err)
	}

	b, err := migrations.FS.ReadFile("0098_quest_mortal_restos_e_ouro.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range comandos(string(b)) {
		if _, err := s.pool.Exec(ctx, c); err != nil {
			t.Fatalf("reaplicando: %v", err)
		}
	}

	var coin int64
	if err := s.pool.QueryRow(ctx, `SELECT coin FROM quest_reward WHERE tier = 0`).Scan(&coin); err != nil {
		t.Fatal(err)
	}
	if coin != 3000 {
		t.Errorf("coin do tier 0 = %d, e o absoluto devia repor 3000", coin)
	}
}

// retratoDa0098 lê tudo o que a migração escreve, com uma chave legível por
// linha.
func retratoDa0098(ctx context.Context, t *testing.T, s *Store) map[string]int64 {
	t.Helper()
	fora := map[string]int64{}

	rows, err := s.pool.Query(ctx, `
		SELECT mob, item, chance FROM drop_rule
		 WHERE item IN (419, 420)
		   AND mob IN ('Cav._Kaizen','Cav._Servo','Hidra_Dourada','Hidra_Imortal','Mestre_Elfo','Servo_Elfo')
		 ORDER BY mob, item`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var mob string
		var item int32
		var chance int64
		if err := rows.Scan(&mob, &item, &chance); err != nil {
			t.Fatal(err)
		}
		fora[mob+"/"+strconv.Itoa(int(item))] = chance
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	qr, err := s.pool.Query(ctx, `SELECT tier, coin, mortal_exp FROM quest_reward ORDER BY tier`)
	if err != nil {
		t.Fatal(err)
	}
	defer qr.Close()
	for qr.Next() {
		var tier int32
		var coin, exp int64
		if err := qr.Scan(&tier, &coin, &exp); err != nil {
			t.Fatal(err)
		}
		fora["tier"+strconv.Itoa(int(tier))+"/coin"] = coin
		fora["tier"+strconv.Itoa(int(tier))+"/exp"] = exp
	}
	return fora
}
