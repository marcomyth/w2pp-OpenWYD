package grpcsrv

import (
	"context"
	"errors"

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
