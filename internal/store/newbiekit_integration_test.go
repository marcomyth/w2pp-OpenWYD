//go:build integration

// Integration test for the /novato kit gate (0062_newbie_kit). Needs a real
// database — the whole point is that POSTGRES settles the race, so a fake would
// test nothing. Run with:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"sync"
	"testing"
)

// TestClaimNewbieKitSoUmaVez: a primeira chamada concede, a segunda não, e
// dezesseis simultâneas concedem exatamente uma.
//
// A corrida é o teste. Duas janelas do jogo na mesma conta, em canais
// diferentes, podem pedir o kit no mesmo instante; se as duas passarem, a conta
// recebe dois kits e a trava não vale nada. Um SELECT seguido de INSERT passaria
// no caso sequencial e falharia aqui.
func TestClaimNewbieKitSoUmaVez(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)

	var conta int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash) VALUES ('novato_kit_test','x') RETURNING id`).Scan(&conta); err != nil {
		t.Fatalf("criar conta: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, conta) })

	if visto, err := s.NewbieKitClaimed(ctx, conta); err != nil || visto {
		t.Fatalf("conta nova já constava com o kit: visto=%v err=%v", visto, err)
	}
	granted, err := s.ClaimNewbieKit(ctx, conta, "Novato")
	if err != nil || !granted {
		t.Fatalf("primeira retirada: granted=%v err=%v, queria true/nil", granted, err)
	}
	granted, err = s.ClaimNewbieKit(ctx, conta, "Novato")
	if err != nil || granted {
		t.Fatalf("segunda retirada: granted=%v err=%v, queria false/nil", granted, err)
	}
	if visto, err := s.NewbieKitClaimed(ctx, conta); err != nil || !visto {
		t.Fatalf("NewbieKitClaimed depois da retirada: visto=%v err=%v", visto, err)
	}

	// A corrida propriamente dita, numa conta limpa.
	var conta2 int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash) VALUES ('novato_kit_corrida','x') RETURNING id`).Scan(&conta2); err != nil {
		t.Fatalf("criar conta da corrida: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, conta2) })

	const tentativas = 16
	var wg sync.WaitGroup
	res := make([]bool, tentativas)
	errs := make([]error, tentativas)
	wg.Add(tentativas)
	for i := 0; i < tentativas; i++ {
		go func(i int) {
			defer wg.Done()
			res[i], errs[i] = s.ClaimNewbieKit(ctx, conta2, "Corrida")
		}(i)
	}
	wg.Wait()

	var concedidos int
	for i := range res {
		if errs[i] != nil {
			t.Fatalf("tentativa %d falhou: %v", i, errs[i])
		}
		if res[i] {
			concedidos++
		}
	}
	if concedidos != 1 {
		t.Fatalf("%d de %d tentativas simultâneas concederam o kit, queria exatamente 1", concedidos, tentativas)
	}
}
