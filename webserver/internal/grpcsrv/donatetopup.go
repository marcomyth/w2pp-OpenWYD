package grpcsrv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/donatetopup"
)

// DonateTopup is the donate top-up surface (satisfied by *donatetopup.Service).
// Kept as an interface so the server is unit-testable.
type DonateTopup interface {
	SavePayerProfile(ctx context.Context, accountID int64, name, cpf string) (donatetopup.Result, error)
	GetPayerProfile(ctx context.Context, accountID int64) (found bool, name, cpf string, err error)
	CreateTopupOrder(ctx context.Context, o domain.TopupOrder) (donatetopup.Result, int64, error)
	ConfirmTopupOrder(ctx context.Context, externalRef string) (donatetopup.ConfirmOutcome, int64, error)
	GetTopupOrder(ctx context.Context, externalRef string, accountID int64) (status int16, credits int32, newBalance int64, err error)
	// AnexarIdentifier guarda o id da processadora, para a varredura poder perguntar
	// sobre o pagamento em vez de esperar ser avisada.
	AnexarIdentifier(ctx context.Context, externalRef, identifier string) (store.ResultadoAnexo, error)
}

// DonateTopupServer implements webv1.DonateTopupServiceServer.
type DonateTopupServer struct {
	webv1.UnimplementedDonateTopupServiceServer
	topup DonateTopup
}

// NewDonateTopup builds the DonateTopupService over the given top-up logic.
func NewDonateTopup(t DonateTopup) *DonateTopupServer { return &DonateTopupServer{topup: t} }

// GetPayerProfile returns the payer's stored name + CPF (found=false when none).
func (s *DonateTopupServer) GetPayerProfile(ctx context.Context, req *webv1.GetPayerProfileRequest) (*webv1.GetPayerProfileResponse, error) {
	found, name, cpf, err := s.topup.GetPayerProfile(ctx, req.GetAccountId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get payer profile: %v", err)
	}
	return &webv1.GetPayerProfileResponse{Found: found, Name: name, Cpf: cpf}, nil
}

// SavePayerProfile upserts the payer's name + CPF.
func (s *DonateTopupServer) SavePayerProfile(ctx context.Context, req *webv1.SavePayerProfileRequest) (*webv1.SavePayerProfileResponse, error) {
	res, err := s.topup.SavePayerProfile(ctx, req.GetAccountId(), req.GetName(), req.GetCpf())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "save payer profile: %v", err)
	}
	return &webv1.SavePayerProfileResponse{Result: topupResultToProto(res)}, nil
}

// CreateTopupOrder persists a PENDING order.
func (s *DonateTopupServer) CreateTopupOrder(ctx context.Context, req *webv1.CreateTopupOrderRequest) (*webv1.CreateTopupOrderResponse, error) {
	res, id, err := s.topup.CreateTopupOrder(ctx, domain.TopupOrder{
		AccountID:         req.GetAccountId(),
		ExternalReference: req.GetExternalReference(),
		Credits:           req.GetCredits(),
		AmountCents:       req.GetAmountCents(),
		PaymentMethod:     int16(req.GetPaymentMethod()),
		// Vazio é aceito e quer dizer doação sem pacote — é toda ordem anterior aos
		// pacotes existirem. Quem recusa id DESCONHECIDO é o serviço, contra a tabela.
		PacoteID: req.GetPackageId(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create topup order: %v", err)
	}
	return &webv1.CreateTopupOrderResponse{Result: topupResultToProto(res), OrderId: id}, nil
}

// ConfirmTopupOrder settles a paid order idempotently.
func (s *DonateTopupServer) ConfirmTopupOrder(ctx context.Context, req *webv1.ConfirmTopupOrderRequest) (*webv1.ConfirmTopupOrderResponse, error) {
	outcome, newBal, err := s.topup.ConfirmTopupOrder(ctx, req.GetExternalReference())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "confirm topup order: %v", err)
	}
	return &webv1.ConfirmTopupOrderResponse{Result: confirmOutcomeToProto(outcome), NewBalance: newBal}, nil
}

// GetTopupOrder returns the current status/credits for the polling portal.
func (s *DonateTopupServer) GetTopupOrder(ctx context.Context, req *webv1.GetTopupOrderRequest) (*webv1.GetTopupOrderResponse, error) {
	st, credits, newBal, err := s.topup.GetTopupOrder(ctx, req.GetExternalReference(), req.GetAccountId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get topup order: %v", err)
	}
	return &webv1.GetTopupOrderResponse{
		Status:     topupStatusToProto(st),
		Credits:    credits,
		NewBalance: newBal,
	}, nil
}

// topupResultToProto maps the profile/create outcome to the shared AdminResult.
func topupResultToProto(r donatetopup.Result) webv1.AdminResult {
	switch r {
	case donatetopup.OK:
		return webv1.AdminResult_ADMIN_RESULT_OK
	case donatetopup.NotFound:
		return webv1.AdminResult_ADMIN_RESULT_NOT_FOUND
	case donatetopup.Forbidden:
		// O FORBIDDEN cai aqui com o sentido que o enum já tem: "caller is not a
		// moderator/admin". É o pacote reservado à staff pedido por quem não é staff,
		// e é a única recusa deste caminho que é sobre QUEM compra — as outras são
		// sobre o pedido.
		return webv1.AdminResult_ADMIN_RESULT_FORBIDDEN
	default:
		return webv1.AdminResult_ADMIN_RESULT_INVALID
	}
}

func confirmOutcomeToProto(o donatetopup.ConfirmOutcome) webv1.TopupResult {
	switch o {
	case donatetopup.Confirmed:
		return webv1.TopupResult_TOPUP_RESULT_CONFIRMED
	case donatetopup.AlreadyConfirmed:
		return webv1.TopupResult_TOPUP_RESULT_ALREADY_CONFIRMED
	default:
		return webv1.TopupResult_TOPUP_RESULT_NOT_FOUND
	}
}

// topupStatusToProto maps the stored status int (0=none, 1=PENDING, 2=PAID) to
// the wire enum; an unknown/absent order (0) is TOPUP_STATUS_UNSPECIFIED.
func topupStatusToProto(st int16) webv1.TopupStatus {
	switch st {
	case 1:
		return webv1.TopupStatus_TOPUP_STATUS_PENDING
	case 2:
		return webv1.TopupStatus_TOPUP_STATUS_PAID
	default:
		return webv1.TopupStatus_TOPUP_STATUS_UNSPECIFIED
	}
}

// AttachTopupCharge guarda o id da processadora para um pedido de doação.
//
// AS RECUSAS VIAJAM NO ENUM e não como erro de transporte, pelo mesmo motivo do
// resto deste arquivo: o site precisa saber O QUE aconteceu. "Já tinha este id" é
// uma repetição e não é nada; "este id já está em outro pedido" é bug ou ataque, e
// tem de virar alarme do lado de lá. Um código de erro gRPC não carrega essa
// diferença, e o site trataria as duas como falha de rede.
func (s *DonateTopupServer) AttachTopupCharge(ctx context.Context,
	req *webv1.AttachTopupChargeRequest,
) (*webv1.AttachTopupChargeResponse, error) {
	res, err := s.topup.AnexarIdentifier(ctx, req.GetExternalReference(), req.GetGatewayIdentifier())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "attach topup charge: %v", err)
	}
	return &webv1.AttachTopupChargeResponse{Result: anexoParaProto(res)}, nil
}

// anexoParaProto traduz explicitamente, e o default é UNSPECIFIED e não um palpite:
// um resultado novo que chegasse aqui sem tradução não pode virar "deu certo".
func anexoParaProto(r store.ResultadoAnexo) webv1.AttachResult {
	switch r {
	case store.AnexoGravado:
		return webv1.AttachResult_ATTACH_RESULT_ATTACHED
	case store.AnexoRepetido:
		return webv1.AttachResult_ATTACH_RESULT_ALREADY
	case store.AnexoConflitoDePedido:
		return webv1.AttachResult_ATTACH_RESULT_CONFLICT_ORDER
	case store.AnexoConflitoDeIdentifier:
		return webv1.AttachResult_ATTACH_RESULT_CONFLICT_IDENTIFIER
	case store.AnexoPedidoInexistente:
		return webv1.AttachResult_ATTACH_RESULT_NOT_FOUND
	case store.AnexoJaPago:
		return webv1.AttachResult_ATTACH_RESULT_ALREADY_PAID
	}
	return webv1.AttachResult_ATTACH_RESULT_UNSPECIFIED
}
