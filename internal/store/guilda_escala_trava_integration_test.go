//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// TestAEscalaDeCidadeTrancaAGuilda prova que a trava existe.
//
// POR QUE ELA IMPORTA: a escalação é "apaga tudo desta cidade e escreve a lista nova",
// e desde o PR 225 ela tira a pessoa das OUTRAS cidades também. Dois chefes da mesma
// guilda escalando cidades DIFERENTES ao mesmo tempo se atropelam: a segunda gravação
// apaga o que a primeira acabou de escrever, as duas respondem "gravado", e metade do
// trabalho some sem erro nenhum.
//
// O TESTE OLHA O PLANO DO POSTGRES, e não tenta reproduzir a corrida. Reproduzir uma
// corrida de duas transações num teste é caro e intermitente — e um teste intermitente
// numa trava de corrida é pior que nenhum, porque as pessoas passam a ignorá-lo. O que
// se quer garantir é uma coisa objetiva: que a transação TRANCA a linha da guilda antes
// de mexer. Isso o catálogo responde.
func TestAEscalaDeCidadeTrancaAGuilda(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Uma guilda de verdade, senão o FOR UPDATE não acha linha e o teste passaria sem
	// provar nada.
	var guildID int32
	if err := pool.QueryRow(ctx, `
		INSERT INTO guild (id, name) VALUES (4242, 'trava-de-teste')
		ON CONFLICT (id) DO UPDATE SET name = 'trava-de-teste'
		RETURNING id`).Scan(&guildID); err != nil {
		t.Skipf("nao consegui criar a guilda de teste (a tabela pode ter outras colunas obrigatorias): %v", err)
	}

	s := New(pool)
	if err := s.SetGuildSquad(ctx, uint16(guildID), 0, []string{"Hero"}); err != nil {
		t.Fatalf("SetGuildSquad: %v", err)
	}

	// A PROVA: com a linha da guilda trancada por FORA, a escalação tem de ESPERAR.
	// Se ela não trancasse, passaria direto.
	outra, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outra.Rollback(ctx) }()
	if _, err := outra.Exec(ctx, `SELECT id FROM guild WHERE id = $1 FOR UPDATE`, guildID); err != nil {
		t.Fatalf("trancando por fora: %v", err)
	}

	pronto := make(chan error, 1)
	go func() { pronto <- s.SetGuildSquad(ctx, uint16(guildID), 1, []string{"HeroB"}) }()

	// A ESPERA TEM DE SER DE VERDADE, e não um select com default.
	//
	// Com default, o teste passaria mesmo SEM a trava: a goroutine acabou de nascer e
	// ainda não rodou, então "não terminou" não prova nada. Meio segundo é muito mais do
	// que a gravação leva sem contenção, e muito menos do que ela levaria esperando.
	select {
	case err := <-pronto:
		t.Fatalf("a escalacao terminou em menos de 500ms com a guilda TRANCADA por fora "+
			"(err=%v): sem o FOR UPDATE, duas gravacoes ao mesmo tempo se atropelam e "+
			"metade some sem erro nenhum", err)
	case <-time.After(500 * time.Millisecond):
		// Ainda esperando, que é o certo.
	}
	_ = outra.Rollback(ctx)
	if err := <-pronto; err != nil {
		t.Errorf("depois de soltar a trava, a escalacao falhou: %v", err)
	}
}
