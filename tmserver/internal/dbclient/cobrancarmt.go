package dbclient

import (
	"context"
	"fmt"
	"time"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/handler"
)

// AbrirCobranca cria a tentativa de pagamento de um comprador contra um anúncio.
//
// Com esta função o *Client satisfaz o handler.CobradorPix, e é assim que a loja em
// dinheiro real sai do "não está ligado".
//
// A TRADUÇÃO DAS RECUSAS ACONTECE AQUI, e é o trabalho principal desta função. O
// contrato devolve um enum e a loja fala por erros-sentinela, porque cada recusa é
// uma frase diferente para o jogador: "esse item acabou de ser vendido" manda ele
// olhar a vitrine de novo, "você já tem um pagamento aberto" manda terminar o que
// começou, "você não pode comprar de si mesmo" é outra coisa ainda. Um erro genérico
// só produziria "não deu", e o jogador ficaria clicando.
//
// A janela vai no pedido, e não fica só no lado do banco, porque o MESMO número tem
// de valer para a mensagem do jogo e para a varredura que expira a cobrança noutro
// processo. Um valor que morasse num lado sairia de sincronia com o outro sem
// ninguém notar — é o que o handler.JanelaDeCobranca já explica.
func (c *Client) AbrirCobranca(ctx context.Context, anuncioID, compradorConta int64,
	referenciaExterna string,
) (handler.CobrancaAberta, error) {
	resp, err := c.api.OpenRmtCharge(ctx, &dbv1.OpenRmtChargeRequest{
		ListingId:         anuncioID,
		BuyerAccountId:    compradorConta,
		ExternalReference: referenciaExterna,
		WindowSeconds:     int64(handler.JanelaDeCobranca / time.Second),
	})
	if err != nil {
		return handler.CobrancaAberta{}, fmt.Errorf("dbclient: open rmt charge: %w", err)
	}

	switch resp.GetResult() {
	case dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_OK,
		dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ALREADY_OPEN:
		// ALREADY_OPEN é sucesso e não recusa: é a idempotência funcionando. A rede
		// engasgou, o clique repetiu, e a MESMA cobrança volta. Tratá-la como erro
		// faria o jogador ver uma falha por uma coisa que deu certo.
		//
		// O CÓDIGO PIX NÃO VEM AQUI, e o campo fica vazio de propósito: ele nasce na
		// primeira leitura da página do comprador, no site. A mensagem que o jogo
		// mostra é a que manda pagar lá, e ela não cita código nenhum.
		return handler.CobrancaAberta{
			CobrancaID: resp.GetChargeId(),
			ExpiraEm:   time.Unix(resp.GetExpiresAt(), 0),
		}, nil

	case dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_TAKEN:
		return handler.CobrancaAberta{}, handler.ErrCobrancaJaAberta

	case dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_SELF_PURCHASE:
		return handler.CobrancaAberta{}, handler.ErrCompradorEOVendedor

	case dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_BUYER_BUSY:
		return handler.CobrancaAberta{}, handler.ErrCompradorJaTemCobranca

	case dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_LISTING_GONE:
		return handler.CobrancaAberta{}, handler.ErrAnuncioIndisponivel

	case dbv1.OpenRmtChargeResult_OPEN_RMT_CHARGE_RESULT_ITEM_NOT_LOCKED:
		// A MESMA FRASE PARA O JOGADOR, e um erro DIFERENTE para nós.
		//
		// O ErrCadeadoNaoBate embrulha o ErrAnuncioIndisponivel, então o `errors.Is`
		// da loja continua casando e a pessoa lê a mesma coisa que leria se o item
		// tivesse sido vendido — o que é certo, porque nenhum dos dois pede uma ação
		// diferente dela.
		//
		// A diferença existe para o NOSSO lado: anúncio ativo com o cadeado apontando
		// para outro lugar é defeito, e não curso normal. Colapsar os dois num erro só
		// faria esse defeito viver para sempre dentro do caso comum, sem ninguém
		// descobrir que ele existe.
		return handler.CobrancaAberta{}, handler.ErrCadeadoNaoBate

	default:
		// UNSPECIFIED, ou um valor novo que este arquivo não conhece. Recusa genérica
		// de propósito: um resultado desconhecido NÃO pode virar "a cobrança abriu",
		// porque isso mandaria o jogador pagar por uma linha que talvez não exista.
		return handler.CobrancaAberta{}, fmt.Errorf(
			"dbclient: open rmt charge: resultado desconhecido %d", int32(resp.GetResult()))
	}
}
