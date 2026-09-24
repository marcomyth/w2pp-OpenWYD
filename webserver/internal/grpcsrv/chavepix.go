package grpcsrv

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// ChavesPix é a superfície da chave de recebimento (satisfeita por *store.Store).
// Interface e não o store concreto pelo mesmo motivo do resto do pacote: o
// servidor fica testável sem banco.
type ChavesPix interface {
	SalvarChavePix(ctx context.Context, accountID int64, chave string, tipo store.TipoChavePix) error
	LerChavePix(ctx context.Context, accountID int64) (store.RecebedorPix, error)
	// CobrancaAtualDoComprador é a única leitura feita para quem PAGA: a cobrança
	// aberta da conta, ou a que fechou há pouco. Ver store/cobranca_do_comprador.go.
	CobrancaAtualDoComprador(ctx context.Context, compradorConta int64,
		janelaRecente time.Duration) (bool, store.CobrancaDoComprador, error)
}

// ServerRmt implementa webv1.RmtWebServiceServer.
//
// SERVIÇO PRÓPRIO, e não dois métodos a mais no AccountWebService: aquele cuida
// de quem entra, este aponta para onde o dinheiro cai. Separados, a superfície de
// dinheiro real pode ser fechada, auditada ou retirada sozinha, e o assunto
// aparece no caminho do método (/web.v1.RmtWebService/SavePixKey) para quem for
// ler um log de acesso.
type ServerRmt struct {
	webv1.UnimplementedRmtWebServiceServer
	pix ChavesPix
}

// NewRmt monta o serviço de dinheiro real sobre a superfície da chave.
func NewRmt(pix ChavesPix) *ServerRmt { return &ServerRmt{pix: pix} }

// SavePixKey grava a chave de recebimento da conta.
//
// As recusas previstas — chave malformada, venda em curso, conta inexistente —
// viajam no enum da resposta e NÃO como erro gRPC. Só falha de infraestrutura
// vira erro, que é a mesma divisão que o CreateAccount faz: o formulário precisa
// saber O QUE dizer à pessoa, e um código de erro de transporte não diz.
func (s *ServerRmt) SavePixKey(ctx context.Context, req *webv1.SavePixKeyRequest) (*webv1.SavePixKeyResponse, error) {
	err := s.pix.SalvarChavePix(ctx, req.GetAccountId(), req.GetKey(), tipoDoProto(req.GetType()))
	switch {
	case err == nil:
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_OK}, nil
	case errors.Is(err, store.ErrChavePixInvalida):
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_INVALID}, nil
	case errors.Is(err, store.ErrVendaEmCurso):
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_SALE_IN_PROGRESS}, nil
	case errors.Is(err, store.ErrNotFound):
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_NO_ACCOUNT}, nil
	default:
		return nil, status.Errorf(codes.Internal, "save pix key: %v", err)
	}
}

// GetPixKey devolve o estado da chave, sempre MASCARADA — a inteira não sai do
// servidor. O mascaramento acontece na camada de banco, e não aqui, para não
// existir um caminho que devolva a chave crua se alguém escrever outro handler
// amanhã.
func (s *ServerRmt) GetPixKey(ctx context.Context, req *webv1.GetPixKeyRequest) (*webv1.GetPixKeyResponse, error) {
	r, err := s.pix.LerChavePix(ctx, req.GetAccountId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get pix key: %v", err)
	}
	return &webv1.GetPixKeyResponse{
		HasKey:    r.TemChave,
		MaskedKey: r.ChaveMascarada,
		Type:      tipoParaProto(r.Tipo),
		Verified:  r.Verificada,
	}, nil
}

func tipoDoProto(t webv1.PixKeyType) store.TipoChavePix {
	switch t {
	case webv1.PixKeyType_PIX_KEY_TYPE_CPF:
		return store.ChavePixCPF
	case webv1.PixKeyType_PIX_KEY_TYPE_EMAIL:
		return store.ChavePixEmail
	case webv1.PixKeyType_PIX_KEY_TYPE_PHONE:
		return store.ChavePixTelefone
	case webv1.PixKeyType_PIX_KEY_TYPE_RANDOM:
		return store.ChavePixAleatoria
	}
	// Zero não é tipo: cai na validação do store e volta como INVALID, que é o
	// que o formulário sabe mostrar.
	return 0
}

func tipoParaProto(t store.TipoChavePix) webv1.PixKeyType {
	switch t {
	case store.ChavePixCPF:
		return webv1.PixKeyType_PIX_KEY_TYPE_CPF
	case store.ChavePixEmail:
		return webv1.PixKeyType_PIX_KEY_TYPE_EMAIL
	case store.ChavePixTelefone:
		return webv1.PixKeyType_PIX_KEY_TYPE_PHONE
	case store.ChavePixAleatoria:
		return webv1.PixKeyType_PIX_KEY_TYPE_RANDOM
	}
	return webv1.PixKeyType_PIX_KEY_TYPE_UNSPECIFIED
}

// GetMyCurrentPixCharge devolve a cobrança que o comprador tem de pagar — ou o que
// aconteceu com ela.
//
// SEM COBRANÇA É RESPOSTA VAZIA E NÃO ERRO, e isto é o caso comum e não a
// exceção: quem pergunta é a página da conta, que as pessoas abrem para ver outras
// coisas. Devolver erro aqui poria vermelho na tela de quase todo mundo, quase
// sempre.
//
// A CONTA VEM DO PEDIDO E NÃO HÁ PARÂMETRO DE "COBRANÇA DE QUEM": o id chega
// resolvido pelo BFF, que é quem sabe quem entrou. Quem garante que uma conta não
// lê a cobrança de outra é a autorização do serviço (authz.go, RmtWebService está
// em servicosDoJogador), e não uma conferência aqui — a mesma divisão da chave
// Pix.
func (s *ServerRmt) GetMyCurrentPixCharge(ctx context.Context, req *webv1.GetMyCurrentPixChargeRequest) (*webv1.GetMyCurrentPixChargeResponse, error) {
	tem, cob, err := s.pix.CobrancaAtualDoComprador(ctx, req.GetAccountId(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get current pix charge: %v", err)
	}
	if !tem {
		return &webv1.GetMyCurrentPixChargeResponse{}, nil
	}
	return &webv1.GetMyCurrentPixChargeResponse{
		HasCharge: true,
		// Vem como o banco guardou, que é como a processadora devolveu. E vem
		// vazio em todo estado fechado, porque a camada de banco só o entrega no
		// estado aberto — cobrança morta ao lado de um código vivo convida alguém
		// a pagar.
		PixCode:     cob.CodigoPix,
		AmountCents: cob.ValorCentavos,
		ExpiresAt:   cob.ExpiraEm.Unix(),
		State:       estadoParaProto(cob.Estado),
		ItemIndex:   int32(cob.ItemIndex),
		RefineLevel: int32(cob.Refino),
		StackSize:   int32(cob.Quantidade),
		SellerName:  cob.VendedorNome,
		// Zero quando nada foi pedido, que é o caso de quase toda cobrança. A
		// página conta os até dois dias úteis a partir desta data.
		RefundRequestedAt: unixOuZero(cob.ReembolsoPedidoEm),
		RefundState:       reembolsoParaProto(cob.Reembolso),
	}, nil
}

// estadoParaProto mapeia o estado do banco no do contrato.
//
// Explícito e não aritmético, mesmo com os números batendo hoje: os dois enums
// vivem em arquivos diferentes e mudam por motivos diferentes, e uma conversão por
// cast passaria a mentir em silêncio no dia em que um deles ganhasse um estado no
// meio.
func estadoParaProto(e store.EstadoCobrancaComprador) webv1.PixChargeState {
	switch e {
	case store.EstadoCobrancaAberta:
		return webv1.PixChargeState_PIX_CHARGE_STATE_OPEN
	case store.EstadoCobrancaExpirada:
		return webv1.PixChargeState_PIX_CHARGE_STATE_EXPIRED
	case store.EstadoCobrancaPaga:
		return webv1.PixChargeState_PIX_CHARGE_STATE_PAID
	case store.EstadoCobrancaCancelada:
		return webv1.PixChargeState_PIX_CHARGE_STATE_CANCELED
	case store.EstadoCobrancaPagaSemItem:
		return webv1.PixChargeState_PIX_CHARGE_STATE_PAID_LATE
	case store.EstadoCobrancaValorDivergente:
		// PAID_LATE TAMBÉM PARA O VALOR DIVERGENTE, e a escolha é do menos errado
		// entre os que existem.
		//
		// O nome fala de atraso e o caso não é de atraso — mas o que o estado diz
		// à pessoa é exatamente o certo: O DINHEIRO CHEGOU, O ITEM NÃO FOI
		// ENTREGUE, E ALGUÉM ESTÁ OLHANDO. É a mesma frase nos dois casos, e a
		// causa (tarde, ou valor diferente) não muda nada do que ela pode fazer.
		//
		// Os outros mentiriam pior. OPEN diz "pague", e ela já pagou — convidaria a
		// pagar duas vezes. PAID diz "o item está indo", e não está. EXPIRED e
		// CANCELED dizem que não houve pagamento, e houve.
		//
		// A diferença aparece no refund_state: no atrasado ele anda (PENDING,
		// REQUESTED, REFUNDED); aqui fica UNSPECIFIED, porque não há reembolso
		// automático — valor diferente do cobrado é caso que precisa de gente, e
		// escolher devolver, cobrar a diferença ou entregar assim mesmo é decidir
		// sobre o dinheiro de duas pessoas.
		//
		// O comentário do .proto ainda diz só "arrived late". Ele pega carona no
		// próximo handshake: mudança de comentário também muda o sha256, e o
		// prebuild do site compara o hash com a main — um handshake inteiro por uma
		// frase não se paga.
		return webv1.PixChargeState_PIX_CHARGE_STATE_PAID_LATE
	}
	return webv1.PixChargeState_PIX_CHARGE_STATE_UNSPECIFIED
}

// unixOuZero devolve 0 para uma data vazia em vez do unix de 1970, que é o que o
// Time zero daria — e 1970 na tela seria uma data de verdade, errada.
func unixOuZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// reembolsoParaProto mapeia o estado do reembolso no do contrato, explícito pelo
// mesmo motivo do estadoParaProto: os dois enums mudam por motivos diferentes.
func reembolsoParaProto(e store.EstadoReembolso) webv1.RefundState {
	switch e {
	case store.ReembolsoPendente:
		return webv1.RefundState_REFUND_STATE_PENDING
	case store.ReembolsoPedido:
		return webv1.RefundState_REFUND_STATE_REQUESTED
	case store.ReembolsoConcluido:
		return webv1.RefundState_REFUND_STATE_REFUNDED
	case store.ReembolsoRecusado:
		return webv1.RefundState_REFUND_STATE_FAILED
	}
	return webv1.RefundState_REFUND_STATE_UNSPECIFIED
}
