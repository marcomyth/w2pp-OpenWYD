//go:build integration

// Testes da transferência de carteira entre contas — o pagamento de uma venda na
// Loja do Servidor. Precisam de banco de verdade e ficam fora do build padrão:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
	"testing"
)

// duasContas semeia um par de contas com a carteira que o teste pedir e devolve
// os ids, já limpando no fim.
func duasContas(t *testing.T, s *Store, coluna string, saldoA, saldoB int32) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	var a, b int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, `+coluna+`) VALUES ('loja_vendedor','x',$1) RETURNING id`,
		saldoA).Scan(&a); err != nil {
		t.Fatalf("semeando vendedor: %v", err)
	}
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash, `+coluna+`) VALUES ('loja_comprador','x',$1) RETURNING id`,
		saldoB).Scan(&b); err != nil {
		t.Fatalf("semeando comprador: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DELETE FROM donate_shop_audit WHERE account_id IN ($1,$2)`, a, b)
		_, _ = s.pool.Exec(ctx, `DELETE FROM account WHERE id IN ($1,$2)`, a, b)
	})
	return a, b
}

func saldoDe(t *testing.T, s *Store, coluna string, conta int64) int32 {
	t.Helper()
	var v int32
	if err := s.pool.QueryRow(context.Background(),
		`SELECT `+coluna+` FROM account WHERE id = $1`, conta).Scan(&v); err != nil {
		t.Fatalf("lendo saldo: %v", err)
	}
	return v
}

// O valor sai de uma conta e entra na outra, e os saldos devolvidos são os que
// ficaram gravados.
func TestTransfereCashEntreContas(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	vendedor, comprador := duasContas(t, s, "donate_balance", 0, 500)

	de, para, err := s.TransferePlayerBalance(ctx, comprador, vendedor, MoedaCash, 300, "venda")
	if err != nil {
		t.Fatalf("transferencia: %v", err)
	}
	if de != 200 || para != 300 {
		t.Errorf("saldos devolvidos = %d e %d; queria 200 e 300", de, para)
	}
	if got := saldoDe(t, s, "donate_balance", comprador); got != 200 {
		t.Errorf("comprador ficou com %d; queria 200", got)
	}
	if got := saldoDe(t, s, "donate_balance", vendedor); got != 300 {
		t.Errorf("vendedor ficou com %d; queria 300", got)
	}
}

// RMT usa a carteira da migração 0077, separada do Cash.
func TestTransfereRMTNaoMexeNoCash(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	vendedor, comprador := duasContas(t, s, "rmt_balance", 0, 50)

	if _, _, err := s.TransferePlayerBalance(ctx, comprador, vendedor, MoedaRMT, 20, "venda"); err != nil {
		t.Fatalf("transferencia: %v", err)
	}
	if got := saldoDe(t, s, "rmt_balance", vendedor); got != 20 {
		t.Errorf("rmt do vendedor = %d; queria 20", got)
	}
	if got := saldoDe(t, s, "donate_balance", comprador); got != 0 {
		t.Errorf("cash do comprador mexeu: %d; queria 0", got)
	}
}

// Saldo curto: nada se move, nem um lado nem o outro.
func TestTransfereSemSaldoNaoMoveNada(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	vendedor, comprador := duasContas(t, s, "donate_balance", 10, 5)

	_, _, err := s.TransferePlayerBalance(ctx, comprador, vendedor, MoedaCash, 100, "venda")
	if !errors.Is(err, ErrSaldoInsuficiente) {
		t.Fatalf("erro = %v; queria saldo insuficiente", err)
	}
	if got := saldoDe(t, s, "donate_balance", comprador); got != 5 {
		t.Errorf("comprador = %d; queria continuar com 5", got)
	}
	if got := saldoDe(t, s, "donate_balance", vendedor); got != 10 {
		t.Errorf("vendedor = %d; queria continuar com 10", got)
	}
}

// Conta que não existe é recusa, não estrago.
func TestTransfereContaInexistente(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	s := New(pool)
	_, comprador := duasContas(t, s, "donate_balance", 0, 100)

	_, _, err := s.TransferePlayerBalance(ctx, comprador, 999_999_999, MoedaCash, 10, "venda")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("erro = %v; queria conta nao encontrada", err)
	}
	if got := saldoDe(t, s, "donate_balance", comprador); got != 100 {
		t.Errorf("comprador = %d; queria continuar com 100", got)
	}
}
