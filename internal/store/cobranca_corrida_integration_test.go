//go:build integration

// A corrida entre ABRIR A COBRANÇA e FECHAR A BARRACA, que é a única corrida
// deste sistema que custa dinheiro de verdade.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
	"time"
)

// A CORRIDA: a cobrança é commitada enquanto o encerramento espera o lock.
//
// Em READ COMMITTED cada COMANDO pega um snapshot. Um comando só que trava a
// linha do anúncio e, na mesma passada, pergunta por EXISTS se há cobrança,
// responde a segunda pergunta com o snapshot VELHO: o Postgres reavalia a linha
// travada contra a versão nova (EvaluatePlanQual), mas não reavalia o que a
// subconsulta leu em OUTRA tabela.
//
// O estrago: o anúncio vira CANCELADO com uma cobrança aberta pendurada nele. O
// Pix do comprador cai depois e a confirmação não encontra anúncio ativo — vira
// PAGA_SEM_ITEM, que é dívida com uma pessoa que pagou direito.
//
// O teste força a ordem exata: a transação da cobrança trava o anúncio primeiro,
// o encerramento tropeça no lock, e só então a cobrança commita.
func TestEncerrarNaoCancelaAnuncioQueGanhouCobrancaNoMeio(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_corrida")
	comprador := contaPix(ctx, t, s, "comprador_corrida")
	anuncio := anuncioAtivoSimples(ctx, t, s, vendedor, 0)

	// Transação A: a cobrança nascendo. Trava o anúncio e insere, sem commitar.
	txA, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(ctx) }()
	var st int16
	if err := txA.QueryRow(ctx,
		`SELECT status FROM rmt_anuncio WHERE id = $1 FOR UPDATE`, anuncio).Scan(&st); err != nil {
		t.Fatalf("travando o anuncio: %v", err)
	}
	if _, err := txA.Exec(ctx, `
		INSERT INTO rmt_cobranca (anuncio_id, comprador_conta, referencia_externa,
			valor_centavos, metodo, status, expira_em)
		VALUES ($1, $2, 'ref-corrida', 5000, 1, 1, now() + interval '5 minutes')`,
		anuncio, comprador); err != nil {
		t.Fatalf("inserindo a cobranca: %v", err)
	}

	// Transação B: a barraca descendo. Vai bloquear no lock que A segura.
	type resultado struct {
		fim []AnuncioEncerrado
		err error
	}
	pronto := make(chan resultado, 1)
	go func() {
		fim, err := s.EncerrarAnunciosRMT(context.Background(), []int64{anuncio})
		pronto <- resultado{fim, err}
	}()

	// Dá tempo de B chegar no lock antes de A soltar. Sem isto o teste às vezes
	// roda na ordem fácil, que não é a que interessa.
	esperaBloquear(ctx, t, s, anuncio)
	if err := txA.Commit(ctx); err != nil {
		t.Fatalf("commit da cobranca: %v", err)
	}

	var r resultado
	select {
	case r = <-pronto:
	case <-time.After(10 * time.Second):
		t.Fatal("o encerramento nao voltou")
	}
	if r.err != nil {
		t.Fatalf("encerrando: %v", r.err)
	}

	if len(r.fim) != 1 || !r.fim[0].CobrancaAberta {
		t.Errorf("o encerramento disse %+v; a cobranca ja estava commitada quando ele leu", r.fim)
	}
	status, caiu := statusDoAnuncio(ctx, t, s, anuncio)
	if status != anuncioAtivo {
		t.Errorf("o anuncio virou %d com uma cobranca aberta pendurada; "+
			"o Pix desse comprador cairia em PAGA_SEM_ITEM", status)
	}
	if !caiu {
		t.Error("o anuncio ficou ativo e ninguem registrou que a barraca caiu")
	}
}

// A ORDEM CONTRÁRIA: o encerramento chega primeiro e a cobrança tenta nascer
// depois. Aqui quem tem de recusar é a abertura.
func TestAbrirCobrancaRecusaAnuncioQueAcabouDeSerCancelado(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_corrida2")
	comprador := contaPix(ctx, t, s, "comprador_corrida2")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)

	if _, err := s.EncerrarAnunciosRMT(ctx, []int64{anuncio}); err != nil {
		t.Fatalf("encerrando: %v", err)
	}

	res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, "ref-tarde", 0)
	if err != nil {
		t.Fatalf("abrindo: %v", err)
	}

	if res != AnuncioNaoDisponivel {
		t.Errorf("resultado = %d, quero AnuncioNaoDisponivel(%d): a barraca ja tinha descido",
			res, AnuncioNaoDisponivel)
	}
}

// esperaBloquear espera até que alguém esteja de fato esperando o lock da linha
// do anúncio. Ler pg_locks é o único jeito honesto: dormir um tempo fixo faria o
// teste passar por sorte numa máquina rápida.
func esperaBloquear(ctx context.Context, t *testing.T, s *Store, anuncio int64) {
	t.Helper()
	for i := 0; i < 200; i++ {
		var esperando int
		if err := s.pool.QueryRow(ctx, `
			SELECT count(*) FROM pg_stat_activity
			 WHERE wait_event_type = 'Lock' AND query LIKE '%rmt_anuncio%'`).Scan(&esperando); err != nil {
			t.Fatalf("lendo pg_stat_activity: %v", err)
		}
		if esperando > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("ninguem ficou esperando o lock; o teste nao forcou a ordem que queria")
	_ = anuncio
}
