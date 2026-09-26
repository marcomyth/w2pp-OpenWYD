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
	// O LÍQUIDO, e não o bruto: desde 25/09/2026 a taxa da casa desconta do que o
	// vendedor recebe. Calculado pela mesma função, e não escrito, para a próxima
	// mudança de taxa não pedir vinte números reescritos à mão.
	if valor != liquidoDaVendaDeTeste() {
		t.Errorf("valor = %d, quero %d", valor, liquidoDaVendaDeTeste())
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
func TestAFilaDePagarTrazMascaraEPulaQuemNaoTemDocumento(t *testing.T) {
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

	fila, err := s.FilaDePagamentoAMao(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Fatalf("a fila tem %d linha(s) sem documento; nao ha onde pagar nenhuma delas", len(fila))
	}

	// Preenchendo o documento, com a MESMA chave, a dívida aparece na fila.
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatalf("preencher o documento foi barrado: %v", err)
	}
	fila, err = s.FilaDePagamentoAMao(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 1 {
		t.Fatalf("a fila tem %d linha(s), quero 1", len(fila))
	}
	r := fila[0]
	// NA FILA, SÓ MÁSCARA. O destino inteiro vem da outra porta, e essa diferença é o
	// conserto de privacidade que veio com o pagamento à mão: a lista abre em toda
	// visita, e o dado inteiro ali seria a chave de todos os vendedores de graça.
	if r.TipoChave != ChavePixEmail {
		t.Errorf("tipo = %d, quero email", r.TipoChave)
	}
	if r.ChaveMascarada == "vendedor@exemplo.com" {
		t.Error("a fila trouxe a chave INTEIRA")
	}
	if r.DocMascarado == "11144477735" {
		t.Error("a fila trouxe o CPF INTEIRO")
	}
	// A FILA DA STAFF MANDA PAGAR O LÍQUIDO, o mesmo número que o jogo prometeu ao
	// vendedor ao montar a barraca.
	if int64(r.ValorCentavos) != liquidoDaVendaDeTeste() {
		t.Errorf("valor = %d, quero %d", r.ValorCentavos, liquidoDaVendaDeTeste())
	}
	// E o "vence em" é a venda mais 48 horas corridas, que é a promessa ao vendedor.
	if quero := r.CriadoEm.Add(PrazoDoPagamento); !r.VenceEm.Equal(quero) {
		t.Errorf("vence em %v, quero %v", r.VenceEm, quero)
	}

	// A outra porta: o destino inteiro, para quem vai pagar, com a leitura registrada.
	inteira, err := s.ChaveParaPagar(ctx, r.ID, staffQuePaga())
	if err != nil {
		t.Fatal(err)
	}
	if inteira.ChavePix != "vendedor@exemplo.com" || inteira.TipoChave != ChavePixEmail {
		t.Errorf("destino = %q tipo %d", inteira.ChavePix, inteira.TipoChave)
	}
	if inteira.Documento != "11144477735" {
		t.Errorf("documento = %q, o Pix a terceiros exige os 11 digitos", inteira.Documento)
	}
}

// AQUI HAVIA O TESTE DAS TRANSICOES AUTOMATICAS (pendente -> enviado -> pago, e a
// corrida entre dois trabalhadores). Ele saiu com o que testava: o saque automatico foi
// apagado em 25/09/2026, e nao existe mais quem escreva ENVIADO.
//
// A regra que ele protegia continua viva em outro lugar: "so sai do estado certo" agora
// e medida no pagamento a mao (repasse_pago_a_mao_integration_test.go), onde o segundo
// clique da staff e a corrida que importa.

// O INCERTO SAI DE PENDENTE E NÃO VOLTA, que é a razão de ele existir: reenviar um
// pagamento que PODE ter saído é a única coisa que não se desfaz.
func TestIncertoNaoApareceParaPagarEVaiParaADeGente(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "repasse-incerto")
	_, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo(), HoraDaProcessadora, precoEmCentavos, taxaZero())
	if err != nil {
		t.Fatal(err)
	}
	id := idDoRepasse(ctx, t, s, venda.CobrancaID)

	// INCERTO à força: nada mais escreve esse estado desde que o saque automático saiu.
	// O que este teste mede continua valendo, e passou a valer para a fila da STAFF:
	// uma linha incerta não pode aparecer para alguém pagar à mão, porque o dinheiro
	// dela pode já ter saído.
	forcaEstadoDoRepasse(ctx, t, s, id, RepasseIncerto)

	fila, err := s.FilaDePagamentoAMao(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(fila) != 0 {
		t.Error("o incerto apareceu na fila de pagar: a staff pagaria o que talvez ja tenha saido")
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
		forcaEstadoDoRepasse(ctx, t, s, id, RepasseIncerto)
		return id
	}

	// NADA MEXE NUM INCERTO por conta própria. As transições automáticas que podiam
	// fazer isso foram apagadas com o saque automático; o que sobrou e ainda precisa ser
	// provado é que ele NÃO aparece para a staff pagar à mão, e que o pagamento à mão o
	// recusa se alguém tentar pelo id.
	travado := fazIncerto("incerto-travado")
	if err := s.MarcarRepassePagoAMao(ctx, travado, staffQuePaga(), "quis fechar o incerto"); err == nil {
		t.Error("a staff fechou um incerto a mao; o dinheiro dele pode ja ter saido")
	}
	if err := s.MarcarRepasseRecusado(ctx, travado, nil, "", "x"); err == nil {
		t.Error("um incerto virou recusado sem ninguem ter olhado")
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
	fila, err := s.FilaDePagamentoAMao(ctx, 10)
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

// AQUI HAVIA O TESTE DAS TENTATIVAS DE REPASSE. Ele saiu junto com elas: a tentativa
// existia para dar idempotencia a chamada da ponte, e nao ha mais chamada.

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
	// À força: nada escreve ENVIADO desde que o saque automático saiu, e a trava tem de
	// continuar valendo para as linhas que já estão nesse estado.
	forcaEstadoDoRepasse(ctx, t, s, id, RepasseEnviado)
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
	forcaEstadoDoRepasse(ctx2, t, s2, id2, RepasseIncerto)
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
	pagar, err := s.FilaDePagamentoAMao(ctx, 10)
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

	// Completando, ela sai da fila de gente e entra na de pagar. É o que prova que a
	// espera é do cadastro e não de outra coisa — e é o que permite ao site dizer
	// "cadastre e o pagamento entra na fila".
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatal(err)
	}
	if pagar, err = s.FilaDePagamentoAMao(ctx, 10); err != nil || len(pagar) != 1 {
		t.Errorf("depois do cadastro: fila de pagar = %d, err = %v", len(pagar), err)
	}
	// O contador de quem esperava cadastro saiu com a varredura: ele existia para o
	// número aparecer no log dela. Quem mostra esse caso agora é a fila de gente, logo
	// acima, que a staff lê.
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
	if fila, err := s.FilaDePagamentoAMao(ctx, 10); err != nil || len(fila) != 0 {
		t.Fatalf("fila de pagar = %d, err = %v", len(fila), err)
	}

	// E ELE CONSEGUE PREENCHER, com a MESMA chave. É esta gravação que a primeira
	// versão da trava barrava.
	if err := s.SalvarChavePix(ctx, v.vendedor, "vendedor@exemplo.com", ChavePixEmail, "11144477735"); err != nil {
		t.Fatalf("a trava barrou o preenchimento do CPF: o dinheiro ficaria preso: %v", err)
	}

	// E aí a dívida entra na fila sozinha.
	fila, err := s.FilaDePagamentoAMao(ctx, 10)
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
