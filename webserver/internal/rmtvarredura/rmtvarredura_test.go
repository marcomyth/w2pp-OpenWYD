package rmtvarredura

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/ponte"
)

type fakeConf struct {
	confirmada map[string]bool
	erro       map[string]error
	pedidas    []string
}

func (f *fakeConf) ConferirEConcluir(_ context.Context, id string) (Resultado, error) {
	f.pedidas = append(f.pedidas, id)
	if err := f.erro[id]; err != nil {
		return Resultado{}, err
	}
	return Resultado{Confirmada: f.confirmada[id]}, nil
}

type fakeDev struct {
	resp    ponte.RespostaReembolso
	erro    error
	chamou  int
	ultimos []string
}

func (f *fakeDev) PedirReembolso(_ context.Context, identifier, referencia, detalhes string,
) (ponte.RespostaReembolso, error) {
	f.chamou++
	f.ultimos = []string{identifier, referencia, detalhes}
	return f.resp, f.erro
}

type fakeBanco struct {
	abertas    []store.CobrancaParaConferir
	vencidas   []int64
	erroListar error

	semConferir   int
	reembolsos    []store.ReembolsoParaPedir
	semIdentifier int

	pedido   []int64
	recusado map[int64]string
	incerto  map[int64]string
}

func novoBanco() *fakeBanco {
	return &fakeBanco{recusado: map[int64]string{}, incerto: map[int64]string{}}
}

func (b *fakeBanco) CobrancasParaConferir(context.Context, int) ([]store.CobrancaParaConferir, error) {
	return b.abertas, b.erroListar
}

func (b *fakeBanco) VencerCobrancaConferida(_ context.Context, id int64) (int64, error) {
	b.vencidas = append(b.vencidas, id)
	return id * 10, nil
}

func (b *fakeBanco) CobrancasVencidasSemConferir(context.Context) (int, error) {
	return b.semConferir, nil
}

func (b *fakeBanco) ReembolsosParaPedir(context.Context, int) ([]store.ReembolsoParaPedir, error) {
	return b.reembolsos, nil
}

func (b *fakeBanco) ReembolsosSemIdentifier(context.Context) (int, error) {
	return b.semIdentifier, nil
}

func (b *fakeBanco) MarcarReembolsoPedido(_ context.Context, id int64) error {
	b.pedido = append(b.pedido, id)
	return nil
}

func (b *fakeBanco) MarcarReembolsoRecusado(_ context.Context, id int64, codigo string) error {
	b.recusado[id] = codigo
	return nil
}

func (b *fakeBanco) MarcarReembolsoIncerto(_ context.Context, id int64, motivo string) error {
	b.incerto[id] = motivo
	return nil
}

func mudo() *slog.Logger { return slog.New(slog.DiscardHandler) }

func vencida(id int64, identifier string) store.CobrancaParaConferir {
	return store.CobrancaParaConferir{ID: id, Identifier: identifier,
		ExpiraEm: time.Now().Add(-time.Minute)}
}

// A CONSULTA QUE FALHA NÃO VENCE A COBRANÇA.
//
// É o teste que justifica o pacote existir. Sem resposta da processadora não se sabe
// se houve pagamento, e vencer na dúvida solta o item do vendedor por cima de um Pix
// que pode ter entrado — a pessoa paga, não recebe, e o item já voltou.
func TestConsultaQueFalhaNaoVence(t *testing.T) {
	b := novoBanco()
	b.abertas = []store.CobrancaParaConferir{vencida(1, "id-1")}
	conf := &fakeConf{erro: map[string]error{"id-1": errors.New("a processadora nao respondeu")}}

	r := Nova(conf, nil, b, mudo()).Conferir(context.Background())

	if len(b.vencidas) != 0 {
		t.Errorf("venceu %v com a consulta falhando", b.vencidas)
	}
	if r.Falhas != 1 || r.Vencidas != 0 {
		t.Errorf("rodada = %+v", r)
	}
}

// A CONFIRMADA NÃO VENCE: ela já fechou pelo caminho certo, e vencer por cima
// desmontaria o que a confirmação acabou de decidir.
func TestCobrancaConfirmadaNaoVence(t *testing.T) {
	b := novoBanco()
	b.abertas = []store.CobrancaParaConferir{vencida(7, "id-7")}
	conf := &fakeConf{confirmada: map[string]bool{"id-7": true}}

	r := Nova(conf, nil, b, mudo()).Conferir(context.Background())

	if len(b.vencidas) != 0 {
		t.Errorf("venceu uma cobranca ja confirmada: %v", b.vencidas)
	}
	if r.Confirmadas != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// DENTRO DO PRAZO, CONFERE E NÃO VENCE. Essa passada é o polling: ela existe para o
// comprador cujo aviso não chegou receber na rodada seguinte ao pagamento, em vez de
// esperar a janela inteira.
func TestDentroDoPrazoConfereENaoVence(t *testing.T) {
	b := novoBanco()
	b.abertas = []store.CobrancaParaConferir{
		{ID: 3, Identifier: "id-3", ExpiraEm: time.Now().Add(2 * time.Minute)},
	}
	conf := &fakeConf{}

	r := Nova(conf, nil, b, mudo()).Conferir(context.Background())

	if len(conf.pedidas) != 1 {
		t.Errorf("nao perguntou a processadora: %v", conf.pedidas)
	}
	if len(b.vencidas) != 0 {
		t.Errorf("venceu antes do prazo: %v", b.vencidas)
	}
	if r.Conferidas != 1 || r.Vencidas != 0 {
		t.Errorf("rodada = %+v", r)
	}
}

// PASSADO O PRAZO E SEM PAGAMENTO, VENCE — que é o caminho normal da maioria.
func TestForaDoPrazoSemPagamentoVence(t *testing.T) {
	b := novoBanco()
	b.abertas = []store.CobrancaParaConferir{vencida(5, "id-5")}

	r := Nova(&fakeConf{}, nil, b, mudo()).Conferir(context.Background())

	if len(b.vencidas) != 1 || b.vencidas[0] != 5 {
		t.Errorf("venceu = %v, quero [5]", b.vencidas)
	}
	if r.Vencidas != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// UMA FALHA NÃO PARA A RODADA: a cobrança seguinte continua sendo conferida.
func TestUmaFalhaNaoParaAsOutras(t *testing.T) {
	b := novoBanco()
	b.abertas = []store.CobrancaParaConferir{vencida(1, "id-1"), vencida(2, "id-2")}
	conf := &fakeConf{erro: map[string]error{"id-1": errors.New("caiu")}}

	r := Nova(conf, nil, b, mudo()).Conferir(context.Background())

	if len(b.vencidas) != 1 || b.vencidas[0] != 2 {
		t.Errorf("venceu = %v, quero so a segunda", b.vencidas)
	}
	if r.Falhas != 1 || r.Vencidas != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// O PEDIDO DE REEMBOLSO QUE NÃO VOLTA VIRA INCERTO, e não volta para a fila.
//
// É o teste que protege o dinheiro da Hanna. Deixado como pendente, o pedido sairia
// de novo na rodada seguinte, e de novo a cada vinte segundos — e se o primeiro tinha
// sido criado, o comprador recebe o dobro do que pagou.
func TestPedidoQueNaoVoltaViraIncerto(t *testing.T) {
	b := novoBanco()
	b.reembolsos = []store.ReembolsoParaPedir{{CobrancaID: 9, Identifier: "id-9", Referencia: "ref-9"}}
	dev := &fakeDev{erro: errors.New("a resposta nao voltou")}

	r := Nova(&fakeConf{}, dev, b, mudo()).Devolver(context.Background())

	if _, marcou := b.incerto[9]; !marcou {
		t.Error("nao marcou incerto: a linha voltaria a ser pedida")
	}
	if len(b.pedido) != 0 {
		t.Errorf("marcou como pedido: %v", b.pedido)
	}
	if r.Incertos != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// OS ESTADOS DA PONTE, cada um no seu destino.
func TestEstadosDoPedidoDeReembolso(t *testing.T) {
	casos := []struct {
		estado  string
		pedido  bool
		recusa  bool
		incerto bool
	}{
		{estado: "pedido", pedido: true},
		// REPETIDO é a idempotência da ponte funcionando: ela não chamou a
		// processadora de novo. Conta como pedido, e é o que se quer que aconteça
		// quando a rodada anterior morreu no meio.
		{estado: "repetido", pedido: true},
		{estado: "recusado", recusa: true},
		{estado: "incerto", incerto: true},
		// ESTADO NOVO VIRA INCERTO, e não recusa. Tratado como recusa, a linha
		// voltaria à fila e seria pedida outra vez — e se o pedido tinha sido criado,
		// o comprador recebe duas vezes.
		{estado: "coisa-que-ninguem-previu", incerto: true},
		{estado: "", incerto: true},
	}
	for _, c := range casos {
		t.Run(c.estado, func(t *testing.T) {
			b := novoBanco()
			b.reembolsos = []store.ReembolsoParaPedir{{CobrancaID: 4, Identifier: "id-4"}}
			dev := &fakeDev{resp: ponte.RespostaReembolso{Estado: c.estado, CodigoSyncpay: "X1"}}

			Nova(&fakeConf{}, dev, b, mudo()).Devolver(context.Background())

			if got := len(b.pedido) == 1; got != c.pedido {
				t.Errorf("pedido = %v, quero %v", got, c.pedido)
			}
			if _, got := b.recusado[4]; got != c.recusa {
				t.Errorf("recusado = %v, quero %v", got, c.recusa)
			}
			if _, got := b.incerto[4]; got != c.incerto {
				t.Errorf("incerto = %v, quero %v", got, c.incerto)
			}
		})
	}
}

// A RECUSA GUARDA O CÓDIGO DELES, inteiro. A diferença entre "reembolso não
// habilitado para esta conta" e "conta do seller não aprovada" decide o que a Hanna
// pede ao suporte, e as duas viram "falhou" se a gente resumir.
func TestRecusaGuardaOCodigoDaProcessadora(t *testing.T) {
	b := novoBanco()
	b.reembolsos = []store.ReembolsoParaPedir{{CobrancaID: 2, Identifier: "id-2"}}
	dev := &fakeDev{resp: ponte.RespostaReembolso{Estado: "recusado", CodigoSyncpay: "REFUND_NOT_ENABLED"}}

	Nova(&fakeConf{}, dev, b, mudo()).Devolver(context.Background())

	if b.recusado[2] != "REFUND_NOT_ENABLED" {
		t.Errorf("codigo = %q", b.recusado[2])
	}
}

// SEM PONTE, NADA É PEDIDO — mas o número de quem está esperando continua saindo.
//
// Uma fila que some quando a ponte está desligada é uma fila que ninguém descobre
// que existe.
func TestSemPonteNaoPedeMasConta(t *testing.T) {
	b := novoBanco()
	b.semIdentifier = 3
	b.reembolsos = []store.ReembolsoParaPedir{{CobrancaID: 1, Identifier: "id-1"}}

	r := Nova(&fakeConf{}, nil, b, mudo()).Devolver(context.Background())

	if len(b.pedido) != 0 || len(b.incerto) != 0 {
		t.Error("mexeu em reembolso sem ponte")
	}
	if r.SemIdentifier != 3 {
		t.Errorf("sem_identifier = %d, quero 3", r.SemIdentifier)
	}
}

// A FILA QUE NINGUÉM CONFERIU APARECE NA RODADA, para o log poder gritar.
func TestVencidasSemConferirEntramNaRodada(t *testing.T) {
	b := novoBanco()
	b.semConferir = 4

	r := Nova(&fakeConf{}, nil, b, mudo()).Conferir(context.Background())

	if r.VencidasSemConferir != 4 {
		t.Errorf("vencidas_sem_conferir = %d, quero 4", r.VencidasSemConferir)
	}
}

// O PEDIDO VAI COM O ID DELES E COM A NOSSA REFERÊNCIA.
//
// O identifier é como a processadora acha o pagamento; a referência é o que torna o
// pedido repetível sem criar um segundo. Trocar os dois de lugar faria a ponte
// devolver "não achei" para sempre — e a fila pararia sem ninguém entender por quê.
func TestOPedidoLevaOIdentifierEAReferencia(t *testing.T) {
	b := novoBanco()
	b.reembolsos = []store.ReembolsoParaPedir{
		{CobrancaID: 12, Identifier: "id-12", Referencia: "ref-12"},
	}
	dev := &fakeDev{resp: ponte.RespostaReembolso{Estado: "pedido"}}

	Nova(&fakeConf{}, dev, b, mudo()).Devolver(context.Background())

	if dev.chamou != 1 {
		t.Fatalf("chamou %d vezes", dev.chamou)
	}
	if dev.ultimos[0] != "id-12" || dev.ultimos[1] != "ref-12" {
		t.Errorf("identifier/referencia = %q / %q", dev.ultimos[0], dev.ultimos[1])
	}
	// E os detalhes dizem qual pagamento é, sem nome de pessoa e sem nome de item:
	// quem lê do outro lado é o suporte deles.
	if !strings.Contains(dev.ultimos[2], "12") {
		t.Errorf("detalhes = %q, nao identificam a cobranca", dev.ultimos[2])
	}
}
