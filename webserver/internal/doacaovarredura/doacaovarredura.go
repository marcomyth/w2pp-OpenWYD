// Package doacaovarredura é a rede embaixo do aviso de pagamento da doação.
//
// O CAMINHO ÚNICO QUE EXISTIA: a processadora avisa o site, o site chama o
// ConfirmTopupOrder, o crédito acontece. Um aviso perdido era dinheiro cobrado e
// Rcoin nunca dado, e NADA do lado do servidor era capaz de perceber — ele nunca
// tinha visto o id da processadora, e não se pergunta por um pagamento que não se
// sabe nomear.
//
// Agora o site entrega o id logo depois de criar a cobrança (AttachTopupCharge), e
// esta varredura pergunta. O aviso continua sendo o caminho rápido; isto é a rede.
//
// O QUE ELA CONFERE ANTES DE CREDITAR, e são três coisas e não uma:
//
//  1. a processadora diz "completed";
//  2. o valor bate com o do pedido;
//  3. a DESCRIÇÃO da transação traz a referência DAQUELE pedido.
//
// A terceira é a que protege dinheiro, e ela existe porque o id vem de fora. Quem
// chama o Attach é o site; um bug dele — ou uma chamada forjada — poderia prender a
// um pedido pendente o id de OUTRA transação já paga, do mesmo valor. Sem conferir a
// descrição, a varredura creditaria aquele pedido com o dinheiro de outra compra:
// uma doação, dois créditos, e o segundo sai do bolso da dona do servidor.
package doacaovarredura

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Consultor é a pergunta à processadora.
type Consultor interface {
	ConsultarTransacao(ctx context.Context, identifier string) (Transacao, error)
}

// Transacao é o que a consulta devolve, do tamanho que este pacote usa.
type Transacao struct {
	Existe        bool
	Status        string
	ValorCentavos int64
	// Descricao é o texto CRU da processadora. Vazio quer dizer "não sei" — a ponte
	// em produção ainda não manda este campo —, e é por isso que o vazio NUNCA
	// confirma. Ver o comentário de conferir.
	Descricao string
}

// Creditador é o caminho que já dá o crédito, e o mesmo que o aviso usa.
type Creditador interface {
	ConfirmTopupOrder(ctx context.Context, externalRef string) error
}

// Banco é o que a varredura lê.
type Banco interface {
	TopupsParaConferir(ctx context.Context, janela time.Duration, limite int) ([]store.TopupParaConferir, error)
	TopupsSemIdentifier(ctx context.Context, janela time.Duration) (int, error)
}

// statusPago é o único que credita. Igual ao do RMT, e pelo mesmo motivo: um
// estorno ou uma contestação também têm data de pagamento.
const statusPago = "completed"

// LimiteDaRodada é quantos pedidos uma passada confere.
const LimiteDaRodada = 100

// Varredura pergunta pelos pedidos pendentes e credita os que a processadora
// confirmar.
type Varredura struct {
	ponte  Consultor
	credit Creditador
	banco  Banco
	log    *slog.Logger
}

// Nova monta a varredura.
func Nova(ponte Consultor, credit Creditador, banco Banco, log *slog.Logger) *Varredura {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Varredura{ponte: ponte, credit: credit, banco: banco, log: log}
}

// Rodada é o que uma passada fez.
type Rodada struct {
	Conferidos int
	Creditados int
	Falhas     int
	// Recusados são os que a processadora disse ter pago mas que NÃO passaram nas
	// três conferências. Cada um é ou um bug nosso, ou uma tentativa de fazer um
	// pagamento creditar duas vezes. Nunca é rotina.
	Recusados int
	// SemIdentifier é o termômetro do Attach: pedidos recentes que a varredura não
	// alcança porque ninguém entregou o id.
	SemIdentifier int
}

// referenciaNaDescricao tira do texto a referência do pedido.
//
// A ÚLTIMA sequência de 32 hex minúsculos, e nada mais. É a mesma regra que o site
// usa para ler o próprio texto ("<pacote> - <site> | ref:<32 hex>"), e ela é a mesma
// de propósito: duas regras para ler o mesmo texto divergem no dia em que só uma for
// corrigida, e a divergência aqui ou deixa de creditar quem pagou, ou credita quem
// não pagou.
//
// A ÚLTIMA e não a primeira porque o nome do pacote vem antes e não é controlado por
// nós: um pacote que um dia se chame algo com 32 hex no meio roubaria a leitura.
var trintaEDoisHex = regexp.MustCompile(`[0-9a-f]{32}`)

func referenciaNaDescricao(texto string) string {
	todas := trintaEDoisHex.FindAllString(texto, -1)
	if len(todas) == 0 {
		return ""
	}
	return todas[len(todas)-1]
}

// Conferir pergunta à processadora sobre cada pedido pendente e credita o que
// passar nas três conferências.
func (v *Varredura) Conferir(ctx context.Context, janela time.Duration) Rodada {
	var r Rodada
	if n, err := v.banco.TopupsSemIdentifier(ctx, janela); err != nil {
		v.log.WarnContext(ctx, "doacao: nao consegui contar os pedidos sem identifier", "erro", err)
	} else {
		r.SemIdentifier = n
	}

	lista, err := v.banco.TopupsParaConferir(ctx, janela, LimiteDaRodada)
	if err != nil {
		v.log.ErrorContext(ctx, "doacao: nao consegui listar os pedidos a conferir", "erro", err)
		r.Falhas++
		return r
	}
	for _, p := range lista {
		r.Conferidos++
		t, err := v.ponte.ConsultarTransacao(ctx, p.Identifier)
		if err != nil {
			v.log.WarnContext(ctx, "doacao: a consulta falhou; tento na proxima rodada",
				"referencia", p.ExternalReference, "erro", err)
			r.Falhas++
			continue
		}
		if !t.Existe || !strings.EqualFold(strings.TrimSpace(t.Status), statusPago) {
			// Ainda não pagou, ou pagou e voltou atrás. Nada a fazer, e nada a
			// registrar: é o estado da maioria dos pedidos na maior parte do tempo.
			continue
		}
		if !v.confere(ctx, p, t) {
			r.Recusados++
			continue
		}
		if err := v.credit.ConfirmTopupOrder(ctx, p.ExternalReference); err != nil {
			v.log.ErrorContext(ctx, "doacao: a processadora confirmou e o credito falhou",
				"referencia", p.ExternalReference, "erro", err)
			r.Falhas++
			continue
		}
		// INFO e não silêncio: este crédito aconteceu porque o aviso NÃO chegou, e
		// cada linha destas é uma compra que teria sumido.
		v.log.InfoContext(ctx, "doacao: creditei um pagamento que o aviso nao trouxe",
			"referencia", p.ExternalReference, "centavos", t.ValorCentavos)
		r.Creditados++
	}
	return r
}

// confere é o que separa creditar de não creditar, depois de a processadora ter
// dito "pago".
//
// CADA RECUSA AQUI SAI COMO ERRO NO LOG, e não como aviso, porque nenhuma delas é
// rotina: ou é bug nosso, ou é alguém tentando fazer um pagamento creditar dois
// pedidos. As duas merecem que uma pessoa olhe.
func (v *Varredura) confere(ctx context.Context, p store.TopupParaConferir, t Transacao) bool {
	if t.ValorCentavos != p.CentavosPedidos {
		v.log.ErrorContext(ctx, "doacao: o valor pago nao e o do pedido; NAO creditei",
			"referencia", p.ExternalReference,
			"pedido_centavos", p.CentavosPedidos, "pago_centavos", t.ValorCentavos)
		return false
	}
	// A DESCRIÇÃO VAZIA NÃO CONFIRMA NADA, e esta é a linha que faz a varredura
	// esperar em vez de arriscar.
	//
	// A ponte em produção ainda não manda este campo: até o próximo git pull da VPS,
	// TODA resposta chega sem ele. Ausente quer dizer "não sei", nunca "não é
	// doação" — então a varredura não credita ninguém até lá, e o aviso continua
	// sendo o caminho, que é exatamente como está hoje.
	//
	// Tratar o vazio como "pode" seria desligar a conferência que protege contra um
	// pagamento creditar dois pedidos, e desligá-la justamente enquanto ninguém
	// consegue percebê-la funcionando.
	if strings.TrimSpace(t.Descricao) == "" {
		v.log.WarnContext(ctx, "doacao: a processadora nao trouxe a descricao; NAO creditei. "+
			"Se isto se repetir depois de a ponte da VPS atualizar, e problema",
			"referencia", p.ExternalReference)
		return false
	}
	if ref := referenciaNaDescricao(t.Descricao); ref != p.ExternalReference {
		// A RECUSA MAIS IMPORTANTE DAS TRÊS. Ela quer dizer que o id anexado a este
		// pedido pertence a OUTRO pagamento — e creditar aqui seria dar Rcoin por um
		// dinheiro que já foi dado a outra pessoa.
		v.log.ErrorContext(ctx, "doacao: a descricao da transacao e de OUTRO pedido; NAO creditei",
			"referencia", p.ExternalReference, "referencia_na_transacao", ref)
		return false
	}
	return true
}

// Registrar põe a rodada no log, e só quando houve o que dizer.
func (v *Varredura) Registrar(ctx context.Context, r Rodada) {
	if r.Conferidos == 0 && r.Falhas == 0 && r.SemIdentifier == 0 {
		return
	}
	nivel := slog.LevelInfo
	if r.Falhas > 0 || r.Recusados > 0 || r.SemIdentifier > 0 {
		nivel = slog.LevelWarn
	}
	v.log.Log(ctx, nivel, "doacao: rodada de conferencia",
		"conferidos", r.Conferidos, "creditados", r.Creditados,
		"recusados", r.Recusados, "falhas", r.Falhas,
		"sem_identifier", r.SemIdentifier)
}
