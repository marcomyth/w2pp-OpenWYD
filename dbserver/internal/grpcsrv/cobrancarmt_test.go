package grpcsrv

import (
	"context"
	"errors"
	"testing"
	"time"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// CADA RESULTADO DO BANCO TEM DE SAIR COMO O SEU, e não como outro.
//
// Este teste nasceu de uma sabotagem que PASSOU: trocar "comprando de si mesmo" por
// OK não quebrava nada, porque este mapa não tinha teste nenhum. O estrago dessa
// troca é direto — o jogo diria ao jogador "pague no site" por uma cobrança que foi
// recusada e não existe, e ele ficaria esperando um código que nunca vai aparecer.
//
// A tabela cobre TODOS os valores do banco de propósito: um mapeamento incompleto
// falha calado, devolvendo UNSPECIFIED, que o cliente trata como erro genérico — e aí
// a recusa certa vira "não deu".
func TestOpenRmtChargeTraduzTodosOsResultados(t *testing.T) {
	casos := []struct {
		nome  string
		banco store.ResultadoAbertura
		quer  dbv1.OpenRmtChargeResult
	}{
		{"abriu", store.CobrancaAbertaOK,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_OK},
		{"ja existia", store.CobrancaJaExistia,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ALREADY_OPEN},
		{"anuncio sumiu", store.AnuncioNaoDisponivel,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_GONE},
		{"outro esta pagando", store.AnuncioComOutraCobranca,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_TAKEN},
		{"comprando de si mesmo", store.CompradorEOVendedor,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_SELF_PURCHASE},
		{"item nao esta preso", store.ItemNaoEstaPreso,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ITEM_NOT_LOCKED},
		{"comprador ocupado", store.CompradorJaTemCobranca,
			dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_BUYER_BUSY},
	}
	for _, c := range casos {
		fs := &fakeStore{cobrancaResultado: c.banco}
		resp, err := New(fs).OpenRmtCharge(context.Background(),
			&dbv1.OpenRmtChargeRequest{ListingId: 1, BuyerAccountId: 2, ExternalReference: "r"})
		if err != nil {
			t.Fatalf("%s: erro inesperado: %v", c.nome, err)
		}
		if resp.GetResult() != c.quer {
			t.Errorf("%s: resultado = %v, quero %v", c.nome, resp.GetResult(), c.quer)
		}
	}
}

// O ID, O VALOR E O PRAZO SÓ SAEM QUANDO EXISTE COBRANÇA.
//
// Nos casos de recusa eles ficam zerados de propósito: um id de mentira seria gravado
// no log de alguém como se fosse real, e um prazo de mentira apareceria numa tela.
// Zero é obviamente "não tem".
func TestOpenRmtChargeSoPreencheOsDadosQuandoHaCobranca(t *testing.T) {
	prazo := time.Now().Add(5 * time.Minute).Truncate(time.Second)
	dados := store.CobrancaRMT{CobrancaID: 77, ValorCentavos: 5000, ExpiraEm: prazo}

	for _, r := range []store.ResultadoAbertura{store.CobrancaAbertaOK, store.CobrancaJaExistia} {
		fs := &fakeStore{cobrancaResultado: r, cobranca: dados}
		resp, err := New(fs).OpenRmtCharge(context.Background(),
			&dbv1.OpenRmtChargeRequest{ListingId: 1, BuyerAccountId: 2, ExternalReference: "r"})
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetChargeId() != 77 || resp.GetAmountCents() != 5000 ||
			resp.GetExpiresAt() != prazo.Unix() {
			t.Errorf("%v: resposta = %+v", r, resp)
		}
	}

	// A recusa leva os mesmos dados na fake, e ainda assim não pode devolvê-los.
	fs := &fakeStore{cobrancaResultado: store.CompradorJaTemCobranca, cobranca: dados}
	resp, err := New(fs).OpenRmtCharge(context.Background(),
		&dbv1.OpenRmtChargeRequest{ListingId: 1, BuyerAccountId: 2, ExternalReference: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetChargeId() != 0 || resp.GetAmountCents() != 0 || resp.GetExpiresAt() != 0 {
		t.Errorf("a recusa veio com dados de cobranca: %+v", resp)
	}
}

// A janela chega em SEGUNDOS no fio e vira Duration no banco. Um fator errado aqui
// daria uma cobrança que vence em cinco segundos ou em cinco horas, e nenhum dos dois
// aparece em teste de compilação.
func TestOpenRmtChargeConverteAJanela(t *testing.T) {
	fs := &fakeStore{cobrancaResultado: store.CobrancaAbertaOK}
	if _, err := New(fs).OpenRmtCharge(context.Background(), &dbv1.OpenRmtChargeRequest{
		ListingId: 1, BuyerAccountId: 2, ExternalReference: "r", WindowSeconds: 420,
	}); err != nil {
		t.Fatal(err)
	}
	if fs.cobrancaJanela != 7*time.Minute {
		t.Errorf("janela no banco = %v, quero 7m", fs.cobrancaJanela)
	}
	if fs.cobrancaRef != "r" || fs.cobrancaAnuncio != 1 || fs.cobrancaComprador != 2 {
		t.Errorf("chegou ref=%q anuncio=%d comprador=%d",
			fs.cobrancaRef, fs.cobrancaAnuncio, fs.cobrancaComprador)
	}
}

// Falha de infraestrutura vira ERRO, e não uma recusa de negócio. Se o banco cair e a
// resposta disser "esse item já tem comprador", o jogador desiste de um item livre.
func TestOpenRmtChargeFalhaDeInfraViraErro(t *testing.T) {
	fs := &fakeStore{cobrancaErro: errors.New("banco fora do ar")}

	if _, err := New(fs).OpenRmtCharge(context.Background(),
		&dbv1.OpenRmtChargeRequest{ListingId: 1, BuyerAccountId: 2, ExternalReference: "r"}); err == nil {
		t.Error("a falha do banco nao virou erro")
	}
}
