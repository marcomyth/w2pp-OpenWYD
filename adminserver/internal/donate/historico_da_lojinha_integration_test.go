//go:build integration

// O histórico da carteira e a venda feita DENTRO do jogo.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./adminserver/internal/donate/
//
// Este arquivo existe por causa de um print: o painel mostrava saldo 1600 logo
// depois de um crédito de +3000, sem uma única linha explicando para onde
// tinham ido os 1400. O dinheiro estava certo — o diário também. Só a TELA não
// lia a ação que o jogo escreve quando um jogador compra do outro.
package donate

import (
	"context"
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

// contaComCash cria uma conta com a carteira já cheia.
func contaComCash(ctx context.Context, t *testing.T, pool *pgxpool.Pool, nome string, cash int32) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO account (name, pass_hash, donate_balance)
		VALUES ($1, 'x', $2) RETURNING id`, nome, cash).Scan(&id); err != nil {
		t.Fatalf("criando a conta %s: %v", nome, err)
	}
	return id
}

// TestAVendaNoJogoApareceNasDuasCarteiras.
//
// UMA linha do diário, DUAS telas. A transferência grava um registro só, com o
// pagador em `de` e o vendedor em `para`, e cada conta tem de encontrar esse
// mesmo registro pelo seu lado — com o sinal certo e com o SEU saldo, não o do
// outro jogador.
//
// O saldo é conferido justamente porque trocá-lo é o erro fácil: `saldo_de` e
// `saldo_para` estão os dois no mesmo JSON, e pegar o errado faria a coluna
// "saldo depois" mostrar o dinheiro de outra pessoa sem nada parecer quebrado.
func TestAVendaNoJogoApareceNasDuasCarteiras(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	s := store.New(pool)
	d := New(pool)

	comprador := contaComCash(ctx, t, pool, "lojinha_comprador", 1_000)
	vendedor := contaComCash(ctx, t, pool, "lojinha_vendedor", 0)

	const preco = int32(250)
	if _, _, err := s.TransferePlayerBalance(ctx, comprador, vendedor, store.MoedaCash, preco,
		"loja do servidor: venda"); err != nil {
		t.Fatalf("transferindo: %v", err)
	}

	// O lado de quem pagou.
	linhas, err := d.Historico(ctx, comprador, 50)
	if err != nil {
		t.Fatal(err)
	}
	ev := aLinhaDaLojinha(t, linhas, "comprador")
	if ev.Tipo != TipoLojinhaCompra {
		t.Errorf("tipo = %q, queria %q", ev.Tipo, TipoLojinhaCompra)
	}
	if ev.Creditos != -int64(preco) {
		t.Errorf("creditos = %d, queria -%d: para quem pagou a compra tem de ser negativa",
			ev.Creditos, preco)
	}
	if ev.Saldo == nil || *ev.Saldo != 750 {
		t.Errorf("saldo depois = %v, queria 750 (o saldo DELE, nao o do vendedor)", ev.Saldo)
	}

	// O lado de quem vendeu.
	linhas, err = d.Historico(ctx, vendedor, 50)
	if err != nil {
		t.Fatal(err)
	}
	ev = aLinhaDaLojinha(t, linhas, "vendedor")
	if ev.Tipo != TipoLojinhaVenda {
		t.Errorf("tipo = %q, queria %q", ev.Tipo, TipoLojinhaVenda)
	}
	if ev.Creditos != int64(preco) {
		t.Errorf("creditos = %d, queria %d: para quem vendeu a venda entra", ev.Creditos, preco)
	}
	if ev.Saldo == nil || *ev.Saldo != 250 {
		t.Errorf("saldo depois = %v, queria 250", ev.Saldo)
	}
}

// TestOHistoricoDeRcoinNaoMostraDinheiroDaOutraCarteira.
//
// A MESMA ação move as duas carteiras: `donate_balance` (Rcoin) e `rmt_balance`
// (dinheiro de verdade). Esta tela é a do Rcoin. Sem o filtro pela moeda, uma
// venda em RMT apareceria aqui como se o Rcoin tivesse saído — e a conta que o
// leitor faz de cabeça, somando as linhas, deixaria de bater com o saldo.
func TestOHistoricoDeRcoinNaoMostraDinheiroDaOutraCarteira(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	s := store.New(pool)
	d := New(pool)

	comprador := contaComCash(ctx, t, pool, "rmt_comprador", 0)
	vendedor := contaComCash(ctx, t, pool, "rmt_vendedor", 0)
	if _, err := pool.Exec(ctx, `UPDATE account SET rmt_balance = 900 WHERE id = $1`, comprador); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.TransferePlayerBalance(ctx, comprador, vendedor, store.MoedaRMT, 300,
		"loja do servidor: venda"); err != nil {
		t.Fatalf("transferindo em RMT: %v", err)
	}

	linhas, err := d.Historico(ctx, comprador, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range linhas {
		if ev.Tipo == TipoLojinhaCompra || ev.Tipo == TipoLojinhaVenda {
			t.Fatalf("uma venda em RMT entrou no historico de Rcoin: %+v", ev)
		}
	}
}

// aLinhaDaLojinha acha o único evento de lojinha da lista.
func aLinhaDaLojinha(t *testing.T, linhas []Evento, lado string) Evento {
	t.Helper()
	var achados []Evento
	for _, ev := range linhas {
		if ev.Tipo == TipoLojinhaCompra || ev.Tipo == TipoLojinhaVenda {
			achados = append(achados, ev)
		}
	}
	if len(achados) != 1 {
		t.Fatalf("%s: linhas de lojinha no historico = %d, queria 1; a venda feita no jogo "+
			"nao aparece na tela", lado, len(achados))
	}
	return achados[0]
}
