//go:build integration

// A confirmação decidindo pela HORA DO PAGAMENTO, e a idempotência do aviso que
// se repete.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"sync"
	"testing"
	"time"
)

// cobrancaPronta monta uma venda pronta para ser paga: vendedor com chave, anúncio
// ativo, item marcado e cobrança aberta.
func cobrancaPronta(ctx context.Context, t *testing.T, s *Store, nome string) (vendedor, comprador int64, ref string) {
	t.Helper()
	vendedor = contaPix(ctx, t, s, "vendedor_"+nome)
	comprador = contaPix(ctx, t, s, "comprador_"+nome)
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador", 0, 0, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	ref = "ref-" + nome
	if res, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, ref, 0); err != nil {
		t.Fatal(err)
	} else if res != CobrancaAbertaOK {
		t.Fatalf("a cobranca nao abriu: resultado %d", res)
	}
	return vendedor, comprador, ref
}

// PAGOU DENTRO DO PRAZO, RECEBE — mesmo que o aviso chegue depois de a nossa
// varredura já ter expirado a cobrança.
//
// É o caso que mais dói e o motivo de a decisão ser pela hora do pagamento: o
// comprador paga no minuto 4:59 e o aviso da processadora atrasa. Decidindo pelo
// NOSSO estado, esse pagamento viraria dívida — ele pagou em dia e ficaria sem o
// item.
func TestPagouNoPrazoRecebeMesmoComOAvisoAtrasado(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "avisoatrasado")

	// A varredura roda e expira a cobrança, porque do NOSSO lado o prazo acabou.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET expira_em = now() - interval '1 minute'
		 WHERE referencia_externa = $1`, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExpirarCobrancasRMT(ctx); err != nil {
		t.Fatal(err)
	}

	// E só então o aviso chega, dizendo que o pagamento aconteceu ANTES do prazo.
	pagoEm := time.Now().Add(-2 * time.Minute)
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, ref, pagoEm, HoraDaProcessadora, 0)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %d, quero confirmada(%d): ele pagou EM DIA e o aviso "+
			"e que se atrasou", res, CobrancaConfirmada)
	}
	if venda.EntregaID == 0 {
		t.Error("nao enfileirou a entrega de quem pagou no prazo")
	}
	// A COLUNA CONTA O LARGO, e a suíte CONTINHA AS DUAS REGRAS antes de este job
	// existir: este teste exigia falso e o TestPagamentoAtrasadoComItemAindaMarcadoEntrega
	// exigia verdadeiro, no MESMO cenário. Nenhum dos dois rodava, então ninguém
	// podia ver a contradição.
	//
	// Ficou o largo, que é o que a 0105 escreveu: "a confirmação que chegou DEPOIS do
	// cancelamento ou da expiração". A razão é a pergunta que a coluna existe para
	// responder — "o prazo está errado?" — e este caso É um sintoma disso: com uma
	// janela maior, a corrida entre o pagamento e a nossa varredura não teria
	// acontecido. Contar só o pagamento genuinamente tardio esconderia a corrida, que
	// é o que mais aparece na prática.
	//
	// A regra ESTREITA continua existindo, com outro nome: `foraDoPrazo`, que é quem
	// decide a entrega. Ela não virou coluna porque o pagamento tardio de verdade já
	// é achável pelo status PAGA_SEM_ITEM.
	if !venda.PagoComAtraso {
		t.Error("nao contou o caso do aviso atrasado; e ele que responde se o prazo esta curto")
	}
}

// PAGOU FORA DO PRAZO NÃO RECEBE, mesmo com o item ainda lá.
//
// Parece desperdício — o item está disponível. Mas o prazo é a única coisa que o
// vendedor tem: ele combinou prender o item por cinco minutos, e entregar aos dez
// seria decidir por ele que a venda ainda valia. E o item ainda estar lá é acaso
// de alguns segundos: a esta altura ele podia já ter sido solto e vendido a outra
// pessoa.
func TestPagouForaDoPrazoNaoRecebeNemComItemDisponivel(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "atrasadoreal")

	// Tudo ainda de pé: anúncio ativo, item marcado, cobrança ABERTA. Só o
	// pagamento é que veio tarde.
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, ref, foraDoPrazo(), HoraDaProcessadora, 0)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaPagaSemItem {
		t.Fatalf("resultado = %d, quero paga-sem-item(%d): o prazo e a unica coisa "+
			"que o vendedor tem", res, CobrancaPagaSemItem)
	}
	if venda.EntregaID != 0 {
		t.Error("entregou um pagamento fora do prazo")
	}
	if !venda.PagoComAtraso {
		t.Error("nao marcou o atraso; e a coluna que diz se a janela esta curta demais")
	}
	// E o item continua com o vendedor, livre para a próxima venda.
	var marcado bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM item WHERE owner_kind = 'account_cargo'
		               AND account_id = (SELECT vendedor_conta FROM rmt_anuncio
		                                  WHERE id = (SELECT anuncio_id FROM rmt_cobranca
		                                               WHERE referencia_externa = $1)))`,
		ref).Scan(&marcado); err != nil {
		t.Fatal(err)
	}
	if !marcado {
		t.Error("o item sumiu do bau do vendedor numa venda que nao aconteceu")
	}
}

// SEM A HORA DO PAGAMENTO, NÃO DECIDE. Adivinhar aqui é decidir sobre o dinheiro
// de alguém no escuro.
func TestSemAHoraDoPagamentoNaoConfirma(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "semhora")

	_, _, err := s.ConfirmarCobrancaRMT(ctx, ref, time.Time{}, HoraDaProcessadora, 0)

	if err == nil {
		t.Fatal("confirmou sem saber quando o pagamento aconteceu")
	}
	// E NADA mudou: uma recusa que deixa metade feita é pior do que nenhuma.
	var status int16
	if err := s.pool.QueryRow(ctx,
		`SELECT status FROM rmt_cobranca WHERE referencia_externa = $1`, ref).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != cobrancaAberta {
		t.Errorf("a cobranca virou %d apesar da recusa", status)
	}
}

// O MESMO AVISO DUAS VEZES ENTREGA UMA VEZ SÓ.
//
// Webhook se repete por desenho — é assim que a processadora garante a entrega da
// mensagem. Duas entregas do mesmo pagamento seriam dois itens saindo de um
// anúncio que tinha um.
func TestOMesmoAvisoDuasVezesEntregaUmaVez(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPronta(ctx, t, s, "repetido")
	pagoEm := dentroDoPrazo()

	res1, venda1, err := s.ConfirmarCobrancaRMT(ctx, ref, pagoEm, HoraDaProcessadora, 0)
	if err != nil {
		t.Fatal(err)
	}
	res2, venda2, err := s.ConfirmarCobrancaRMT(ctx, ref, pagoEm, HoraDaProcessadora, 0)
	if err != nil {
		t.Fatal(err)
	}

	if res1 != CobrancaConfirmada {
		t.Errorf("a primeira devolveu %d, quero confirmada", res1)
	}
	if res2 != CobrancaJaConfirmada {
		t.Errorf("a segunda devolveu %d, quero ja-confirmada(%d) — repetir e o caminho "+
			"NORMAL, nao uma anomalia", res2, CobrancaJaConfirmada)
	}
	if venda2.EntregaID != venda1.EntregaID {
		t.Errorf("a segunda apontou para a entrega %d; a primeira foi %d",
			venda2.EntregaID, venda1.EntregaID)
	}
	if n := entregasDe(ctx, t, s, comprador); n != 1 {
		t.Errorf("a caixa postal tem %d entrega(s), quero 1: dois itens sairiam de um "+
			"anuncio que tinha um", n)
	}
}

// E DOIS AVISOS AO MESMO TEMPO TAMBÉM ENTREGAM UMA VEZ SÓ.
//
// O repetido em sequência é pego pela leitura do status; o SIMULTÂNEO não — as
// duas transações leem o mesmo estado antigo. Quem resolve é o FOR UPDATE, e é
// este teste que prova que ele está lá.
func TestDoisAvisosAoMesmoTempoEntregamUmaVez(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPronta(ctx, t, s, "paralelo")
	pagoEm := dentroDoPrazo()

	type saida struct {
		res ResultadoCobranca
		err error
	}
	fora := make(chan saida, 2)
	var largada sync.WaitGroup
	largada.Add(1)
	for i := 0; i < 2; i++ {
		go func() {
			largada.Wait() // as duas partem juntas
			res, _, err := s.ConfirmarCobrancaRMT(context.Background(), ref, pagoEm, HoraDaProcessadora, 0)
			fora <- saida{res, err}
		}()
	}
	largada.Done()

	confirmadas, jaConfirmadas := 0, 0
	for i := 0; i < 2; i++ {
		r := <-fora
		if r.err != nil {
			t.Fatalf("confirmando em paralelo: %v", r.err)
		}
		switch r.res {
		case CobrancaConfirmada:
			confirmadas++
		case CobrancaJaConfirmada:
			jaConfirmadas++
		default:
			t.Errorf("resultado inesperado: %d", r.res)
		}
	}

	if confirmadas != 1 || jaConfirmadas != 1 {
		t.Errorf("confirmadas = %d, ja-confirmadas = %d; quero 1 e 1",
			confirmadas, jaConfirmadas)
	}
	if n := entregasDe(ctx, t, s, comprador); n != 1 {
		t.Errorf("a caixa postal tem %d entrega(s), quero 1", n)
	}
}

// O COMPRADOR FECHA O JOGO PARA PAGAR NO CELULAR, O VENDEDOR DERRUBA A BARRACA, E
// O PIX CAI DENTRO DO PRAZO. O item tem de ser ENTREGUE.
//
// É o caso mais comum que existe, e hoje ele perde dinheiro:
//
// O caminho ANTIGO perdia dinheiro:
//
//  1. o comprador saía do jogo — o movimento natural de quem vai pagar no celular;
//  2. o logout CANCELAVA a cobrança dele;
//  3. cancelada, ela deixava de contar como "aberta" para o escrow;
//  4. o vendedor derrubava a barraca, e sem cobrança aberta o anúncio era
//     cancelado e o item solto NA HORA;
//  5. o Pix caía trinta segundos depois, DENTRO do prazo, e não achava item.
//
// Resultado: PAGA_SEM_ITEM. Reembolso que custa R$ 1,00 mais as taxas à Hanna, de
// um comprador que fez tudo certo.
//
// O conserto foi tirar o passo 2. Cancelar no logout não soltava o item de
// ninguém — a marca do escrow segura até o prazo acabar de qualquer jeito —, então
// ele não comprava nada e quebrava esta compra. A cobrança passa a fechar de dois
// jeitos só: paga ou vencida.
func TestCompradorQueSaiParaPagarNoCelularRecebe(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor, _, ref := cobrancaPronta(ctx, t, s, "celular")

	var anuncio int64
	if err := s.pool.QueryRow(ctx,
		`SELECT anuncio_id FROM rmt_cobranca WHERE referencia_externa = $1`, ref).
		Scan(&anuncio); err != nil {
		t.Fatal(err)
	}

	// 1: ele sai do jogo. E É SÓ ISSO: o logout do comprador NÃO mexe mais na
	// cobrança dela. Cancelar ali não soltava o item de ninguém — a marca segura
	// até o prazo acabar de qualquer jeito — e quebrava exatamente esta compra.
	//
	// Não há nada a chamar aqui, e a ausência de chamada É o conserto. A prova de
	// que o logout não cancela está no que vem depois: a cobrança segue ABERTA.
	if st := statusDaCobranca(ctx, t, s, ref); st != cobrancaAberta {
		t.Fatalf("a cobranca esta no status %d depois do logout; ela tem de seguir aberta", st)
	}

	// 2: o vendedor derruba a barraca.
	if _, err := s.EncerrarAnunciosRMT(ctx, []int64{anuncio}); err != nil {
		t.Fatal(err)
	}
	// E entra em jogo de novo, o que dispara a reconciliacao.
	if _, err := s.ReconciliarEscrowRMT(ctx, vendedor); err != nil {
		t.Fatal(err)
	}

	// 3: o Pix cai, DENTRO do prazo.
	res, venda, err := s.ConfirmarCobrancaRMT(ctx, ref, dentroDoPrazo(), HoraDaProcessadora, 0)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %d, quero confirmada(%d). Ele pagou DENTRO do prazo e "+
			"fez tudo certo; isto vira reembolso que custa dinheiro a Hanna",
			res, CobrancaConfirmada)
	}
	if venda.EntregaID == 0 {
		t.Error("nao enfileirou a entrega de quem pagou no prazo")
	}
	if venda.PagoComAtraso {
		t.Error("marcou como atraso um pagamento feito dentro do prazo")
	}
}

// E A PÁGINA CONTINUA MOSTRANDO O CÓDIGO DEPOIS DE ELE SAIR DO JOGO.
//
// É a outra metade do conserto, e sem ela o resto não serve de nada: se o código
// some da página quando ele fecha o jogo, ele não tem como pagar no celular, que é
// exatamente o que a mudança existe para permitir.
//
// O teste anterior prova que o pagamento SERIA aceito; este prova que ele consegue
// FAZER o pagamento.
func TestDepoisDeSairDoJogoOCodigoContinuaNaPagina(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPronta(ctx, t, s, "codigovivo")
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET codigo_pix = $2 WHERE referencia_externa = $1`,
		ref, "00020126BR.GOV.BCB.PIX6304ABCD"); err != nil {
		t.Fatal(err)
	}

	// Ele sai do jogo. Nada acontece com a cobrança — é esse o conserto.

	tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}

	if !tem {
		t.Fatal("a pagina esvaziou depois de ele sair do jogo")
	}
	if cob.Estado != EstadoCobrancaAberta {
		t.Errorf("estado = %d, quero aberta(%d): sair do jogo nao fecha cobranca",
			cob.Estado, EstadoCobrancaAberta)
	}
	if cob.CodigoPix != "00020126BR.GOV.BCB.PIX6304ABCD" {
		t.Errorf("codigo = %q; sem ele a pessoa nao tem como pagar no celular", cob.CodigoPix)
	}
}

// VALOR DIFERENTE NÃO ENTREGA E NÃO FECHA A COBRANÇA.
//
// Pagar menos e receber o item seria comprar com desconto de si mesmo. Pagar mais
// e receber sem troco seria o contrário. Os dois pedem uma pessoa, e nenhum pede
// decisão automática — é por isso que a linha fica como está em vez de virar
// PAGA_SEM_ITEM, que já tem destino (reembolso) e este caso ainda não tem.
func TestValorDiferenteNaoEntregaENaoFechaACobranca(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPronta(ctx, t, s, "valorerrado")

	// A cobrança pede 12345 centavos; a processadora diz ter recebido 100.
	res, _, err := s.ConfirmarCobrancaRMT(ctx, ref, dentroDoPrazo(), HoraDaProcessadora, 100)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaValorDivergente {
		t.Fatalf("resultado = %d, quero valor-divergente(%d)", res, CobrancaValorDivergente)
	}
	if st := statusDaCobranca(ctx, t, s, ref); st != cobrancaAberta {
		t.Errorf("a cobranca virou %d; ela tem de ficar aberta, porque nao se resolveu", st)
	}
	if n := entregasDe(ctx, t, s, comprador); n != 0 {
		t.Errorf("enfileirou %d entrega(s) por um valor que nao bate", n)
	}

	// O DINHEIRO NÃO SOME DO REGISTRO, e é este o conserto. Antes, este caminho
	// não gravava nada: alguém pagava e não sobrava linha que dissesse isso.
	fila, err := s.ValoresDivergentes(ctx)
	if err != nil {
		t.Fatalf("lendo a fila: %v", err)
	}
	if len(fila) != 1 {
		t.Fatalf("a fila da staff tem %d linha(s), quero 1: entrou dinheiro e ninguem "+
			"decidiu o que fazer", len(fila))
	}
	if fila[0].ValorCobrado != 12345 || fila[0].ValorRecebido != 100 {
		t.Errorf("fila = %+v; quero os DOIS valores, porque a decisao depende da diferenca",
			fila[0])
	}

	// E A VARREDURA DO PRAZO NÃO A VENCE. Vencer soltaria o item de quem já
	// recebeu o pagamento — o contrário exato do que o prazo existe para fazer.
	if _, err := s.pool.Exec(ctx, `
		UPDATE rmt_cobranca SET expira_em = now() - interval '1 hour'
		 WHERE referencia_externa = $1`, ref); err != nil {
		t.Fatal(err)
	}
	if venceram, err := s.ExpirarCobrancasRMT(ctx); err != nil {
		t.Fatal(err)
	} else if len(venceram) != 0 {
		t.Errorf("a varredura venceu %v; a divergente tem dinheiro dentro e espera gente",
			venceram)
	}

	// E a página do comprador não convida a pagar de novo.
	_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cob.Estado != EstadoCobrancaValorDivergente {
		t.Errorf("estado na pagina = %d, quero valor-divergente(%d)",
			cob.Estado, EstadoCobrancaValorDivergente)
	}
	if cob.CodigoPix != "" {
		t.Error("mostrou o codigo de uma cobranca que ja recebeu dinheiro")
	}
}

// E O VALOR CERTO PASSA, que é o que dá valor ao teste de cima: uma conferência
// que recusasse sempre passaria lá e quebraria toda venda.
func TestValorCertoPassa(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "valorcerto")

	res, _, err := s.ConfirmarCobrancaRMT(ctx, ref, dentroDoPrazo(), HoraDaProcessadora, 12345)
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if res != CobrancaConfirmada {
		t.Errorf("resultado = %d, quero confirmada", res)
	}
}

// A ORIGEM DO RELÓGIO FICA GRAVADA.
//
// A hora vem da processadora quando ela a dá, e do nosso relógio quando a consulta
// cai para a V1, que não documenta o campo. São precisões muito diferentes, e no
// dia em que alguém perguntar "por que essa venda foi reembolsada" a resposta tem
// de poder dizer se a gente SABIA a hora ou ESTIMOU.
func TestAOrigemDoRelogioFicaGravada(t *testing.T) {
	s, ctx := freshStore(t)
	_, _, ref := cobrancaPronta(ctx, t, s, "relogio")

	if _, _, err := s.ConfirmarCobrancaRMT(ctx, ref, dentroDoPrazo(),
		HoraDoServidor, 0); err != nil {
		t.Fatal(err)
	}

	var origem *string
	if err := s.pool.QueryRow(ctx,
		`SELECT origem_da_hora FROM rmt_cobranca WHERE referencia_externa = $1`, ref).
		Scan(&origem); err != nil {
		t.Fatal(err)
	}
	if origem == nil || *origem != string(HoraDoServidor) {
		t.Errorf("origem = %v, quero %q: sem ela nao da para saber se a hora era "+
			"conhecida ou estimada", origem, HoraDoServidor)
	}
}
