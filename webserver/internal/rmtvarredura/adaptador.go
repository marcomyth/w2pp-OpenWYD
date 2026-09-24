package rmtvarredura

import (
	"context"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/rmtpagamento"
)

// DoPagamento liga a varredura ao serviço que já sabe decidir.
//
// Existe só para traduzir o Resultado de lá no daqui. A tradução é uma linha, e
// mesmo assim vale a casca: sem ela, este pacote importaria o rmtpagamento inteiro
// para usar um campo, e o teste da varredura precisaria montar um serviço de
// pagamento com ponte, banco e jogo para exercitar um laço.
type DoPagamento struct {
	Servico *rmtpagamento.Servico
}

// ConferirEConcluir repassa a chamada e devolve só o que a varredura usa.
func (d DoPagamento) ConferirEConcluir(ctx context.Context, identifier string) (Resultado, error) {
	res, err := d.Servico.ConferirEConcluir(ctx, identifier)
	return Resultado{Confirmada: res.Confirmada}, err
}
