package grpcsrv

import (
	"context"
	"errors"
	"testing"
	"time"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// abertaSemCodigo é a cobrança recém-nascida: aberta, com prazo, e ainda sem o
// código — que é o único estado em que a leitura deve criar algo.
func abertaSemCodigo() *fakePix {
	return &fakePix{
		temCobranca: true,
		cobranca: store.CobrancaDoComprador{
			CobrancaID: 42, CodigoPix: "", ValorCentavos: 5000,
			ExpiraEm: time.Now().Add(5 * time.Minute),
			Estado:   store.EstadoCobrancaAberta,
		},
	}
}

// criadorInerte é chamado e não faz nada de interessante. Os testes que o usam
// olham SE o handler pediu a criação e com quais condições, e não o que a ponte
// devolveu — esse é papel do teste de integração do store.
func criadorInerte() CriadorDePixComTexto {
	return func(context.Context, string, int64, string) (string, string, error) {
		return "codigo-inerte", "ident-inerte", nil
	}
}

// Sem criador ligado, a leitura NÃO cria nada. É o servidor sem ponte configurada, e
// ele tem de continuar respondendo a página em vez de falhar.
func TestSemCriadorALeituraNaoCria(t *testing.T) {
	f := abertaSemCodigo()

	resp, err := NewRmt(f).GetMyCurrentPixCharge(context.Background(),
		&webv1.GetMyCurrentPixChargeRequest{AccountId: 7})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if f.criarChamadas != 0 {
		t.Errorf("pediu a criacao %d vezes sem criador ligado", f.criarChamadas)
	}
	if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_OPEN || resp.GetPixCode() != "" {
		t.Errorf("resposta = estado %v codigo %q", resp.GetState(), resp.GetPixCode())
	}
}

// Com criador ligado e cobrança aberta sem código, a leitura pede a criação e devolve
// o código gravado.
func TestLeituraCriaEDevolveOCodigo(t *testing.T) {
	f := abertaSemCodigo()
	f.pixCriado = store.PixDaCobranca{CodigoPix: "00020126", Identifier: "sp-1", Criado: true}

	resp, err := NewRmt(f).ComCriadorDePix(criadorInerte(), nil, nil).
		GetMyCurrentPixCharge(context.Background(), &webv1.GetMyCurrentPixChargeRequest{AccountId: 7})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if f.criarChamadas != 1 || f.cobrancaPedida != 42 {
		t.Errorf("chamadas=%d cobranca=%d", f.criarChamadas, f.cobrancaPedida)
	}
	if f.minimoPedido != MinimoParaCriarPix {
		t.Errorf("minimo pedido = %v, queria %v", f.minimoPedido, MinimoParaCriarPix)
	}
	if resp.GetPixCode() != "00020126" {
		t.Errorf("codigo = %q", resp.GetPixCode())
	}
}

// COBRANÇA QUE JÁ TEM CÓDIGO NÃO PEDE CRIAÇÃO. É o caso mais comum de todos: a página
// relê a cada cinco segundos, e uma releitura que pedisse criação toda vez seria uma
// chamada paga à processadora a cada cinco segundos por comprador.
func TestComCodigoNaoPedeCriacao(t *testing.T) {
	f := abertaSemCodigo()
	f.cobranca.CodigoPix = "00020126"

	if _, err := NewRmt(f).ComCriadorDePix(criadorInerte(), nil, nil).
		GetMyCurrentPixCharge(context.Background(),
			&webv1.GetMyCurrentPixChargeRequest{AccountId: 7}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if f.criarChamadas != 0 {
		t.Errorf("pediu a criacao %d vezes para uma cobranca que ja tinha codigo", f.criarChamadas)
	}
}

// ESTADO FECHADO NÃO PEDE CRIAÇÃO. Criar um código pagável para uma cobrança vencida
// ou cancelada geraria um Pix por um item que já foi solto para outra pessoa.
func TestEstadoFechadoNaoPedeCriacao(t *testing.T) {
	for _, e := range []store.EstadoCobrancaComprador{
		store.EstadoCobrancaExpirada,
		store.EstadoCobrancaCancelada,
		store.EstadoCobrancaPaga,
		store.EstadoCobrancaPagaSemItem,
		store.EstadoCobrancaValorDivergente,
	} {
		f := abertaSemCodigo()
		f.cobranca.Estado = e

		if _, err := NewRmt(f).ComCriadorDePix(criadorInerte(), nil, nil).
			GetMyCurrentPixCharge(context.Background(),
				&webv1.GetMyCurrentPixChargeRequest{AccountId: 7}); err != nil {
			t.Fatalf("%v: erro inesperado: %v", e, err)
		}
		if f.criarChamadas != 0 {
			t.Errorf("estado %v pediu a criacao %d vezes", e, f.criarChamadas)
		}
	}
}

// SEM PRAZO A PESSOA VÊ VENCIDA, e não um QR de dez segundos. O banco diz que a linha
// está aberta; a tela diz vencida de propósito, porque não há tempo de pagar e um
// código criado agora quase certamente viraria reembolso.
func TestSemPrazoAPaginaMostraVencida(t *testing.T) {
	f := abertaSemCodigo()
	f.pixCriado = store.PixDaCobranca{SemPrazo: true}

	resp, err := NewRmt(f).ComCriadorDePix(criadorInerte(), nil, nil).
		GetMyCurrentPixCharge(context.Background(), &webv1.GetMyCurrentPixChargeRequest{AccountId: 7})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_EXPIRED {
		t.Errorf("estado = %v, queria EXPIRED", resp.GetState())
	}
	if resp.GetPixCode() != "" {
		t.Errorf("codigo = %q, queria vazio numa cobranca sem prazo", resp.GetPixCode())
	}
}

// FALHA EM CRIAR NÃO PÕE VERMELHO NA TELA. A página relê a cada cinco segundos: um
// tropeço de rede se resolve na leitura seguinte, com a mesma referência, e a ponte
// reconhece a repetição em vez de criar uma segunda cobrança. Devolver erro aqui faria
// a tela quebrar por um tropeço que se conserta sozinho.
func TestFalhaAoCriarNaoViraErroNaPagina(t *testing.T) {
	f := abertaSemCodigo()
	f.erroCriarPix = errors.New("ponte fora do ar")

	resp, err := NewRmt(f).ComCriadorDePix(criadorInerte(), nil, nil).
		GetMyCurrentPixCharge(context.Background(), &webv1.GetMyCurrentPixChargeRequest{AccountId: 7})

	if err != nil {
		t.Fatalf("a falha de criar virou erro de transporte: %v", err)
	}
	if resp.GetState() != webv1.PixChargeState_PIX_CHARGE_STATE_OPEN {
		t.Errorf("estado = %v, queria OPEN: a cobranca continua valendo", resp.GetState())
	}
	if resp.GetPixCode() != "" {
		t.Errorf("codigo = %q, queria vazio: nada foi criado", resp.GetPixCode())
	}
}

// O nome do item chega na descrição, e a falta dele não derruba a venda.
func TestNomeDoItemEntraNaDescricao(t *testing.T) {
	f := abertaSemCodigo()
	f.cobranca.ItemIndex = 1234
	f.cobranca.Refino = 9

	var visto string
	criar := func(_ context.Context, _ string, _ int64, descricao string) (string, string, error) {
		visto = descricao
		return "cod", "ident", nil
	}
	// Chamado à mão porque a fake do store não repassa o criador: o que este teste
	// prova é que o HANDLER monta a descrição a partir da cobrança.
	s := NewRmt(f).ComCriadorDePix(criar, func(i int32) string {
		if i == 1234 {
			return "Espada Sagrada"
		}
		return ""
	}, nil)

	if d := s.descricaoDaCobranca(f.cobranca); d != "Compra no mercado: Espada Sagrada +9" {
		t.Errorf("descricao = %q", d)
	}
	// E sem catálogo ligado sai a genérica, em vez de recusar a cobrança.
	semCatalogo := NewRmt(f).ComCriadorDePix(criar, nil, nil)
	if d := semCatalogo.descricaoDaCobranca(f.cobranca); d != "Compra no mercado de jogadores" {
		t.Errorf("sem catalogo a descricao = %q", d)
	}
	_ = visto
}

// O CHAMADOR DESISTIR NÃO PODE CANCELAR A CRIAÇÃO, e este é o teste do conserto
// que mais importa neste arquivo.
//
// O que acontecia: o site desiste da leitura em 10 s, o gRPC cancela o contexto da
// requisição, e esse contexto era o mesmo que ia para a chamada à processadora e para
// a transação. A chamada morria no meio, a transação era desfeita, e NADA ficava
// gravado — enquanto a cobrança podia já ter nascido do outro lado com um identifier
// que ninguém aqui jamais saberia.
//
// Aqui o contexto do chamador é cancelado ANTES de a ponte responder. A criação tem
// de continuar e receber a resposta, porque é ela que grava.
func TestChamadorDesistirNaoCancelaACriacao(t *testing.T) {
	f := abertaSemCodigo()
	f.pixCriado = store.PixDaCobranca{CodigoPix: "00020126", Identifier: "sp-1", Criado: true}

	ctx, cancelar := context.WithCancel(context.Background())

	// O erro é lido DENTRO do criador, e não depois da chamada. Depois não serve: o
	// handler tem um `defer cancel()` do próprio prazo, então qualquer contexto
	// aparece cancelado quando a função retorna. A primeira versão deste teste lia
	// depois e acusava o código por uma coisa que o próprio teste tinha causado.
	var erroVistoNaPonte error
	var chamado bool
	entrou := make(chan struct{})
	criar := func(c context.Context, _ string, _ int64, _ string) (string, string, error) {
		chamado = true
		close(entrou)
		// A ponte demora. O chamador desiste no meio disto.
		time.Sleep(50 * time.Millisecond)
		erroVistoNaPonte = c.Err()
		return "00020126", "sp-1", nil
	}

	go func() {
		<-entrou
		cancelar()
	}()

	resp, err := NewRmt(f).ComCriadorDePix(criar, nil, nil).
		GetMyCurrentPixCharge(ctx, &webv1.GetMyCurrentPixChargeRequest{AccountId: 7})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// A prova: o contexto que chegou na ponte NÃO foi cancelado junto com o do
	// chamador. Se a criação usasse o ctx da requisição, aqui haveria
	// context.Canceled — e nada teria sido gravado.
	if !chamado {
		t.Fatal("a criacao nao foi nem chamada")
	}
	if erroVistoNaPonte != nil {
		t.Errorf("o contexto da criacao morreu com o do chamador: %v; "+
			"nada teria sido gravado", erroVistoNaPonte)
	}
	if f.criarChamadas != 1 {
		t.Errorf("pediu a criacao %d vezes", f.criarChamadas)
	}
	if resp.GetPixCode() != "00020126" {
		t.Errorf("codigo = %q", resp.GetPixCode())
	}
}

// E o prazo da criação é MAIOR que o da ponte, de propósito: quem desiste da chamada
// à processadora tem de ser o cliente da ponte, com a mensagem dele. Um prazo de fora
// menor transformaria toda lentidão da processadora num erro genérico, escondendo qual
// das duas coisas falhou.
func TestPrazoDaCriacaoEMaiorQueODaPonte(t *testing.T) {
	const prazoDaPonte = 15 * time.Second // ponte.timeoutPadrao
	if PrazoDaCriacao <= prazoDaPonte {
		t.Errorf("PrazoDaCriacao = %v e o da ponte e %v; o de fora tem de ser maior",
			PrazoDaCriacao, prazoDaPonte)
	}
}
