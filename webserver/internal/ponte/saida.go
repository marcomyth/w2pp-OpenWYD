package ponte

import (
	"context"
	"errors"
)

// O lado de SAÍDA: os dois pedidos que nós fazemos à ponte.
//
// ELES AINDA NÃO EXISTEM DO OUTRO LADO. Em 22/09/2026 a dupla do site confirmou
// três coisas: a ponte escuta só em 127.0.0.1, sem domínio e sem interface
// pública; `gerar-cobranca` e `cancelar-cobranca` são construção nova do lado
// deles; e ela nunca moveu dinheiro real — o caminho de saque só rodou em modo
// falso, e mesmo no modo real recusa antes de qualquer chamada de rede.
//
// Por isso este arquivo tem a interface e um talão que RECUSA, e não uma
// implementação pela metade. Quando o endereço e a credencial chegarem, é
// configuração e não reescrita.
//
// RECUSAR ALTO, E NUNCA DEVOLVER SILÊNCIO. Uma interface sem implementação que
// devolve zero e nil é armadilha: o código que a usa parece funcionar, o teste do
// caminho feliz passa, e o defeito só aparece quando alguém está esperando um QR
// que nunca vai chegar. Recusando desde o primeiro dia, o caminho triste existe e
// é testado antes de a ponte existir.
//
// É o mesmo desenho do `saldoNaoLigado` em tmserver/internal/handler/lojasaldo.go,
// e pelo mesmo motivo: melhor não vender do que vender sem cobrar.

// ErrSaidaNaoLigada é o que sai enquanto a ponte não estiver publicada e
// configurada.
var ErrSaidaNaoLigada = errors.New("ponte: a saida para a ponte ainda nao esta ligada")

// Cobranca é o que a ponte devolve quando aceita gerar uma.
//
// O `valor_liquido` NÃO está aqui de propósito, e a razão é um fato que só
// apareceu quando o par do site foi ler a documentação da processadora: o webhook
// de saque devolve `amount` E `final_amount` — ela desconta uma taxa no saque, e
// quanto é só se sabe medindo o primeiro saque real.
//
// Daí a regra: o servidor NUNCA promete valor líquido antes de o repasse voltar.
// Ao vendedor se mostra o valor da VENDA; o líquido só vira número na tela depois
// que a ponte disser quanto saiu. Calcular por conta própria e exibir seria
// mentir com precisão.
type Cobranca struct {
	// ID da cobrança do lado da ponte. Guardamos para poder cancelá-la.
	ID string
	// PixCopiaCola é o texto que o jogador cola no aplicativo do banco.
	PixCopiaCola string
	// QRPayload é o mesmo conteúdo na forma que o cliente desenha.
	QRPayload string
	// ExpiraEmUnix é o prazo que a PONTE deu, e pode não ser o que pedimos: quem
	// decide o prazo de verdade é a processadora. Guardamos o dela, não o nosso.
	ExpiraEmUnix int64
}

// Saida é o que o webServer usa para falar com a ponte. Só o webServer a usa, e
// isso é a regra e não o acaso: dois serviços falando com a ponte seriam dois
// lugares guardando o segredo e dois lugares para conciliar dinheiro.
type Saida interface {
	// GerarCobranca pede uma cobrança de `valorCentavos` sob `referenciaExterna`,
	// que é NOSSA e é a chave pela qual a conciliação inteira acontece.
	GerarCobranca(ctx context.Context, referenciaExterna string, valorCentavos int64, descricao string) (Cobranca, error)

	// CancelarCobranca mata uma cobrança aberta. É chamada quando o comprador sai
	// do jogo: o QR morre com a sessão.
	//
	// CANCELAR NÃO É GARANTIA DE QUE NINGUÉM PAGOU. Entre o cancelamento e a
	// leitura do QR pelo banco cabe uma corrida que não é nossa para vencer, e por
	// isso o lado de entrada trata a confirmação atrasada como caso NORMAL e não
	// como erro — ver o estado PAGA_SEM_ITEM na migração 0105.
	CancelarCobranca(ctx context.Context, referenciaExterna string) error
}

// saidaNaoLigada é a implementação até a ponte existir: recusa as duas, sem
// tocar na rede.
type saidaNaoLigada struct{}

func (saidaNaoLigada) GerarCobranca(context.Context, string, int64, string) (Cobranca, error) {
	return Cobranca{}, ErrSaidaNaoLigada
}

func (saidaNaoLigada) CancelarCobranca(context.Context, string) error {
	return ErrSaidaNaoLigada
}

// NaoLigada devolve o talão que recusa. É o padrão: quem não configurou a ponte
// fica com ele, e descobre na primeira tentativa em vez de descobrir quando um
// jogador reclamar.
func NaoLigada() Saida { return saidaNaoLigada{} }
