package panel

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// umaDividaAPagar é uma linha da fila, com chave e CPF já mascarados — como o store
// entrega.
func umaDividaAPagar() store.PagamentoNaFila {
	criado := time.Now().Add(-3 * time.Hour)
	return store.PagamentoNaFila{
		ID: 7, VendedorConta: 42, ValorCentavos: 4_990,
		ChaveMascarada: "ven...or@exemplo.com", TipoChave: store.ChavePixEmail,
		DocMascarado: "***.456.789-**",
		CriadoEm:     criado, VenceEm: criado.Add(store.PrazoDoPagamento),
	}
}

// TestAFilaDePagarNaoMostraChaveInteira.
//
// A lista abre com todas as linhas de uma vez, em toda visita — inclusive quando a staff
// só passou para ver quantas faltavam. Chave e CPF inteiros ali seriam dado pessoal de
// todos os vendedores na tela, de graça, sem ninguém precisar deles.
func TestAFilaDePagarNaoMostraChaveInteira(t *testing.T) {
	f := novoFakeRepasses()
	f.aPagar = []store.PagamentoNaFila{umaDividaAPagar()}
	f.chaveInteira = store.ChaveInteira{ChavePix: "vendedor@exemplo.com", Documento: "12345678909"}
	h := painelComRepasses(t, f)

	corpo := signedIn(t, h)("/repasses").Body.String()
	if !strings.Contains(corpo, "ven...or@exemplo.com") {
		t.Error("a fila nao mostrou a chave mascarada")
	}
	if strings.Contains(corpo, "vendedor@exemplo.com") {
		t.Error("a fila mostrou a chave INTEIRA sem ninguem pedir")
	}
	if strings.Contains(corpo, "12345678909") {
		t.Error("a fila mostrou o CPF INTEIRO sem ninguem pedir")
	}
	if len(f.chavesLidas) != 0 {
		t.Error("abrir a lista leu a chave, e isso encheria a auditoria de leitura que ninguem pediu")
	}
}

// TestMostrarAChaveRegistraELeva: o clique traz o dado inteiro E passa pelo store, que é
// quem grava a leitura na mesma transação. Se a tela lesse por outro caminho, o registro
// dependeria de a tela lembrar de registrar.
func TestMostrarAChaveRegistraELeva(t *testing.T) {
	f := novoFakeRepasses()
	f.aPagar = []store.PagamentoNaFila{umaDividaAPagar()}
	f.chaveInteira = store.ChaveInteira{ChavePix: "vendedor@exemplo.com", Documento: "12345678909"}
	h := painelComRepasses(t, f)
	post, token := signedInPost(t, h)

	rec := post("/repasses/7/chave", url.Values{"csrf": {token}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a resposta desenha a pagina): %s", rec.Code, rec.Body.String())
	}
	if len(f.chavesLidas) != 1 || f.chavesLidas[0] != 7 {
		t.Fatalf("leituras registradas = %v, queria [7]", f.chavesLidas)
	}
	corpo := rec.Body.String()
	if !strings.Contains(corpo, "vendedor@exemplo.com") {
		t.Error("a chave inteira nao apareceu para quem pediu")
	}
	if !strings.Contains(corpo, "12345678909") {
		t.Error("o CPF inteiro nao apareceu para quem pediu")
	}
	// E a chave NÃO pode ter ido para a URL: ali ela entraria em log de acesso, no
	// histórico do navegador e no Referer da próxima requisição.
	if destino := rec.Header().Get("Location"); destino != "" {
		t.Errorf("a resposta redirecionou para %q; a chave viajaria na URL", destino)
	}
}

// TestMarcarComoPagoExigeObservacao: sem comprovante nosso atrás dessa escrita, a
// observação é o único lugar onde fica dito ONDE o dinheiro foi pago.
func TestMarcarComoPagoExigeObservacao(t *testing.T) {
	f := novoFakeRepasses()
	f.aPagar = []store.PagamentoNaFila{umaDividaAPagar()}
	h := painelComRepasses(t, f)
	post, token := signedInPost(t, h)

	rec := post("/repasses/7/pagar", url.Values{"csrf": {token}, "nota": {""}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if len(f.pagosAMao) != 0 {
		t.Fatal("marcou como pago sem observacao")
	}
}

// TestMarcarComoPagoGuardaQuemFez: a auditoria é gravada dentro da transação do store,
// então o que a tela tem de provar é que ela entrega o ator e a nota.
func TestMarcarComoPagoGuardaQuemFez(t *testing.T) {
	f := novoFakeRepasses()
	f.aPagar = []store.PagamentoNaFila{umaDividaAPagar()}
	h := painelComRepasses(t, f)
	post, token := signedInPost(t, h)

	rec := post("/repasses/7/pagar", url.Values{
		"csrf": {token}, "nota": {"pago no app, comprovante E123"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: %s", rec.Code, rec.Body.String())
	}
	if len(f.pagosAMao) != 1 {
		t.Fatalf("pagamentos = %d, want 1", len(f.pagosAMao))
	}
	p := f.pagosAMao[0]
	if p.ID != 7 || p.Nota != "pago no app, comprovante E123" {
		t.Errorf("pedido = %+v", p)
	}
	if p.Ator == "" || p.AtorConta == 0 || p.AtorPapel == "" {
		t.Errorf("o ator nao chegou inteiro a auditoria: %+v", p)
	}
}

// TestOSegundoCliqueNaoViraErroDeServidor.
//
// Entre a página e o botão a linha pode ter saído de pendente — outra pessoa pagou, ou
// foi o segundo clique da mesma. Um 500 ali faria quem clicou não saber se pagou duas
// vezes; o aviso manda recarregar e conferir.
func TestOSegundoCliqueNaoViraErroDeServidor(t *testing.T) {
	f := novoFakeRepasses()
	f.aPagar = []store.PagamentoNaFila{umaDividaAPagar()}
	f.erroPagarMao = errors.New("repasse 7: repasse inexistente")
	h := painelComRepasses(t, f)
	post, token := signedInPost(t, h)

	rec := post("/repasses/7/pagar", url.Values{"csrf": {token}, "nota": {"paguei"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if destino := rec.Header().Get("Location"); !strings.Contains(destino, "aviso=") {
		t.Errorf("a recusa nao avisou nada: %q", destino)
	}
}
