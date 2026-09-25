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

type fakePix struct {
	erro      error
	leitura   store.RecebedorPix
	erroLer   error
	salvouKey string
	salvouTip store.TipoChavePix
	salvouDoc string

	temCobranca  bool
	cobranca     store.CobrancaDoComprador
	erroCobranca error
	janelaPedida time.Duration

	// Criação tardia do Pix.
	pixCriado      store.PixDaCobranca
	erroCriarPix   error
	criarChamadas  int
	minimoPedido   time.Duration
	cobrancaPedida int64
	erroDoCriador  error

	// O fechamento por recusa definitiva: quantas vezes foi pedido e de qual
	// cobrança. Contar as chamadas é o que distingue "fechou" de "tentou de novo".
	fechouPorRecusa  int
	fechadaPorRecusa int64
	erroFecharRecusa error

	// O que há a receber, que sai na mesma resposta da chave.
	pendente     int64
	motivo       store.MotivoDaEspera
	erroPendente error
}

// CriarPixSeFaltar finge o store, e CHAMA o criador que recebeu.
//
// Chamar importa, e a primeira versão desta fake não chamava: sem isso, o teste de
// que o cancelamento do chamador não mata a criação não observava nada — o contexto
// que interessa é justamente o que chega no criador. Uma fake que pula o pedaço sob
// teste dá um teste verde que não prova nada.
//
// O resultado continua vindo dos campos, e não do criador: o que esta fake observa é
// o HANDLER, e quem prova que a chamada à ponte sai uma vez só é o teste de
// integração do store, que é onde a trava mora.
func (f *fakePix) CriarPixSeFaltar(ctx context.Context, cobrancaID int64,
	minimo time.Duration, criar store.CriadorDePix,
) (store.PixDaCobranca, error) {
	f.criarChamadas++
	f.cobrancaPedida = cobrancaID
	f.minimoPedido = minimo
	if criar != nil {
		_, _, f.erroDoCriador = criar(ctx, "ref-fake", 5000)
	}
	return f.pixCriado, f.erroCriarPix
}

func (f *fakePix) FecharCobrancaPorRecusaDefinitiva(_ context.Context, cobrancaID int64) (bool, error) {
	f.fechouPorRecusa++
	f.fechadaPorRecusa = cobrancaID
	if f.erroFecharRecusa != nil {
		return false, f.erroFecharRecusa
	}
	return true, nil
}

func (f *fakePix) SalvarChavePix(_ context.Context, _ int64, chave string,
	tipo store.TipoChavePix, documento string,
) error {
	f.salvouKey, f.salvouTip, f.salvouDoc = chave, tipo, documento
	return f.erro
}

func (f *fakePix) LerChavePix(context.Context, int64) (store.RecebedorPix, error) {
	return f.leitura, f.erroLer
}

func (f *fakePix) RepasseDoVendedor(context.Context, int64) (int64, store.MotivoDaEspera, error) {
	return f.pendente, f.motivo, f.erroPendente
}

func (f *fakePix) CobrancaAtualDoComprador(_ context.Context, _ int64,
	janela time.Duration,
) (bool, store.CobrancaDoComprador, error) {
	f.janelaPedida = janela
	return f.temCobranca, f.cobranca, f.erroCobranca
}

// As recusas previstas viajam no ENUM e não como erro de transporte.
//
// A diferença não é de estilo: o formulário precisa saber O QUE dizer à pessoa —
// "a chave está malformada" e "você tem uma venda em andamento" levam a ações
// diferentes —, e um código de erro gRPC não carrega isso. Só falha de
// infraestrutura vira erro, porque aí não há o que a pessoa faça.
func TestSavePixKeyTraduzAsRecusas(t *testing.T) {
	casos := []struct {
		nome string
		erro error
		quer webv1.PixKeyResult
	}{
		{"deu certo", nil, webv1.PixKeyResult_PIX_KEY_RESULT_OK},
		{"chave malformada", store.ErrChavePixInvalida, webv1.PixKeyResult_PIX_KEY_RESULT_INVALID},
		{"venda em curso", store.ErrVendaEmCurso, webv1.PixKeyResult_PIX_KEY_RESULT_SALE_IN_PROGRESS},
		{"conta não existe", store.ErrNotFound, webv1.PixKeyResult_PIX_KEY_RESULT_NO_ACCOUNT},
		// CÓDIGO PRÓPRIO PARA O DOCUMENTO, e não o INVALID genérico. O formulário tem
		// DOIS campos: dizer só "inválido" faria a pessoa corrigir a chave, que estava
		// certa, e tentar de novo com o mesmo documento errado — sem entender por quê.
		{"documento inválido", store.ErrDocumentoInvalido,
			webv1.PixKeyResult_PIX_KEY_RESULT_INVALID_TAX_ID},
	}
	for _, c := range casos {
		s := NewRmt(&fakePix{erro: c.erro})
		resp, err := s.SavePixKey(context.Background(), &webv1.SavePixKeyRequest{
			AccountId: 7, Key: "12345678901", Type: webv1.PixKeyType_PIX_KEY_TYPE_CPF,
			TaxId: "11144477735",
		})
		if err != nil {
			t.Errorf("%s: virou erro de transporte: %v", c.nome, err)
			continue
		}
		if resp.GetResult() != c.quer {
			t.Errorf("%s: resultado = %v, quero %v", c.nome, resp.GetResult(), c.quer)
		}
	}
}

// E o contrário: falha de infraestrutura NÃO pode virar uma recusa de negócio. Se
// o banco cair e a resposta disser "chave inválida", a pessoa corrige uma chave
// que estava certa e continua sem conseguir.
func TestSavePixKeyFalhaDeInfraViraErro(t *testing.T) {
	s := NewRmt(&fakePix{erro: errors.New("banco fora do ar")})

	_, err := s.SavePixKey(context.Background(), &webv1.SavePixKeyRequest{
		AccountId: 7, Key: "12345678901", Type: webv1.PixKeyType_PIX_KEY_TYPE_CPF,
		TaxId: "11144477735",
	})

	if status.Code(err) != codes.Internal {
		t.Errorf("código = %v, quero Internal", status.Code(err))
	}
}

// O tipo atravessa o contrato sem se embaralhar. É um teste bobo até o dia em que
// alguém acrescenta um tipo de chave no meio do enum: aí ele é o que pega.
func TestTipoAtravessaNasDuasDirecoes(t *testing.T) {
	casos := []struct {
		proto  webv1.PixKeyType
		dentro store.TipoChavePix
	}{
		{webv1.PixKeyType_PIX_KEY_TYPE_CPF, store.ChavePixCPF},
		{webv1.PixKeyType_PIX_KEY_TYPE_EMAIL, store.ChavePixEmail},
		{webv1.PixKeyType_PIX_KEY_TYPE_PHONE, store.ChavePixTelefone},
		{webv1.PixKeyType_PIX_KEY_TYPE_RANDOM, store.ChavePixAleatoria},
	}
	for _, c := range casos {
		f := &fakePix{}
		s := NewRmt(f)
		if _, err := s.SavePixKey(context.Background(), &webv1.SavePixKeyRequest{
			AccountId: 7, Key: "x", Type: c.proto, TaxId: "11144477735",
		}); err != nil {
			t.Fatalf("%v: %v", c.proto, err)
		}
		if f.salvouTip != c.dentro {
			t.Errorf("%v chegou no store como %d, quero %d", c.proto, f.salvouTip, c.dentro)
		}
		if got := tipoParaProto(c.dentro); got != c.proto {
			t.Errorf("volta de %d = %v, quero %v", c.dentro, got, c.proto)
		}
	}
}

// A leitura devolve o que o banco mascarou e nada mais. O mascaramento vive na
// camada de banco de propósito: assim não existe caminho que devolva a chave crua
// se alguém escrever outro handler amanhã.
func TestGetPixKeyDevolveOQueVemMascarado(t *testing.T) {
	s := NewRmt(&fakePix{leitura: store.RecebedorPix{
		TemChave: true, ChaveMascarada: "***8901", Tipo: store.ChavePixCPF, Verificada: false,
	}})

	resp, err := s.GetPixKey(context.Background(), &webv1.GetPixKeyRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !resp.GetHasKey() || resp.GetMaskedKey() != "***8901" {
		t.Errorf("resposta = %+v", resp)
	}
	if resp.GetType() != webv1.PixKeyType_PIX_KEY_TYPE_CPF {
		t.Errorf("tipo = %v, quero CPF", resp.GetType())
	}
	if resp.GetVerified() {
		t.Error("verified = true, e o banco disse false")
	}
}

// Sem chave cadastrada não é erro: é a resposta normal para quem ainda não
// preencheu o formulário.
func TestGetPixKeySemChaveNaoEErro(t *testing.T) {
	s := NewRmt(&fakePix{})

	resp, err := s.GetPixKey(context.Background(), &webv1.GetPixKeyRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if resp.GetHasKey() || resp.GetMaskedKey() != "" {
		t.Errorf("resposta = %+v, queria vazia", resp)
	}
}

// Falha de leitura também é erro de transporte, e não uma resposta "não tem
// chave": dizer "não tem" quando o banco caiu faria o formulário oferecer um
// cadastro novo em cima de uma chave que existe.
func TestGetPixKeyFalhaDeInfraViraErro(t *testing.T) {
	s := NewRmt(&fakePix{erroLer: errors.New("banco fora do ar")})

	if _, err := s.GetPixKey(context.Background(), &webv1.GetPixKeyRequest{AccountId: 7}); status.Code(err) != codes.Internal {
		t.Errorf("código = %v, quero Internal", status.Code(err))
	}
}

// O QUE HÁ A RECEBER SAI NA RESPOSTA DA CHAVE, com o motivo traduzido.
func TestGetPixKeyDevolveOQueHaAReceber(t *testing.T) {
	casos := []struct {
		nome   string
		motivo store.MotivoDaEspera
		quer   webv1.PayoutWaitReason
	}{
		{"nada a receber", store.SemEspera, webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_UNSPECIFIED},
		{"falta cadastro", store.EsperaCadastro, webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_NO_KEY_OR_TAX_ID},
		{"a caminho", store.EsperaPagamento, webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_IN_PROGRESS},
		{"precisa de gente", store.EsperaGente, webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_NEEDS_STAFF},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &fakePix{pendente: 12345, motivo: c.motivo}
			s := NewRmt(f)
			r, err := s.GetPixKey(context.Background(), &webv1.GetPixKeyRequest{AccountId: 7})
			if err != nil {
				t.Fatal(err)
			}
			if r.GetPendingPayoutCents() != 12345 {
				t.Errorf("pendente = %d", r.GetPendingPayoutCents())
			}
			if r.GetPayoutWaitReason() != c.quer {
				t.Errorf("motivo = %v, quero %v", r.GetPayoutWaitReason(), c.quer)
			}
		})
	}
}

// E A FALHA DA SEGUNDA LEITURA NÃO DERRUBA A PRIMEIRA.
//
// Este é o teste que importa dos dois. A tela da chave é onde a pessoa CONSERTA o
// cadastro, e cadastro incompleto é a causa mais comum de haver dinheiro parado. Se
// uma falha ao somar a dívida derrubasse a resposta inteira, o erro tiraria do ar
// justamente a tela que resolve o problema — e a pessoa com dinheiro preso seria a
// única que não conseguiria abri-la.
func TestGetPixKeyNaoCaiQuandoOTotalFalha(t *testing.T) {
	f := &fakePix{
		leitura:      store.RecebedorPix{TemChave: true, ChaveMascarada: "***1234"},
		pendente:     999,
		motivo:       store.EsperaGente,
		erroPendente: errors.New("o banco caiu"),
	}
	s := NewRmt(f)
	r, err := s.GetPixKey(context.Background(), &webv1.GetPixKeyRequest{AccountId: 7})
	if err != nil {
		t.Fatalf("a falha do total derrubou a resposta da chave: %v", err)
	}
	if !r.GetHasKey() || r.GetMaskedKey() != "***1234" {
		t.Errorf("a chave nao voltou: %+v", r)
	}
	// Zero e UNSPECIFIED, e não os valores da fake: na dúvida não se afirma nada.
	if r.GetPendingPayoutCents() != 0 ||
		r.GetPayoutWaitReason() != webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_UNSPECIFIED {
		t.Errorf("a falha virou numero na tela: %d / %v",
			r.GetPendingPayoutCents(), r.GetPayoutWaitReason())
	}
}

// CADA ERRO DO STORE TEM UM RESULTADO PRÓPRIO, e o desconhecido vira Internal.
//
// Este teste nasce de um defeito que eu quase subi: separei ErrRepasseEmCurso de
// ErrVendaEmCurso no store e não mapeei o novo aqui. O erro caía no default e virava
// codes.Internal — ou seja, o vendedor com repasse a caminho, que antes lia "venda em
// curso", passaria a receber ERRO DE SERVIDOR. Regressão em produção num caminho que
// funcionava, no minuto do deploy.
//
// A TABELA É O CONSERTO DURÁVEL, e não o caso que faltava. Dividir um erro em dois
// obriga a percorrer todos os lugares que traduziam o antigo, e é aí que se esquece um.
// Com a tabela, o próximo erro novo que ninguém mapear aparece como falha e não como
// "erro interno" na tela de alguém.
func TestCadaErroDoStoreTemSeuResultado(t *testing.T) {
	casos := []struct {
		nome string
		erro error
		quer webv1.PixKeyResult
		grpc bool // true = espera erro gRPC em vez de resultado
	}{
		{"chave invalida", store.ErrChavePixInvalida, webv1.PixKeyResult_PIX_KEY_RESULT_INVALID, false},
		{"documento invalido", store.ErrDocumentoInvalido, webv1.PixKeyResult_PIX_KEY_RESULT_INVALID_TAX_ID, false},
		{"venda em curso", store.ErrVendaEmCurso, webv1.PixKeyResult_PIX_KEY_RESULT_SALE_IN_PROGRESS, false},
		{"repasse em curso", store.ErrRepasseEmCurso, webv1.PixKeyResult_PIX_KEY_RESULT_PAYOUT_PENDING, false},
		{"conta inexistente", store.ErrNotFound, webv1.PixKeyResult_PIX_KEY_RESULT_NO_ACCOUNT, false},
		{"sem erro", nil, webv1.PixKeyResult_PIX_KEY_RESULT_OK, false},
		// O DESCONHECIDO TEM DE VIRAR ERRO, e não um resultado qualquer: um erro que
		// ninguém previu virando "ok" ou "inválido" é o que faz a tela mentir.
		{"erro que ninguem previu", errors.New("o banco pegou fogo"), 0, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &fakePix{erro: c.erro}
			res, err := NewRmt(f).SavePixKey(context.Background(),
				&webv1.SavePixKeyRequest{AccountId: 1, Key: "a@b.com",
					Type: webv1.PixKeyType_PIX_KEY_TYPE_EMAIL, TaxId: "11144477735"})
			if c.grpc {
				if err == nil {
					t.Fatalf("erro desconhecido virou resultado %v em vez de falhar", res.GetResult())
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if res.GetResult() != c.quer {
				t.Errorf("resultado = %v, queria %v", res.GetResult(), c.quer)
			}
		})
	}
}
