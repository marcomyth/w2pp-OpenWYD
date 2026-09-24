// Package rmtvarredura é o que acontece com uma cobrança quando ninguém está
// olhando para ela.
//
// DUAS COISAS, e as duas existem porque o aviso da processadora não é confiável
// como único gatilho:
//
//  1. NADA VENCE SEM PERGUNTAR. Antes de fechar uma cobrança por prazo, o servidor
//     consulta a processadora. Um Pix pago no último segundo com o aviso atrasado
//     virava, até aqui, um pagamento sem item: a pessoa pagava, não recebia, e o
//     item já tinha voltado ao vendedor.
//
//  2. O QUE ENTROU FORA DO PRAZO VOLTA. A devolução era marcada como devida no
//     banco e nunca era pedida a ninguém — a coluna dizia "pendente" para sempre.
//
// AS DUAS MORAM AQUI E NÃO NO DBSERVER porque as duas falam com a ponte, e as
// credenciais dela são do webserver.
package rmtvarredura

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

// Conferidor é o caminho que já decide o que fazer com um pagamento.
//
// A VARREDURA NÃO TEM REGRA PRÓPRIA, e isso é o ponto: ela chama o MESMO
// ConferirEConcluir que o aviso do site chama. Duas versões da regra que decide se
// um item sai do baú de alguém divergiriam no dia em que só uma fosse corrigida.
type Conferidor interface {
	ConferirEConcluir(ctx context.Context, identifier string) (Resultado, error)
}

// Resultado é o pouco que a varredura precisa saber da conferência.
//
// Tipo próprio, e não o do rmtpagamento, para este pacote não depender daquele: o
// que a varredura faz com a resposta é uma decisão só — vencer ou não vencer.
type Resultado struct {
	// Confirmada diz que a venda foi concluída nesta chamada ou antes. Uma cobrança
	// confirmada não é vencida: ela já fechou pelo caminho certo.
	Confirmada bool
}

// Devolvedor é o pedido de devolução na ponte.
type Devolvedor interface {
	PedirReembolso(ctx context.Context, identifier, referencia, detalhes string) (ponte.RespostaReembolso, error)
}

// Banco é o que as duas varreduras leem e escrevem.
type Banco interface {
	CobrancasParaConferir(ctx context.Context, limite int) ([]store.CobrancaParaConferir, error)
	VencerCobrancaConferida(ctx context.Context, cobrancaID int64) (int64, error)
	CobrancasVencidasSemConferir(ctx context.Context) (int, error)
	ReembolsosParaPedir(ctx context.Context, limite int) ([]store.ReembolsoParaPedir, error)
	ReembolsosSemIdentifier(ctx context.Context) (int, error)
	MarcarReembolsoPedido(ctx context.Context, cobrancaID int64) error
	MarcarReembolsoRecusado(ctx context.Context, cobrancaID int64, codigo string) error
	MarcarReembolsoIncerto(ctx context.Context, cobrancaID int64, motivo string) error
}

// Varredura é o laço das cobranças abertas e o das devoluções devidas.
type Varredura struct {
	conf  Conferidor
	dev   Devolvedor
	banco Banco
	log   *slog.Logger
}

// Nova monta a varredura. dev pode ser nulo: sem ponte configurada, as devoluções
// continuam sendo marcadas como devidas e não são pedidas — que é o que já
// acontecia, e aí o número aparece no log em vez de sumir.
func Nova(conf Conferidor, dev Devolvedor, banco Banco, log *slog.Logger) *Varredura {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Varredura{conf: conf, dev: dev, banco: banco, log: log}
}

// LimiteDaRodada é quantas cobranças uma passada confere.
//
// Cem é folgado para o tamanho de hoje — há no máximo uma cobrança aberta por
// anúncio —, e existe como teto e não como expectativa: é uma consulta à ponte por
// cobrança, e uma fila inesperadamente grande não pode virar uma rajada sem fim
// contra a processadora. O que sobra fica para a rodada seguinte, e a ordem por
// prazo garante que o que sobra é o que tem mais tempo.
const LimiteDaRodada = 100

// RodadaDeConferencia é o que uma passada fez, para caber numa linha de log.
type RodadaDeConferencia struct {
	Conferidas  int
	Confirmadas int
	Vencidas    int
	Falhas      int
	// VencidasSemConferir é a fila que a passada NÃO deu conta: cobranças que já
	// passaram do prazo e continuam abertas. Zero é o normal.
	VencidasSemConferir int
}

// Conferir pergunta à processadora sobre cada cobrança aberta e vence as que
// passaram do prazo sem pagamento.
//
// A ORDEM É A REGRA INTEIRA: pergunta PRIMEIRO, vence DEPOIS, e só vence quem a
// resposta não confirmou. Invertida, ela é o bug que este pacote existe para tapar.
//
// E confere TODAS as abertas, não só as que estão vencendo. Essa parte é o polling:
// o aviso da processadora pode não chegar, e sem ele o comprador que pagou no
// primeiro minuto esperaria os cinco até alguém reparar. Perguntando a cada rodada,
// a entrega acontece na rodada seguinte ao pagamento.
//
// UMA FALHA NÃO PARA A RODADA. Cada cobrança é independente das outras, e a
// processadora fora do ar não pode deixar de conferir as que voltariam.
func (v *Varredura) Conferir(ctx context.Context) RodadaDeConferencia {
	var r RodadaDeConferencia
	lista, err := v.banco.CobrancasParaConferir(ctx, LimiteDaRodada)
	if err != nil {
		v.log.ErrorContext(ctx, "rmt: nao consegui listar as cobrancas a conferir", "erro", err)
		r.Falhas++
		return r
	}
	agora := time.Now()
	for _, c := range lista {
		r.Conferidas++
		res, err := v.conf.ConferirEConcluir(ctx, c.Identifier)
		if err != nil {
			// NÃO VENCE quando a consulta falhou. É o coração da regra: sem resposta
			// não se sabe se houve pagamento, e vencer na dúvida é a aposta que custa
			// o item de alguém que pagou. A próxima rodada tenta de novo.
			v.log.WarnContext(ctx, "rmt: a consulta falhou; a cobranca nao vai vencer agora",
				"cobranca", c.ID, "erro", err)
			r.Falhas++
			continue
		}
		if res.Confirmada {
			// Fechou pelo caminho certo: a venda aconteceu, ou o pagamento foi para a
			// fila de reembolso ou de divergência. Vencer por cima disso desmontaria
			// o que a confirmação acabou de decidir.
			r.Confirmadas++
			continue
		}
		if c.ExpiraEm.After(agora) {
			// Ainda tem prazo. A consulta desta rodada era o polling, e ele respondeu
			// "ninguém pagou ainda".
			continue
		}
		anuncio, err := v.banco.VencerCobrancaConferida(ctx, c.ID)
		if err != nil {
			v.log.ErrorContext(ctx, "rmt: nao consegui vencer a cobranca conferida",
				"cobranca", c.ID, "erro", err)
			r.Falhas++
			continue
		}
		if anuncio != 0 {
			r.Vencidas++
		}
	}
	if n, err := v.banco.CobrancasVencidasSemConferir(ctx); err != nil {
		v.log.WarnContext(ctx, "rmt: nao consegui contar as vencidas sem conferir", "erro", err)
	} else {
		r.VencidasSemConferir = n
	}
	return r
}

// RodadaDeReembolso é o que uma passada das devoluções fez.
type RodadaDeReembolso struct {
	Pedidos   int
	Recusados int
	Incertos  int
	Falhas    int
	// SemIdentifier são as devoluções que não dá para pedir, porque falta o id da
	// processadora. Elas não entram na fila e por isso precisam de um número
	// próprio: uma fila que exclui linhas caladamente faz o log dizer "nada a
	// fazer" enquanto o dinheiro de alguém está parado.
	SemIdentifier int
}

// Devolver pede à processadora as devoluções que estão marcadas como devidas.
//
// SÓ AS PENDENTES, e o resto do estado nunca é pedido de novo: recusado espera uma
// pessoa, pedido está em análise, e incerto pode já ter sido criado. Um segundo
// pedido sobre o mesmo dinheiro devolveria o dobro ao comprador — e o dobro sai da
// conta da Hanna, não da processadora.
func (v *Varredura) Devolver(ctx context.Context) RodadaDeReembolso {
	var r RodadaDeReembolso
	if n, err := v.banco.ReembolsosSemIdentifier(ctx); err != nil {
		v.log.WarnContext(ctx, "rmt: nao consegui contar os reembolsos sem identifier", "erro", err)
	} else {
		r.SemIdentifier = n
	}
	if v.dev == nil {
		return r
	}
	lista, err := v.banco.ReembolsosParaPedir(ctx, LimiteDaRodada)
	if err != nil {
		v.log.ErrorContext(ctx, "rmt: nao consegui listar os reembolsos a pedir", "erro", err)
		r.Falhas++
		return r
	}
	for _, d := range lista {
		v.pedirUm(ctx, d, &r)
	}
	return r
}

// detalhes é o texto que acompanha o pedido na processadora.
//
// Sem nome de pessoa e sem nome de item: quem lê isso do outro lado é o suporte
// deles, e a única coisa que precisa ser reconhecível é qual pagamento é.
func detalhes(d store.ReembolsoParaPedir) string {
	return fmt.Sprintf("devolucao de pagamento fora do prazo; cobranca %d", d.CobrancaID)
}

func (v *Varredura) pedirUm(ctx context.Context, d store.ReembolsoParaPedir, r *RodadaDeReembolso) {
	resp, err := v.dev.PedirReembolso(ctx, d.Identifier, d.Referencia, detalhes(d))
	if err != nil {
		// A CHAMADA FALHOU SEM RESPOSTA, e aqui a distinção decide se alguém recebe
		// duas vezes. O erro da ponte cobre tanto "não saiu" quanto "saiu e a
		// resposta se perdeu", e não há consulta de reembolso para desempatar.
		//
		// Então o estado vira INCERTO e a linha para de ser pedida. Perde-se uma
		// devolução que talvez precisasse ser feita à mão; o outro lado da moeda
		// seria pedir de novo a cada minuto e devolver o dobro.
		v.log.ErrorContext(ctx, "rmt: o pedido de reembolso nao voltou; marquei incerto",
			"cobranca", d.CobrancaID, "erro", err)
		v.marca(ctx, d.CobrancaID, store.ReembolsoIncerto,
			v.banco.MarcarReembolsoIncerto(ctx, d.CobrancaID, "a chamada nao voltou"))
		r.Incertos++
		return
	}
	switch strings.ToLower(strings.TrimSpace(resp.Estado)) {
	case "pedido", "repetido":
		// REPETIDO conta como pedido, e não como erro: significa que a ponte já tinha
		// visto esta referência e NÃO chamou a processadora de novo. É a idempotência
		// funcionando, e é exatamente o que se quer que aconteça quando a rodada
		// anterior morreu no meio.
		v.marca(ctx, d.CobrancaID, store.ReembolsoPedido, v.banco.MarcarReembolsoPedido(ctx, d.CobrancaID))
		r.Pedidos++
	case "recusado":
		// Recusado é CERTEZA de que nada foi criado. Vai para a fila de gente com o
		// código deles intacto: a diferença entre "reembolso não habilitado para esta
		// conta" e "conta do seller não aprovada" decide o que a Hanna pede ao
		// suporte, e as duas viram "falhou" se a gente resumir.
		// O código deles, e o HTTP quando não houver código: a linha da staff tem de
		// dizer alguma coisa, e "recusado" sozinho não diz nada a ninguém.
		codigo := resp.CodigoSyncpay
		if codigo == "" {
			codigo = fmt.Sprintf("http %d", resp.HTTPSyncpay)
		}
		v.log.WarnContext(ctx, "rmt: a processadora recusou o reembolso",
			"cobranca", d.CobrancaID, "codigo", codigo, "http", resp.HTTPSyncpay)
		v.marca(ctx, d.CobrancaID, store.ReembolsoRecusado,
			v.banco.MarcarReembolsoRecusado(ctx, d.CobrancaID, codigo))
		r.Recusados++
	case "incerto":
		v.log.ErrorContext(ctx, "rmt: reembolso incerto; NAO sera pedido de novo",
			"cobranca", d.CobrancaID, "codigo", resp.CodigoSyncpay)
		v.marca(ctx, d.CobrancaID, store.ReembolsoIncerto,
			v.banco.MarcarReembolsoIncerto(ctx, d.CobrancaID, resp.CodigoSyncpay))
		r.Incertos++
	default:
		// ESTADO QUE ESTE CÓDIGO NÃO CONHECE VIRA INCERTO, e não "recusado".
		//
		// A diferença é quem paga o engano: tratado como recusa, a linha voltaria à
		// fila e seria pedida de novo — e se o pedido tinha sido criado, o comprador
		// recebe duas vezes. Como incerto, ela para e espera uma pessoa.
		v.log.ErrorContext(ctx, "rmt: estado de reembolso que eu nao conheco; marquei incerto",
			"cobranca", d.CobrancaID, "estado", resp.Estado)
		v.marca(ctx, d.CobrancaID, store.ReembolsoIncerto,
			v.banco.MarcarReembolsoIncerto(ctx, d.CobrancaID, "estado desconhecido: "+resp.Estado))
		r.Incertos++
	}
}

// marca registra a falha de gravar a transição sem derrubar a rodada.
//
// A transição pode ser recusada de propósito — a máquina de estados do reembolso
// recusa o que andaria para trás —, e isso é AVISO e não erro: acontece com resposta
// atrasada, que é o normal de quem fala com uma processadora pela rede.
func (v *Varredura) marca(ctx context.Context, cobrancaID int64, destino store.EstadoReembolso, err error) {
	if err == nil {
		return
	}
	v.log.WarnContext(ctx, "rmt: nao consegui gravar o estado do reembolso",
		"cobranca", cobrancaID, "destino", int(destino), "erro", err)
}

// Registrar põe as duas rodadas no log, e SÓ quando houve o que dizer.
//
// Uma linha por minuto dizendo "zero, zero, zero" treina quem lê a pular o bloco — e
// aí o dia em que ele traz um número some junto. O que nunca é silenciado: as
// falhas, os incertos, e as duas filas que precisam de gente.
func (v *Varredura) Registrar(ctx context.Context, c RodadaDeConferencia, d RodadaDeReembolso) {
	if c.Conferidas > 0 || c.Falhas > 0 || c.VencidasSemConferir > 0 {
		nivel := slog.LevelInfo
		if c.Falhas > 0 || c.VencidasSemConferir > 0 {
			nivel = slog.LevelWarn
		}
		v.log.Log(ctx, nivel, "rmt: rodada de conferencia",
			"conferidas", c.Conferidas, "confirmadas", c.Confirmadas,
			"vencidas", c.Vencidas, "falhas", c.Falhas,
			"vencidas_sem_conferir", c.VencidasSemConferir)
	}
	if d.Pedidos > 0 || d.Recusados > 0 || d.Incertos > 0 || d.Falhas > 0 || d.SemIdentifier > 0 {
		nivel := slog.LevelInfo
		if d.Recusados > 0 || d.Incertos > 0 || d.Falhas > 0 || d.SemIdentifier > 0 {
			nivel = slog.LevelWarn
		}
		v.log.Log(ctx, nivel, "rmt: rodada de reembolso",
			"pedidos", d.Pedidos, "recusados", d.Recusados, "incertos", d.Incertos,
			"falhas", d.Falhas, "sem_identifier", d.SemIdentifier)
	}
}
