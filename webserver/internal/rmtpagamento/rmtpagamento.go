// Package rmtpagamento recebe o aviso de pagamento e conclui a venda em dinheiro
// real.
//
// O CAMINHO, em uma frase: chega um aviso, o servidor NÃO ACREDITA NELE, vai
// perguntar à processadora o que de fato aconteceu, e só então decide.
//
// Essa desconfiança é o desenho inteiro. Um aviso é uma mensagem que chega pela
// rede dizendo "alguém te pagou"; acreditar nela é deixar que quem souber forjá-la
// tire itens de jogadores. A autenticação do transporte ajuda e não basta, porque o
// segredo pode vazar e porque o aviso pode chegar repetido ou fora de ordem.
//
// Perguntando à processadora, o pior que um aviso forjado consegue é gastar UMA
// CONSULTA.
package rmtpagamento

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Transacao é o que a consulta à processadora devolve, já do tamanho que este
// pacote usa.
//
// TIPO PRÓPRIO e não o da ponte, por um motivo prático que só apareceu ao escrever
// o teste: o campo de hora da resposta da ponte tem um tipo NÃO EXPORTADO, então um
// teste fora daquele pacote não consegue montar uma resposta com a hora preenchida.
// Um contrato que não se consegue falsificar não se consegue testar.
type Transacao struct {
	// Existe é falso quando a processadora não conhece este identifier.
	Existe bool
	// Status vai CRU, como a processadora devolveu. Quem normaliza é este pacote, e
	// a lista está logo abaixo.
	Status string
	// Referencia é a referência que a PROCESSADORA diz que este pagamento tem.
	// Nossa de origem — nós a geramos —, mas voltando pela boca dela, e por isso
	// conferida contra a gravada antes de valer.
	Referencia string
	// ValorCentavos é o que ela diz ter recebido.
	ValorCentavos int64
	// PagoEm é o paid_at DELES, e é NULO quando a resposta veio da fonte que não
	// tem esse campo. Ponteiro e não zero porque time.Time{} diria "ano 1", que
	// compararia como muito antigo e faria todo pagamento parecer dentro do prazo.
	PagoEm *time.Time
}

// O VOCABULÁRIO DE STATUS DA PROCESSADORA É DOCUMENTAÇÃO, NÃO MEDIÇÃO, e isso muda
// o que se pode concluir dele. Nenhuma cobrança real passou por aqui até hoje: os
// valores abaixo foram lidos nas páginas da SyncPay pelo par que escreveu a ponte,
// e não vistos numa resposta de verdade. O primeiro pagamento real é que confirma.
//
// É por isso que o padrão é NÃO ENTREGAR: um status que não está nesta lista não
// vira entrega, vira aviso com o valor cru no log — que é também como a gente
// aprende o vocabulário verdadeiro sem ninguém perder item.
const (
	// statusPago é o único que entrega, nas duas fontes.
	statusPago = "completed"
	// Os que significam que o dinheiro está indo embora. Eles NÃO entregam, e a
	// razão é o que este bloco existe para registrar: uma venda estornada ou em
	// contestação CONTINUA COM paid_at PREENCHIDO, porque ela foi paga um dia.
	// Decidir só por "tem paid_at" entregaria item em cima de dinheiro que já
	// voltou para o comprador.
	statusEstornado  = "refunded"
	statusEstornando = "refunding"
	statusContestado = "med" // contestação do Pix
	statusPendente   = "pending"
	statusFalhou     = "failed"
	statusRecusado   = "refused"
)

// Consultor é o pedaço da ponte que este pacote usa.
type Consultor interface {
	// ConsultarTransacao pergunta à processadora o que aconteceu de verdade, pelo
	// identifier DELES — nunca pelo nosso id.
	ConsultarTransacao(ctx context.Context, identifier string) (Transacao, error)
}

// Banco é o que este caminho precisa do banco.
type Banco interface {
	CobrancaDoIdentifier(ctx context.Context, identifier string) (referencia string, achou bool, err error)
	ConfirmarCobrancaRMT(ctx context.Context, referenciaExterna string, pagoEm time.Time,
		origem store.OrigemDaHora, valorObservado int64) (store.ResultadoCobranca, store.VendaRMT, error)
	MarcarReembolsoPendente(ctx context.Context, cobrancaID int64) error
	RegistrarPagamentoOrfao(ctx context.Context, p store.PagamentoOrfao) error
	NomeDaConta(ctx context.Context, accountID int64) (string, error)
}

// Jogo encurta a espera de quem está jogando.
//
// AS DUAS CHAMADAS SÃO CORTESIA e não parte da venda: quando elas rodam, a venda já
// está gravada, o item do comprador está na caixa postal e o slot do vendedor está
// marcado. Se falharem, o login de cada um faz o mesmo trabalho. É por isso que a
// falha delas não desfaz nada e não vira erro para quem avisou.
//
// Pode ser nulo, e é o que acontece quando o servidor de jogo não está ligado a
// este processo. Aí a venda continua acontecendo: ela só demora mais a aparecer.
type Jogo interface {
	Entrega(ctx context.Context, conta string) (bool, error)
	LiberaVenda(ctx context.Context, conta string) (bool, error)
}

// Servico é o caminho do aviso de pagamento.
type Servico struct {
	ponte Consultor
	banco Banco
	jogo  Jogo
	log   *slog.Logger
}

// Novo monta o serviço. jogo pode ser nulo.
func Novo(ponte Consultor, banco Banco, jogo Jogo, log *slog.Logger) *Servico {
	return &Servico{ponte: ponte, banco: banco, jogo: jogo, log: log}
}

// Resultado é o que o aviso produziu.
type Resultado struct {
	// Tratado é falso quando o evento não era nosso. NÃO é erro: é a resposta
	// ordinária ao tráfego da loja de doação, que chega pelo mesmo webhook porque
	// ele é por CONTA e não por cobrança. Tratar como falha faria o site repetir
	// para sempre o aviso de um pagamento de outra pessoa.
	Tratado bool
	// Confirmada diz que a venda foi concluída no banco nesta chamada ou antes.
	Confirmada bool
	// Entregue é falso no pagamento fora do prazo e no valor divergente.
	Entregue   bool
	CobrancaID int64
}

// ConferirEConcluir pergunta à processadora o que houve com um pagamento e conclui
// a venda, se for o caso.
//
// UM LUGAR SÓ, E É DE PROPÓSITO QUE O NOME NÃO FALA DE AVISO. Dois caminhos
// diferentes chegam aqui e têm de decidir igual: o aviso que o site repassa, e a
// varredura das cobranças abertas. Se cada um tivesse a sua versão da regra, as duas
// divergiriam no dia em que só uma fosse corrigida — e a regra em questão é a que
// decide se um item sai do baú de alguém.
//
// A ORDEM É DELIBERADA, e cada passo só existe porque o anterior não basta:
//
//  1. CONSULTA A PROCESSADORA. É ela quem diz se houve pagamento e o que ele é. O
//     aviso é um toque de campainha, e o único campo dele que se usa é o
//     identifier — usado para PERGUNTAR, nunca para concluir.
//  2. DECIDE SE É PAGAMENTO PARA ENTREGAR. Status "completed", e mais nada. Um
//     estorno ou uma contestação também tem paid_at e não entrega nada.
//  3. DESCOBRE DE QUE COBRANÇA É, conferindo o que está gravado contra o que a
//     processadora disse. Divergência não entrega: vai para a fila da staff.
//  4. CONFIRMA NO BANCO. É aí que a venda acontece, numa transação, idempotente, e
//     é o banco que compara o valor.
//  5. ENTREGA AGORA, se der. Cortesia: falhar aqui não desfaz nada.
func (s *Servico) ConferirEConcluir(ctx context.Context, identifier string) (Resultado, error) {
	if strings.TrimSpace(identifier) == "" {
		// Aviso sem identifier não dá nem para perguntar. Não é nosso e não há o
		// que registrar: sem o id deles, ninguém acha o pagamento lá.
		s.log.Warn("rmt: pedido de conferencia sem identifier")
		return Resultado{}, nil
	}

	t, err := s.ponte.ConsultarTransacao(ctx, identifier)
	if err != nil {
		// Erro de verdade: a consulta é a única fonte, e sem ela não se decide
		// nada. Quem chamou repete, e repetir é seguro — a confirmação é
		// idempotente pela referência.
		return Resultado{}, err
	}
	if !t.Existe {
		// A processadora não conhece este identifier. NÃO é pagamento órfão: não
		// existe pagamento nenhum para pôr em fila. O caso comum é aviso de outro
		// sistema; o outro caso é aviso forjado, e ele custou uma consulta.
		s.log.Info("rmt: aviso para um identifier que a processadora nao conhece",
			"identifier", identifier)
		return Resultado{}, nil
	}

	status := strings.ToLower(strings.TrimSpace(t.Status))
	if status != statusPago {
		s.naoEhParaEntregar(ctx, identifier, status, t)
		return Resultado{}, nil
	}

	referencia, ok := s.deQualCobranca(ctx, identifier, t)
	if !ok {
		return Resultado{}, nil
	}

	// A hora do pagamento decide entre entregar e devolver, então de que relógio
	// ela veio fica gravado. Sem o paid_at da processadora, vale o instante em que
	// a consulta VIU o pagamento — que é mais tarde do que o real, e portanto erra
	// para o lado de "fora do prazo", que é o lado que devolve dinheiro em vez de
	// dar item de graça.
	pagoEm, origem := time.Now().UTC(), store.HoraDoServidor
	if t.PagoEm != nil {
		pagoEm, origem = *t.PagoEm, store.HoraDaProcessadora
	}

	res, venda, err := s.banco.ConfirmarCobrancaRMT(ctx, referencia, pagoEm, origem, t.ValorCentavos)
	if err != nil {
		return Resultado{}, err
	}

	switch res {
	case store.CobrancaNaoEncontrada:
		// A referência não é de nenhuma cobrança nossa, e o dinheiro entrou. Isto
		// vai para a fila de gente: é o caso em que um aviso mal tratado faz
		// alguém pagar e não haver linha que responda por aquele pagamento.
		s.orfao(ctx, identifier, t, store.MotivoOrfaoSemCobranca)
		return Resultado{Tratado: true}, nil

	case store.CobrancaPagaSemItem:
		// Pago fora do prazo, ou sem item para entregar. O dinheiro volta.
		if err := s.banco.MarcarReembolsoPendente(ctx, venda.CobrancaID); err != nil {
			// A venda já está gravada como paga-sem-item; o que falhou foi marcar
			// que devemos o reembolso. Erro de verdade, porque sem essa marca o
			// dinheiro de alguém fica sem destino e ninguém sabe.
			return Resultado{}, err
		}
		s.log.Warn("rmt: pagamento sem item, reembolso pendente",
			"identifier", identifier, "cobranca", venda.CobrancaID,
			"comprador", venda.CompradorConta, "atrasado", venda.PagoComAtraso)
		return Resultado{Tratado: true, Confirmada: true, CobrancaID: venda.CobrancaID}, nil

	case store.CobrancaValorDivergente:
		// Entrou valor diferente do cobrado. A cobrança FICA ABERTA e o banco já
		// gravou a divergência; a fila é a ValoresDivergentes, e não a dos órfãos.
		// Ninguém decide aqui: devolver, cobrar a diferença ou entregar assim mesmo
		// é escolha sobre o dinheiro de duas pessoas.
		s.log.Warn("rmt: valor divergente, esperando uma pessoa",
			"identifier", identifier, "cobranca", venda.CobrancaID,
			"recebido", t.ValorCentavos)
		return Resultado{Tratado: true, Confirmada: true, CobrancaID: venda.CobrancaID}, nil

	case store.CobrancaJaConfirmada:
		// Aviso repetido, que a processadora manda de propósito. Nada a fazer, e a
		// pressa também não: se a entrega imediata falhou na primeira vez, o login
		// já resolveu ou vai resolver.
		s.log.Info("rmt: aviso repetido de uma cobranca ja confirmada",
			"identifier", identifier, "cobranca", venda.CobrancaID)
		return Resultado{Tratado: true, Confirmada: true, Entregue: true,
			CobrancaID: venda.CobrancaID}, nil

	case store.CobrancaConfirmada:
		// A venda aconteceu agora e está gravada. O que vem é pressa, e pressa que
		// falha não desfaz venda.
		s.entregaAgora(ctx, venda)
		return Resultado{Tratado: true, Confirmada: true, Entregue: true,
			CobrancaID: venda.CobrancaID}, nil

	default:
		// CASO EXPLÍCITO E NÃO CAMINHO PADRÃO, e a diferença é quem paga o erro. Se
		// a entrega fosse o que sobra depois do switch, um resultado NOVO que alguém
		// acrescentasse ao banco amanhã cairia direto em "entrega o item" sem
		// ninguém escrever essa decisão. Um estado que este arquivo não conhece não
		// entrega nada.
		s.log.Error("rmt: resultado de confirmacao que eu nao conheco; nao entreguei nada",
			"identifier", identifier, "resultado", int(res), "cobranca", venda.CobrancaID)
		return Resultado{Tratado: true, Confirmada: true, CobrancaID: venda.CobrancaID}, nil
	}
}

// naoEhParaEntregar registra um status que não entrega, com o barulho que cada um
// merece.
func (s *Servico) naoEhParaEntregar(ctx context.Context, identifier, status string, t Transacao) {
	switch status {
	case statusContestado, statusEstornado, statusEstornando:
		// Dinheiro saindo em cima de uma venda nossa é o pior caso do mercado: se a
		// venda foi entregue, o item já está com o comprador e o dinheiro volta
		// para ele. Uma pessoa tem de ver isto, então vai para a fila e não para o
		// log.
		if _, achou, err := s.banco.CobrancaDoIdentifier(ctx, identifier); err == nil && achou {
			s.orfao(ctx, identifier, t, store.MotivoOrfaoContestado)
			s.log.Warn("rmt: venda do mercado contestada ou estornada",
				"identifier", identifier, "status", status)
			return
		}
		// Não é de cobrança nossa: é estorno na loja de doação ou de outra coisa.
		s.log.Info("rmt: estorno ou contestacao que nao e de cobranca nossa",
			"identifier", identifier, "status", status)

	case statusPendente, statusFalhou, statusRecusado:
		// O aviso chegou antes do dinheiro, ou o pagamento não deu certo. A
		// processadora avisa de novo, e a varredura das abertas também olha.
		s.log.Info("rmt: aviso sem pagamento concluido",
			"identifier", identifier, "status", status)

	default:
		// O QUE A GENTE NÃO CONHECE NÃO ENTREGA, e sai com o valor cru justamente
		// para deixar de ser desconhecido. O vocabulário todo é documentação e
		// nunca foi medido: é por aqui que o de verdade aparece.
		s.log.Warn("rmt: status que eu nao conheco; nao entreguei nada",
			"identifier", identifier, "status_cru", t.Status)
	}
}

// deQualCobranca decide por qual referência confirmar, e é onde a desconfiança
// termina.
//
// DUAS FONTES PARA A MESMA COISA, de propósito: a referência gravada por nós contra
// a que a processadora devolveu. Elas quase sempre concordam; o valor do par está no
// dia em que não concordarem, porque aí confirmar por qualquer uma das duas
// entregaria item ao comprador errado e marcaria vendido o item de outro vendedor.
//
// A ordem de preferência não é arbitrária:
//
//   - As duas iguais: o caso normal.
//   - Só a GRAVADA: a processadora não mandou referência. A gravada é nossa e vale.
//   - Só a DA PROCESSADORA: é a corrida no nascimento. O webhook chegou antes de a
//     gente ter gravado o identifier. A referência dela é nossa de origem, e se não
//     for de nenhuma cobrança o passo seguinte devolve "não encontrada".
//   - Diferentes: NÃO CONFIRMA NADA. Fila da staff.
//   - Nenhuma das duas: fila da staff.
func (s *Servico) deQualCobranca(ctx context.Context, identifier string, t Transacao) (string, bool) {
	gravada, achou, err := s.banco.CobrancaDoIdentifier(ctx, identifier)
	if err != nil {
		// Não dá para conferir, então não confirma. Quem chamou repete.
		s.log.Warn("rmt: nao consegui ler a cobranca do identifier",
			"identifier", identifier, "err", err)
		return "", false
	}
	daProcessadora := strings.TrimSpace(t.Referencia)

	switch {
	case achou && daProcessadora != "" && gravada != daProcessadora:
		s.orfao(ctx, identifier, t, store.MotivoOrfaoReferenciaDivergente)
		s.log.Warn("rmt: a referencia da processadora nao bate com a gravada",
			"identifier", identifier, "gravada", gravada, "da_processadora", daProcessadora)
		return "", false
	case achou:
		return gravada, true
	case daProcessadora != "":
		s.log.Info("rmt: aviso chegou antes de o identifier estar gravado",
			"identifier", identifier, "referencia", daProcessadora)
		return daProcessadora, true
	default:
		s.orfao(ctx, identifier, t, store.MotivoOrfaoSemCobranca)
		return "", false
	}
}

// orfao põe o pagamento na fila de gente, e ENGOLE a própria falha de propósito.
//
// Engole porque quem chama já decidiu não entregar nada: se a fila falhar, insistir
// não melhora a situação de ninguém, e transformar isso em erro faria o site repetir
// o aviso em laço por um pagamento que continua sem cobrança. O que sobra é o log,
// que aqui é a segunda linha de defesa e não a primeira.
func (s *Servico) orfao(ctx context.Context, identifier string, t Transacao, motivo string) {
	valor := t.ValorCentavos
	p := store.PagamentoOrfao{
		Identifier:      identifier,
		ReferenciaVista: t.Referencia,
		Motivo:          motivo,
		PagoEm:          t.PagoEm,
	}
	if valor > 0 {
		p.ValorCentavos = &valor
	}
	if err := s.banco.RegistrarPagamentoOrfao(ctx, p); err != nil {
		s.log.Error("rmt: NAO CONSEGUI registrar o pagamento orfao",
			"identifier", identifier, "motivo", motivo, "valor", valor, "err", err)
		return
	}
	s.log.Warn("rmt: pagamento na fila da staff",
		"identifier", identifier, "motivo", motivo, "valor", valor)
}

// entregaAgora encurta a espera dos dois lados, e engole as próprias falhas.
//
// ENGOLE DE PROPÓSITO, e isto é o oposto do que o resto deste arquivo faz. A venda
// já está no banco: o item do comprador está na caixa postal e o do vendedor está
// marcado como vendido. Se estas chamadas falharem, o login de cada um faz o mesmo
// trabalho. Propagar o erro faria o site repetir o aviso por uma coisa que já deu
// certo — e a repetição é idempotente, mas o alarme seria falso.
func (s *Servico) entregaAgora(ctx context.Context, venda store.VendaRMT) {
	if s.jogo == nil {
		// O servidor de jogo não está ligado a este processo. A venda está feita; o
		// login de cada um entrega. Info e não warn: é configuração, não falha.
		s.log.Info("rmt: sem link com o jogo; a entrega sai no login",
			"cobranca", venda.CobrancaID)
		return
	}

	if comprador, err := s.banco.NomeDaConta(ctx, venda.CompradorConta); err != nil {
		s.log.Warn("rmt: nao achei o nome do comprador para entregar agora",
			"conta", venda.CompradorConta, "err", err)
	} else if emJogo, err := s.jogo.Entrega(ctx, comprador); err != nil {
		s.log.Warn("rmt: entrega imediata falhou; o comprador recebe no login",
			"conta", comprador, "err", err)
	} else if !emJogo {
		s.log.Info("rmt: comprador fora do jogo; recebe no login", "conta", comprador)
	}

	if vendedor, err := s.banco.NomeDaConta(ctx, venda.VendedorConta); err != nil {
		s.log.Warn("rmt: nao achei o nome do vendedor para liberar agora",
			"conta", venda.VendedorConta, "err", err)
	} else if emJogo, err := s.jogo.LiberaVenda(ctx, vendedor); err != nil {
		s.log.Warn("rmt: liberacao imediata falhou; o item sai no login do vendedor",
			"conta", vendedor, "err", err)
	} else if !emJogo {
		s.log.Info("rmt: vendedor fora do jogo; o item sai no login dele", "conta", vendedor)
	}
}

// AvisoDeSaida registra um aviso de dinheiro SAINDO.
//
// ACREDITAR NO AVISO É ACEITÁVEL AQUI E EM NENHUM OUTRO LUGAR DESTE CAMINHO, e vale
// dizer por quê: nada é entregue nem pago por causa dele, o dinheiro já se moveu, e
// não há outra fonte — a consulta responde 404 para transação que não é de venda. O
// que interessa no evento é a diferença entre o que a gente pediu e o que chegou,
// que é a taxa do saque.
//
// NESTE MOMENTO NÃO EXISTE CAMINHO DE PAGAMENTO AO VENDEDOR NO SERVIDOR, então todo
// saque que chegar aqui é da loja de doação ou uma retirada feita à mão no painel da
// processadora — nenhum dos dois é nosso. Por isso ele é registrado no log e nada
// mais, e devolve "tratado" para o site não repetir em laço um evento que ninguém
// vai reclamar. Quando o pagamento ao vendedor existir, é aqui que a taxa passa a
// ser gravada contra o repasse.
func (s *Servico) AvisoDeSaida(ctx context.Context, identifier string, pedidoCentavos, chegouCentavos int64) (Resultado, error) {
	_ = ctx
	s.log.Info("rmt: aviso de saque",
		"identifier", identifier,
		"pedido_centavos", pedidoCentavos,
		"chegou_centavos", chegouCentavos,
		"taxa_centavos", pedidoCentavos-chegouCentavos)
	return Resultado{Tratado: true}, nil
}
