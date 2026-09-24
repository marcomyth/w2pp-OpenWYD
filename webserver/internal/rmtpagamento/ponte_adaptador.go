package rmtpagamento

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

// LimiteDaDescricao é o corte que a ponte aceita.
const LimiteDaDescricao = 60

// A MARCA "rmt:" NÃO É POSTA AQUI, e escrever isto é o ponto deste comentário: a
// ponte monta o description final sozinha, como "rmt:<referencia> | <descricao>",
// com a marca no começo para sobreviver a um corte de quem for exibir. Ela RECUSA
// com 400 uma descrição que contenha a sequência "rmt:", justamente para ninguém
// conseguir forjar uma segunda marca e fazer a consulta ler a referência errada.
//
// Então o que sai daqui é SÓ o texto do item. A referência viaja no campo dela.
//
// E o texto é escrito para ser LIDO por uma pessoa, porque é ele que aparece no
// aplicativo do banco de quem paga — e é o que essa pessoa vai ter na frente se um
// dia abrir contestação de Pix sobre esta compra. "Compra no mercado" e o nome do
// item respondem "o que eu paguei"; a referência em hexadecimal não responde nada.
//
// Descricao devolve o texto do item, cortado com segurança.
func Descricao(nomeDoItem string, refino int) string {
	txt := "Compra no mercado de jogadores"
	if n := limpa(nomeDoItem); n != "" {
		txt = "Compra no mercado: " + n
		if refino > 0 {
			txt += fmt.Sprintf(" +%d", refino)
		}
	}
	// A MARCA É ARRANCADA, e não só evitada no texto que escrevemos aqui.
	//
	// O nome do item vem do ItemList.csv do cliente legado: ninguém nosso escolhe
	// esses nomes, e o conteúdo é editável. Um item chamado "rmt:algo" — por acidente
	// ou porque alguém mexeu no catálogo — faria a ponte recusar com 400 TODA cobrança
	// daquele item, e a recusa não diria por quê. Ficaria um item invendável e um erro
	// sem pista.
	//
	// Sem distinguir caixa porque a comparação do outro lado pode ser de qualquer
	// jeito, e tirar demais aqui custa uma palavra feia na descrição, enquanto tirar
	// de menos custa a venda.
	return corta(semMarca(txt), LimiteDaDescricao)
}

// marca é a sequência que a ponte reserva para si. Aqui ela só serve para ser
// PROCURADA e removida: quem a põe no description final é a ponte.
const marca = "rmt:"

// contemMarca diz se o texto carrega a marca, ignorando caixa.
func contemMarca(s string) bool {
	return strings.Contains(strings.ToLower(s), marca)
}

// semMarca tira a marca, deixando o resto legível.
//
// Troca por "rmt " e não por nada: o que ofende a ponte é os dois-pontos depois de
// "rmt", e apagar a palavra inteira mudaria o nome do item na cara de quem paga.
func semMarca(s string) string {
	if !contemMarca(s) {
		return s
	}
	var b strings.Builder
	baixo := strings.ToLower(s)
	for i := 0; i < len(s); {
		if strings.HasPrefix(baixo[i:], marca) {
			b.WriteString(s[i : i+len(marca)-1]) // o "rmt", sem os dois-pontos
			b.WriteByte(' ')
			i += len(marca)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// limpa tira controle e espaço de sobra do nome que veio do catálogo.
//
// O catálogo nasce de um CSV do cliente legado, que não tem citação nem escape: um
// nome com um byte estranho no meio é plausível, e a ponte recusa caractere de
// controle. Recusar uma cobrança por causa de um nome de item é o pior jeito de
// descobrir isso.
func limpa(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
			// Descartado sem substituto: um espaço aqui viraria espaço duplo.
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// corta trunca por RUNA e não por byte.
//
// Cortar no meio de uma runa deixa bytes inválidos no fim do texto, e a ponte
// recusa isso como caractere de controle — quer dizer, um nome de item acentuado
// perto do limite derrubaria a cobrança de um jeito que não se reproduz com nome
// curto.
func corta(s string, limite int) string {
	r := []rune(s)
	if len(r) <= limite {
		return s
	}
	return strings.TrimSpace(string(r[:limite]))
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
func (p PonteDeVerdade) CriarPix(ctx context.Context, referencia string, centavos int64,
	descricao string,
) (string, string, error) {
	r, err := p.Cliente.CriarCobranca(ctx, referencia, centavos, descricao)
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
