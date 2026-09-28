package doacaovarredura

import (
	"context"
	"fmt"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/donatetopup"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

// DaPonte é a consulta de verdade à processadora.
//
// Adaptador próprio, e não a do rmtpagamento, porque o que a doação precisa da
// resposta é diferente: lá o que importa é a referência QUE A PONTE EXTRAIU, e aqui
// é a descrição CRUA, que é de onde este pacote tira a referência dele. O tipo
// separado impede que uma mudança na leitura de um lado mude o outro em silêncio.
type DaPonte struct {
	Cliente *ponte.Cliente
}

// ConsultarTransacao pergunta e traduz.
func (d DaPonte) ConsultarTransacao(ctx context.Context, identifier string) (Transacao, error) {
	r, err := d.Cliente.ConsultarCobranca(ctx, identifier)
	if err != nil {
		return Transacao{}, fmt.Errorf("doacao: consultando %q: %w", identifier, err)
	}
	return Transacao{
		Existe:        strings.EqualFold(strings.TrimSpace(r.Estado), "achada"),
		Status:        r.Status,
		ValorCentavos: r.ValorCentavos,
		Descricao:     r.Descricao,
	}, nil
}

// DoServico liga a varredura ao caminho que já dá o crédito.
//
// O MESMO ConfirmTopupOrder do aviso, e não uma escrita própria: é ele que é
// idempotente na referência, e é ele que credita exatamente uma vez. Uma segunda
// forma de creditar seria a segunda chance de creditar duas vezes.
type DoServico struct {
	Servico *donatetopup.Service
}

// ConfirmTopupOrder credita, e devolve erro só quando a infraestrutura falha.
//
// "Já confirmado" NÃO é erro: é o normal quando o aviso chegou entre a consulta e
// esta chamada, que é uma corrida que acontece e não estraga nada — o crédito saiu
// uma vez só.
func (d DoServico) ConfirmTopupOrder(ctx context.Context, externalRef string) error {
	_, _, err := d.Servico.ConfirmTopupOrder(ctx, externalRef)
	return err
}
