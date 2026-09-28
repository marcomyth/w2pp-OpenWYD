//go:build integration

// Testes de integração do FIM do anúncio: o que a barraca descendo faz, e o que
// a faxina do login encontra.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
)

// anuncioAtivoSimples cria um anúncio ativo, sem cobrança nenhuma, e devolve o id.
func anuncioAtivoSimples(ctx context.Context, t *testing.T, s *Store, vendedor int64, slot int16) int64 {
	t.Helper()
	var id int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO rmt_anuncio (vendedor_conta, cargo_slot, item_index, preco_centavos, status)
		VALUES ($1, $2, 1100, 5000, 1) RETURNING id`, vendedor, slot).Scan(&id); err != nil {
		t.Fatalf("criando o anuncio: %v", err)
	}
	return id
}

// itemMarcado põe um item no baú da conta com a marca do escrow apontando para o
// anúncio. É o cadeado que a faxina tem de soltar, ou não.
func itemMarcado(ctx context.Context, t *testing.T, s *Store, conta int64, slot int16, anuncio int64) {
	t.Helper()
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO item (owner_kind, account_id, slot, item_index, rmt_anuncio)
		VALUES ('account_cargo', $1, $2, 1100, $3)`, conta, slot, anuncio); err != nil {
		t.Fatalf("marcando o item no slot %d: %v", slot, err)
	}
}

func statusDoAnuncio(ctx context.Context, t *testing.T, s *Store, id int64) (int16, bool) {
	t.Helper()
	var st int16
	var caiu bool
	if err := s.pool.QueryRow(ctx,
		`SELECT status, barraca_caiu FROM rmt_anuncio WHERE id = $1`, id).Scan(&st, &caiu); err != nil {
		t.Fatalf("lendo o anuncio %d: %v", id, err)
	}
	return st, caiu
}

// A BARRACA DESCE E O ANÚNCIO SEM DINHEIRO EM JOGO É CANCELADO.
//
// É o caso que faltava no sistema inteiro: sem ele o anúncio fica ativo para
// sempre e a marca deixa o item intocável para sempre junto. O jogo pedia ao
// jogador que cancelasse o anúncio, e não existia como.
func TestBarracaQueDesceCancelaOAnuncioSemCobranca(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_fecha")
	id := anuncioAtivoSimples(ctx, t, s, vendedor, 0)

	fim, err := s.EncerrarAnunciosRMT(ctx, []int64{id})
	if err != nil {
		t.Fatalf("encerrando: %v", err)
	}

	if len(fim) != 1 || fim[0].AnuncioID != id {
		t.Fatalf("encerrados = %+v, quero o anuncio %d", fim, id)
	}
	if fim[0].CobrancaAberta {
		t.Error("disse que havia cobranca aberta, e nao havia nenhuma")
	}
	if fim[0].CargoSlot != 0 {
		t.Errorf("slot = %d, quero 0 — quem chamou precisa dele para achar o cadeado", fim[0].CargoSlot)
	}
	if st, caiu := statusDoAnuncio(ctx, t, s, id); st != anuncioCancelado || caiu {
		t.Errorf("status = %d, barraca_caiu = %v; quero cancelado(%d) e sem a marca de espera",
			st, caiu, anuncioCancelado)
	}
}

// COM COBRANÇA ABERTA O ANÚNCIO CONTINUA VIVO, e só ganha a marca de que a
// barraca caiu.
//
// Cancelar aqui seria o pior caso do sistema: o comprador está com o QR na mão,
// o Pix dele cai daqui a um minuto, e a confirmação não encontra anúncio ativo —
// vira PAGA_SEM_ITEM, que é dívida com uma pessoa que pagou direito.
func TestBarracaQueDesceNaoCancelaComCobrancaAberta(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_espera")
	comprador := contaPix(ctx, t, s, "comprador_espera")
	id := anuncioComCobrancaAberta(ctx, t, s, vendedor, comprador, "ref-espera-1")

	fim, err := s.EncerrarAnunciosRMT(ctx, []int64{id})
	if err != nil {
		t.Fatalf("encerrando: %v", err)
	}

	if len(fim) != 1 || !fim[0].CobrancaAberta {
		t.Fatalf("encerrados = %+v, quero um com CobrancaAberta", fim)
	}
	st, caiu := statusDoAnuncio(ctx, t, s, id)
	if st != anuncioAtivo {
		t.Errorf("status = %d, quero ativo(%d): o Pix atrasado ainda tem de encontrar o que entregar",
			st, anuncioAtivo)
	}
	if !caiu {
		t.Error("o anuncio continuou ativo e ninguem registrou que a barraca caiu; o cadeado nunca sairia")
	}
}

// Encerrar o que já não está ativo não mexe em nada. A barraca pode descer
// depois de a venda ter acontecido — e um cancelamento aqui apagaria a prova de
// uma venda.
func TestEncerrarNaoMexeNoQueJaSaiu(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_vendido")
	id := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, id, anuncioVendido); err != nil {
		t.Fatal(err)
	}

	fim, err := s.EncerrarAnunciosRMT(ctx, []int64{id})
	if err != nil {
		t.Fatalf("encerrando: %v", err)
	}

	if len(fim) != 0 {
		t.Errorf("encerrados = %+v, quero nenhum", fim)
	}
	if st, _ := statusDoAnuncio(ctx, t, s, id); st != anuncioVendido {
		t.Errorf("o vendido virou %d; encerrar apagou a prova de uma venda", st)
	}
}

// A RECONCILIAÇÃO: o que ela solta e o que ela deixa preso.
//
// Um teste só com os casos lado a lado, porque o valor está na DIFERENÇA entre
// eles. Separados, cada um passaria com uma função que sempre responde a mesma
// coisa.
//
// NÃO EXISTE AQUI O CASO "anúncio ativo numa barraca de pé", e a ausência é o
// desenho: esta função roda no LOGIN do vendedor, e quem está entrando não tem
// barraca. Um anúncio ativo dele é, neste instante, um anúncio sem vitrine — e o
// único motivo de ele ainda estar ativo é uma cobrança em jogo. Chamar esta
// função com o vendedor em jogo e barraca de pé CANCELARIA a venda dele; é por
// isso que o único chamador é o login.
func TestReconciliarSoltaOCadeadoMortoEDeixaOVivo(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_faxina")
	comprador := contaPix(ctx, t, s, "comprador_faxina")
	// DOIS COMPRADORES, e não um, porque a 0116 só permite UMA cobrança aberta por
	// comprador — e é assim que o jogo produz este cenário de verdade: um comprador
	// está com o QR na mão de um item, e a cobrança de OUTRO comprador, em outro
	// item, venceu. Com um comprador só, este arranjo é impossível, e um teste que o
	// monta não prova nada sobre o sistema.
	outroComprador := contaPix(ctx, t, s, "comprador_faxina_2")

	// slot 0: anúncio cancelado — o cadeado é lixo, solta.
	cancelado := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, cancelado, anuncioCancelado); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 0, cancelado)

	// slot 2: anúncio ativo, barraca caída, cobrança ainda aberta — alguém pode
	// estar com o QR na mão. Fica.
	comQR := anuncioComCobrancaAberta(ctx, t, s, vendedor, comprador, "ref-faxina-1")
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET cargo_slot = 2, barraca_caiu = TRUE WHERE id = $1`, comQR); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 2, comQR)

	// slot 3: anúncio ativo, barraca caída, a cobrança expirou. Ninguém mais vai
	// pagar. Solta.
	expirou := anuncioComCobrancaAberta(ctx, t, s, vendedor, outroComprador, "ref-faxina-2")
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET cargo_slot = 3, barraca_caiu = TRUE WHERE id = $1`, expirou); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET status = $2 WHERE anuncio_id = $1`, expirou, cobrancaExpirada); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 3, expirou)

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 2 || slots[0] != 0 || slots[1] != 3 {
		t.Errorf("slots = %v, quero [0 3]: o cancelado e o que a cobranca expirou", slots)
	}
}

// O VENDIDO NÃO ENTRA NA FAXINA, e esta é a distinção mais cara do arquivo.
//
// A faxina DEVOLVE o item ao dono. O vendido tem de ser RETIRADO: ele já foi pago
// e já foi entregue a outra pessoa, e devolvê-lo criaria a segunda cópia que o
// escrow inteiro existe para impedir. São duas listas porque são dois destinos
// opostos.
func TestFaxinaNaoDevolveOQueFoiVendido(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_vendeu")
	id := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, id, anuncioVendido); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 0, id)

	solta, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}
	retira, err := s.SlotsVendidosPendentes(ctx, vendedor)
	if err != nil {
		t.Fatalf("vendidos: %v", err)
	}

	if len(solta) != 0 {
		t.Errorf("a faxina quer devolver %v ao vendedor; o item ja foi pago por outra pessoa", solta)
	}
	if len(retira) != 1 || retira[0] != 0 {
		t.Errorf("os vendidos = %v, quero [0]", retira)
	}
}

// Marca apontando para anúncio que NÃO EXISTE é lixo, e lixo prende o item do
// mesmo jeito. Sem este caso o jogador ficaria com um item travado e nada que o
// explicasse — nem no jogo, nem no banco.
func TestFaxinaSoltaMarcaOrfa(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_orfao")
	itemMarcado(ctx, t, s, vendedor, 7, 999999) // anúncio que nunca existiu

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 1 || slots[0] != 7 {
		t.Errorf("slots = %v, quero [7]: marca sem anuncio prende o item a toa", slots)
	}
}
