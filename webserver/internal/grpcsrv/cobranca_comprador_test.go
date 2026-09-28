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
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
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

// O VALOR DIVERGENTE VIRA PAID_LATE, e não OPEN.
//
// A cobrança continua ABERTA no banco — ela não se resolveu —, mas OPEN diria
// "pague" a alguém que já pagou, e convidaria a pagar duas vezes. PAID_LATE diz o
// que de fato aconteceu: o dinheiro chegou, o item não foi entregue, e alguém está
// olhando.
//
// E o refund_state fica UNSPECIFIED, que é o certo: não há reembolso automático
// para valor divergente, ao contrário do pagamento atrasado.
func TestValorDivergenteViraPaidLateENaoOpen(t *testing.T) {
	f := &fakePix{temCobranca: true,
		cobranca: store.CobrancaDoComprador{Estado: store.EstadoCobrancaValorDivergente}}

	resp, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("GetMyCurrentPixCharge: %v", err)
	}

	if resp.GetState() == webv1.PixChargeState_PIX_CHARGE_STATE_OPEN {
		t.Fatal("estado OPEN diz \"pague\" a quem ja pagou; convida a pagar duas vezes")
	}
	if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_PAID_LATE {
		t.Errorf("estado = %v, quero PAID_LATE", resp.GetState())
	}
	if resp.GetRefundState() != webv1.RefundState_REFUND_STATE_UNSPECIFIED {
		t.Errorf("refund_state = %v; nao ha reembolso automatico para valor divergente",
			resp.GetRefundState())
	}
}

// A RECUSA DEFINITIVA FECHA A COBRANÇA, em vez de repetir para sempre.
//
// O defeito de 24/09/2026: a criação do Pix tratava TODA falha como tropeço de rede,
// porque a página relê a cada cinco segundos e um erro passageiro se resolve na
// tentativa seguinte. Certo para o passageiro. Só que a referência saía num formato
// que a ponte não aceita, e o servidor repetiu a MESMA chamada recusada a cada cinco
// segundos, indefinidamente — "gerando o código" para sempre, sem explicação e com o
// comprador preso a uma cobrança que nunca ia nascer.
func TestRecusaDefinitivaFechaACobranca(t *testing.T) {
	casos := []struct {
		nome   string
		err    error
		fecha  bool
		porque string
	}{
		{
			nome:   "400 da ponte fecha",
			err:    &ponte.ErroHTTP{Rota: "/cobranca", Codigo: 400, Corpo: `{"estado":"recusado"}`},
			fecha:  true,
			porque: "repetir a mesma chamada recusada nao muda o resultado",
		},
		{
			nome:   "429 NAO fecha",
			err:    &ponte.ErroHTTP{Rota: "/cobranca", Codigo: 429},
			fecha:  false,
			porque: "limite de taxa e o caso mais passageiro que existe; fechar puniria o comprador por um aperto nosso",
		},
		{
			nome:   "500 NAO fecha",
			err:    &ponte.ErroHTTP{Rota: "/cobranca", Codigo: 500},
			fecha:  false,
			porque: "o outro lado tropecou, e a tentativa de cinco segundos depois pode dar",
		},
		{
			nome:   "incerta NAO fecha",
			err:    ponte.ErrIncerta,
			fecha:  false,
			porque: "a chamada pode ter saido e a resposta ter se perdido: pode existir cobranca do outro lado, e fechar seria esquecer um pagamento possivel",
		},
		{
			nome:   "erro de rede NAO fecha",
			err:    errors.New("dial tcp: connection refused"),
			fecha:  false,
			porque: "sem resposta HTTP nao ha recusa; e rede",
		},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &fakePix{
				temCobranca: true,
				cobranca: store.CobrancaDoComprador{
					CobrancaID: 77, ValorCentavos: 5000,
					ExpiraEm: time.Now().Add(10 * time.Minute),
					Estado:   store.EstadoCobrancaAberta,
				},
				erroCriarPix: c.err,
			}
			// COM O CRIADOR LIGADO, e isto nao e detalhe: sem ele a criacao inteira
			// e pulada (s.criarPix != nil, chavepix.go:276) e o teste passaria sem
			// exercitar nada. Foi o que aconteceu na primeira versao deste teste —
			// quatro dos cinco casos "passaram" a toa.
			resp, err := NewRmt(f).ComCriadorDePix(criadorInerte(), nil, nil).
				GetMyCurrentPixCharge(context.Background(),
					&webv1.GetMyCurrentPixChargeRequest{AccountId: 1})
			if err != nil {
				t.Fatalf("a leitura da pagina nao pode virar erro: %v", err)
			}

			if c.fecha {
				if f.fechouPorRecusa != 1 {
					t.Errorf("fechou %d vezes, queria 1 — %s", f.fechouPorRecusa, c.porque)
				}
				if f.fechadaPorRecusa != 77 {
					t.Errorf("fechou a cobranca %d, queria 77", f.fechadaPorRecusa)
				}
				if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_CANCELED {
					t.Errorf("estado = %v, queria CANCELED", resp.GetState())
				}
				// E SEM CÓDIGO: um código ao lado de uma cobrança morta convida
				// alguém a pagar por nada.
				if resp.GetPixCode() != "" {
					t.Errorf("veio codigo numa cobranca fechada: %q", resp.GetPixCode())
				}
				return
			}
			if f.fechouPorRecusa != 0 {
				t.Errorf("fechou %d vezes e nao devia — %s", f.fechouPorRecusa, c.porque)
			}
			// E continua ABERTA, para a releitura tentar de novo com a mesma
			// referência.
			if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_OPEN {
				t.Errorf("estado = %v, queria OPEN: a releitura precisa poder tentar de novo",
					resp.GetState())
			}
		})
	}
}
