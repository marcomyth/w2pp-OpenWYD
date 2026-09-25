//go:build integration

// O repasse ao vendedor: a dívida nascendo com a venda, e os caminhos em que ela NÃO
// pode nascer.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"errors"
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

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
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
		if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
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

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
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

	res, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos-1, taxaZero())
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
// E ela EXCLUI quem não tem DOCUMENTO. O caso é o do vendedor cadastrado antes de o CPF
// ser obrigatório: ele tem chave, vende normalmente, e a ponte recusaria o repasse por
// falta de documento. Uma linha assim na fila de pagar faria quem paga tropeçar nela uma
// por uma.
//
// (Sem CHAVE não existe: o AbrirAnunciosRMT recusa anunciar sem ela.)
func TestRepassesAPagarTrazemODestinoEPulamQuemNaoTemDocumento(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-fila")

	// O estado do vendedor antigo: chave sim, documento não.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_recebedor SET documento = NULL WHERE account_id = $1`, v.vendedor); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	fila, err := s.RepassesAPagar(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Fatalf("a fila tem %d linha(s) sem documento; a ponte recusaria cada uma", len(fila))
	}

	// Preenchendo o documento, com a MESMA chave, a dívida aparece com o destino.
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatalf("preencher o documento foi barrado: %v", err)
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
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
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
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
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
		_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
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
		_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
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

// A TRAVA DE "UM PAGAMENTO POR VENDA" PASSOU A SER NOSSA, e este teste é o que a amarra.
//
// Enquanto a referência era a da cobrança, a PONTE sabia que duas chamadas eram a mesma
// dívida e recusava a segunda sozinha. Com uma referência por TENTATIVA ela não sabe
// mais: para ela, cada tentativa é um repasse diferente, e ela paga as duas.
//
// A trava agora é o estado PENDENTE, e ela tem de segurar o caso que mais dói: uma
// dívida INCERTA, que pode já ter sido paga.
func TestTentativaNovaSoNasceDePendente(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "tentativa-trava")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	// A primeira nasce sem ninguém autorizar: ela é automática.
	t1, err := s.AbrirTentativa(ctx, id, "")
	if err != nil {
		t.Fatalf("primeira tentativa: %v", err)
	}
	if t1.Numero != 1 || t1.Referencia != ReferenciaDaTentativa(id, 1) {
		t.Errorf("primeira tentativa = %+v", t1)
	}

	// Mandada, a dívida sai de pendente — e NENHUMA tentativa nova nasce.
	if err := s.MarcarRepasseEnviado(ctx, id, "saque-1", precoEmCentavos); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbrirTentativa(ctx, id, ""); !errors.Is(err, ErrRepasseNaoPendente) {
		t.Errorf("abriu tentativa numa divida ja enviada: %v", err)
	}

	// E o caso que mais importa: a dívida vira INCERTA e continua travada. Uma
	// tentativa aqui é o caminho direto para pagar duas vezes, porque o incerto PODE
	// ter pago e ninguém sabe.
	s2, ctx2 := freshStore(t)
	v2 := montaVenda(ctx2, t, s2, "tentativa-incerta")
	_, venda2, err := s2.ConfirmarCobrancaRMT(ctx2, v2.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id2 := idDoRepasse(ctx2, t, s2, venda2.CobrancaID)
	if _, err := s2.AbrirTentativa(ctx2, id2, ""); err != nil {
		t.Fatal(err)
	}
	if err := s2.MarcarRepasseIncerto(ctx2, id2, "a resposta nao voltou"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.AbrirTentativa(ctx2, id2, ""); !errors.Is(err, ErrRepasseNaoPendente) {
		t.Errorf("abriu tentativa num INCERTO; seria o segundo pagamento da mesma divida: %v", err)
	}

	// Só depois de uma PESSOA conferir no painel que não pagou é que a segunda nasce —
	// e ela carrega o nome de quem autorizou, senão pareceria um bug.
	if err := s2.ResolverIncertoComoNaoPago(ctx2, id2, AtorDoRepasse{Nome: "hanna"}, "nao saiu"); err != nil {
		t.Fatal(err)
	}
	t2, err := s2.AbrirTentativa(ctx2, id2, "hanna")
	if err != nil {
		t.Fatalf("segunda tentativa depois da conferencia: %v", err)
	}
	if t2.Numero != 2 {
		t.Errorf("numero da segunda = %d, quero 2", t2.Numero)
	}
	// A REFERÊNCIA MUDOU, que é a razão de tudo isto: a da primeira ficou travada para
	// sempre na ponte, e reenviar com ela devolveria o resultado que ninguem sabe.
	if t2.Referencia == ReferenciaDaTentativa(id2, 1) {
		t.Error("a segunda tentativa repetiu a referencia da primeira; a ponte devolveria o resultado velho")
	}

	// E o histórico guarda as duas, que é o que uma disputa pergunta.
	hist, err := s2.TentativasDoRepasse(ctx2, id2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Errorf("historico tem %d tentativas, quero 2", len(hist))
	}
	var liberado string
	if err := s2.pool.QueryRow(ctx2,
		`SELECT coalesce(liberado_por, '') FROM rmt_repasse_tentativa
		  WHERE repasse_id = $1 AND tentativa = 2`, id2).Scan(&liberado); err != nil {
		t.Fatal(err)
	}
	if liberado != "hanna" {
		t.Errorf("a segunda tentativa nao registrou quem a autorizou: %q", liberado)
	}
}

// A TRAVA DA CHAVE VALE ATÉ O DINHEIRO SAIR, e LIBERA quem precisa corrigir.
//
// O golpe: esperar a venda CONCLUIR e trocar a chave antes do repasse. A chave é lida na
// hora de pagar, então entre a venda e o saque — dois minutos, ou dias enquanto a trava
// do saque estiver desligada — quem entrasse na conta desviaria o dinheiro de uma venda
// que já aconteceu. A trava antiga só cobria cobrança ABERTA e não alcançava isso.
//
// E a segunda metade é tão importante quanto a primeira: no RECUSADO a trava SOLTA. A
// recusa mais comum é justamente a chave estar errada, e uma trava que impedisse o
// vendedor de corrigi-la prenderia o dinheiro dele para sempre — ela passaria a causar o
// problema que existe para evitar.
func TestTrocarChaveComRepasseEmAbertoERecusadoLiberaNoRecusado(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "trava-repasse")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	trocar := func() error {
		return s.SalvarChavePix(ctx, v.vendedor, "ladrao@exemplo.com", ChavePixEmail, "52998224725")
	}

	// PENDENTE: travado. A venda já aconteceu e o dinheiro ainda não saiu.
	//
	// ErrRepasseEmCurso e não ErrVendaEmCurso: os dois foram separados em 25/09/2026.
	// Cobrança aberta e repasse a caminho pediam coisas diferentes de quem lê — uma
	// passa sozinha quando a compra fechar, a outra só quando o dinheiro sair — e liam
	// a mesma frase.
	if err := trocar(); !errors.Is(err, ErrRepasseEmCurso) {
		t.Errorf("pendente: erro = %v, quero ErrRepasseEmCurso", err)
	}

	// ENVIADO: travado. O saque está a caminho da chave que valia.
	if err := s.MarcarRepasseEnviado(ctx, id, "saque-1", precoEmCentavos); err != nil {
		t.Fatal(err)
	}
	if err := trocar(); !errors.Is(err, ErrRepasseEmCurso) {
		t.Errorf("enviado: erro = %v, quero ErrRepasseEmCurso", err)
	}

	// INCERTO: travado, e é o mais importante dos três — o dinheiro PODE já ter saído
	// para a chave antiga, e trocar agora embaralharia quem recebeu o quê.
	s2, ctx2 := freshStore(t)
	v2 := montaVenda(ctx2, t, s2, "trava-incerto")
	_, venda2, err := s2.ConfirmarCobrancaRMT(ctx2, v2.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id2 := idDoRepasse(ctx2, t, s2, venda2.CobrancaID)
	if err := s2.MarcarRepasseIncerto(ctx2, id2, "a resposta nao voltou"); err != nil {
		t.Fatal(err)
	}
	if err := s2.SalvarChavePix(ctx2, v2.vendedor, "outra@exemplo.com", ChavePixEmail, "52998224725"); !errors.Is(err, ErrRepasseEmCurso) {
		t.Errorf("incerto: erro = %v, quero ErrRepasseEmCurso", err)
	}

	// RECUSADO: LIBERA. Sem isto, o vendedor cuja chave estava errada nunca conseguiria
	// corrigi-la, e o dinheiro dele ficaria preso para sempre.
	s3, ctx3 := freshStore(t)
	v3 := montaVenda(ctx3, t, s3, "trava-recusado")
	_, venda3, err := s3.ConfirmarCobrancaRMT(ctx3, v3.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id3 := idDoRepasse(ctx3, t, s3, venda3.CobrancaID)
	http := int32(422)
	if err := s3.MarcarRepasseRecusado(ctx3, id3, &http, "PIX_KEY_NOT_FOUND", "chave nao existe"); err != nil {
		t.Fatal(err)
	}
	if err := s3.SalvarChavePix(ctx3, v3.vendedor, "certa@exemplo.com", ChavePixEmail, "52998224725"); err != nil {
		t.Errorf("recusado: a trava prendeu quem precisava corrigir a chave: %v", err)
	}
}

// QUEM NÃO TEM O CADASTRO COMPLETO APARECE NA FILA DE GENTE, com o motivo.
//
// Antes disto essas linhas eram invisíveis: o JOIN da fila de pagar as excluía e nenhum
// contador as mencionava. A pessoa tinha dinheiro a receber, não sabia, e o log dizia
// que estava tudo certo.
func TestQuemNaoTemCadastroCompletoApareceNaFilaDeGente(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "sem-cadastro")
	// Vendedor antigo: chave sim, documento não — que é o caso que existe de verdade.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_recebedor SET documento = NULL WHERE account_id = $1`, v.vendedor); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	// Não está na fila de pagar: a ponte recusaria por falta de documento.
	pagar, err := s.RepassesAPagar(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagar) != 0 {
		t.Errorf("a fila de pagar tem %d linha(s) sem documento", len(pagar))
	}

	// MAS aparece na fila de gente, com o motivo — e é essa a diferença.
	gente, err := s.RepassesQuePrecisamDeGente(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gente) != 1 {
		t.Fatalf("fila de gente = %d linhas, quero 1", len(gente))
	}
	if gente[0].Estado != RepasseEsperandoCadastro {
		t.Errorf("estado = %v, quero esperando-cadastro", gente[0].Estado)
	}
	if gente[0].RecusaTexto == "" {
		t.Error("a linha nao diz por que esta parada")
	}

	// E o contador existe, para o número aparecer no log da varredura.
	n, err := s.RepassesEsperandoCadastro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("contador = %d, quero 1", n)
	}

	// Completando, ela sai da fila de gente e entra na de pagar. É o que prova que a
	// espera é do cadastro e não de outra coisa — e é o que permite ao site dizer
	// "cadastre e o pagamento entra na fila".
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatal(err)
	}
	if pagar, err = s.RepassesAPagar(ctx, 10); err != nil || len(pagar) != 1 {
		t.Errorf("depois do cadastro: fila de pagar = %d, err = %v", len(pagar), err)
	}
	if n, err = s.RepassesEsperandoCadastro(ctx); err != nil || n != 0 {
		t.Errorf("depois do cadastro: contador = %d, err = %v", n, err)
	}
}

// O VENDEDOR ANTIGO TEM DE CONSEGUIR PREENCHER O CPF QUE FALTA, e este teste existe
// porque eu quase prendi o dinheiro dessas pessoas para sempre.
//
// A primeira versão da trava olhava só o estado do repasse, e barrava QUALQUER gravação
// da chave. Com ela assim, o vendedor cadastrado antes de o CPF ser obrigatório caía num
// nó fechado: o repasse fica pendente por falta de documento, e o documento não pode ser
// gravado porque o repasse está pendente. A trava causaria exatamente o que existe para
// evitar — a pessoa não receber.
//
// A regra certa é sobre DESVIO: trava quando a CHAVE muda. Gravar a mesma chave com o
// documento que faltava não desvia nada.
//
// (E "vendeu sem nunca ter cadastrado" NÃO existe: o AbrirAnunciosRMT recusa anunciar
// sem chave. O caso real é este — chave certa, documento faltando.)
func TestVendedorAntigoConseguePreencherOCPFQueFalta(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "cpf-faltando")

	// O estado do vendedor antigo: chave cadastrada, documento NULO. O montaVenda já põe
	// a chave — porque sem ela não se anuncia —, e aqui o documento é apagado, que é o
	// que a migração do CPF encontrou nas contas que já existiam.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_recebedor SET documento = NULL WHERE account_id = $1`, v.vendedor); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero()); err != nil {
		t.Fatal(err)
	}

	// Sem documento, a dívida não entra na fila de pagar — não há como mandar.
	if fila, err := s.RepassesAPagar(ctx, 10); err != nil || len(fila) != 0 {
		t.Fatalf("fila de pagar = %d, err = %v", len(fila), err)
	}

	// E ELE CONSEGUE PREENCHER, com a MESMA chave. É esta gravação que a primeira
	// versão da trava barrava.
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatalf("a trava barrou o preenchimento do CPF: o dinheiro ficaria preso: %v", err)
	}

	// E aí a dívida entra na fila sozinha.
	fila, err := s.RepassesAPagar(ctx, 10)
	if err != nil || len(fila) != 1 {
		t.Fatalf("depois do CPF: fila = %d, err = %v", len(fila), err)
	}

	// MAS trocar a CHAVE continua travado: é aí que o desvio seria possível.
	if err := s.SalvarChavePix(ctx, v.vendedor, "ladrao@exemplo.com", ChavePixEmail, "52998224725"); !errors.Is(err, ErrRepasseEmCurso) {
		// ErrRepasseEmCurso e nao ErrVendaEmCurso: os dois foram separados em
		// 25/09/2026 porque pediam coisas diferentes de quem le. A cobranca aberta
		// passa sozinha quando a compra fechar; o repasse so quando o dinheiro sair.
		t.Errorf("a troca de chave com divida pendente passou: erro = %v", err)
	}
}
