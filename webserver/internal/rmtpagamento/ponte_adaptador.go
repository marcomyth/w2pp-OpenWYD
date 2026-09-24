package rmtpagamento

import (
	"context"
	"fmt"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

// MarcaRMT é o prefixo da descrição que vai para a processadora.
//
// EXISTE PARA O SITE, e não para o comprador: o webhook da processadora é por
// CONTA e não por cobrança, então a cobrança do mercado e a recarga da loja de
// doação se anunciam pelo mesmo canal. É por esta marca que o site reconhece qual
// dos dois chegou e decide se repassa para cá.
//
// O resto da descrição é escrito para ser LIDO: ela aparece no aplicativo do banco
// de quem paga, e "rmt:ref-7f3a" não diz a ninguém o que está sendo comprado.
const MarcaRMT = "rmt:"

// Descricao monta o texto que a processadora mostra ao pagador.
func Descricao(referencia string) string {
	return fmt.Sprintf("%s compra no mercado de jogadores (%s)", MarcaRMT, referencia)
}

// PonteDeVerdade adapta o cliente da ponte ao que este pacote precisa.
//
// A TRADUÇÃO EXISTE POR UM MOTIVO CONCRETO, e não por gosto de camada: o campo de
// hora da resposta da ponte tem tipo NÃO EXPORTADO, então nenhum teste fora daquele
// pacote consegue montar uma resposta com a hora preenchida. Sem esta tradução, o
// caminho que decide entregar ou devolver um item ficaria sem teste justamente no
// campo que toma essa decisão.
type PonteDeVerdade struct {
	Cliente *ponte.Cliente
}

// ConsultarTransacao pergunta à processadora e traduz a resposta.
func (p PonteDeVerdade) ConsultarTransacao(ctx context.Context, identifier string) (Transacao, error) {
	r, err := p.Cliente.ConsultarCobranca(ctx, identifier)
	if err != nil {
		return Transacao{}, fmt.Errorf("consultando a transacao %q: %w", identifier, err)
	}
	t := Transacao{
		Status:        r.Status,
		Referencia:    r.Referencia,
		ValorCentavos: r.ValorCentavos,
	}
	// "achada" é o único estado que diz que existe transação. Comparado em minúsculo
	// e sem espaço porque a única coisa pior do que um contrato mudar é ele mudar de
	// caixa e a comparação falhar calada.
	t.Existe = strings.EqualFold(strings.TrimSpace(r.Estado), "achada")
	if r.PagoEm != nil {
		t.PagoEm = r.PagoEm.Valor()
	}
	return t, nil
}

// CriarPix é o que o store chama, com a linha da cobrança travada, para nascer o
// código na processadora.
//
// Devolve o código e o identifier DELES. Os dois vazios com erro nulo não é caminho
// previsto — quem chama recusa gravar código vazio, porque um código vazio gravado é
// indistinguível de nunca ter chamado.
func (p PonteDeVerdade) CriarPix(ctx context.Context, referencia string, centavos int64) (string, string, error) {
	r, err := p.Cliente.CriarCobranca(ctx, referencia, centavos, Descricao(referencia))
	if err != nil {
		return "", "", err
	}
	// "criada" e "repetida" são as duas respostas boas, e a segunda é a que faz a
	// idempotência valer: uma tentativa anterior que se perdeu no caminho já criou
	// esta cobrança lá, e a ponte devolve o MESMO identifier em vez de criar outra.
	switch estado := strings.ToLower(strings.TrimSpace(r.Estado)); estado {
	case "criada", "repetida":
		return r.PixCode, r.Identifier, nil
	default:
		// "recusada" e "incerta" e qualquer coisa nova. Erro, e NADA gravado: a
		// leitura seguinte tenta de novo com a mesma referência, e é exatamente o que
		// faz a resposta incerta ser segura de repetir.
		return "", "", fmt.Errorf("ponte recusou ou nao concluiu a cobranca %q: estado %q motivo %q http %d",
			referencia, r.Estado, r.Motivo, r.HTTPSyncpay)
	}
}
