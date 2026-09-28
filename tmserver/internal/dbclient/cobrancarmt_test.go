package dbclient

import (
	"context"
	"errors"
	"testing"
	"time"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/handler"
)

// A TRADUÇÃO DAS RECUSAS É O TRABALHO DESTE CLIENTE, e é o que este teste cobre.
//
// Cada recusa é uma frase diferente para o jogador, e elas mandam ações diferentes:
// "esse item acabou de ser vendido" manda olhar a vitrine de novo, "você já tem um
// pagamento aberto" manda terminar o que começou, "você não pode comprar de si mesmo"
// é outra coisa ainda. Trocar uma pela outra faz o jogador tentar o que não resolve.
func TestAbrirCobrancaTraduzCadaRecusa(t *testing.T) {
	casos := []struct {
		nome string
		res  dbv1.OpenRmtChargeResult
		quer error
	}{
		{"outro comprador pegou o item",
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_TAKEN,
			handler.ErrCobrancaJaAberta},
		{"comprando de si mesmo",
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_SELF_PURCHASE,
			handler.ErrCompradorEOVendedor},
		{"ja tem pagamento aberto",
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_BUYER_BUSY,
			handler.ErrCompradorJaTemCobranca},
		{"anuncio sumiu",
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_GONE,
			handler.ErrAnuncioIndisponivel},
		// O cadeado que não aponta para o anúncio CASA com o genérico, porque o
		// ErrCadeadoNaoBate o embrulha — é isso que faz o jogador ler a mesma frase.
		{"cadeado nao aponta para o anuncio",
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ITEM_NOT_LOCKED,
			handler.ErrAnuncioIndisponivel},
	}
	for _, c := range casos {
		api := &fakeAPI{cobrancaResp: &dbv1.OpenRmtChargeResponse{Result: c.res}}
		_, err := newClient(api).AbrirCobranca(context.Background(), 10, 20, "ref-1")
		if !errors.Is(err, c.quer) {
			t.Errorf("%s: erro = %v, quero %v", c.nome, err, c.quer)
		}
	}
}

// OK e ALREADY_OPEN são os DOIS sucessos, e o segundo é o que faz a idempotência
// valer: a rede engasgou, o clique repetiu, e a MESMA cobrança volta. Tratá-la como
// erro mostraria uma falha ao jogador por uma coisa que deu certo.
func TestAbrirCobrancaAceitaOsDoisSucessos(t *testing.T) {
	prazo := time.Now().Add(5 * time.Minute).Truncate(time.Second)
	for _, res := range []dbv1.OpenRmtChargeResult{
		dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_OK,
		dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ALREADY_OPEN,
	} {
		api := &fakeAPI{cobrancaResp: &dbv1.OpenRmtChargeResponse{
			Result: res, ChargeId: 77, AmountCents: 5000, ExpiresAt: prazo.Unix(),
		}}
		cob, err := newClient(api).AbrirCobranca(context.Background(), 10, 20, "ref-1")
		if err != nil {
			t.Fatalf("%v: erro inesperado: %v", res, err)
		}
		if cob.CobrancaID != 77 {
			t.Errorf("%v: cobranca = %d, quero 77", res, cob.CobrancaID)
		}
		if !cob.ExpiraEm.Equal(prazo) {
			t.Errorf("%v: expira em %v, quero %v", res, cob.ExpiraEm, prazo)
		}
		// O CÓDIGO PIX NÃO VEM DAQUI, e o campo tem de ficar VAZIO. Ele nasce na
		// primeira leitura da página do comprador, no site. Um código preenchido aqui
		// seria inventado, e a mensagem que o jogo mostra não cita código nenhum.
		if cob.CodigoPix != "" {
			t.Errorf("%v: veio codigo pix %q do servidor de jogo", res, cob.CodigoPix)
		}
	}
}

// UM RESULTADO DESCONHECIDO NÃO PODE VIRAR "A COBRANÇA ABRIU". Isso mandaria o
// jogador pagar por uma linha que talvez não exista — e o zero do enum, que é o que
// chega de um campo não preenchido, cai justamente aqui.
func TestAbrirCobrancaRecusaResultadoDesconhecido(t *testing.T) {
	for _, res := range []dbv1.OpenRmtChargeResult{
		dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_UNSPECIFIED,
		dbv1.OpenRmtChargeResult(99),
	} {
		api := &fakeAPI{cobrancaResp: &dbv1.OpenRmtChargeResponse{Result: res, ChargeId: 77}}
		cob, err := newClient(api).AbrirCobranca(context.Background(), 10, 20, "ref-1")
		if err == nil {
			t.Errorf("%v: aceitou como sucesso, devolvendo %+v", res, cob)
		}
		if cob.CobrancaID != 0 {
			t.Errorf("%v: devolveu cobranca %d junto com a recusa", res, cob.CobrancaID)
		}
	}
}

// O PEDIDO LEVA A JANELA, e não deixa o prazo só do lado do banco: o MESMO número
// tem de valer para a mensagem do jogo e para a varredura que expira a cobrança
// noutro processo. Um valor que morasse num lado sairia de sincronia com o outro sem
// ninguém notar.
func TestAbrirCobrancaMandaAJanelaEAReferencia(t *testing.T) {
	anterior := handler.JanelaDeCobranca
	handler.DefineJanelaDeCobranca(7 * time.Minute)
	defer handler.DefineJanelaDeCobranca(anterior)

	api := &fakeAPI{cobrancaResp: &dbv1.OpenRmtChargeResponse{
		Result: dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_OK, ChargeId: 1,
	}}
	if _, err := newClient(api).AbrirCobranca(context.Background(), 10, 20, "ref-abc"); err != nil {
		t.Fatal(err)
	}

	req := api.cobrancaPedida
	if req == nil {
		t.Fatal("nao mandou pedido nenhum")
	}
	if req.GetWindowSeconds() != 420 {
		t.Errorf("janela = %d s, quero 420", req.GetWindowSeconds())
	}
	if req.GetExternalReference() != "ref-abc" {
		t.Errorf("referencia = %q", req.GetExternalReference())
	}
	if req.GetListingId() != 10 || req.GetBuyerAccountId() != 20 {
		t.Errorf("anuncio = %d comprador = %d", req.GetListingId(), req.GetBuyerAccountId())
	}
}

// Falha de transporte vira erro, e não recusa de negócio: se a rede cair e a loja
// disser "esse item já tem comprador", o jogador desiste de um item que está livre.
func TestAbrirCobrancaFalhaDeTransporteViraErro(t *testing.T) {
	api := &fakeAPI{cobrancaErro: errors.New("dbserver fora do ar")}

	_, err := newClient(api).AbrirCobranca(context.Background(), 10, 20, "ref-1")
	if err == nil {
		t.Fatal("a falha de transporte nao virou erro")
	}
	for _, sentinela := range []error{
		handler.ErrCobrancaJaAberta, handler.ErrCompradorEOVendedor,
		handler.ErrCompradorJaTemCobranca, handler.ErrAnuncioIndisponivel,
	} {
		if errors.Is(err, sentinela) {
			t.Errorf("a falha de rede virou a recusa de negocio %v", sentinela)
		}
	}
}

// E O CADEADO QUE NÃO BATE É DISTINGUÍVEL DO ANÚNCIO QUE SUMIU, mesmo os dois lendo
// igual para o jogador.
//
// A distinção não é preciosismo: anúncio vendido é curso normal e acontece o dia
// inteiro; cadeado apontando para outro lugar com o anúncio ativo é DEFEITO NOSSO.
// Colapsar os dois num erro só faria esse defeito viver para sempre dentro do caso
// comum, e a loja não teria como saber que precisa avisar.
func TestCadeadoQueNaoBateEDistinguivelDoAnuncioQueSumiu(t *testing.T) {
	semCadeado := &fakeAPI{cobrancaResp: &dbv1.OpenRmtChargeResponse{
		Result: dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ITEM_NOT_LOCKED}}
	_, err := newClient(semCadeado).AbrirCobranca(context.Background(), 10, 20, "ref-1")
	if !errors.Is(err, handler.ErrCadeadoNaoBate) {
		t.Errorf("o cadeado que nao bate nao e distinguivel: %v", err)
	}
	if !errors.Is(err, handler.ErrAnuncioIndisponivel) {
		t.Error("deixou de casar com o generico; o jogador veria outra frase")
	}

	// E o contrário: o anúncio que sumiu NÃO pode virar o erro do defeito, senão o
	// log enche de aviso por uma coisa que é o curso normal do mercado.
	sumiu := &fakeAPI{cobrancaResp: &dbv1.OpenRmtChargeResponse{
		Result: dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_GONE}}
	_, err = newClient(sumiu).AbrirCobranca(context.Background(), 10, 20, "ref-1")
	if errors.Is(err, handler.ErrCadeadoNaoBate) {
		t.Error("o anuncio vendido virou aviso de defeito; o log encheria de ruido")
	}
}
