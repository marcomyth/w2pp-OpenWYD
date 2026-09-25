package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A cobrança em Pix da prateleira em dinheiro real, vista do lado do jogo.
//
// O caminho inteiro, para quem chegar aqui sem contexto: o comprador clica em
// comprar NO JOGO, o servidor abre a cobrança, e ele PAGA NO SITE, na conta dele.
// O cliente do jogo não mostra o pagamento — conferido no GamePatch, não suposto:
// ele não desenha QR, não abre link e não copia para a área de transferência.
//
// Este arquivo é a porta entre o laço do jogo e quem sabe falar com a
// processadora. O jogo não fala com ninguém de fora: quem fala é o webserver, que
// é o dono da ponte.

// ErrPixNaoLigado é o que sai enquanto a ponte não está montada.
//
// Recusa PREVISTA e não falha: o modo sem ponte existe e é o normal em
// desenvolvimento. Melhor não vender do que vender sem cobrar.
var ErrPixNaoLigado = errors.New("loja: a cobranca por pix ainda nao esta ligada")

// CobrancaAberta é o que volta quando a cobrança nasce.
//
// O código vem como a processadora o devolveu e NÃO é tocado: um código Pix tem
// checksum de ponta a ponta, e um servidor que o "arrume" quebra o pagamento de um
// jeito que ninguém vê até o dinheiro não chegar.
type CobrancaAberta struct {
	CobrancaID int64
	CodigoPix  string
	ExpiraEm   time.Time
}

// CobradorPix é quem sabe abrir uma cobrança. Interface e não o cliente concreto
// pelo mesmo motivo do saldo: o jogo fica testável sem processadora e sem rede.
type CobradorPix interface {
	// AbrirCobranca cria a cobrança do anúncio para este comprador.
	//
	// A referência externa é NOSSA e vem pronta de quem chama: ela nasce antes de
	// qualquer chamada de rede porque é a âncora da idempotência — se a resposta
	// se perder no caminho, é por ela que a confirmação encontra a mesma linha.
	AbrirCobranca(ctx context.Context, anuncioID, compradorConta int64,
		referenciaExterna string) (CobrancaAberta, error)
}

type pixNaoLigado struct{}

func (pixNaoLigado) AbrirCobranca(context.Context, int64, int64, string) (CobrancaAberta, error) {
	return CobrancaAberta{}, ErrPixNaoLigado
}

var cobradorPix CobradorPix = pixNaoLigado{}

// UsaCobradorPix liga a loja à cobrança de verdade, na montagem do servidor.
func UsaCobradorPix(c CobradorPix) {
	if c != nil {
		cobradorPix = c
	}
}

// JanelaDeCobranca é quanto tempo o comprador tem para pagar.
//
// CONFIGURAÇÃO E NÃO CONSTANTE, e a diferença é o que se pode fazer com ela.
//
// QUINZE MINUTOS desde 25/09/2026, e a troca tem motivo medido: eram cinco, e cinco
// é apertado para uma pessoa pagando Pix DE VERDADE — abrir o aplicativo do banco,
// achar o Pix, colar o código, confirmar. Um pagamento que cai em 5min30 não entrega
// o item: ele vai para o caminho do PAGO COM ATRASO, com reembolso, e a pessoa vê
// "venceu" tendo pagado. Com dinheiro real no meio, esse susto custa mais do que o
// item do vendedor ficar preso dez minutos a mais.
//
// É número para MEDIR, e o que responde é o volume de `pago_com_atraso`. A troca
// é dos dois lados — janela curta prende menos o item do vendedor e faz o
// pagamento atrasado ser mais comum; janela longa faz o contrário. Quem responde é
// o volume de `pago_com_atraso`, que a 0105 guarda para esta pergunta.
//
// Variável de pacote e não campo de Config porque o valor tem de valer também para
// a expiração do lado do banco, que roda noutro processo: os dois leem a mesma
// variável de ambiente, e um valor que morasse só na Config do tmServer sairia de
// sincronia sem ninguém notar.
var JanelaDeCobranca = 15 * time.Minute

// DefineJanelaDeCobranca ajusta o prazo na montagem do servidor. Valor inválido é
// ignorado: um prazo zero faria toda cobrança nascer vencida.
func DefineJanelaDeCobranca(d time.Duration) {
	if d > 0 {
		JanelaDeCobranca = d
	}
}

// msgPagueNoSite é o que o comprador lê ao clicar em comprar.
//
// DUAS COISAS, e as duas são necessárias: ONDE pagar, porque o pagamento não está
// no jogo e ele não tem como adivinhar isso, e QUANTO TEMPO, porque a cobrança
// morre sozinha.
//
// ELA JÁ DISSE "NÃO SAIA DO JOGO", e a frase saiu porque o motivo dela saiu. Sair
// do jogo cancelava a cobrança, e o movimento natural de quem vai pagar no celular
// é fechar o jogo — o aviso existia para salvar a compra de quem fizesse o óbvio.
// Agora o logout do comprador não mexe na cobrança, então pagar pelo celular é
// seguro e pedir que ele fique seria pedir por nada.
//
// O PRAZO VEM DA CONFIGURAÇÃO e não está escrito na frase: com o número fixo, mudar
// a janela deixaria a mensagem mentindo, e mentir sobre prazo de pagamento é a
// mentira mais cara que esta tela pode contar.
//
// O ENDEREÇO É www.wydretry.com, MEDIDO e não suposto: `wydretry.com` sem o www
// não responde nada (curl 000), e só `www.wydretry.com` atende. A frase anterior
// mandava o jogador para um endereço morto.
func msgPagueNoSite() string {
	return fmt.Sprintf(
		"Pague em www.wydretry.com/conta em até %d minutos.",
		int(JanelaDeCobranca.Minutes()))
}

// msgCobrancaNaoSaiu é a falha de quem tentou comprar e não conseguiu nem começar.
//
// Diz para tentar de novo porque é isso que resolve: nada foi cobrado e nada foi
// reservado, então a segunda tentativa é limpa.
const msgCobrancaNaoSaiu = "Não deu para gerar o pagamento. Tente de novo em instantes."

// msgItemJaTemComprador é a recusa de quem chegou depois.
//
// Um item só pode ter uma cobrança aberta por vez — é a invariante que impede duas
// pessoas de pagarem pelo mesmo item e as duas terem razão. Quem chega depois
// espera o prazo do primeiro acabar, e a mensagem diz isso em vez de um "não pode"
// seco.
const msgItemJaTemComprador = "Alguém está pagando esse item agora. Tente de novo em alguns minutos."

// msgNaoComprarDeSiMesmo é a recusa de comprar o próprio anúncio.
//
// Não há ganho — o item volta para o mesmo baú e o dinheiro sai da própria conta.
// O custo é a CHAMADA: cada cobrança bate na processadora, que cobra por ela.
const msgNaoComprarDeSiMesmo = "Esse anúncio é seu."

// msgJaTemPagamentoAberto é a recusa de quem já está pagando outra coisa.
//
// Uma cobrança aberta por comprador, e não só uma por anúncio: sem isso, uma conta
// clica em todas as prateleiras do mercado e prende o estoque inteiro por cinco
// minutos, de graça. A mensagem diz o que resolve — terminar o que começou — em vez
// de um "não pode" seco.
const msgJaTemPagamentoAberto = "Você já tem um pagamento aberto. Pague ou espere ele vencer."

// msgRMTNaoEstaAberta é o que o jogador lê quando o mercado em dinheiro real não está
// disponível para ele.
//
// UMA FRASE PARA TRÊS CAMINHOS, de propósito: a ponte não montada, o mercado FECHADO
// (W2PP_RMT, 25/09/2026) e o mercado em "staff" para quem não é da casa. Para o jogador os
// três são a mesma coisa — não está aberto —, e frases diferentes convidariam a testar a
// outra ponta para ver se aquela funciona.
//
// E é uma CONSTANTE só, e não uma por caminho com o mesmo texto: duas constantes iguais é
// como uma delas muda e as telas passam a discordar sobre o mesmo fato.
//
// SEPARADA da falha, e a diferença não é de estilo: "tente de novo em instantes"
// manda a pessoa repetir uma coisa que nunca vai dar certo. Isto aqui não é uma
// falha passageira, é um caminho que ainda não existe, e dizer isso é o que evita
// que ela fique clicando.
const msgRMTNaoEstaAberta = "Venda por dinheiro real ainda não está aberta."

// abreCobrancaPix vai a processadora, pela ponte, e volta com o codigo para o
// comprador pagar no site.
//
// FORA DO LACO, porque fala com a rede: uma processadora lenta nao pode segurar o
// servidor inteiro. E por `Go` e nao `GoDetached` porque a volta e um AVISO ao
// comprador - se ele ja se foi, nao ha a quem avisar, e a cobranca que ficou
// aberta morre sozinha pelo prazo.
//
// O ANUNCIO E IDENTIFICADO PELA MARCA DO BAU e nao por um numero que viaje na
// prateleira. A marca e o que o banco tambem enxerga: e ela que a abertura da
// cobranca confere, e conferir contra a mesma coisa dos dois lados e o que impede
// que uma prateleira desatualizada cobre pelo anuncio errado.
func (d *Dispatcher) abreCobrancaPix(w *world.World, s *world.Session, anuncioID int64, preco int32) {
	if anuncioID == 0 {
		// Prateleira em dinheiro real sem marca no item: e o anuncio orfao, que a
		// reconciliacao limpa. Nao ha o que cobrar, e cobrar assim mesmo geraria
		// dinheiro entrando sem nada para entregar.
		d.log.Warn("loja: prateleira em dinheiro real sem marca de escrow",
			"conn", s.Conn, "conta", s.AccountID)
		sendClientMessage(w, s, msgCobrancaNaoSaiu)
		return
	}
	conta, conn := s.AccountID, s.Conn
	// A REFERENCIA NASCE AQUI, ANTES DA CHAMADA, e e nossa. Ela e a ancora da
	// idempotencia: se a resposta da processadora se perder no caminho, e por ela
	// que a confirmacao encontra a mesma linha em vez de criar outra.
	//
	// Gerada de um id que nao se repete, e nao do par anuncio+comprador: o mesmo
	// comprador pode tentar o mesmo anuncio de novo depois de um prazo vencido, e
	// isso e uma cobranca NOVA, nao a mesma.
	referencia, err := referenciaDeCobranca()
	if err != nil {
		// NÃO ABRE A COBRANÇA. O caminho antigo caía para um número derivado do
		// relógio, e o comentário dele mesmo dizia que número previsível é aviso
		// que se forja — quem adivinhasse a referência poderia forjar a
		// confirmação de um pagamento que nunca existiu. Não gerar é o erro
		// barato: ninguém perde nada e o clique seguinte tenta de novo.
		d.log.Error("loja: nao consegui gerar a referencia da cobranca",
			"conn", s.Conn, "conta", s.AccountID, "err", err)
		sendClientMessage(w, s, msgCobrancaNaoSaiu)
		return
	}
	cobrador := cobradorPix
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cob, err := cobrador.AbrirCobranca(ctx, anuncioID, conta, referencia)
		return func(w *world.World, s *world.Session) {
			switch {
			case errors.Is(err, ErrCobrancaJaAberta):
				sendClientMessage(w, s, msgItemJaTemComprador)
			case errors.Is(err, ErrCompradorEOVendedor):
				sendClientMessage(w, s, msgNaoComprarDeSiMesmo)
			case errors.Is(err, ErrCompradorJaTemCobranca):
				sendClientMessage(w, s, msgJaTemPagamentoAberto)
			case errors.Is(err, ErrPixNaoLigado):
				// Nem log de aviso: enquanto a ponte não está montada, isto é o
				// caminho NORMAL, e um aviso por clique encheria o log de ruído.
				sendClientMessage(w, s, msgRMTNaoEstaAberta)
			case errors.Is(err, ErrAnuncioIndisponivel):
				// Vendeu para outra pessoa, ou o vendedor fechou a barraca entre o
				// clique e a ida ao banco. A vitrine do comprador esta velha.
				//
				// O cadeado que nao bate cai aqui tambem, e o jogador ve a MESMA
				// coisa — mas ele e defeito nosso e nao curso normal, entao sai no
				// log com a conta e o anuncio para alguem poder ir olhar. Sem isso
				// ele some dentro do caso comum e ninguem descobre que existe.
				if errors.Is(err, ErrCadeadoNaoBate) {
					d.log.Warn("loja: o cadeado do item nao aponta para o anuncio",
						"conn", conn, "conta", conta, "anuncio", anuncioID)
				}
				d.notify(w, s, NoticeCantAutoTrade)
				d.mercadoMudou(w)
			case err != nil:
				d.log.Warn("loja: nao consegui abrir a cobranca em pix",
					"conn", conn, "conta", conta, "anuncio", anuncioID, "err", err)
				sendClientMessage(w, s, msgCobrancaNaoSaiu)
			default:
				d.log.Info("loja: cobranca em pix aberta", "conn", conn, "conta", conta,
					"anuncio", anuncioID, "cobranca", cob.CobrancaID,
					"centavos", preco, "expira_em", cob.ExpiraEm)
				sendClientMessage(w, s, msgPagueNoSite())
			}
		}
	})
}

// As recusas previstas da abertura, traduzidas para o jogo.
//
// Sao erros e nao campos de resposta porque atravessam uma fronteira de rede ate
// aqui; do lado do banco elas viajam num resultado, que e onde recusa prevista
// deve viajar. A traducao acontece no webserver.
var (
	// ErrCobrancaJaAberta: outra pessoa esta pagando este item agora.
	ErrCobrancaJaAberta = errors.New("loja: o anuncio ja tem cobranca aberta")
	// ErrCompradorEOVendedor: comprar o proprio anuncio.
	ErrCompradorEOVendedor = errors.New("loja: o comprador e o vendedor")
	// ErrAnuncioIndisponivel: o anuncio nao esta mais ativo.
	ErrAnuncioIndisponivel = errors.New("loja: o anuncio nao esta disponivel")
	// ErrCompradorJaTemCobranca: a conta ja esta pagando outro item.
	ErrCompradorJaTemCobranca = errors.New("loja: o comprador ja tem cobranca aberta")
	// ErrCadeadoNaoBate: o anuncio esta ATIVO e a marca do escrow no item nao aponta
	// para ele. As duas coisas discordam, e discordancia em linha de dinheiro se
	// resolve nao cobrando.
	//
	// EMBRULHA O ErrAnuncioIndisponivel de proposito, e e essa a ideia: para o
	// jogador os dois sao a mesma frase, porque nenhum dos dois sugere uma acao
	// diferente — "este item nao esta compravel e a sua vitrine esta velha". Mas este
	// aqui e DEFEITO NOSSO, e nao o curso normal de um item que foi vendido, entao
	// alguem tem de ver. O errors.Is continua casando com o generico, e quem quiser
	// distinguir consegue.
	ErrCadeadoNaoBate = fmt.Errorf("loja: o cadeado do item nao aponta para o anuncio: %w",
		ErrAnuncioIndisponivel)
)

// referenciaDeCobranca gera a referencia externa da cobranca.
//
// Aleatoria e nao sequencial: ela vai viajar ate a processadora e voltar num
// aviso, e um numero que se pode adivinhar e um aviso que se pode forjar. O
// prefixo e so para quem for ler um log saber de onde ela veio.
func referenciaDeCobranca() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Sem saida de emergencia. Um numero derivado do relogio seria adivinhavel,
		// e a referencia e o que a confirmacao usa para reconhecer um pagamento -
		// adivinha-la e poder forjar a confirmacao de um pagamento que nao houve.
		return "", fmt.Errorf("loja: gerando a referencia da cobranca: %w", err)
	}
	// TRINTA E DOIS HEX MINÚSCULOS, SEM PREFIXO, e o "sem prefixo" é o conserto.
	//
	// Isto devolvia "rmt-" + 32 hex = 36 caracteres, e a ponte recusava TODA cobrança
	// com http 400 e "referencia fora do formato (32 caracteres hex minusculos)". O
	// teste de venda real da Hanna ficou parado em "gerando o código", repetindo a
	// recusa a cada cinco segundos, em 24/09/2026.
	//
	// O contrato é de 32 hex e está escrito em dois lugares que eu podia ter lido: a
	// ponte valida em src/cobranca.js, e o reconhecimento do aviso de pagamento
	// procura /rmt:([0-9a-f]{32})/ na descrição — o "rmt:" ali é do TEXTO da
	// descrição, não da referência. Foi essa a confusão.
	//
	// E o gerador irmão, o do repasse (store.ReferenciaDaTentativa), já fazia certo
	// com um comentário dizendo "a ponte valida 32 hex minúsculos". A regra estava no
	// arquivo vizinho.
	return hex.EncodeToString(b[:]), nil
}
