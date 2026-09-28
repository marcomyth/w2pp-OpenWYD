package panel

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/session"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

type fakeRepasses struct {
	fila []store.RepasseNaFila
	err  error

	pago     map[int64]int64
	naoPago  map[int64]string
	recusa   []int64
	erroAcao error

	// ajustes guarda o que a tela pediu, e o valor ANTIGO devolvido, para o teste poder
	// conferir que a auditoria recebe de-para e nao so o numero novo.
	ajustes    []ajustePedido
	antigoFake int64
	erroAjuste error

	// A fila de pagar a mao e as duas acoes dela.
	aPagar       []store.PagamentoNaFila
	erroAPagar   error
	pagosAMao    []pagamentoAMaoPedido
	erroPagarMao error
	chaveInteira store.ChaveInteira
	chavesLidas  []int64
	erroChave    error
}

type pagamentoAMaoPedido struct {
	ID        int64
	Nota      string
	Ator      string
	AtorConta int64
	AtorPapel string
}

func (f *fakeRepasses) FilaDePagamentoAMao(context.Context, int) ([]store.PagamentoNaFila, error) {
	return f.aPagar, f.erroAPagar
}

func (f *fakeRepasses) MarcarRepassePagoAMao(_ context.Context, id int64,
	ator store.AtorDoAjuste, nota string,
) error {
	if f.erroPagarMao != nil {
		return f.erroPagarMao
	}
	f.pagosAMao = append(f.pagosAMao, pagamentoAMaoPedido{
		ID: id, Nota: nota, Ator: ator.Nome, AtorConta: ator.ContaID, AtorPapel: ator.Papel,
	})
	return nil
}

func (f *fakeRepasses) ChaveParaPagar(_ context.Context, id int64, _ store.AtorDoAjuste,
) (store.ChaveInteira, error) {
	if f.erroChave != nil {
		return store.ChaveInteira{}, f.erroChave
	}
	f.chavesLidas = append(f.chavesLidas, id)
	return f.chaveInteira, nil
}

type ajustePedido struct {
	ID        int64
	Novo      int64
	Nota      string
	Ator      string
	AtorConta int64
	AtorPapel string
}

func novoFakeRepasses(fila ...store.RepasseNaFila) *fakeRepasses {
	return &fakeRepasses{fila: fila, pago: map[int64]int64{}, naoPago: map[int64]string{}}
}

func (f *fakeRepasses) RepassesQuePrecisamDeGente(context.Context) ([]store.RepasseNaFila, error) {
	return f.fila, f.err
}

func (f *fakeRepasses) ResolverIncertoComoPago(_ context.Context, id int64,
	_ store.AtorDoRepasse, chegou int64,
) error {
	if f.pago == nil {
		f.pago = map[int64]int64{}
	}
	f.pago[id] = chegou
	return f.erroAcao
}

func (f *fakeRepasses) ResolverIncertoComoNaoPago(_ context.Context, id int64,
	_ store.AtorDoRepasse, nota string,
) error {
	if f.naoPago == nil {
		f.naoPago = map[int64]string{}
	}
	f.naoPago[id] = nota
	return f.erroAcao
}

func (f *fakeRepasses) ResolverRecusa(_ context.Context, id int64, _ store.AtorDoRepasse) error {
	f.recusa = append(f.recusa, id)
	return f.erroAcao
}

func (f *fakeRepasses) AjustarValorDoRepasse(_ context.Context, id int64, novo int64,
	ator store.AtorDoAjuste, nota string,
) (int64, error) {
	if f.erroAjuste != nil {
		return 0, f.erroAjuste
	}
	f.ajustes = append(f.ajustes, ajustePedido{
		ID: id, Novo: novo, Nota: nota, Ator: ator.Nome, AtorConta: ator.ContaID, AtorPapel: ator.Papel,
	})
	return f.antigoFake, nil
}

// umaFilaDeRepasse tem um de cada espécie, para a página mostrar os três textos.
func umaFilaDeRepasse() []store.RepasseNaFila {
	agora := time.Now().Add(-2 * time.Hour)
	http422 := int32(422)
	return []store.RepasseNaFila{
		{ID: 1, CobrancaID: 10, VendedorConta: 7, ValorCentavos: 1234,
			Estado: store.RepasseIncerto, RecusaTexto: "a resposta nao voltou", CriadoEm: agora},
		{ID: 2, CobrancaID: 11, VendedorConta: 8, ValorCentavos: 500,
			Estado: store.RepasseRecusado, RecusaHTTP: &http422,
			RecusaCodigo: "PIX_KEY_NOT_FOUND", RecusaTexto: "chave nao existe", CriadoEm: agora},
		{ID: 3, CobrancaID: 12, VendedorConta: 9, ValorCentavos: 9900,
			Estado: store.RepasseEsperandoCadastro, RecusaTexto: "falta o documento", CriadoEm: agora},
	}
}

// painelComRepasses monta o painel com a fila ligada.
func painelComRepasses(t *testing.T, r Repasses) http.Handler {
	t.Helper()
	h, err := New(Config{
		Accounts: withTarget(roleAdmin), Writer: newFakeWriter(), Audit: newFakeAudit(),
		Repasses: r, Sessions: session.New(time.Hour),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), SecureOnly: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h.Routes()
}

// postSigned manda o formulario ja com a sessao e o token.
func postSigned(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	post, token := signedInPost(t, h)
	form.Set("csrf", token)
	return post(path, form)
}

// O VALOR APARECE EM REAIS, e é o número que alguém vai comparar com o extrato da
// processadora. Centavo fora de lugar numa tela de dinheiro é o erro que só aparece
// depois de alguém agir sobre ele.
func TestEmReais(t *testing.T) {
	casos := map[int64]string{
		0: "R$ 0,00", 5: "R$ 0,05", 99: "R$ 0,99", 100: "R$ 1,00",
		1234: "R$ 12,34", 100000: "R$ 1000,00", -250: "-R$ 2,50",
	}
	for centavos, quer := range casos {
		if got := emReais(centavos); got != quer {
			t.Errorf("emReais(%d) = %q, quero %q", centavos, got, quer)
		}
	}
}

// O QUE A PESSOA DIGITA NO CAMPO DE VALOR.
//
// Ela está copiando do extrato da processadora, então as duas formas de separador
// chegam aqui. E o que não se entende é RECUSADO: numa tela de dinheiro, adivinhar o
// que alguém quis digitar é pior do que pedir de novo.
func TestCentavosDoFormulario(t *testing.T) {
	bons := map[string]int64{
		"12,34": 1234, "12.34": 1234, "12": 1200, "0,05": 5,
		"R$ 12,34": 1234, " 12,34 ": 1234, "1000,00": 100000,
		// "12,3" é doze e trinta, como se lê em voz alta — e não doze e três.
		"12,3": 1230,
	}
	for txt, quer := range bons {
		got, err := centavosDoFormulario(txt)
		if err != nil || got != quer {
			t.Errorf("centavosDoFormulario(%q) = %d, %v; quero %d", txt, got, err, quer)
		}
	}
	ruins := []string{"", "abc", "-5", "12,345", "12,3,4", "12a", "R$"}
	for _, txt := range ruins {
		if got, err := centavosDoFormulario(txt); err == nil {
			t.Errorf("centavosDoFormulario(%q) = %d sem erro; o valor errado viraria pagamento", txt, got)
		}
	}
}

// A TELA MOSTRA OS TRÊS ESTADOS COM NOMES DIFERENTES, porque eles pedem coisas
// diferentes de quem está olhando: um pede ir ao painel da processadora, outro pede
// um clique, e o terceiro não pede nada.
func TestAPaginaSeparaOsTresEstados(t *testing.T) {
	h := painelComRepasses(t, novoFakeRepasses(umaFilaDeRepasse()...))
	rec := signedIn(t, h)("/repasses")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	corpo := rec.Body.String()
	for _, quero := range []string{"incerto", "recusado", "esperando cadastro",
		"R$ 12,34", "R$ 5,00", "R$ 99,00", "PIX_KEY_NOT_FOUND"} {
		if !strings.Contains(corpo, quero) {
			t.Errorf("a pagina nao diz %q", quero)
		}
	}
}

// MARCAR COMO PAGO USA O VALOR DIGITADO, e não o da dívida.
//
// Quem está olhando o extrato vê quanto CHEGOU, que pode não ser o que foi pedido —
// a processadora cobra taxa. Copiar a dívida faria a tela inventar um número e
// chamar de medição.
func TestMarcarPagoUsaOValorDigitado(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	rec := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"pago"}, "chegou": {"11,50"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d: %s", rec.Code, primeiraLinha(rec.Body.String()))
	}
	if f.pago[1] != 1150 {
		t.Errorf("chegou = %d, quero 1150 (e nao os 1234 da divida)", f.pago[1])
	}
}

// E UM VALOR ILEGÍVEL NÃO VIRA PAGAMENTO. Ele volta com um aviso, e nada muda.
func TestValorIlegivelNaoMarcaNada(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	rec := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"pago"}, "chegou": {"onze e meio"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if len(f.pago) != 0 {
		t.Errorf("marcou %v com um valor ilegivel", f.pago)
	}
}

// "NÃO FOI PAGO" EXIGE A NOTA, e esta é a trava que importa desta tela.
//
// Essa decisão devolve a dívida para a fila e faz o dinheiro sair DE NOVO. A única
// proteção contra pagar duas vezes é alguém ter escrito onde olhou para ter certeza
// de que o primeiro pagamento não saiu.
func TestNaoFoiPagoExigeANota(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	rec := postSigned(t, h, "/repasses/1/resolver", url.Values{"decisao": {"nao-pago"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if len(f.naoPago) != 0 {
		t.Errorf("devolveu a divida para a fila sem ninguem dizer onde conferiu: %v", f.naoPago)
	}

	// E com a nota, passa — e a nota fica guardada.
	rec = postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"nao-pago"}, "nota": {"painel da SyncPay, 24/09"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if f.naoPago[1] != "painel da SyncPay, 24/09" {
		t.Errorf("nota = %q", f.naoPago[1])
	}
}

// A DECISÃO QUE ESTE CÓDIGO NÃO CONHECE NÃO FAZ NADA. Um campo adulterado no
// formulário não pode cair em nenhum dos três caminhos por descuido.
func TestDecisaoDesconhecidaNaoMexeEmNada(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	rec := postSigned(t, h, "/repasses/1/resolver", url.Values{"decisao": {"pagar-tudo"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("codigo = %d, quero 400", rec.Code)
	}
	if len(f.pago) != 0 || len(f.naoPago) != 0 || len(f.recusa) != 0 {
		t.Error("mexeu em alguma coisa com uma decisao desconhecida")
	}
}
