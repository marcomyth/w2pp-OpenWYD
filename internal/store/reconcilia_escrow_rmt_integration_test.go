//go:build integration

// A reconciliação do escrow: os buracos que o encerramento da barraca não fecha
// porque o vendedor não estava lá.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"testing"
)

// O ANÚNCIO ÓRFÃO PRENDIA O SLOT PARA SEMPRE, e soltar só o cadeado não
// resolvia.
//
// O anúncio continuava ATIVO, e o índice `rmt_anuncio_um_ativo_por_slot` então
// recusava todo anúncio novo naquele slot. O jogador veria "não deu para montar a
// barraca" para sempre, sem nada no jogo que explicasse por quê — e o item
// destravado só tornaria o mistério pior.
//
// A prova é a de baixo e não a de cima: depois da reconciliação, um anúncio NOVO
// no mesmo slot tem de nascer.
func TestReconciliarLiberaOSlotParaAnunciarDeNovo(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_orfao_slot")
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF); err != nil {
		t.Fatal(err)
	}
	// Um anúncio ativo com a barraca já caída e sem cobrança: o servidor reiniciou
	// com a barraca de pé, ou o encerramento falhou.
	orfao := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET barraca_caiu = TRUE WHERE id = $1`, orfao); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 0, orfao)

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 1 || slots[0] != 0 {
		t.Errorf("cadeados a soltar = %v, quero [0]", slots)
	}
	if st, _ := statusDoAnuncio(ctx, t, s, orfao); st != anuncioCancelado {
		t.Errorf("o anuncio orfao ficou no status %d; o indice de um ativo por slot "+
			"continuaria recusando todo anuncio novo naquele slot", st)
	}
	// A prova que importa: dá para anunciar de novo.
	if _, err := s.AbrirAnunciosRMT(ctx, vendedor,
		[]ItemAnunciado{{CargoSlot: 0, ItemIndex: 1100, PrecoCentavos: 5000}}); err != nil {
		t.Errorf("o slot continua preso: %v", err)
	}
}

// ANÚNCIO ATIVO SEM MARCA — o servidor caiu entre criar o anúncio e salvar o baú.
//
// A varredura antiga não o via, porque ela partia do ITEM marcado e aqui não há
// item marcado. Este parte do BANCO, e por isso o enxerga.
//
// Sem conserto, este anúncio fica na vitrine prometendo um item que nada segura:
// não vende (a confirmação confere a marca), mas prende o slot pelo índice.
func TestReconciliarFechaAnuncioAtivoSemMarca(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_sem_marca")
	id := anuncioAtivoSimples(ctx, t, s, vendedor, 3)
	// Nenhum itemMarcado: é exatamente esse o caso.

	if _, err := s.ReconciliarEscrowRMT(ctx, vendedor); err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if st, _ := statusDoAnuncio(ctx, t, s, id); st != anuncioCancelado {
		t.Errorf("status = %d, quero cancelado(%d): anuncio ativo sem cadeado promete "+
			"um item que nada segura", st, anuncioCancelado)
	}
}

// O RELOGIN RÁPIDO: o vendedor volta antes de o encerramento da saída commitar.
//
// Na volta, o anúncio ainda está ATIVO e não barraca_caiu — do ponto de vista do
// banco, nada aconteceu. Sem a varredura de anúncios ativos, a reconciliação
// partiria do item marcado, veria um anúncio vivo, e deixaria o cadeado onde
// está: o item ficaria preso até um login em que a sorte fosse outra.
//
// Quem resolve é o gancho ser o LOGIN: quem está entrando não tem barraca, então
// anúncio ativo dele é anúncio sem vitrine, e acaba aqui.
func TestReloginAntesDoEncerramentoDestravaOItem(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_relogin")
	// O estado de quem acabou de cair com a barraca de pé: anúncio ATIVO, sem
	// barraca_caiu, com o item marcado. É o que o banco tem se o encerramento não
	// chegou a rodar.
	id := anuncioAtivoSimples(ctx, t, s, vendedor, 1)
	itemMarcado(ctx, t, s, vendedor, 1, id)

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 1 || slots[0] != 1 {
		t.Errorf("cadeados a soltar = %v, quero [1]: o item tem de destravar nesta entrada", slots)
	}
	if st, _ := statusDoAnuncio(ctx, t, s, id); st != anuncioCancelado {
		t.Errorf("status = %d, quero cancelado(%d)", st, anuncioCancelado)
	}
}

// COM COBRANÇA ABERTA A RECONCILIAÇÃO NÃO SOLTA NADA, e nem cancela.
//
// É a mesma regra do encerramento, e ela vale aqui com mais força: o comprador
// pode estar com o QR na mão neste exato instante, e o vendedor entrando em jogo
// não é motivo para desfazer a venda dele.
func TestReconciliarNaoDesfazVendaComQRAberto(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_qr")
	comprador := contaPix(ctx, t, s, "comprador_qr")
	id := anuncioComCobrancaAberta(ctx, t, s, vendedor, comprador, "ref-qr-1")
	itemMarcado(ctx, t, s, vendedor, 0, id)

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 0 {
		t.Errorf("quer soltar %v com uma cobranca aberta; o Pix a caminho ficaria sem o que entregar", slots)
	}
	st, caiu := statusDoAnuncio(ctx, t, s, id)
	if st != anuncioAtivo {
		t.Errorf("status = %d, quero ativo(%d)", st, anuncioAtivo)
	}
	if !caiu {
		t.Error("o vendedor entrou em jogo sem barraca e ninguem registrou que a barraca caiu")
	}
}

// E O VENDIDO CONTINUA FORA. A reconciliação DEVOLVE o item; o vendido tem de ser
// RETIRADO, porque já foi pago e entregue a outra pessoa.
func TestReconciliarNaoDevolveOVendido(t *testing.T) {
	s, ctx := freshStore(t)
	vendedor := contaPix(ctx, t, s, "vendedor_vendido_rec")
	id := anuncioAtivoSimples(ctx, t, s, vendedor, 0)
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_anuncio SET status = $2 WHERE id = $1`, id, anuncioVendido); err != nil {
		t.Fatal(err)
	}
	itemMarcado(ctx, t, s, vendedor, 0, id)

	slots, err := s.ReconciliarEscrowRMT(ctx, vendedor)
	if err != nil {
		t.Fatalf("reconciliando: %v", err)
	}

	if len(slots) != 0 {
		t.Errorf("quer devolver %v ao vendedor; o item ja foi pago por outra pessoa", slots)
	}
	if st, _ := statusDoAnuncio(ctx, t, s, id); st != anuncioVendido {
		t.Errorf("o vendido virou %d; a reconciliacao apagou a prova de uma venda", st)
	}
}

// O BOOT varre o mundo inteiro.
//
// Uma queda com barracas de pé deixaria todo aquele estoque anunciado para
// sempre: as barracas morrem com o processo, os anúncios ficam, e os slots ficam
// presos. Os donos só descobririam ao tentar montar de novo.
func TestReconciliarNoBootFechaOQueSobreviveuAQueda(t *testing.T) {
	s, ctx := freshStore(t)
	a := contaPix(ctx, t, s, "vendedor_boot_a")
	b := contaPix(ctx, t, s, "vendedor_boot_b")
	comprador := contaPix(ctx, t, s, "comprador_boot")

	semCobranca1 := anuncioAtivoSimples(ctx, t, s, a, 0)
	semCobranca2 := anuncioAtivoSimples(ctx, t, s, b, 0)
	// O com cobrança vai para OUTRA conta: o helper cria sempre no slot 0, e o
	// índice de um ativo por slot — que é exatamente a invariante que este
	// trabalho existe para respeitar — recusaria o segundo na mesma conta.
	terceiro := contaPix(ctx, t, s, "vendedor_boot_c")
	comCobranca := anuncioComCobrancaAberta(ctx, t, s, terceiro, comprador, "ref-boot-1")

	cancelados, esperando, err := s.ReconciliarEscrowNoBoot(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	if cancelados != 2 || esperando != 1 {
		t.Errorf("cancelados = %d, esperando = %d; quero 2 e 1", cancelados, esperando)
	}
	for _, id := range []int64{semCobranca1, semCobranca2} {
		if st, _ := statusDoAnuncio(ctx, t, s, id); st != anuncioCancelado {
			t.Errorf("o anuncio %d sobreviveu a queda no status %d", id, st)
		}
	}
	st, caiu := statusDoAnuncio(ctx, t, s, comCobranca)
	if st != anuncioAtivo || !caiu {
		t.Errorf("o anuncio com QR aberto ficou status=%d barraca_caiu=%v; "+
			"quero ativo e marcado, para o Pix atrasado ainda entregar", st, caiu)
	}
}

// E o boot é IDEMPOTENTE: rodar duas vezes não conta o mesmo anúncio duas vezes
// nem mexe no que já fechou. Um boot é a coisa mais fácil de repetir que existe.
func TestReconciliarNoBootPodeRodarDuasVezes(t *testing.T) {
	s, ctx := freshStore(t)
	a := contaPix(ctx, t, s, "vendedor_boot2")
	anuncioAtivoSimples(ctx, t, s, a, 0)

	if _, _, err := s.ReconciliarEscrowNoBoot(ctx); err != nil {
		t.Fatalf("primeiro boot: %v", err)
	}
	cancelados, esperando, err := s.ReconciliarEscrowNoBoot(ctx)
	if err != nil {
		t.Fatalf("segundo boot: %v", err)
	}

	if cancelados != 0 || esperando != 0 {
		t.Errorf("o segundo boot mexeu em %d/%d linhas; quero 0 e 0", cancelados, esperando)
	}
}
