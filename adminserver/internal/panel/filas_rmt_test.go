package panel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

type fakeFilas struct {
	orfaos      []store.PagamentoOrfao
	reembolsos  []store.ReembolsoRecusadoNaFila
	divergentes []store.ValorDivergenteNaFila
	semID       int
	erroSemID   error

	orfaoResolvido map[int64]string
	deNovo         []int64
	naMao          []int64
	achado         []int64
	erroAcao       error
}

func novoFakeFilas() *fakeFilas {
	return &fakeFilas{
		orfaos:         umaFilaDeOrfaos(),
		reembolsos:     umaFilaDeReembolsos(),
		divergentes:    umaFilaDeDivergentes(),
		orfaoResolvido: map[int64]string{},
	}
}

func (f *fakeFilas) PagamentosOrfaos(context.Context) ([]store.PagamentoOrfao, error) {
	return f.orfaos, nil
}

func (f *fakeFilas) ResolverPagamentoOrfao(_ context.Context, id int64, _, nota string) error {
	if f.orfaoResolvido == nil {
		f.orfaoResolvido = map[int64]string{}
	}
	f.orfaoResolvido[id] = nota
	return f.erroAcao
}

func (f *fakeFilas) ReembolsosRecusados(context.Context) ([]store.ReembolsoRecusadoNaFila, error) {
	return f.reembolsos, nil
}

func (f *fakeFilas) ReabrirReembolsoRecusado(_ context.Context, id int64, _ store.AtorDaStaff) error {
	f.deNovo = append(f.deNovo, id)
	return f.erroAcao
}

func (f *fakeFilas) ResolverReembolsoNaMao(_ context.Context, id int64, _ store.AtorDaStaff) error {
	f.naMao = append(f.naMao, id)
	return f.erroAcao
}

func (f *fakeFilas) ConfirmarReembolsoPedido(_ context.Context, id int64, _ store.AtorDaStaff) error {
	f.achado = append(f.achado, id)
	return f.erroAcao
}

func (f *fakeFilas) ValoresDivergentes(context.Context) ([]store.ValorDivergenteNaFila, error) {
	return f.divergentes, nil
}

func (f *fakeFilas) ReembolsosSemIdentifier(context.Context) (int, error) {
	return f.semID, f.erroSemID
}

func umaFilaDeOrfaos() []store.PagamentoOrfao {
	valor := int64(2990)
	pago := time.Now().Add(-3 * time.Hour)
	return []store.PagamentoOrfao{
		{ID: 1, Identifier: "tx-com-valor", ValorCentavos: &valor, PagoEm: &pago,
			ReferenciaVista: "ref-que-nao-existe", Motivo: "referencia sem cobranca",
			VistoEm: time.Now().Add(-2 * time.Hour)},
		// O caso mais comum: a processadora disse "pago" e não disse quanto.
		{ID: 2, Identifier: "tx-sem-valor", Motivo: "pago sem valor",
			VistoEm: time.Now().Add(-time.Hour)},
	}
}

func umaFilaDeReembolsos() []store.ReembolsoRecusadoNaFila {
	return []store.ReembolsoRecusadoNaFila{
		{CobrancaID: 10, CompradorConta: 7, ValorCentavos: 100, Identifier: "tx-a",
			Erro: "REFUND_NOT_ENABLED", Estado: store.ReembolsoRecusado},
		{CobrancaID: 11, CompradorConta: 8, ValorCentavos: 500, Identifier: "tx-b",
			Erro: "a chamada nao voltou", Estado: store.ReembolsoIncerto},
	}
}

func umaFilaDeDivergentes() []store.ValorDivergenteNaFila {
	return []store.ValorDivergenteNaFila{
		{CobrancaID: 20, CompradorConta: 9, ValorCobrado: 1000, ValorRecebido: 700, Identifier: "tx-c"},
		{CobrancaID: 21, CompradorConta: 9, ValorCobrado: 1000, ValorRecebido: 1500, Identifier: "tx-d"},
	}
}

func painelComFilas(t *testing.T, f FilasRMT) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		FilasRMT: f, Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// O VALOR AUSENTE DO ÓRFÃO NÃO VIRA ZERO NA TELA.
//
// É o caso mais comum de órfão: a processadora disse "pago" e não disse quanto. Um
// "R$ 0,00" ali seria afirmar que entrou nada, que é uma informação diferente de "ela
// não contou" — e é sobre essa diferença que alguém vai decidir o que devolver.
func TestOrfaoSemValorNaoMostraZero(t *testing.T) {
	h := painelComFilas(t, novoFakeFilas())
	corpo := signedIn(t, h)("/orfaos").Body.String()

	if !strings.Contains(corpo, "R$ 29,90") {
		t.Error("o valor conhecido nao apareceu")
	}
	if strings.Contains(corpo, "R$ 0,00") {
		t.Error("o valor ausente virou R$ 0,00 na tela")
	}
	if !strings.Contains(corpo, "não disse o valor") {
		t.Error("a tela nao diz que a processadora nao informou o valor")
	}
	// E o identifier aparece, que é por onde a pessoa acha o pagamento lá.
	if !strings.Contains(corpo, "tx-sem-valor") {
		t.Error("o identifier nao apareceu")
	}
}

// RESOLVER UM ÓRFÃO EXIGE A NOTA.
//
// A linha sai da fila e não volta. Sem a nota, daqui a um mês ninguém sabe o que foi
// feito com o dinheiro de alguém que pagou e não recebeu nada.
func TestResolverOrfaoExigeANota(t *testing.T) {
	f := novoFakeFilas()
	h := painelComFilas(t, f)

	rec := postSigned(t, h, "/orfaos/1/resolver", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if len(f.orfaoResolvido) != 0 {
		t.Fatalf("resolveu sem nota: %v", f.orfaoResolvido)
	}

	rec = postSigned(t, h, "/orfaos/1/resolver", url.Values{"nota": {"devolvido no painel"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if f.orfaoResolvido[1] != "devolvido no painel" {
		t.Errorf("nota = %q", f.orfaoResolvido[1])
	}
}

// A TELA DE REEMBOLSO SEPARA RECUSADO DE INCERTO, e só o incerto ganha o botão de
// "achei no painel" — que é o único que faz sentido sobre um pedido que talvez exista.
func TestReembolsoSeparaRecusadoDeIncerto(t *testing.T) {
	h := painelComFilas(t, novoFakeFilas())
	corpo := signedIn(t, h)("/reembolsos").Body.String()

	for _, quero := range []string{"recusado", "incerto", "REFUND_NOT_ENABLED",
		"R$ 1,00", "R$ 5,00", "Achei no painel"} {
		if !strings.Contains(corpo, quero) {
			t.Errorf("a pagina nao diz %q", quero)
		}
	}
	// O botão do incerto aparece uma vez só: a linha recusada não o tem.
	if n := strings.Count(corpo, `value="achado"`); n != 1 {
		t.Errorf("o botao de 'achei no painel' aparece %d vezes, quero 1", n)
	}
}

// AS TRÊS DECISÕES DO REEMBOLSO CHEGAM NA FUNÇÃO CERTA.
func TestAsTresDecisoesDoReembolso(t *testing.T) {
	casos := []struct {
		decisao string
		conta   func(*fakeFilas) int
	}{
		{"de-novo", func(f *fakeFilas) int { return len(f.deNovo) }},
		{"na-mao", func(f *fakeFilas) int { return len(f.naMao) }},
		{"achado", func(f *fakeFilas) int { return len(f.achado) }},
	}
	for _, c := range casos {
		t.Run(c.decisao, func(t *testing.T) {
			f := novoFakeFilas()
			h := painelComFilas(t, f)
			rec := postSigned(t, h, "/reembolsos/11/resolver", url.Values{"decisao": {c.decisao}})
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("codigo = %d: %s", rec.Code, primeiraLinha(rec.Body.String()))
			}
			if c.conta(f) != 1 {
				t.Errorf("a decisao %q nao chegou", c.decisao)
			}
		})
	}
}

// A LINHA QUE MUDOU DE ESTADO NO MEIO DO CAMINHO NÃO VIRA ERRO FEIO.
//
// Duas pessoas na mesma tela, ou a varredura mexendo enquanto ela estava aberta. A
// pessoa precisa saber que não foi ela — e que recarregar resolve.
func TestReembolsoQueMudouDeEstadoAvisaEmVezDeQuebrar(t *testing.T) {
	f := novoFakeFilas()
	f.erroAcao = store.ErrReembolsoNaoEstaRecusado
	h := painelComFilas(t, f)

	rec := postSigned(t, h, "/reembolsos/11/resolver", url.Values{"decisao": {"de-novo"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d, quero um aviso e nao um erro", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "mudou+de+estado") {
		t.Errorf("destino = %q", rec.Header().Get("Location"))
	}
}

// A CONTAGEM QUE NÃO DEU PARA LER NÃO VIRA ZERO.
//
// Um zero ao lado da lista é uma afirmação — "não há devolução parada invisível" —, e
// é justamente a afirmação que ninguém pode fazer quando a leitura falhou.
func TestContagemQueFalhaNaoViraZero(t *testing.T) {
	f := novoFakeFilas()
	f.erroSemID = errors.New("o banco caiu")
	h := painelComFilas(t, f)

	rec := signedIn(t, h)("/reembolsos")

	if rec.Code != http.StatusOK {
		t.Fatalf("a falha do contador derrubou a pagina: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "falha") &&
		!strings.Contains(rec.Body.String(), "não") {
		t.Error("a pagina nao avisa que a contagem falhou")
	}
}

// A TELA DOS DIVERGENTES MOSTRA OS DOIS LADOS DA DIFERENÇA, e NÃO tem botão.
//
// A ausência é a decisão: devolver, cobrar a diferença ou entregar assim mesmo é
// escolha sobre o dinheiro de duas pessoas. Um botão daria a impressão de que existe
// um caminho automático certo.
func TestDivergentesMostraADiferencaENaoTemBotao(t *testing.T) {
	h := painelComFilas(t, novoFakeFilas())
	rec := signedIn(t, h)("/divergentes")
	corpo := rec.Body.String()

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	for _, quero := range []string{"R$ 10,00", "R$ 7,00", "faltaram R$ 3,00",
		"R$ 15,00", "sobraram R$ 5,00"} {
		if !strings.Contains(corpo, quero) {
			t.Errorf("a pagina nao diz %q", quero)
		}
	}
	// A busca e o "sair" da barra de cima são formulários e não contam; o que não
	// pode existir é um que aja sobre uma cobrança.
	if strings.Contains(corpo, `action="/divergentes`) {
		t.Error("a tela dos divergentes ganhou um formulario; a falta dele e a decisao")
	}
}
