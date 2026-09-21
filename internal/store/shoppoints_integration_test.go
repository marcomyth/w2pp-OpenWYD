//go:build integration

// Testes da carteira de pontos de lojinha — o crédito da barraca aberta e o gasto
// da Loja de Honra. Precisam de banco de verdade e ficam fora do build padrão:
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// Este arquivo existe por causa de um bug que só o banco de verdade mostra. O
// gasto era o mesmo INSERT ... ON CONFLICT do crédito, com delta negativo, e a
// aposta era que o CHECK (balance >= 0) seria o piso. Não é: o PostgreSQL avalia o
// CHECK na tupla que vai INSERIR (balance = delta) antes de descobrir o conflito e
// virar para o UPDATE, então TODO gasto batia em check_violation, com saldo de mil
// ou de zero. Os testes de unidade passavam porque o dobro do banco não roda SQL.
package store

import (
	"context"
	"errors"
	"testing"
)

// contaComCarteira semeia uma conta e, se saldo >= 0, a carteira de pontos dela.
// saldo negativo quer dizer "conta sem linha na shop_points".
func contaComCarteira(t *testing.T, s *Store, nome string, saldo int32) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO account (name, pass_hash) VALUES ($1,'x') RETURNING id`, nome).Scan(&id); err != nil {
		t.Fatalf("semeando conta: %v", err)
	}
	if saldo >= 0 {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO shop_points (account_id, balance) VALUES ($1,$2)`, id, saldo); err != nil {
			t.Fatalf("semeando carteira: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DELETE FROM shop_points_audit WHERE account_id = $1`, id)
		_, _ = s.pool.Exec(ctx, `DELETE FROM shop_points WHERE account_id = $1`, id)
		_, _ = s.pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, id)
	})
	return id
}

// abreStore liga no banco de teste e aplica as migracoes, como os outros testes
// de integracao fazem.
func abreStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return New(pool)
}

func carteira(t *testing.T, s *Store, conta int64) int32 {
	t.Helper()
	var v int32
	if err := s.pool.QueryRow(context.Background(),
		`SELECT COALESCE((SELECT balance FROM shop_points WHERE account_id=$1),0)`,
		conta).Scan(&v); err != nil {
		t.Fatalf("lendo carteira: %v", err)
	}
	return v
}

func linhasDeExtrato(t *testing.T, s *Store, conta int64) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM shop_points_audit WHERE account_id=$1`, conta).Scan(&n); err != nil {
		t.Fatalf("contando extrato: %v", err)
	}
	return n
}

// TestGastaPontosDeLojinha é o caso que estava quebrado: com saldo de sobra, o
// gasto tem de passar.
func TestGastaPontosDeLojinha(t *testing.T) {
	s := abreStore(t)
	conta := contaComCarteira(t, s, "honra_gasto", 1000)

	saldo, err := s.AddShopPoints(context.Background(), conta, -30, "Honrada", "loja de honra")
	if err != nil {
		t.Fatalf("gasto de 30 com saldo de 1000 recusado: %v", err)
	}
	if saldo != 970 {
		t.Errorf("saldo devolvido = %d, esperado 970", saldo)
	}
	if got := carteira(t, s, conta); got != 970 {
		t.Errorf("carteira = %d, esperado 970", got)
	}
	if n := linhasDeExtrato(t, s, conta); n != 1 {
		t.Errorf("%d linhas de extrato, esperado 1", n)
	}
}

// TestGastoExatoZeraACarteira: gastar o saldo inteiro vale — o piso é zero, e zero
// é permitido.
func TestGastoExatoZeraACarteira(t *testing.T) {
	s := abreStore(t)
	conta := contaComCarteira(t, s, "honra_exato", 40)

	saldo, err := s.AddShopPoints(context.Background(), conta, -40, "Honrada", "loja de honra")
	if err != nil {
		t.Fatalf("gasto do saldo inteiro recusado: %v", err)
	}
	if saldo != 0 {
		t.Errorf("saldo devolvido = %d, esperado 0", saldo)
	}
}

// TestGastoMaiorQueOSaldoNaoMoveNada: a recusa é a sentinela, e nem a carteira nem
// o extrato se movem. Um extrato de um gasto que não aconteceu seria pior que a
// recusa.
func TestGastoMaiorQueOSaldoNaoMoveNada(t *testing.T) {
	s := abreStore(t)
	conta := contaComCarteira(t, s, "honra_sem", 10)

	_, err := s.AddShopPoints(context.Background(), conta, -40, "Honrada", "loja de honra")
	if !errors.Is(err, ErrPontosInsuficientes) {
		t.Fatalf("erro = %v, esperado ErrPontosInsuficientes", err)
	}
	if got := carteira(t, s, conta); got != 10 {
		t.Errorf("carteira = %d, esperado 10 (intacta)", got)
	}
	if n := linhasDeExtrato(t, s, conta); n != 0 {
		t.Errorf("%d linhas de extrato de um gasto recusado, esperado 0", n)
	}
}

// TestGastoSemCarteiraNenhuma: conta que nunca abriu lojinha não tem linha, e isso
// é "não tem pontos" — não é erro de banco, e não cria carteira negativa.
func TestGastoSemCarteiraNenhuma(t *testing.T) {
	s := abreStore(t)
	conta := contaComCarteira(t, s, "honra_nova", -1)

	_, err := s.AddShopPoints(context.Background(), conta, -5, "Honrada", "loja de honra")
	if !errors.Is(err, ErrPontosInsuficientes) {
		t.Fatalf("erro = %v, esperado ErrPontosInsuficientes", err)
	}
	if got := carteira(t, s, conta); got != 0 {
		t.Errorf("carteira = %d, esperado 0", got)
	}
}

// TestCreditoCriaACarteira: o crédito da barraca continua criando a linha na
// primeira vez, que é o caminho que já existia.
func TestCreditoCriaACarteira(t *testing.T) {
	s := abreStore(t)
	conta := contaComCarteira(t, s, "honra_credito", -1)

	saldo, err := s.AddShopPoints(context.Background(), conta, 3, "Vendedora", "lojinha")
	if err != nil {
		t.Fatalf("crédito recusado: %v", err)
	}
	if saldo != 3 {
		t.Errorf("saldo devolvido = %d, esperado 3", saldo)
	}
	saldo, err = s.AddShopPoints(context.Background(), conta, 7, "Vendedora", "lojinha")
	if err != nil {
		t.Fatalf("segundo crédito recusado: %v", err)
	}
	if saldo != 10 {
		t.Errorf("saldo devolvido = %d, esperado 10", saldo)
	}
}
