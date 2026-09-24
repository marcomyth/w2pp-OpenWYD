//go:build integration

// O repasse ao vendedor: a dívida nascendo com a venda, e os caminhos em que ela NÃO
// pode nascer.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
)

// repasseDe lê a linha de repasse de uma cobrança, se existir.
func repasseDe(ctx context.Context, t *testing.T, s *Store, cobranca int64) (int16, int64, bool) {
	t.Helper()
	var status int16
	var valor int64
	err := s.pool.QueryRow(ctx, `
		SELECT status, valor_centavos FROM rmt_repasse WHERE cobranca_id = $1`, cobranca).
		Scan(&status, &valor)
	if err != nil {
		return 0, 0, false
	}
	return status, valor, true
}

// idDoRepasse acha a linha de repasse de uma cobrança, falhando o teste se não houver.
func idDoRepasse(ctx context.Context, t *testing.T, s *Store, cobranca int64) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM rmt_repasse WHERE cobranca_id = $1`, cobranca).Scan(&id); err != nil {
		t.Fatalf("lendo o repasse da cobranca %d: %v", cobranca, err)
	}
	return id
}

// A DÍVIDA NASCE COM A VENDA, pendente e com o valor da venda.
//
// Antes deste trabalho ela não nascia em lugar nenhum: o comprador recebia o item, o
// dinheiro entrava, e nada dizia a quem ele pertencia.
func TestVendaConcluidaAbreORepasse(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-ok")

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %v, quero confirmada", res)
	}

	status, valor, achou := repasseDe(ctx, t, s, venda.CobrancaID)
	if !achou {
		t.Fatal("a venda concluiu e nao abriu repasse: o vendedor ficaria invisivel")
	}
	if status != repassePendente {
		t.Errorf("status = %d, quero pendente(%d)", status, repassePendente)
	}
	if valor != precoEmCentavos {
		t.Errorf("valor = %d, quero %d", valor, precoEmCentavos)
	}
}

// A CONFIRMAÇÃO REPETIDA NÃO ABRE UMA SEGUNDA DÍVIDA. A processadora repete aviso por
// desenho, e duas linhas pela mesma venda viram dois pagamentos — que não se desfazem.
func TestConfirmacaoRepetidaNaoDuplicaORepasse(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-repetido")

	for i := 0; i < 3; i++ {
		if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos); err != nil {
			t.Fatalf("confirmacao %d: %v", i, err)
		}
	}

	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM rmt_repasse`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d linhas de repasse depois de tres confirmacoes, quero 1", n)
	}
}

// PAGA_SEM_ITEM NÃO ABRE REPASSE, e este é o caso que mais importa dos dois.
//
// O dinheiro entrou e o comprador não recebeu nada: ele vai VOLTAR. Uma linha de dívida
// aqui apareceria na fila, alguém tentaria pagar o vendedor, e o mesmo dinheiro sairia
// duas vezes — uma para o vendedor e outra de volta para o comprador.
func TestPagaSemItemNaoAbreRepasse(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-sem-item")

	// O anúncio é cancelado e a marca sai: não há item para entregar.
	if _, err := s.pool.Exec(ctx,
		`UPDATE item SET rmt_anuncio = 0 WHERE account_id = $1 AND slot = 3`, v.vendedor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, v.anuncio, anuncioCancelado); err != nil {
		t.Fatal(err)
	}

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if res != CobrancaPagaSemItem {
		t.Fatalf("resultado = %v, quero paga-sem-item", res)
	}
	if _, _, achou := repasseDe(ctx, t, s, venda.CobrancaID); achou {
		t.Error("abriu repasse numa venda sem item: o dinheiro sairia duas vezes")
	}
}

// VALOR DIVERGENTE TAMBÉM NÃO ABRE REPASSE. A cobrança fica aberta esperando uma
// pessoa, e o destino do dinheiro ainda não está decidido — pode voltar.
func TestValorDivergenteNaoAbreRepasse(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-divergente")

	res, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos-1)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if res != CobrancaValorDivergente {
		t.Fatalf("resultado = %v, quero valor divergente", res)
	}

	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM rmt_repasse`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d linha(s) de repasse com valor divergente, quero 0", n)
	}
}

// A FILA DE PAGAR TRAZ A CHAVE E O DOCUMENTO INTEIRO, que é o que a ponte exige — e é
// a única consulta do código que lê o documento sem máscara.
//
// E ela EXCLUI quem não tem chave: não há para onde mandar, e uma linha sem destino
// faria quem paga tropeçar nela uma por uma.
func TestRepassesAPagarTrazemODestinoEPulamQuemNaoTem(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-fila")

	// Sem chave ainda: a venda conclui e a dívida nasce, mas ela não entra na fila.
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos); err != nil {
		t.Fatal(err)
	}
	fila, err := s.RepassesAPagar(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Fatalf("a fila tem %d linha(s) para quem nao cadastrou chave", len(fila))
	}

	// Agora o vendedor cadastra, e a dívida dele aparece com o destino.
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatal(err)
	}
	fila, err = s.RepassesAPagar(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 {
		t.Fatalf("a fila tem %d linha(s), quero 1", len(fila))
	}
	r := fila[0]
	if r.ChavePix != "vendedor@exemplo.com" || r.TipoChave != ChavePixEmail {
		t.Errorf("destino = %q tipo %d", r.ChavePix, r.TipoChave)
	}
	if r.Documento != "11144477735" {
		t.Errorf("documento = %q, a ponte exige os 11 digitos", r.Documento)
	}
	if r.Referencia != v.ref {
		t.Errorf("referencia = %q, quero a da cobranca %q", r.Referencia, v.ref)
	}
	if r.ValorCentavos != precoEmCentavos {
		t.Errorf("valor = %d", r.ValorCentavos)
	}
}

// AS TRANSIÇÕES SÓ SAEM DO ESTADO CERTO, e zero linhas afetadas é ERRO e não silêncio.
//
// É o que impede o pior caso: dois trabalhadores pegando a mesma linha, os dois
// mandando, e o vendedor recebendo duas vezes. Quem perder a corrida vê que perdeu.
func TestTransicoesDoRepasseSoSaemDoEstadoCerto(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-transicao")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos)
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	// Pendente -> enviado funciona.
	if err := s.MarcarRepasseEnviado(ctx, id, "saque-1", precoEmCentavos); err != nil {
		t.Fatalf("enviando: %v", err)
	}
	// E a SEGUNDA tentativa falha: a linha já saiu de pendente. Se ela passasse, o
	// segundo trabalhador mandaria o mesmo pagamento de novo.
	if err := s.MarcarRepasseEnviado(ctx, id, "saque-2", precoEmCentavos); err == nil {
		t.Error("enviou duas vezes a mesma linha; o vendedor receberia dobrado")
	}
	// Recusar depois de enviado também não pega.
	if err := s.MarcarRepasseRecusado(ctx, id, nil, "", "tarde demais"); err == nil {
		t.Error("recusou uma linha ja enviada")
	}

	// O aviso do saque fecha, e só de ENVIADO.
	if err := s.MarcarRepassePago(ctx, "saque-1", precoEmCentavos-100); err != nil {
		t.Fatalf("pagando: %v", err)
	}
	if err := s.MarcarRepassePago(ctx, "saque-1", precoEmCentavos-100); err == nil {
		t.Error("pagou duas vezes o mesmo saque")
	}
	// Um aviso de saque que ninguém conhece não fecha nada.
	if err := s.MarcarRepassePago(ctx, "saque-de-outro", 100); err == nil {
		t.Error("um saque desconhecido fechou uma linha")
	}

	var chegou int64
	if err := s.pool.QueryRow(ctx,
		`SELECT chegou_centavos FROM rmt_repasse WHERE id = $1`, id).Scan(&chegou); err != nil {
		t.Fatal(err)
	}
	if chegou != precoEmCentavos-100 {
		t.Errorf("chegou = %d; a taxa do saque so se sabe por esta diferenca", chegou)
	}
}

// O INCERTO SAI DE PENDENTE E NÃO VOLTA, que é a razão de ele existir: reenviar um
// pagamento que PODE ter saído é a única coisa que não se desfaz.
func TestIncertoSaiDaFilaDePagarEVaiParaADeGente(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-incerto")
	if err := s.SalvarChavePix(ctx, v.vendedor, "v@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatal(err)
	}
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos)
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	if err := s.MarcarRepasseIncerto(ctx, id, "a chamada saiu e a resposta nao voltou"); err != nil {
		t.Fatal(err)
	}

	// Saiu da fila de pagar: nenhuma varredura vai mandar de novo.
	fila, err := s.RepassesAPagar(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Error("o incerto continua na fila de pagar: seria reenviado e pagaria duas vezes")
	}

	// E entrou na fila de gente, que é onde ele se resolve.
	gente, err := s.RepassesQuePrecisamDeGente(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gente) != 1 || gente[0].Estado != RepasseIncerto {
		t.Fatalf("fila de gente = %+v", gente)
	}
}

// A RECUSA DA PONTE NÃO É CULPA DO VENDEDOR, e o nulo no http é o que diz isso.
//
// Enquanto a trava do saque estiver desligada na ponte, TODO repasse volta assim. Se
// quem olhar a fila tratar isso como "a chave está errada", vai mandar o vendedor mexer
// no que estava certo.
func TestRecusaDaPonteSeDistingueDaRecusaDaProcessadora(t *testing.T) {
	s, ctx := freshStore(t)

	fazRepasse := func(sufixo string) int64 {
		v := montaVenda(ctx, t, s, sufixo)
		_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos)
		if err != nil {
			t.Fatal(err)
		}
		return idDoRepasse(ctx, t, s, venda.CobrancaID)
	}

	daPonte := fazRepasse("recusa-ponte")
	if err := s.MarcarRepasseRecusado(ctx, daPonte, nil, "", "saque desligado na ponte"); err != nil {
		t.Fatal(err)
	}
	http := int32(422)
	daSyncpay := fazRepasse("recusa-syncpay")
	if err := s.MarcarRepasseRecusado(ctx, daSyncpay, &http, "PIX_KEY_NOT_FOUND", "chave nao existe"); err != nil {
		t.Fatal(err)
	}

	gente, err := s.RepassesQuePrecisamDeGente(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gente) != 2 {
		t.Fatalf("fila = %d linhas, quero 2", len(gente))
	}
	var achouPonte, achouSyncpay bool
	for _, r := range gente {
		switch r.ID {
		case daPonte:
			achouPonte = true
			if r.RecusaHTTP != nil {
				t.Errorf("a recusa da ponte veio com http %d; nulo e o que diz que nao e culpa do vendedor", *r.RecusaHTTP)
			}
		case daSyncpay:
			achouSyncpay = true
			if r.RecusaHTTP == nil || *r.RecusaHTTP != 422 {
				t.Errorf("a recusa da processadora perdeu o http: %v", r.RecusaHTTP)
			}
			if r.RecusaCodigo != "PIX_KEY_NOT_FOUND" {
				t.Errorf("o codigo da processadora nao passou intacto: %q", r.RecusaCodigo)
			}
		}
	}
	if !achouPonte || !achouSyncpay {
		t.Error("a fila nao trouxe as duas recusas")
	}
}

// O INCERTO É TERMINAL PARA A MÁQUINA, e este teste é o que amarra isso.
//
// NADA o reenvia: nem a varredura que paga, nem um caminho de "tentar de novo". A única
// saída é uma pessoa que foi olhar o painel da processadora e disse o que viu — porque
// reenviar um pagamento que PODE ter saído é a única coisa deste sistema que não se
// desfaz, e não existe consulta de saque para desempatar.
func TestIncertoSoSaiPelaMaoDeUmaPessoa(t *testing.T) {
	s, ctx := freshStore(t)

	fazIncerto := func(sufixo string) int64 {
		v := montaVenda(ctx, t, s, sufixo)
		if err := s.SalvarChavePix(ctx, v.vendedor, "v@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
			t.Fatal(err)
		}
		_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos)
		if err != nil {
			t.Fatal(err)
		}
		id := idDoRepasse(ctx, t, s, venda.CobrancaID)
		if err := s.MarcarRepasseIncerto(ctx, id, "a resposta nao voltou"); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// NENHUMA transição automática pega num incerto. Se alguma pegasse, a linha
	// voltaria para a fila de pagar e o vendedor receberia duas vezes.
	travado := fazIncerto("incerto-travado")
	if err := s.MarcarRepasseEnviado(ctx, travado, "saque-x", precoEmCentavos); err == nil {
		t.Error("um incerto foi reenviado; o vendedor poderia receber duas vezes")
	}
	if err := s.MarcarRepasseRecusado(ctx, travado, nil, "", "x"); err == nil {
		t.Error("um incerto virou recusado sem ninguem ter olhado")
	}
	if err := s.MarcarRepassePago(ctx, "saque-x", precoEmCentavos); err == nil {
		t.Error("um aviso de saque fechou um incerto que nao tem id de saque")
	}

	// A pessoa olhou e viu que PAGOU: a linha fecha, com o nome de quem disse.
	pago := fazIncerto("incerto-pago")
	if err := s.ResolverIncertoComoPago(ctx, pago, AtorDoRepasse{Nome: "hanna"}, precoEmCentavos-100); err != nil {
		t.Fatalf("resolvendo como pago: %v", err)
	}
	var status int16
	var por string
	if err := s.pool.QueryRow(ctx,
		`SELECT status, coalesce(resolvido_por, '') FROM rmt_repasse WHERE id = $1`, pago).
		Scan(&status, &por); err != nil {
		t.Fatal(err)
	}
	if status != repassePago || por != "hanna" {
		t.Errorf("status = %d resolvido_por = %q", status, por)
	}

	// A pessoa olhou e viu que NÃO pagou: a dívida volta para a fila, e só por aqui.
	naoPago := fazIncerto("incerto-nao-pago")
	if err := s.ResolverIncertoComoNaoPago(ctx, naoPago, AtorDoRepasse{Nome: "hanna"}, "nao saiu"); err != nil {
		t.Fatalf("resolvendo como nao pago: %v", err)
	}
	fila, err := s.RepassesAPagar(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var voltou bool
	for _, r := range fila {
		if r.ID == naoPago {
			voltou = true
		}
	}
	if !voltou {
		t.Error("a divida conferida como nao paga nao voltou para a fila de pagar")
	}
	// E a que foi conferida como PAGA não voltou junto.
	for _, r := range fila {
		if r.ID == pago {
			t.Error("a divida ja paga voltou para a fila: seria paga duas vezes")
		}
	}
}
