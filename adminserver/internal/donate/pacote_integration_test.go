//go:build integration

package donate

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestEnviarPacoteCreditaEEnfileiraJuntos: o Supremo pelo painel dá os mesmos
// 20.000 Rcoins e os mesmos brindes que a compra — lidos das tabelas da 0123 —,
// e a carteira mostra o crédito com o pacote no motivo.
func TestEnviarPacoteCreditaEEnfileiraJuntos(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	d := New(pool)
	conta := contaComCash(ctx, t, pool, "pacote_influencer", 100)

	env, err := d.EnviarPacote(ctx, 1, conta, "apoiador-supremo", "influencer Fulano")
	if err != nil {
		t.Fatalf("EnviarPacote: %v", err)
	}
	if env.Saldo != 20_100 {
		t.Errorf("saldo = %d, want 20100", env.Saldo)
	}
	// Seis brindes, todos cabendo numa linha cada: o maior é 64 baús, abaixo da
	// pilha de 120.
	if len(env.Entregas) != 6 {
		t.Fatalf("entregas = %d, want 6", len(env.Entregas))
	}

	rows, err := pool.Query(ctx, `
		SELECT payload, source FROM delivery_queue
		 WHERE account_id = $1 AND status = 'pending' ORDER BY id`, conta)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var achou64 bool
	for rows.Next() {
		var body []byte
		var origem string
		if err := rows.Scan(&body, &origem); err != nil {
			t.Fatal(err)
		}
		// "painel:" primeiro: é o que a API do site lê para mostrar "equipe".
		if origem != "painel:1:pacote:apoiador-supremo" {
			t.Errorf("source = %q", origem)
		}
		var p map[string]any
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatal(err)
		}
		if p["expires_at"] != float64(0) {
			t.Errorf("brinde com prazo absoluto, devia começar no primeiro uso: %s", body)
		}
		if p["item_index"] == float64(3305) && p["eff1"] == float64(61) && p["effv1"] == float64(64) {
			achou64 = true
		}
	}
	if !achou64 {
		t.Error("os 64 Baús do Apoiador não saíram numa pilha com EF_AMOUNT")
	}

	hist, err := d.Historico(ctx, conta, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) == 0 || hist[0].Tipo != TipoAjuste || hist[0].Creditos != 20_000 ||
		!strings.Contains(hist[0].Detalhe, "Apoiador Supremo") {
		t.Errorf("a carteira não mostra o pacote: %+v", hist)
	}
}

// TestEnviarPacoteRepetidoERecusado: o segundo clique no mesmo minuto não dá um
// segundo Supremo, e não credita nem enfileira nada.
func TestEnviarPacoteRepetidoERecusado(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	d := New(pool)
	conta := contaComCash(ctx, t, pool, "pacote_duplo", 0)

	if _, err := d.EnviarPacote(ctx, 1, conta, "apoiador-bronze", "x"); err != nil {
		t.Fatalf("primeiro envio: %v", err)
	}
	if _, err := d.EnviarPacote(ctx, 1, conta, "apoiador-bronze", "x"); !errors.Is(err, ErrPacoteRepetido) {
		t.Fatalf("segundo envio = %v, want ErrPacoteRepetido", err)
	}
	if saldo, _ := d.Saldo(ctx, conta); saldo != 575 {
		t.Errorf("saldo = %d, want 575 (um Bronze só)", saldo)
	}

	// Outro pacote para a mesma conta passa: a trava é do clique duplo, não da
	// conta.
	if _, err := d.EnviarPacote(ctx, 1, conta, "apoiador-iniciante", "x"); err != nil {
		t.Errorf("outro pacote foi recusado: %v", err)
	}
}

// TestEnviarPacoteRecusaEspelhoEDesconhecido: os espelhos "teste-*" são só staff
// e existem para testar pagamento; o painel não os envia.
func TestEnviarPacoteRecusaEspelhoEDesconhecido(t *testing.T) {
	ctx := context.Background()
	pool := poolDeTeste(t)
	d := New(pool)
	conta := contaComCash(ctx, t, pool, "pacote_espelho", 0)

	if _, err := d.EnviarPacote(ctx, 1, conta, "teste-apoiador-supremo", "x"); !errors.Is(err, ErrPacoteIndisponivel) {
		t.Errorf("espelho = %v, want ErrPacoteIndisponivel", err)
	}
	if _, err := d.EnviarPacote(ctx, 1, conta, "nao-existe", "x"); !errors.Is(err, ErrPacoteDesconhecido) {
		t.Errorf("desconhecido = %v, want ErrPacoteDesconhecido", err)
	}
	if _, err := d.EnviarPacote(ctx, 1, conta, "apoiador-supremo", "  "); !errors.Is(err, ErrMotivoVazio) {
		t.Errorf("sem motivo = %v, want ErrMotivoVazio", err)
	}
	if saldo, _ := d.Saldo(ctx, conta); saldo != 0 {
		t.Errorf("uma recusa creditou: saldo %d", saldo)
	}

	pacotes, err := d.Pacotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pacotes) != 9 {
		t.Errorf("Pacotes = %d, want os 9 pacotes à venda", len(pacotes))
	}
	for _, p := range pacotes {
		if strings.HasPrefix(p.ID, "teste-") {
			t.Errorf("a lista oferece o espelho %q", p.ID)
		}
	}
}
