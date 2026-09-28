//go:build integration

// O usuário do painel, conferido contra o banco de verdade.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./webserver/internal/painelator/
package painelator

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

func poolDeTeste(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("W2PP_TEST_DSN")
	if dsn == "" {
		t.Skip("W2PP_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := store.Pool(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func criaUsuario(ctx context.Context, t *testing.T, pool *pgxpool.Pool, login, papel string, ativo bool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO painel_usuario (login, senha_hash, papel, ativo)
		VALUES ($1, 'x', $2, $3) RETURNING id`, login, papel, ativo).Scan(&id); err != nil {
		t.Fatalf("criando %q: %v", login, err)
	}
	return id
}

// TestQuemPassaEQuemNaoPassa.
//
// O caso que vale dinheiro é o DESATIVADO. Desativar alguém no painel tem de valer no
// próximo clique dela — não no próximo reinício —, e é por isso que o Confere vai ao
// banco a cada chamada em vez de guardar em memória.
func TestQuemPassaEQuemNaoPassa(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	l := Novo(pool)

	admin := criaUsuario(ctx, t, pool, "pa_admin", "admin", true)
	mod := criaUsuario(ctx, t, pool, "pa_mod", "moderator", true)
	demitido := criaUsuario(ctx, t, pool, "pa_demitido", "admin", false)

	if a, err := l.Confere(ctx, admin); err != nil || a.Papel != "admin" || a.Login != "pa_admin" {
		t.Errorf("admin ativo: %+v, %v", a, err)
	}
	if a, err := l.Confere(ctx, mod); err != nil || a.Papel != "moderator" {
		t.Errorf("moderador ativo: %+v, %v", a, err)
	}
	if _, err := l.Confere(ctx, demitido); !errors.Is(err, ErrNaoServe) {
		t.Errorf("DESATIVADO passou: %v. Quem foi desligado continuaria editando o jogo", err)
	}
	if _, err := l.Confere(ctx, 999_999); !errors.Is(err, ErrNaoServe) {
		t.Errorf("um id inexistente passou: %v", err)
	}
}

// TestDesativarValeNoProximoClique: o estado é lido AGORA, e não no boot.
func TestDesativarValeNoProximoClique(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	l := Novo(pool)

	id := criaUsuario(ctx, t, pool, "pa_agora", "admin", true)
	if _, err := l.Confere(ctx, id); err != nil {
		t.Fatalf("ativo foi recusado: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE painel_usuario SET ativo = FALSE WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Confere(ctx, id); !errors.Is(err, ErrNaoServe) {
		t.Errorf("continuou passando depois de desativado: %v", err)
	}
}
