package grpcsrv

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/rmtpagamento"
)

// ChavesPix é a superfície da chave de recebimento (satisfeita por *store.Store).
// Interface e não o store concreto pelo mesmo motivo do resto do pacote: o
// servidor fica testável sem banco.
type ChavesPix interface {
	SalvarChavePix(ctx context.Context, accountID int64, chave string,
		tipo store.TipoChavePix, documento string) error
	LerChavePix(ctx context.Context, accountID int64) (store.RecebedorPix, error)
	// RepasseDoVendedor é quanto esta conta tem a receber e por que ainda não
	// chegou. Sai na MESMA resposta da chave, porque a página da chave é onde a
	// pessoa vai quando está procurando o dinheiro dela.
	RepasseDoVendedor(ctx context.Context, accountID int64) (int64, store.MotivoDaEspera, error)
	// CobrancaAtualDoComprador é a única leitura feita para quem PAGA: a cobrança
	// aberta da conta, ou a que fechou há pouco. Ver store/cobranca_do_comprador.go.
	CobrancaAtualDoComprador(ctx context.Context, compradorConta int64,
		janelaRecente time.Duration) (bool, store.CobrancaDoComprador, error)
	// CriarPixSeFaltar faz nascer o código na processadora na PRIMEIRA leitura de
	// uma cobrança aberta que ainda não tem um. Ver store/pix_da_cobranca.go, que é
	// onde vive a trava contra duas abas criarem duas cobranças.
	CriarPixSeFaltar(ctx context.Context, cobrancaID int64, minimoRestante time.Duration,
		criar store.CriadorDePix) (store.PixDaCobranca, error)
}

// MinimoParaCriarPix é quanto prazo tem de sobrar para valer a pena criar o código.
//
// CRIAR UM PIX COM DEZ SEGUNDOS DE VIDA É FABRICAR REEMBOLSO. A pessoa abre o
// aplicativo do banco, escaneia, confirma — e o dinheiro cai numa cobrança que já
// venceu. Aí o item já foi solto, ela não recebe nada na hora, e a gente devolve
// pagando taxa de reembolso mais as taxas da venda, que não voltam. Os dois lados
// perdem por causa de um código que nunca devia ter nascido.
//
// Sessenta segundos contra uma janela de cinco minutos: quem chega com menos de um
// minuto vê a cobrança como vencida e pode abrir outra, que nasce com o prazo
// inteiro. É melhor do que um QR que quase sempre falha.
var MinimoParaCriarPix = 60 * time.Second

// PrazoDaCriacao é quanto a criação do Pix tem para terminar, contado do PRÓPRIO
// relógio dela e não do de quem pediu a página.
//
// MAIOR QUE O PRAZO DA PONTE (15 s) de propósito: quem tem de desistir da chamada à
// processadora é o cliente da ponte, com a mensagem dele, e não este prazo por cima.
// Um prazo de fora menor transformaria toda lentidão da processadora num erro
// genérico, escondendo qual das duas coisas falhou.
var PrazoDaCriacao = 20 * time.Second

// CriadorDePixComTexto é a chamada à ponte, já com o texto que o pagador lê.
//
// A DESCRIÇÃO NÃO PODE DESCER ATÉ O STORE, e é por isso que este tipo existe em vez
// de o store.CriadorDePix ganhar um parâmetro: o texto tem o NOME do item, e para
// saber o nome de um índice é preciso o catálogo do cliente legado, que é coisa do
// webserver. O store não conhece catálogo e não deveria passar a conhecer para
// montar uma frase.
type CriadorDePixComTexto func(ctx context.Context, referencia string, centavos int64,
	descricao string) (codigoPix, identifier string, err error)

// NomeDeItem traduz um índice no nome que uma pessoa lê, ou devolve vazio quando
// não sabe. Pode ser nulo: aí a descrição sai genérica, o que é feio e não é erro.
type NomeDeItem func(index int32) string

// ServerRmt implementa webv1.RmtWebServiceServer.
//
// SERVIÇO PRÓPRIO, e não dois métodos a mais no AccountWebService: aquele cuida
// de quem entra, este aponta para onde o dinheiro cai. Separados, a superfície de
// dinheiro real pode ser fechada, auditada ou retirada sozinha, e o assunto
// aparece no caminho do método (/web.v1.RmtWebService/SavePixKey) para quem for
// ler um log de acesso.
type ServerRmt struct {
	webv1.UnimplementedRmtWebServiceServer
	pix      ChavesPix
	criarPix CriadorDePixComTexto
	nomeItem NomeDeItem
	log      *slog.Logger
	// vitrine é opcional: sem o link com o servidor de jogo não há mercado para
	// listar, e o método responde Unavailable em vez de mentir uma lista vazia.
	vitrine Vitrine
}

// NewRmt monta o serviço de dinheiro real sobre a superfície da chave.
//
// Sem criador de Pix: a leitura da cobrança devolve o que está gravado e nunca
// chama a processadora. É o comportamento certo quando a ponte não está
// configurada — a cobrança existe, a página mostra que está sendo gerada, e
// ninguém paga um código que não nasceu.
func NewRmt(pix ChavesPix) *ServerRmt {
	return &ServerRmt{pix: pix, log: slog.New(slog.DiscardHandler)}
}

// ComCriadorDePix liga a criação tardia do código.
//
// Construtor separado, e não um parâmetro a mais no NewRmt, por duas razões: a
// criação é opcional de verdade (sem ponte configurada ela não existe), e assim o
// caminho SEM ponte continua sendo o que os testes antigos já cobrem, em vez de
// todos passarem a carregar um nulo.
func (s *ServerRmt) ComCriadorDePix(criar CriadorDePixComTexto, nome NomeDeItem, log *slog.Logger) *ServerRmt {
	s.criarPix = criar
	s.nomeItem = nome
	if log != nil {
		s.log = log
	}
	return s
}

// descricaoDaCobranca monta o texto que a processadora mostra a quem paga.
//
// Sem catálogo ligado, ou com um índice que ele não conhece, sai o texto genérico.
// Perder o nome do item NÃO pode derrubar a venda: a alternativa seria recusar a
// cobrança porque o catálogo não carregou, e ninguém deixa de vender por causa de
// uma frase.
func (s *ServerRmt) descricaoDaCobranca(cob store.CobrancaDoComprador) string {
	var nome string
	if s.nomeItem != nil {
		nome = s.nomeItem(int32(cob.ItemIndex))
	}
	return rmtpagamento.Descricao(nome, cob.Refino)
}

// SavePixKey grava a chave de recebimento da conta.
//
// As recusas previstas — chave malformada, venda em curso, conta inexistente —
// viajam no enum da resposta e NÃO como erro gRPC. Só falha de infraestrutura
// vira erro, que é a mesma divisão que o CreateAccount faz: o formulário precisa
// saber O QUE dizer à pessoa, e um código de erro de transporte não diz.
func (s *ServerRmt) SavePixKey(ctx context.Context, req *webv1.SavePixKeyRequest) (*webv1.SavePixKeyResponse, error) {
	err := s.pix.SalvarChavePix(ctx, req.GetAccountId(), req.GetKey(),
		tipoDoProto(req.GetType()), req.GetTaxId())
	switch {
	case err == nil:
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_OK}, nil
	case errors.Is(err, store.ErrChavePixInvalida):
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_INVALID}, nil
	case errors.Is(err, store.ErrDocumentoInvalido):
		// CÓDIGO PRÓPRIO, e não o INVALID genérico: o formulário tem dois campos, e
		// dizer só "inválido" faria a pessoa corrigir a chave, que estava certa. O
		// contrato já previa este valor; o servidor é que nunca o produzia.
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_INVALID_TAX_ID}, nil
	case errors.Is(err, store.ErrVendaEmCurso):
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_SALE_IN_PROGRESS}, nil
	case errors.Is(err, store.ErrNotFound):
		return &webv1.SavePixKeyResponse{Result: webv1.PixKeyResult_PIX_KEY_RESULT_NO_ACCOUNT}, nil
	default:
		return nil, status.Errorf(codes.Internal, "save pix key: %v", err)
	}
}

// GetPixKey devolve o estado da chave, sempre MASCARADA — a inteira não sai do
// servidor. O mascaramento acontece na camada de banco, e não aqui, para não
// existir um caminho que devolva a chave crua se alguém escrever outro handler
// amanhã.
func (s *ServerRmt) GetPixKey(ctx context.Context, req *webv1.GetPixKeyRequest) (*webv1.GetPixKeyResponse, error) {
	r, err := s.pix.LerChavePix(ctx, req.GetAccountId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get pix key: %v", err)
	}
	// O QUE HÁ A RECEBER SAI JUNTO, e uma falha aqui NÃO derruba a resposta.
	//
	// São duas perguntas de peso diferente na mesma chamada: "qual é a minha chave"
	// é o que o formulário precisa para funcionar, e "quanto eu tenho a receber" é
	// informação. Deixar a segunda derrubar a primeira tiraria do ar a única tela
	// onde a pessoa conserta o cadastro — e o cadastro incompleto é justamente a
	// causa mais comum de haver dinheiro parado.
	//
	// Na falha vai zero com motivo UNSPECIFIED, que é o que a tela já sabe mostrar
	// como "nada pendente". Ela erra para o lado de não afirmar nada.
	pendente, motivo, err := s.pix.RepasseDoVendedor(ctx, req.GetAccountId())
	if err != nil {
		s.log.ErrorContext(ctx, "lendo o repasse pendente do vendedor",
			"account_id", req.GetAccountId(), "erro", err)
		pendente, motivo = 0, store.SemEspera
	}
	return &webv1.GetPixKeyResponse{
		HasKey:             r.TemChave,
		MaskedKey:          r.ChaveMascarada,
		Type:               tipoParaProto(r.Tipo),
		Verified:           r.Verificada,
		MaskedTaxId:        r.DocumentoMascarado,
		PendingPayoutCents: pendente,
		PayoutWaitReason:   motivoParaProto(motivo),
	}, nil
}

// motivoParaProto traduz o motivo da espera. O default é UNSPECIFIED e não um
// palpite: motivo novo que chegasse aqui sem tradução viraria uma frase errada na
// tela de alguém que está atrás do próprio dinheiro.
func motivoParaProto(m store.MotivoDaEspera) webv1.PayoutWaitReason {
	switch m {
	case store.EsperaCadastro:
		return webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_NO_KEY_OR_TAX_ID
	case store.EsperaPagamento:
		return webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_IN_PROGRESS
	case store.EsperaGente:
		return webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_NEEDS_STAFF
	}
	return webv1.PayoutWaitReason_PAYOUT_WAIT_REASON_UNSPECIFIED
}

func tipoDoProto(t webv1.PixKeyType) store.TipoChavePix {
	switch t {
	case webv1.PixKeyType_PIX_KEY_TYPE_CPF:
		return store.ChavePixCPF
	case webv1.PixKeyType_PIX_KEY_TYPE_EMAIL:
		return store.ChavePixEmail
	case webv1.PixKeyType_PIX_KEY_TYPE_PHONE:
		return store.ChavePixTelefone
	case webv1.PixKeyType_PIX_KEY_TYPE_RANDOM:
		return store.ChavePixAleatoria
	}
	// Zero não é tipo: cai na validação do store e volta como INVALID, que é o
	// que o formulário sabe mostrar.
	return 0
}

func tipoParaProto(t store.TipoChavePix) webv1.PixKeyType {
	switch t {
	case store.ChavePixCPF:
		return webv1.PixKeyType_PIX_KEY_TYPE_CPF
	case store.ChavePixEmail:
		return webv1.PixKeyType_PIX_KEY_TYPE_EMAIL
	case store.ChavePixTelefone:
		return webv1.PixKeyType_PIX_KEY_TYPE_PHONE
	case store.ChavePixAleatoria:
		return webv1.PixKeyType_PIX_KEY_TYPE_RANDOM
	}
	return webv1.PixKeyType_PIX_KEY_TYPE_UNSPECIFIED
}

// GetMyCurrentPixCharge devolve a cobrança que o comprador tem de pagar — ou o que
// aconteceu com ela.
//
// SEM COBRANÇA É RESPOSTA VAZIA E NÃO ERRO, e isto é o caso comum e não a
// exceção: quem pergunta é a página da conta, que as pessoas abrem para ver outras
// coisas. Devolver erro aqui poria vermelho na tela de quase todo mundo, quase
// sempre.
//
// A CONTA VEM DO PEDIDO E NÃO HÁ PARÂMETRO DE "COBRANÇA DE QUEM": o id chega
// resolvido pelo BFF, que é quem sabe quem entrou. Quem garante que uma conta não
// lê a cobrança de outra é a autorização do serviço (authz.go, RmtWebService está
// em servicosDoJogador), e não uma conferência aqui — a mesma divisão da chave
// Pix.
func (s *ServerRmt) GetMyCurrentPixCharge(ctx context.Context, req *webv1.GetMyCurrentPixChargeRequest) (*webv1.GetMyCurrentPixChargeResponse, error) {
	tem, cob, err := s.pix.CobrancaAtualDoComprador(ctx, req.GetAccountId(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get current pix charge: %v", err)
	}
	if !tem {
		return &webv1.GetMyCurrentPixChargeResponse{}, nil
	}

	estado := cob.Estado
	// A LEITURA CRIA, e é a única escrita que este método faz. Antes ela não criava
	// nada, e o comentário do .proto dizia isso — a mudança está registrada lá.
	//
	// Por que aqui e não na hora do clique no jogo: assim quem clica em comprar e
	// nunca abre a página não gasta uma chamada na processadora, que é cobrada. E o
	// servidor de jogo não precisa do segredo nem do certificado da ponte, o que
	// seria o preço da outra saída.
	if estado == store.EstadoCobrancaAberta && cob.CodigoPix == "" && s.criarPix != nil {
		// A descrição é montada AQUI e fechada dentro da função que o store chama,
		// porque quem sabe o nome do item é este pacote e quem tem a linha travada é
		// o store. Assim o store continua sem saber que existe catálogo.
		descricao := s.descricaoDaCobranca(cob)
		criar := func(ctx context.Context, referencia string, centavos int64) (string, string, error) {
			// MEDE QUANTO A PROCESSADORA DEMORA, porque esse número decide um
			// desenho e hoje ninguém o tem.
			//
			// O site desiste da leitura em 10 s e o cliente da ponte espera até 15.
			// Se a criação passar dos 10, a tela mostra erro e PARA de reler — a
			// pessoa fica com uma página morta até recarregar.
			//
			// O conserto óbvio seria encurtar o prazo daqui para menos de 10 s, e
			// ele é ARMADILHA: se a processadora for consistentemente mais lenta
			// que o corte, NENHUMA tentativa termina e o código nunca nasce. Trocar
			// uma página morta por um item que não se consegue comprar é pior.
			//
			// Então primeiro o número aparece, e a decisão vem depois dele. Sai no
			// log em toda criação, inclusive quando falha, que é o caso cuja
			// duração interessa mais.
			inicio := time.Now()
			cod, ident, err := s.criarPix(ctx, referencia, centavos, descricao)
			s.log.Info("rmt: a processadora respondeu a criacao do pix",
				"cobranca", cob.CobrancaID,
				"levou_ms", time.Since(inicio).Milliseconds(),
				"deu_certo", err == nil)
			return cod, ident, err
		}
		// A CRIAÇÃO É DESLIGADA DO CHAMADOR, e este é o conserto que mais importa
		// neste arquivo.
		//
		// O QUE ACONTECIA: o site desiste da leitura em 10 s. Aí o gRPC cancela o
		// contexto da requisição, e esse contexto era o mesmo que ia para a chamada à
		// processadora E para a transação. Resultado: a chamada morria no meio e a
		// transação era desfeita, então NADA ficava gravado — enquanto a cobrança
		// podia já ter nascido do outro lado, com um identifier que ninguém aqui
		// jamais saberia.
		//
		// E o estrago não era eventual: se a processadora fosse consistentemente mais
		// lenta que os 10 s, TODA tentativa morreria no mesmo ponto, o código nunca
		// nasceria, o item ficaria invendável, e cada clique deixaria uma cobrança
		// órfã na processadora.
		//
		// Com o contexto desligado, a criação termina e GRAVA mesmo que quem pediu já
		// tenha ido embora. A leitura seguinte do site encontra o código gravado — o
		// FOR UPDATE faz a segunda esperar a primeira em vez de criar outra.
		//
		// O preço, que é aceito e não ignorado: um leitor que desistiu continua
		// ocupando uma conexão do banco até este prazo acabar. Com poucas cobranças
		// por dia isso é irrelevante; se um dia o volume crescer, o conserto é a
		// criação sair para um trabalhador de fundo em vez de viver na leitura.
		ctxCriar, cancelarCriacao := context.WithTimeout(
			context.WithoutCancel(ctx), PrazoDaCriacao)
		defer cancelarCriacao()

		pix, err := s.pix.CriarPixSeFaltar(ctxCriar, cob.CobrancaID, MinimoParaCriarPix, criar)
		switch {
		case err != nil:
			// NÃO VIRA ERRO PARA A PÁGINA, de propósito. A página relê a cada cinco
			// segundos: a tentativa seguinte tenta de novo, com a MESMA referência,
			// e a ponte reconhece a repetição em vez de criar uma segunda cobrança.
			// Um erro aqui poria vermelho na tela por um tropeço de rede que se
			// resolve em cinco segundos.
			//
			// Error e não Warn porque, se isto NÃO se resolver, a venda não acontece:
			// o comprador fica olhando "gerando o código" até o prazo vencer, e
			// ninguém do lado dele consegue fazer nada.
			s.log.Error("rmt: nao consegui criar o codigo pix da cobranca",
				"cobranca", cob.CobrancaID, "err", err)
		case pix.SemPrazo:
			// Não sobrou prazo útil, então nada foi criado. A pessoa vê VENCIDA e
			// pode abrir outra cobrança, que nasce com a janela inteira — em vez de
			// receber um QR que quase certamente vira reembolso.
			estado = store.EstadoCobrancaExpirada
		default:
			cob.CodigoPix = pix.CodigoPix
		}
	}

	return &webv1.GetMyCurrentPixChargeResponse{
		HasCharge: true,
		// Vem como o banco guardou, que é como a processadora devolveu. E vem
		// vazio em todo estado fechado, porque a camada de banco só o entrega no
		// estado aberto — cobrança morta ao lado de um código vivo convida alguém
		// a pagar.
		PixCode:     cob.CodigoPix,
		AmountCents: cob.ValorCentavos,
		ExpiresAt:   cob.ExpiraEm.Unix(),
		State:       estadoParaProto(estado),
		ItemIndex:   int32(cob.ItemIndex),
		RefineLevel: int32(cob.Refino),
		StackSize:   int32(cob.Quantidade),
		SellerName:  cob.VendedorNome,
		// Zero quando nada foi pedido, que é o caso de quase toda cobrança. A
		// página conta os até dois dias úteis a partir desta data.
		RefundRequestedAt: unixOuZero(cob.ReembolsoPedidoEm),
		RefundState:       reembolsoParaProto(cob.Reembolso),
	}, nil
}

// estadoParaProto mapeia o estado do banco no do contrato.
//
// Explícito e não aritmético, mesmo com os números batendo hoje: os dois enums
// vivem em arquivos diferentes e mudam por motivos diferentes, e uma conversão por
// cast passaria a mentir em silêncio no dia em que um deles ganhasse um estado no
// meio.
func estadoParaProto(e store.EstadoCobrancaComprador) webv1.PixChargeState {
	switch e {
	case store.EstadoCobrancaAberta:
		return webv1.PixChargeState_PIX_CHARGE_STATE_OPEN
	case store.EstadoCobrancaExpirada:
		return webv1.PixChargeState_PIX_CHARGE_STATE_EXPIRED
	case store.EstadoCobrancaPaga:
		return webv1.PixChargeState_PIX_CHARGE_STATE_PAID
	case store.EstadoCobrancaCancelada:
		return webv1.PixChargeState_PIX_CHARGE_STATE_CANCELED
	case store.EstadoCobrancaPagaSemItem:
		return webv1.PixChargeState_PIX_CHARGE_STATE_PAID_LATE
	case store.EstadoCobrancaValorDivergente:
		// PAID_LATE TAMBÉM PARA O VALOR DIVERGENTE, e a escolha é do menos errado
		// entre os que existem.
		//
		// O nome fala de atraso e o caso não é de atraso — mas o que o estado diz
		// à pessoa é exatamente o certo: O DINHEIRO CHEGOU, O ITEM NÃO FOI
		// ENTREGUE, E ALGUÉM ESTÁ OLHANDO. É a mesma frase nos dois casos, e a
		// causa (tarde, ou valor diferente) não muda nada do que ela pode fazer.
		//
		// Os outros mentiriam pior. OPEN diz "pague", e ela já pagou — convidaria a
		// pagar duas vezes. PAID diz "o item está indo", e não está. EXPIRED e
		// CANCELED dizem que não houve pagamento, e houve.
		//
		// A diferença aparece no refund_state: no atrasado ele anda (PENDING,
		// REQUESTED, REFUNDED); aqui fica UNSPECIFIED, porque não há reembolso
		// automático — valor diferente do cobrado é caso que precisa de gente, e
		// escolher devolver, cobrar a diferença ou entregar assim mesmo é decidir
		// sobre o dinheiro de duas pessoas.
		//
		// O comentário do .proto ainda diz só "arrived late". Ele pega carona no
		// próximo handshake: mudança de comentário também muda o sha256, e o
		// prebuild do site compara o hash com a main — um handshake inteiro por uma
		// frase não se paga.
		return webv1.PixChargeState_PIX_CHARGE_STATE_PAID_LATE
	}
	return webv1.PixChargeState_PIX_CHARGE_STATE_UNSPECIFIED
}

// unixOuZero devolve 0 para uma data vazia em vez do unix de 1970, que é o que o
// Time zero daria — e 1970 na tela seria uma data de verdade, errada.
func unixOuZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// reembolsoParaProto mapeia o estado do reembolso no do contrato, explícito pelo
// mesmo motivo do estadoParaProto: os dois enums mudam por motivos diferentes.
func reembolsoParaProto(e store.EstadoReembolso) webv1.RefundState {
	switch e {
	case store.ReembolsoPendente:
		return webv1.RefundState_REFUND_STATE_PENDING
	case store.ReembolsoPedido:
		return webv1.RefundState_REFUND_STATE_REQUESTED
	case store.ReembolsoConcluido:
		return webv1.RefundState_REFUND_STATE_REFUNDED
	case store.ReembolsoRecusado:
		return webv1.RefundState_REFUND_STATE_FAILED
	}
	return webv1.RefundState_REFUND_STATE_UNSPECIFIED
}
