//go:build integration

// Testes de integração da confirmação de pagamento em dinheiro real.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
//
// Aqui não cabe teste de unidade: cada cenário é sobre o que acontece ENTRE as
// tabelas — a cobrança, o anúncio, a marca do escrow na linha do item e a caixa
// postal do comprador — e sobre a transação segurar as quatro juntas.
package store

import (
	"context"
	"testing"
	"time"
)

const (
	itemDoAnuncio   = 1030
	precoEmCentavos = 5000
)

// vendaMontada é o estado de uma venda pronta para ser paga: anúncio ativo, item
// marcado no baú do vendedor, cobrança aberta.
type vendaMontada struct {
	vendedor  int64
	comprador int64
	anuncio   int64
	ref       string
}

// dentroDoPrazo é a hora de um pagamento que chegou a tempo.
//
// Agora a confirmação decide pela HORA DO PAGAMENTO, e não pelo estado da nossa
// linha: quem paga dentro do prazo recebe o item, mesmo que o aviso chegue depois
// e mesmo que a varredura já tenha expirado a cobrança. Quem paga fora, não
// recebe — o dinheiro volta pelo reembolso.
func dentroDoPrazo() time.Time { return time.Now() }

// foraDoPrazo é a hora de um pagamento que chegou tarde. Bem depois de qualquer
// janela plausível, para o teste não depender do relógio.
func foraDoPrazo() time.Time { return time.Now().Add(2 * time.Hour) }

func montaVenda(ctx context.Context, t *testing.T, s *Store, sufixo string) vendaMontada {
	t.Helper()
	v := vendaMontada{
		vendedor:  contaPix(ctx, t, s, "vendedor_"+sufixo),
		comprador: contaPix(ctx, t, s, "comprador_"+sufixo),
		ref:       "ref-" + sufixo,
	}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index, eff1, effv1, preco_centavos, status)
		VALUES ($1, 3, $2, 7, 9, $3, 1) RETURNING id`,
		v.vendedor, itemDoAnuncio, precoEmCentavos).Scan(&v.anuncio); err != nil {
		t.Fatalf("criando o anuncio: %v", err)
	}
	// O item no baú do vendedor, com a marca do escrow apontando para o anúncio.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO item (owner_kind, account_id, slot, item_index, eff1, effv1, rmt_anuncio)
		VALUES ('account_cargo', $1, 3, $2, 7, 9, $3)`,
		v.vendedor, itemDoAnuncio, v.anuncio); err != nil {
		t.Fatalf("pondo o item marcado no bau: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO rmt_cobranca (anuncio_id, comprador_conta, referencia_externa,
		                          valor_centavos, metodo, status, expira_em)
		VALUES ($1, $2, $3, $4, 1, 1, now() + interval '30 minutes')`,
		v.anuncio, v.comprador, v.ref, precoEmCentavos); err != nil {
		t.Fatalf("abrindo a cobranca: %v", err)
	}
	return v
}

func entregasDe(ctx context.Context, t *testing.T, s *Store, conta int64) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM delivery_queue WHERE account_id = $1`, conta).Scan(&n); err != nil {
		t.Fatalf("contando entregas: %v", err)
	}
	return n
}

// O caminho feliz, e tudo o que ele tem de deixar no banco.
//
// Repare no último caso: A MARCA DO ESCROW CONTINUA LÁ. Não é esquecimento — é o
// desenho. Enquanto ela está no slot o item é intocável, então tirá-lo pode
// esperar o vendedor entrar em jogo. É isso que faz o comprador nunca depender de
// o vendedor estar online.
func TestConfirmarCobrancaCaminhoFeliz(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "feliz")

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %v, quero CobrancaConfirmada", res)
	}
	if venda.EntregaID == 0 {
		t.Error("a venda voltou sem entrega_id")
	}
	if venda.VendedorConta != v.vendedor || venda.CargoSlot != 3 || venda.AnuncioID != v.anuncio {
		t.Errorf("a venda nao aponta o slot certo para a retirada: %+v", venda)
	}
	if venda.PagoComAtraso {
		t.Error("pagamento no prazo veio marcado como atrasado")
	}

	var statusCobranca, statusAnuncio int16
	var entregaID *int64
	if err := s.pool.QueryRow(ctx,
		`SELECT status, entrega_id FROM rmt_cobranca WHERE referencia_externa = $1`, v.ref).
		Scan(&statusCobranca, &entregaID); err != nil {
		t.Fatal(err)
	}
	if statusCobranca != cobrancaPaga {
		t.Errorf("cobranca ficou no status %d, quero PAGA (%d)", statusCobranca, cobrancaPaga)
	}
	if entregaID == nil || *entregaID != venda.EntregaID {
		t.Error("o entrega_id nao foi gravado na cobranca")
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT status FROM rmt_anuncio WHERE id = $1`, v.anuncio).Scan(&statusAnuncio); err != nil {
		t.Fatal(err)
	}
	if statusAnuncio != anuncioVendido {
		t.Errorf("anuncio ficou no status %d, quero VENDIDO (%d)", statusAnuncio, anuncioVendido)
	}
	if n := entregasDe(ctx, t, s, v.comprador); n != 1 {
		t.Errorf("entregas para o comprador = %d, quero 1", n)
	}

	var marca int64
	if err := s.pool.QueryRow(ctx,
		`SELECT rmt_anuncio FROM item WHERE account_id = $1 AND slot = 3`, v.vendedor).Scan(&marca); err != nil {
		t.Fatal(err)
	}
	if marca != v.anuncio {
		t.Errorf("a marca do escrow virou %d; ela tem de FICAR ate o laco tirar o item", marca)
	}
}

// O que o comprador recebe vem da FOTOGRAFIA do anúncio, com os efeitos. Se
// viesse do item do baú, a entrega dependeria de o vendedor estar em jogo — e o
// comprador não tem nada com isso.
func TestEntregaCarregaAFotografiaDoAnuncio(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "foto")

	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo()); err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	entregas, err := s.PendingItemDeliveries(ctx, v.comprador)
	if err != nil {
		t.Fatalf("lendo a caixa postal: %v", err)
	}
	if len(entregas) != 1 {
		t.Fatalf("entregas = %d, quero 1", len(entregas))
	}
	it := entregas[0].Item
	if it.Index != itemDoAnuncio {
		t.Errorf("item entregue = %d, quero %d", it.Index, itemDoAnuncio)
	}
	if it.Eff1 != 7 || it.EffV1 != 9 {
		t.Errorf("os efeitos nao atravessaram: eff1=%d effv1=%d, quero 7/9", it.Eff1, it.EffV1)
	}
}

// A CONFIRMAÇÃO CHEGA DUAS VEZES, que é o caminho normal e não a exceção: a
// processadora repete de propósito, porque é assim que ela garante entrega.
//
// Duas entregas aqui seriam duas cópias do item no mundo, pagas uma vez só.
func TestConfirmarCobrancaDuasVezesEntregaUmaSo(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "dobrada")

	res1, venda1, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("primeira: %v", err)
	}
	res2, venda2, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}

	if res1 != CobrancaConfirmada {
		t.Errorf("a primeira devolveu %v", res1)
	}
	if res2 != CobrancaJaConfirmada {
		t.Errorf("a segunda devolveu %v, quero CobrancaJaConfirmada", res2)
	}
	if venda2.EntregaID != venda1.EntregaID {
		t.Errorf("a segunda apontou outra entrega: %d contra %d", venda2.EntregaID, venda1.EntregaID)
	}
	// E o que importa de verdade: uma linha na caixa postal, e não duas.
	if n := entregasDe(ctx, t, s, v.comprador); n != 1 {
		t.Errorf("entregas para o comprador = %d; a confirmacao repetida duplicou o item", n)
	}
	// O aviso repetido continua dizendo ONDE está o item do vendedor, para a
	// retirada poder acontecer numa passada posterior.
	if venda2.VendedorConta != v.vendedor || venda2.CargoSlot != 3 {
		t.Errorf("o aviso repetido veio sem o slot da retirada: %+v", venda2)
	}
}

// O PAGAMENTO QUE CONFIRMA DEPOIS DO CANCELAMENTO.
//
// O dinheiro entrou e não há item: o anúncio foi cancelado e a marca do escrow
// soltou, então o vendedor já pode ter usado ou vendido aquele item. Entregar do
// mesmo jeito criaria uma cópia; fingir que entregou seria pior. Vira PAGA_SEM_ITEM,
// que é dívida com uma pessoa numa fila que alguém olha.
func TestPagamentoDepoisDoCancelamentoViraPagaSemItem(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "atrasada")

	// O cancelamento: o anúncio fecha, a marca solta, e a cobrança é cancelada.
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2, encerrado_em = now() WHERE id = $1`,
		v.anuncio, anuncioCancelado); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE item SET rmt_anuncio = 0 WHERE account_id = $1 AND slot = 3`, v.vendedor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET status = $2, encerrada_em = now() WHERE anuncio_id = $1`,
		v.anuncio, cobrancaCancelada); err != nil {
		t.Fatal(err)
	}

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaPagaSemItem {
		t.Fatalf("resultado = %v, quero CobrancaPagaSemItem", res)
	}
	if venda.EntregaID != 0 {
		t.Error("enfileirou entrega para uma venda que nao tinha item")
	}
	if !venda.PagoComAtraso {
		t.Error("a confirmacao atrasada nao foi marcada como atrasada")
	}
	if n := entregasDe(ctx, t, s, v.comprador); n != 0 {
		t.Errorf("entregas = %d; nao havia item para entregar", n)
	}

	var status int16
	var atraso bool
	if err := s.pool.QueryRow(ctx,
		`SELECT status, pago_com_atraso FROM rmt_cobranca WHERE referencia_externa = $1`, v.ref).
		Scan(&status, &atraso); err != nil {
		t.Fatal(err)
	}
	if status != cobrancaPagaSemItem {
		t.Errorf("status = %d, quero PAGA_SEM_ITEM (%d)", status, cobrancaPagaSemItem)
	}
	if !atraso {
		t.Error("pago_com_atraso ficou falso no banco")
	}
	// E o dinheiro NÃO some do registro: a linha continua lá, paga, achável pela
	// fila do estado 5.
	var pagaEm *string
	if err := s.pool.QueryRow(ctx,
		`SELECT paga_em::text FROM rmt_cobranca WHERE referencia_externa = $1`, v.ref).Scan(&pagaEm); err != nil {
		t.Fatal(err)
	}
	if pagaEm == nil {
		t.Error("o pagamento nao ficou registrado; a dívida some da fila")
	}
}

// A cobrança EXPIRADA cujo anúncio continua de pé e marcado: aí há item, e
// entregar é o certo. Só que a exceção fica contada — se isso virar rotina, o
// prazo da cobrança está errado, e é a coluna que vai dizer isso.
func TestPagamentoAtrasadoComItemAindaMarcadoEntrega(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "expirada")

	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET status = $2 WHERE anuncio_id = $1`, v.anuncio, cobrancaExpirada); err != nil {
		t.Fatal(err)
	}

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	if res != CobrancaConfirmada {
		t.Fatalf("resultado = %v, quero CobrancaConfirmada", res)
	}
	if !venda.PagoComAtraso {
		t.Error("a entrega saiu, mas o atraso nao foi contado")
	}
	if n := entregasDe(ctx, t, s, v.comprador); n != 1 {
		t.Errorf("entregas = %d, quero 1", n)
	}
}

// A marca do escrow apontando para OUTRO anúncio não entrega.
//
// É a segunda pergunta do `temItemParaEntregar`, e ela existe para o caso em que
// o status do anúncio e a marca discordam. Discordarem significa que uma
// invariante quebrou em algum lugar, e aí o certo é não entregar e deixar uma
// pessoa olhar — nunca adivinhar em cima de dinheiro.
func TestMarcaDeOutroAnuncioNaoEntrega(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "marcatrocada")

	if _, err := s.pool.Exec(ctx,
		`UPDATE item SET rmt_anuncio = $2 WHERE account_id = $1 AND slot = 3`,
		v.vendedor, v.anuncio+999); err != nil {
		t.Fatal(err)
	}

	res, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if res != CobrancaPagaSemItem {
		t.Errorf("resultado = %v; com a marca de outro anuncio nao se entrega", res)
	}
	if n := entregasDe(ctx, t, s, v.comprador); n != 0 {
		t.Errorf("entregas = %d, quero 0", n)
	}
}

// Referência que não é nossa não é erro nem entrega: é um aviso que não nos diz
// respeito, ou forjado. Devolver erro faria a processadora repetir para sempre.
func TestReferenciaDesconhecidaNaoFazNada(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "desconhecida")

	res, _, err := s.ConfirmarCobrancaRMT(ctx, "ref-que-nao-existe", dentroDoPrazo())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if res != CobrancaNaoEncontrada {
		t.Errorf("resultado = %v, quero CobrancaNaoEncontrada", res)
	}
	if n := entregasDe(ctx, t, s, v.comprador); n != 0 {
		t.Errorf("entregas = %d; uma referencia desconhecida nao entrega nada", n)
	}
	var status int16
	if err := s.pool.QueryRow(ctx,
		`SELECT status FROM rmt_cobranca WHERE referencia_externa = $1`, v.ref).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != cobrancaAberta {
		t.Errorf("a cobranca de verdade mexeu: status = %d", status)
	}
}

// NEM O COMPRADOR NEM O VENDEDOR PRECISAM ESTAR EM JOGO, e este teste é a prova
// de que isso é verdade por construção e não por sorte: nesta camada não existe
// sessão nenhuma, e a confirmação inteira acontece assim mesmo.
//
// Para o comprador, o item fica na caixa postal e o dreno do login entrega. Para
// o vendedor, a marca segura o item onde está até o laço tirar.
func TestConfirmacaoNaoDependeDeNinguemEstarEmJogo(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "offline")

	res, venda, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo())
	if err != nil {
		t.Fatalf("confirmando: %v", err)
	}
	if res != CobrancaConfirmada || venda.EntregaID == 0 {
		t.Fatalf("res = %v, entrega = %d", res, venda.EntregaID)
	}
	pendentes, err := s.PendingItemDeliveries(ctx, v.comprador)
	if err != nil || len(pendentes) != 1 {
		t.Fatalf("a caixa postal do comprador tem %d entrega(s) (err=%v)", len(pendentes), err)
	}
	var marca int64
	if err := s.pool.QueryRow(ctx,
		`SELECT rmt_anuncio FROM item WHERE account_id = $1 AND slot = 3`, v.vendedor).Scan(&marca); err != nil {
		t.Fatal(err)
	}
	if marca != v.anuncio {
		t.Errorf("a marca soltou (%d) com o vendedor fora; o item ficaria solto sem dono decidido", marca)
	}
}

// SlotsVendidosPendentes nomeia só o que o vendedor ainda segura de uma venda
// FECHADA.
//
// Os três casos que importam estão aqui, e o valor está nos dois últimos: um
// anúncio ATIVO não pode entrar (o item ainda é do vendedor e ele pode cancelar),
// e um item sem marca também não (nunca foi anunciado). Uma consulta que só
// olhasse a marca, ou só o status, passaria num dos dois e erraria no outro.
func TestSlotsVendidosPendentesNomeiaSoOQueVendeu(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "listagem")

	// Um segundo item, marcado por um anúncio que continua ATIVO.
	var ativo int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index, preco_centavos, status)
		VALUES ($1, 7, 1040, 3000, 1) RETURNING id`, v.vendedor).Scan(&ativo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO item (owner_kind, account_id, slot, item_index, rmt_anuncio)
		VALUES ('account_cargo', $1, 7, 1040, $2)`, v.vendedor, ativo); err != nil {
		t.Fatal(err)
	}
	// E um terceiro sem marca nenhuma.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO item (owner_kind, account_id, slot, item_index, rmt_anuncio)
		VALUES ('account_cargo', $1, 11, 1050, 0)`, v.vendedor); err != nil {
		t.Fatal(err)
	}

	// Antes da venda, nada pendente: o anúncio do slot 3 ainda está ativo.
	antes, err := s.SlotsVendidosPendentes(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if len(antes) != 0 {
		t.Errorf("com tudo ativo veio %v, queria vazio", antes)
	}

	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo()); err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	depois, err := s.SlotsVendidosPendentes(ctx, v.vendedor)
	if err != nil {
		t.Fatal(err)
	}
	if len(depois) != 1 || depois[0] != 3 {
		t.Errorf("slots pendentes = %v, quero [3] — o 7 ainda está à venda e o 11 nunca foi anunciado", depois)
	}
}

// E o vendedor de outra conta não entra na lista desta. É o erro de um WHERE
// esquecido, e aqui ele apagaria item de quem não vendeu nada.
func TestSlotsVendidosNaoVazamEntreContas(t *testing.T) {
	s, ctx := freshStore(t)
	v := montaVenda(ctx, t, s, "vazamento")
	outro := contaPix(ctx, t, s, "outro_vendedor")

	if _, _, err := s.ConfirmarCobrancaRMT(ctx, v.ref, dentroDoPrazo()); err != nil {
		t.Fatalf("confirmando: %v", err)
	}

	slots, err := s.SlotsVendidosPendentes(ctx, outro)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Errorf("a conta que nao vendeu nada recebeu %v", slots)
	}
}
