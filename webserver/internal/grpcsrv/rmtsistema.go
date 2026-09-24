package grpcsrv

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/rmtpagamento"
)

// Pagamentos é o caminho do aviso de pagamento (satisfeito por
// *rmtpagamento.Servico).
type Pagamentos interface {
	// ConferirEConcluir serve ao aviso e, no PR seguinte, à varredura das
	// cobranças abertas. Ver rmtpagamento: a regra de entregar mora num lugar só.
	ConferirEConcluir(ctx context.Context, identifier string) (rmtpagamento.Resultado, error)
	AvisoDeSaida(ctx context.Context, identifier string, pedidoCentavos, chegouCentavos int64) (rmtpagamento.Resultado, error)
}

// ServerRmtSistema implementa webv1.RmtSystemServiceServer.
//
// SERVIÇO SEPARADO DO RmtWebService, e a separação é de QUEM CHAMA e não de
// assunto. O RmtWebService responde ao jogador logado: cada chamada dele traz um
// account_id e a autorização é a chave do site. Este responde ao SITE falando por
// si mesmo, sobre um evento que a processadora mandou, sem jogador nenhum atrás.
//
// São duas chaves diferentes no authz.go — RmtWebService em `servicosDoJogador`,
// este em `servicosDeSistema` —, e é isso que a separação compra: a chave que o
// navegador faz o BFF usar não alcança este método. Se os dois morassem no mesmo
// serviço, ou o aviso de pagamento aceitaria a chave do jogador, ou a página da
// cobrança exigiria a chave de sistema.
type ServerRmtSistema struct {
	webv1.UnimplementedRmtSystemServiceServer
	pagamentos Pagamentos
	log        *slog.Logger
}

// NewRmtSistema monta o serviço do aviso de pagamento.
func NewRmtSistema(p Pagamentos, log *slog.Logger) *ServerRmtSistema {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &ServerRmtSistema{pagamentos: p, log: log}
}

// NotifySyncpayEvent recebe um evento da processadora que o site repassou.
//
// NADA AQUI É ACREDITADO no caminho de entrada: o serviço vai perguntar à
// processadora o que de fato aconteceu. Ver rmtpagamento.
//
// O QUE VIRA ERRO E O QUE NÃO VIRA é a decisão que mais importa neste método,
// porque quem chama REPETE em cima de erro:
//
//   - Evento que não é nosso devolve handled=false e NÃO é erro. O webhook da
//     processadora é por CONTA, então a recarga da loja de doação chega pelo mesmo
//     canal. Devolver erro faria o site repetir para sempre o pagamento de outra
//     pessoa.
//   - Falha em CONSULTAR a processadora é erro. Sem a consulta não se decide nada,
//     e repetir é seguro — a confirmação é idempotente pela referência.
//   - Falha DEPOIS de a venda estar gravada não é erro, e isso é tratado lá dentro:
//     a entrega imediata é cortesia, e o login de cada um faz o mesmo trabalho.
func (s *ServerRmtSistema) NotifySyncpayEvent(ctx context.Context, req *webv1.NotifySyncpayEventRequest) (*webv1.NotifySyncpayEventResponse, error) {
	switch req.GetKind() {
	case webv1.SyncpayEventKind_SYNCPAY_EVENT_KIND_CASHIN:
		res, err := s.pagamentos.ConferirEConcluir(ctx, req.GetIdentifier())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "notify syncpay cashin: %v", err)
		}
		return &webv1.NotifySyncpayEventResponse{Handled: res.Tratado}, nil

	case webv1.SyncpayEventKind_SYNCPAY_EVENT_KIND_CASHOUT:
		res, err := s.pagamentos.AvisoDeSaida(ctx, req.GetIdentifier(),
			req.GetAmountCents(), req.GetFinalAmountCents())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "notify syncpay cashout: %v", err)
		}
		return &webv1.NotifySyncpayEventResponse{Handled: res.Tratado}, nil

	default:
		// Tipo que este servidor não conhece — ou o zero, que é o que chega quando o
		// campo não foi preenchido.
		//
		// handled=false E NÃO ERRO, de propósito: um erro faria o site repetir em
		// laço um evento que este servidor nunca vai saber tratar, e a repetição não
		// ensinaria nada a ninguém. O aviso no log é o que faz alguém descobrir que a
		// processadora ganhou um tipo novo.
		s.log.Warn("rmt: tipo de evento da processadora que eu nao conheco",
			"tipo", int32(req.GetKind()), "identifier", req.GetIdentifier())
		return &webv1.NotifySyncpayEventResponse{}, nil
	}
}
