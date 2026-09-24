// Package ponte fala com a ponte de repasse, que é quem fala com a processadora.
//
// O JOGO E O WEB-API NUNCA FALAM COM A PROCESSADORA DIRETO, e a razão não é
// organização: a API de saque dela exige IP autorizado, e a Railway não dá IP de
// saída fixo no plano comum. A ponte roda na VPS, que tem IP fixo. Ela é o único
// lugar do sistema que conhece as credenciais da processadora.
//
// DUAS TRAVAS NA PORTA, e as duas são necessárias:
//
//   - mTLS: o nginx da VPS recusa quem não apresentar certificado de cliente
//     assinado pela CA da ponte, antes de a requisição chegar no Node;
//   - HMAC: cada corpo vai assinado com um segredo compartilhado, com o instante
//     dentro da assinatura e uma janela de cinco minutos.
//
// Uma sozinha não bastaria. O mTLS morre se o certificado vazar; o HMAC morre se o
// segredo vazar. Juntas, precisam dos dois.
//
// SEM AS QUATRO VARIÁVEIS, O CLIENTE FICA DESLIGADO e o boot avisa — não derruba o
// servidor. É o mesmo desenho do saldo e da cobrança: melhor um caminho recusando
// em voz alta do que um servidor que não sobe.
package ponte

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrDesligada é o que sai quando faltou configuração. Recusa PREVISTA: o modo sem
// ponte é o normal em desenvolvimento.
var ErrDesligada = errors.New("ponte: o cliente nao esta configurado")

// ErrIncerta é a resposta que NÃO diz se deu certo.
//
// Ela existe separada do erro comum porque a diferença muda o que quem chama pode
// fazer: uma falha de rede antes do envio é segura para repetir; uma resposta
// perdida DEPOIS de a chamada sair pode ter criado a cobrança lá. Repetir com a
// mesma referência é seguro (a ponte é idempotente), mas tratar como "não
// aconteceu" e soltar o item é o que este erro impede de virar descuido.
var ErrIncerta = errors.New("ponte: a chamada saiu e a resposta nao voltou")

// ErroHTTP e a recusa na porta: corpo malformado, assinatura errada, corpo grande
// demais, rota que nao existe.
//
// Carrega o CODIGO porque quem chama precisa distinguir: 401 e segredo, 404 e URL,
// 400 e o corpo. Sao consertos diferentes, e um erro que dissesse so "recusado"
// mandaria procurar nos tres.
type ErroHTTP struct {
	Rota   string
	Codigo int
	Corpo  string
}

func (e *ErroHTTP) Error() string {
	return fmt.Sprintf("ponte: %s recusado pela ponte: http %d: %s", e.Rota, e.Codigo, e.Corpo)
}

// Config são as quatro variáveis que a Hanna põe na Railway. Nenhuma delas é lida
// de arquivo: a Railway não tem sistema de arquivos persistente, então o
// certificado e a chave vêm como conteúdo PEM na própria variável.
type Config struct {
	URL      string
	Segredo  string
	CertPEM  string
	ChavePEM string
	// Timeout vale para cada chamada. Zero usa o padrão.
	Timeout time.Duration
}

// Completa diz se as quatro estão presentes. Faltando uma, o cliente não sobe.
func (c Config) Completa() bool {
	return strings.TrimSpace(c.URL) != "" && strings.TrimSpace(c.Segredo) != "" &&
		strings.TrimSpace(c.CertPEM) != "" && strings.TrimSpace(c.ChavePEM) != ""
}

// Faltando lista, por NOME, o que não veio — para o aviso do boot dizer o que
// fazer em vez de só dizer que não deu.
//
// Nomes e nunca valores: um log que imprima metade de um segredo para "ajudar a
// depurar" é um segredo publicado.
func (c Config) Faltando() []string {
	var falta []string
	for _, v := range []struct {
		nome, valor string
	}{
		{"PONTE_URL", c.URL},
		{"PONTE_SEGREDO", c.Segredo},
		{"PONTE_CERT_CLIENTE", c.CertPEM},
		{"PONTE_CHAVE_CLIENTE", c.ChavePEM},
	} {
		if strings.TrimSpace(v.valor) == "" {
			falta = append(falta, v.nome)
		}
	}
	return falta
}

// pemDeVariavel aceita o PEM dos dois jeitos que ele chega de uma variavel de
// ambiente: com quebras de linha de verdade, ou com a sequencia de duas letras
// barra-n no lugar delas.
//
// Os dois acontecem, e qual deles depende de como a pessoa colou o bloco no painel
// da Railway. Aceitar so um faria o certificado "nao carregar" por um motivo
// invisivel - o valor esta certo, a formatacao e que nao -, e o erro de TLS que
// sairia disso nao aponta para lugar nenhum. O site ja faz o mesmo.
func pemDeVariavel(v string) []byte {
	v = strings.TrimSpace(v)
	if !strings.Contains(v, "\n") {
		v = strings.ReplaceAll(v, "\\n", "\n")
	}
	return []byte(v)
}

// horaOpcional aceita null, string vazia e RFC3339.
//
// A ponte converte string vazia em null antes de responder, então o caso não
// deveria chegar. Tratá-lo custa três linhas e evita que a venda inteira pare com
// "não consegui ler a data" se aquele lado mudar — e um campo de data vazio é
// exatamente o tipo de coisa que muda sem ninguém avisar.
type horaOpcional struct{ time.Time }

func (h *horaOpcional) UnmarshalJSON(b []byte) error {
	txt := strings.Trim(string(b), `"`)
	if txt == "" || txt == "null" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, txt)
	if err != nil {
		return fmt.Errorf("ponte: hora %q ilegivel: %w", txt, err)
	}
	h.Time = t
	return nil
}

// Quando devolve nil, a processadora não disse a hora — e é isso que faz o
// servidor cair para o próprio relógio.
func (h *horaOpcional) Valor() *time.Time {
	if h == nil || h.IsZero() {
		return nil
	}
	t := h.Time
	return &t
}

const timeoutPadrao = 15 * time.Second

// Cliente fala com a ponte.
type Cliente struct {
	url     string
	segredo []byte
	http    *http.Client
	agora   func() time.Time // injetável para o teste da assinatura
}

// Novo monta o cliente. Devolve ErrDesligada quando falta configuração.
func Novo(c Config) (*Cliente, error) {
	if !c.Completa() {
		return nil, fmt.Errorf("%w: faltam %s", ErrDesligada, strings.Join(c.Faltando(), ", "))
	}
	par, err := tls.X509KeyPair(pemDeVariavel(c.CertPEM), pemDeVariavel(c.ChavePEM))
	if err != nil {
		return nil, fmt.Errorf("ponte: o certificado de cliente nao carrega: %w", err)
	}
	prazo := c.Timeout
	if prazo <= 0 {
		prazo = timeoutPadrao
	}
	return &Cliente{
		url:     strings.TrimRight(strings.TrimSpace(c.URL), "/"),
		segredo: []byte(c.Segredo),
		http: &http.Client{
			Timeout: prazo,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{par},
					MinVersion:   tls.VersionTLS12,
					// NADA DE InsecureSkipVerify. O certificado do servidor é
					// Let's Encrypt e o sistema sabe verificá-lo. Pular a
					// verificação aqui tornaria o mTLS teatro: qualquer um no
					// caminho poderia se passar pela ponte e receber os pedidos
					// de cobrança.
				},
			},
		},
		agora: time.Now,
	}, nil
}

// --- os corpos, exatamente como o contrato ---

type pedidoCobranca struct {
	Referencia    string `json:"referencia"`
	ValorCentavos int64  `json:"valorCentavos"`
	Descricao     string `json:"descricao"`
}

// RespostaCobranca é o que volta de /cobranca.
type RespostaCobranca struct {
	Estado      string `json:"estado"` // criada | repetida | recusada | incerta
	Identifier  string `json:"identifier"`
	PixCode     string `json:"pixCode"`
	Motivo      string `json:"motivo"`
	HTTPSyncpay int    `json:"httpSyncpay"`
}

type pedidoConsulta struct {
	Identifier string `json:"identifier"`
}

// RespostaConsulta é o que volta de /cobranca/consulta.
//
// O status vai CRU, como a processadora o devolveu, e quem normaliza é quem chama.
// A regra vale para os dois lados desta fronteira: a ponte não interpreta e nós não
// pedimos que ela interprete.
type RespostaConsulta struct {
	Estado        string `json:"estado"` // achada | inexistente
	Fonte         string `json:"fonte"`  // v2 | v1
	Status        string `json:"status"`
	ValorCentavos int64  `json:"valorCentavos"`
	Referencia    string `json:"referencia"`
	// PagoEm e DevolvidoEm são ponteiros porque NULO e "zero" são coisas
	// diferentes: nulo quer dizer "a processadora não disse", e é isso que faz o
	// servidor cair para o próprio relógio. Um time.Time zero diria "1 de janeiro
	// do ano 1", que compararia como muito antigo e entregaria tudo.
	PagoEm      *horaOpcional `json:"pagoEm"`
	DevolvidoEm *horaOpcional `json:"devolvidoEm"`
}

type pedidoReembolso struct {
	Identifier string `json:"identifier"`
	Referencia string `json:"referencia"`
	Detalhes   string `json:"detalhes"`
}

// RespostaReembolso é o que volta de /cobranca/reembolso.
//
// O CodigoSyncpay passa INTACTO. A diferença entre "reembolso não habilitado para
// esta conta" e "conta do seller não está aprovada" decide o que pedir ao suporte,
// e as duas viram "falhou" se a gente resumir.
type RespostaReembolso struct {
	Estado        string `json:"estado"` // pedido | repetido | recusado | incerto
	HTTPSyncpay   int    `json:"httpSyncpay"`
	CodigoSyncpay string `json:"codigoSyncpay"`
}

// CriarCobranca pede o Pix.
func (c *Cliente) CriarCobranca(ctx context.Context, referencia string, centavos int64,
	descricao string,
) (RespostaCobranca, error) {
	var r RespostaCobranca
	err := c.chama(ctx, "/cobranca", pedidoCobranca{
		Referencia: referencia, ValorCentavos: centavos, Descricao: descricao,
	}, &r)
	return r, err
}

// ConsultarCobranca pergunta o que aconteceu de verdade.
//
// É por ela que o servidor decide, e só por ela: o aviso de pagamento é um toque de
// campainha, não prova.
func (c *Cliente) ConsultarCobranca(ctx context.Context, identifier string) (RespostaConsulta, error) {
	var r RespostaConsulta
	err := c.chama(ctx, "/cobranca/consulta", pedidoConsulta{Identifier: identifier}, &r)
	return r, err
}

// PedirReembolso abre o pedido de devolução.
func (c *Cliente) PedirReembolso(ctx context.Context, identifier, referencia,
	detalhes string,
) (RespostaReembolso, error) {
	var r RespostaReembolso
	err := c.chama(ctx, "/cobranca/reembolso", pedidoReembolso{
		Identifier: identifier, Referencia: referencia, Detalhes: detalhes,
	}, &r)
	return r, err
}

type pedidoRepasse struct {
	Referencia    string `json:"referencia"`
	ValorCentavos int64  `json:"valorCentavos"`
	ChavePix      string `json:"chavePix"`
	TipoChave     string `json:"tipoChave"`
	Documento     string `json:"documento"`
}

// RespostaRepasse é o que volta de /repasse.
//
// OS QUATRO ESTADOS NÃO SÃO GRAUS DE SUCESSO, são coisas diferentes, e confundir dois
// deles custa um pagamento a mais:
//
//	aceito    o saque foi criado. ChaveGateway é o id dele lá.
//	repetido  esta referência já tinha sido usada. NÃO chamou a processadora de novo,
//	          e devolve a ChaveGateway da primeira vez. É a idempotência funcionando.
//	recusado  CERTEZA de que nada saiu. É o único estado em que tentar de novo é
//	          seguro, e o único que libera a referência.
//	incerto   a chamada saiu e a resposta não voltou. PODE TER PAGO. A referência
//	          trava para sempre, e reenviar é a única coisa que não se desfaz.
type RespostaRepasse struct {
	Estado string `json:"estado"`
	// ChaveGateway é o id do saque na processadora, e é por ele que o aviso de saque
	// encontra o repasse depois. Vem no aceito e no repetido.
	ChaveGateway string `json:"chaveGateway"`
	Motivo       string `json:"motivo"`
	// HTTPSyncpay NULO quer dizer que quem recusou foi a PRÓPRIA PONTE — teto, ou a
	// trava do saque desligada — e isso não é culpa do vendedor. Com número, a recusa é
	// da processadora, e o CodigoSyncpay passa intacto ao lado.
	HTTPSyncpay   *int32 `json:"httpSyncpay"`
	CodigoSyncpay string `json:"codigoSyncpay"`
}

// TipoDeChaveNaPonte traduz o tipo da chave para o texto que a ponte espera.
//
// Mapa explícito e não conversão: os dois conjuntos vivem em lados diferentes da rede e
// mudam por motivos diferentes. Um cast passaria a mentir em silêncio no dia em que um
// deles ganhasse um valor no meio — e aqui a mentira manda dinheiro para o tipo errado
// de chave, que a processadora recusa sem dizer por quê.
//
// Devolve vazio para tipo desconhecido, e quem chama recusa antes de mandar: um tipo
// vazio no corpo é 400, e é melhor falhar aqui, onde dá para dizer qual era o tipo.
func TipoDeChaveNaPonte(tipo int16) string {
	switch tipo {
	case 1:
		return "cpf"
	case 2:
		return "email"
	case 3:
		return "phone"
	case 4:
		return "evp"
	}
	return ""
}

// Repassar manda o dinheiro da venda para o vendedor.
//
// A REFERÊNCIA É A CHAVE DE IDEMPOTÊNCIA DELES, e é a única. Valor, chave e documento
// são conferidos contra o que foi mandado da primeira vez: na mesma referência com
// qualquer um deles diferente, a ponte recusa com alerta e NÃO paga uma segunda vez.
//
// Quem escolhe a referência é quem chama, e ela é por TENTATIVA e não por dívida — ver
// store.ReferenciaDaTentativa, que explica o que isso custou.
//
// COMO OS QUATRO ESTADOS CHEGAM AQUI, porque dois deles NÃO vêm na resposta:
//
//	aceito, repetido  200, decodificados na resposta.
//	recusado          422, TAMBÉM decodificado na resposta, e NÃO é erro Go. É
//	                  informação: nada saiu, e o motivo está nos campos.
//	incerto           5xx, e vem como ErrIncerta — erro Go, com a resposta VAZIA.
//
// Quem chama TEM de tratar o ErrIncerta como o estado incerto, e nunca como uma falha
// qualquer que se possa repetir. É o caso em que o pagamento PODE ter saído: repetir é
// a única coisa deste sistema que não se desfaz, e não há consulta de saque para
// desempatar. A referência trava para sempre do lado da ponte.
//
// O `motivo` do incerto se perde nessa tradução, e é uma perda aceita: o que decide o
// que fazer é o estado, e ele chega inteiro.
func (c *Cliente) Repassar(ctx context.Context, referencia string, centavos int64,
	chavePix, tipoChave, documento string,
) (RespostaRepasse, error) {
	var r RespostaRepasse
	err := c.chama(ctx, "/repasse", pedidoRepasse{
		Referencia: referencia, ValorCentavos: centavos,
		ChavePix: chavePix, TipoChave: tipoChave, Documento: documento,
	}, &r)
	return r, err
}

// chama monta, assina e envia.
//
// A ASSINATURA COBRE O INSTANTE E O CORPO, nessa ordem, separados por uma quebra de
// linha: `HMAC(segredo, "<epoch ms>\n<corpo cru>")`. O instante dentro da
// assinatura é o que impede repetir uma requisição capturada — mudá-lo invalida a
// assinatura, e a ponte recusa o que estiver fora de uma janela de cinco minutos.
//
// ASSINA OS BYTES QUE VÃO NO FIO, e não o objeto. Serializar duas vezes pode dar
// dois textos diferentes; assinar um e mandar o outro dá 401 num lugar onde a
// mensagem de erro não explica nada.
func (c *Cliente) chama(ctx context.Context, rota string, corpo, saida any) error {
	cru, err := json.Marshal(corpo)
	if err != nil {
		return fmt.Errorf("ponte: montando o corpo de %s: %w", rota, err)
	}
	em := strconv.FormatInt(c.agora().UnixMilli(), 10)

	mac := hmac.New(sha256.New, c.segredo)
	mac.Write([]byte(em))
	mac.Write([]byte("\n"))
	mac.Write(cru)
	assinatura := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+rota, bytes.NewReader(cru))
	if err != nil {
		return fmt.Errorf("ponte: montando o pedido de %s: %w", rota, err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-ponte-em", em)
	req.Header.Set("x-ponte-assinatura", assinatura)

	resp, err := c.http.Do(req)
	if err != nil {
		// A chamada SAIU e a resposta não voltou. Para a criação de cobrança isso
		// pode significar que existe uma cobrança lá que ninguém conhece — sem
		// pix_code entregue a ninguém, e portanto impagável, mas existindo.
		return fmt.Errorf("%w: %s: %v", ErrIncerta, rota, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Teto na leitura: a ponte é nossa e responde pouco, mas um corpo sem fim do
	// outro lado não pode virar memória sem fim deste.
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %s: lendo a resposta: %v", ErrIncerta, rota, err)
	}

	switch {
	case resp.StatusCode >= 500:
		// DEFEITO DA PONTE, e o servidor trata como "não sei" e NUNCA como recusa.
		// Ler um 500 como "a processadora disse não" transformaria um bug nosso em
		// venda negada.
		return fmt.Errorf("%w: %s: http %d", ErrIncerta, rota, resp.StatusCode)
	case resp.StatusCode == http.StatusOK, resp.StatusCode == http.StatusUnprocessableEntity:
		// 200 e 422 trazem corpo do contrato; o 422 é recusa PREVISTA e o estado
		// dela vem no corpo, não no código.
		if err := json.Unmarshal(dados, saida); err != nil {
			return fmt.Errorf("ponte: %s: resposta ilegivel: %w", rota, err)
		}
		return nil
	default:
		// 400, 401, 413 e afins: erro NOSSO — corpo malformado, assinatura errada,
		// corpo grande demais. Não é recusa da processadora e não pode ser tratado
		// como tal.
		return &ErroHTTP{Rota: rota, Codigo: resp.StatusCode, Corpo: primeirasLinhas(dados)}
	}
}

// primeirasLinhas corta a resposta de erro para o log. Sem isto, um HTML de erro
// do nginx entraria inteiro na linha de log.
func primeirasLinhas(b []byte) string {
	const teto = 200
	s := strings.TrimSpace(string(b))
	if len(s) > teto {
		return s[:teto] + "..."
	}
	return s
}

// Sonda prova, no boot, que as DUAS travas da porta estao certas — sem efeito
// nenhum do outro lado.
//
// Ela existe porque o segredo e o certificado sao SELADOS na Railway: ninguem
// consegue rele-los para conferir se o bloco foi colado inteiro. A unica prova e
// uma chamada que funcione.
//
// E ELA NAO USA O /saude, que seria o palpite obvio: aquele e GET e NAO e assinado,
// entao passaria com o segredo errado e provaria so metade. Um teste que passa com
// a configuracao quebrada e pior do que nenhum.
//
// O que ela faz: POST assinado em /cobranca/consulta com o corpo vazio. A ponte
// confere a assinatura ANTES de validar o corpo, entao:
//
//	400  -> SUCESSO. mTLS e assinatura conferem, e o corpo vazio foi recusado na
//	        validacao, sem chegar na processadora. Nada foi criado, nada foi pago.
//	401  -> o SEGREDO esta errado, ou o relogio esta fora da janela de 5 minutos.
//	404  -> a URL esta errada.
//	erro de TLS -> o CERTIFICADO ou a CHAVE estao errados, ou nao formam par.
//
// SIM, 400 E O SUCESSO, e nao e engano. Se alguem "consertar" isto daqui a um mes
// trocando por 200, a sonda vai reprovar uma configuracao correta. O custo do
// desenho e uma linha de recusa por boot no log da ponte, e ele foi aceito por
// quem cuida dela.
func (c *Cliente) Sonda(ctx context.Context) error {
	var vazio struct{}
	err := c.chama(ctx, "/cobranca/consulta", struct{}{}, &vazio)

	var http *ErroHTTP
	if errors.As(err, &http) && http.Codigo == 400 {
		return nil
	}
	if err != nil {
		return err
	}
	// 200 num corpo vazio significa que a ponte parou de validar. Nao e sucesso:
	// e um aviso de que a porta ficou mais permissiva do que o contrato diz.
	return errors.New("ponte: a consulta aceitou um corpo vazio; a validacao dela mudou")
}
