package grpcsrv

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// SEM COBRANÇA É RESPOSTA VAZIA E NÃO ERRO.
//
// Este é o caso COMUM e não a exceção: quem pergunta é a página da conta, que as
// pessoas abrem para ver outras coisas. Erro aqui poria vermelho na tela de quase
// todo mundo, quase sempre.
func TestSemCobrancaRespostaVaziaENaoErro(t *testing.T) {
	f := &fakePix{temCobranca: false}
	resp, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("erro numa conta sem cobranca: %v", err)
	}
	if resp.GetHasCharge() {
		t.Error("disse que tem cobranca quando nao tem")
	}
	if resp.GetPixCode() != "" || resp.GetAmountCents() != 0 {
		t.Errorf("resposta vazia veio com conteudo: %+v", resp)
	}
}

// A cobrança aberta chega inteira, com o código intocado.
//
// O copia-e-cola tem checksum de ponta a ponta: um servidor que o "arrume" quebra
// o pagamento de um jeito que ninguém vê até o dinheiro não chegar. O teste passa
// uma string com espaço e caixa mista de propósito.
func TestCobrancaAbertaChegaComOCodigoIntocado(t *testing.T) {
	const codigo = "00020126 BR.GOV.BCB.PIX abcDEF==+/ 6304AbCd"
	prazo := time.Now().Add(4 * time.Minute).Truncate(time.Second)
	f := &fakePix{
		temCobranca: true,
		cobranca: store.CobrancaDoComprador{
			CodigoPix:     codigo,
			ValorCentavos: 12345,
			ExpiraEm:      prazo,
			Estado:        store.EstadoCobrancaAberta,
			ItemIndex:     1030,
			Refino:        9,
			Quantidade:    3,
			VendedorNome:  "Vendedor",
		},
	}

	resp, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("GetMyCurrentPixCharge: %v", err)
	}

	if !resp.GetHasCharge() {
		t.Fatal("nao devolveu a cobranca aberta")
	}
	if resp.GetPixCode() != codigo {
		t.Errorf("o codigo chegou %q; tem de chegar byte a byte como veio", resp.GetPixCode())
	}
	if resp.GetAmountCents() != 12345 {
		t.Errorf("valor = %d centavos, quero 12345", resp.GetAmountCents())
	}
	if resp.GetExpiresAt() != prazo.Unix() {
		t.Errorf("prazo = %d, quero %d — é PRAZO e não contagem", resp.GetExpiresAt(), prazo.Unix())
	}
	if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_OPEN {
		t.Errorf("estado = %v, quero OPEN", resp.GetState())
	}
	if resp.GetItemIndex() != 1030 || resp.GetRefineLevel() != 9 || resp.GetStackSize() != 3 {
		t.Errorf("item = %d refino = %d qtd = %d; quero 1030/9/3",
			resp.GetItemIndex(), resp.GetRefineLevel(), resp.GetStackSize())
	}
	if resp.GetSellerName() != "Vendedor" {
		t.Errorf("vendedor = %q", resp.GetSellerName())
	}
}

// TODO ESTADO FECHADO TEM NOME, e nenhum deles é resposta vazia.
//
// Cada um é uma FRASE DIFERENTE que a página tem de conseguir dizer a alguém que
// acabou de mandar dinheiro. Os quatro lado a lado porque o valor está na
// diferença: uma conversão que devolvesse sempre o mesmo passaria em qualquer um
// deles isolado.
func TestCadaFechamentoTemSeuEstado(t *testing.T) {
	casos := []struct {
		nome     string
		daLoja   store.EstadoCobrancaComprador
		noWire   webv1.PixChargeState
		porqueNa string
	}{
		{"pago", store.EstadoCobrancaPaga, webv1.PixChargeState_PIX_CHARGE_STATE_PAID,
			"acabou de mandar dinheiro; tela em branco aqui e o pior momento possivel"},
		{"cancelado", store.EstadoCobrancaCancelada, webv1.PixChargeState_PIX_CHARGE_STATE_CANCELED,
			"saiu do jogo para pagar no celular e a cobranca fechou"},
		{"expirado", store.EstadoCobrancaExpirada, webv1.PixChargeState_PIX_CHARGE_STATE_EXPIRED,
			"o prazo acabou, e isso e uma explicacao"},
		{"pago atrasado", store.EstadoCobrancaPagaSemItem, webv1.PixChargeState_PIX_CHARGE_STATE_PAID_LATE,
			"o dinheiro chegou tarde e esta em analise"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &fakePix{temCobranca: true, cobranca: store.CobrancaDoComprador{Estado: c.daLoja}}
			resp, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
				&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})
			if err != nil {
				t.Fatalf("GetMyCurrentPixCharge: %v", err)
			}
			if !resp.GetHasCharge() {
				t.Errorf("esvaziou a resposta: %s", c.porqueNa)
			}
			if resp.GetState() != c.noWire {
				t.Errorf("estado = %v, quero %v", resp.GetState(), c.noWire)
			}
		})
	}
}

// Estado que a camada de banco não conhece NÃO vira um estado com significado.
//
// O zero do enum é UNSPECIFIED de propósito: um status novo no banco que caísse em
// OPEN por descuido mostraria um código de pagamento para uma cobrança que ninguém
// sabe o que é.
func TestEstadoDesconhecidoNaoViraAberta(t *testing.T) {
	f := &fakePix{temCobranca: true,
		cobranca: store.CobrancaDoComprador{Estado: store.EstadoCobrancaDesconhecido}}
	resp, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("GetMyCurrentPixCharge: %v", err)
	}
	if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_UNSPECIFIED {
		t.Errorf("estado = %v, quero UNSPECIFIED", resp.GetState())
	}
}

// Falha de banco VIRA erro, ao contrário das recusas previstas. É a mesma divisão
// do SavePixKey: a página precisa distinguir "não tem nada" de "não deu para
// saber".
func TestFalhaDeBancoViraErro(t *testing.T) {
	f := &fakePix{erroCobranca: errors.New("banco fora do ar")}
	_, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})
	if status.Code(err) != codes.Internal {
		t.Errorf("codigo = %v, quero Internal", status.Code(err))
	}
}

// A janela da cobrança recente é decidida pela camada de banco, não pelo serviço.
//
// Passar zero é o jeito de dizer "use a configuração". Se este serviço mandasse um
// número, haveria duas respostas para a mesma pergunta e a do banco perderia sem
// ninguém notar.
func TestServicoNaoDecideAJanela(t *testing.T) {
	f := &fakePix{}
	if _, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7}); err != nil {
		t.Fatal(err)
	}
	if f.janelaPedida != 0 {
		t.Errorf("o servico mandou a janela %v; quem decide e a camada de banco", f.janelaPedida)
	}
}
