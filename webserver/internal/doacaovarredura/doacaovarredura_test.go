package doacaovarredura

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

const (
	refUm    = "aaaaaaaabbbbbbbbccccccccdddddddd"
	refOutro = "11111111222222223333333344444444"
)

type fakePonte struct {
	resp map[string]Transacao
	erro map[string]error
}

func (f *fakePonte) ConsultarTransacao(_ context.Context, id string) (Transacao, error) {
	if err := f.erro[id]; err != nil {
		return Transacao{}, err
	}
	return f.resp[id], nil
}

type fakeCredito struct {
	creditados []string
	erro       error
}

func (f *fakeCredito) ConfirmTopupOrder(_ context.Context, ref string) error {
	if f.erro != nil {
		return f.erro
	}
	f.creditados = append(f.creditados, ref)
	return nil
}

type fakeBanco struct {
	pendentes     []store.TopupParaConferir
	semID         int
	janelaPedida  time.Duration
	erroPendentes error
}

func (b *fakeBanco) TopupsParaConferir(_ context.Context, janela time.Duration, _ int,
) ([]store.TopupParaConferir, error) {
	b.janelaPedida = janela
	return b.pendentes, b.erroPendentes
}

func (b *fakeBanco) TopupsSemIdentifier(context.Context, time.Duration) (int, error) {
	return b.semID, nil
}

func mudo() *slog.Logger { return slog.New(slog.DiscardHandler) }

func pedido(ref, id string, centavos int64) store.TopupParaConferir {
	return store.TopupParaConferir{
		ExternalReference: ref, Identifier: id, CentavosPedidos: centavos,
		CriadoEm: time.Now().Add(-time.Minute),
	}
}

func descricaoDe(ref string) string { return "Apoiador Prata - WYD Retry | ref:" + ref }

// O CAMINHO QUE O PACOTE EXISTE PARA COBRIR: o aviso não chegou, e a varredura
// credita.
func TestPagoSemAvisoEhCreditado(t *testing.T) {
	b := &fakeBanco{pendentes: []store.TopupParaConferir{pedido(refUm, "tx-1", 2990)}}
	p := &fakePonte{resp: map[string]Transacao{"tx-1": {
		Existe: true, Status: "completed", ValorCentavos: 2990, Descricao: descricaoDe(refUm),
	}}}
	c := &fakeCredito{}

	r := Nova(p, c, b, mudo()).Conferir(context.Background(), time.Hour)

	if len(c.creditados) != 1 || c.creditados[0] != refUm {
		t.Fatalf("creditados = %v", c.creditados)
	}
	if r.Creditados != 1 || r.Recusados != 0 {
		t.Errorf("rodada = %+v", r)
	}
}

// O IDENTIFIER DE OUTRO PAGAMENTO NÃO CREDITA. É a trava que protege dinheiro.
//
// O id vem de fora: quem chama o Attach é o site. Um bug dele, ou uma chamada
// forjada, poderia prender a um pedido pendente o id de OUTRA transação já paga, do
// mesmo valor. Sem conferir a descrição, este pedido seria creditado com o dinheiro
// de outra compra — uma doação, dois créditos.
func TestIdentifierDeOutroPagamentoNaoCredita(t *testing.T) {
	b := &fakeBanco{pendentes: []store.TopupParaConferir{pedido(refUm, "tx-roubado", 2990)}}
	p := &fakePonte{resp: map[string]Transacao{"tx-roubado": {
		Existe: true, Status: "completed", ValorCentavos: 2990,
		// Mesmo valor, mesma cara — e a descrição é de OUTRO pedido.
		Descricao: descricaoDe(refOutro),
	}}}
	c := &fakeCredito{}

	r := Nova(p, c, b, mudo()).Conferir(context.Background(), time.Hour)

	if len(c.creditados) != 0 {
		t.Fatalf("creditou com o pagamento de outro pedido: %v", c.creditados)
	}
	if r.Recusados != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// A DESCRIÇÃO VAZIA NÃO CREDITA NINGUÉM.
//
// A ponte em produção ainda não manda esse campo: até o próximo git pull da VPS,
// TODA resposta chega sem ele. Vazio quer dizer "não sei", e tratá-lo como "pode"
// desligaria a conferência de cima justamente enquanto ninguém consegue vê-la
// funcionando.
func TestDescricaoVaziaNaoCredita(t *testing.T) {
	for _, texto := range []string{"", "   "} {
		b := &fakeBanco{pendentes: []store.TopupParaConferir{pedido(refUm, "tx-1", 2990)}}
		p := &fakePonte{resp: map[string]Transacao{"tx-1": {
			Existe: true, Status: "completed", ValorCentavos: 2990, Descricao: texto,
		}}}
		c := &fakeCredito{}

		Nova(p, c, b, mudo()).Conferir(context.Background(), time.Hour)

		if len(c.creditados) != 0 {
			t.Errorf("creditou com a descricao %q", texto)
		}
	}
}

// O VALOR DIFERENTE NÃO CREDITA, mesmo com a referência certa.
func TestValorDiferenteNaoCredita(t *testing.T) {
	b := &fakeBanco{pendentes: []store.TopupParaConferir{pedido(refUm, "tx-1", 2990)}}
	p := &fakePonte{resp: map[string]Transacao{"tx-1": {
		Existe: true, Status: "completed", ValorCentavos: 100, Descricao: descricaoDe(refUm),
	}}}
	c := &fakeCredito{}

	r := Nova(p, c, b, mudo()).Conferir(context.Background(), time.Hour)

	if len(c.creditados) != 0 {
		t.Fatalf("creditou R$ 29,90 por um pagamento de R$ 1,00: %v", c.creditados)
	}
	if r.Recusados != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// SÓ "COMPLETED" CREDITA. Um estorno ou uma contestação também têm data de
// pagamento, e nenhum dos dois é dinheiro que ficou.
func TestSoOCompletedCredita(t *testing.T) {
	for _, st := range []string{"pending", "refunded", "refunding", "med", "failed", "refused", ""} {
		b := &fakeBanco{pendentes: []store.TopupParaConferir{pedido(refUm, "tx-1", 2990)}}
		p := &fakePonte{resp: map[string]Transacao{"tx-1": {
			Existe: true, Status: st, ValorCentavos: 2990, Descricao: descricaoDe(refUm),
		}}}
		c := &fakeCredito{}

		r := Nova(p, c, b, mudo()).Conferir(context.Background(), time.Hour)

		if len(c.creditados) != 0 {
			t.Errorf("status %q creditou", st)
		}
		// E não conta como recusa: não passar de pendente é o estado normal da
		// maioria dos pedidos na maior parte do tempo, e um contador que sobe com
		// isso vira ruído que esconde a recusa de verdade.
		if r.Recusados != 0 {
			t.Errorf("status %q virou recusa; e so um pedido ainda nao pago", st)
		}
	}
}

// A CONSULTA QUE FALHA NÃO CREDITA E NÃO PARA A RODADA.
func TestConsultaQueFalhaNaoParaAsOutras(t *testing.T) {
	b := &fakeBanco{pendentes: []store.TopupParaConferir{
		pedido(refUm, "tx-ruim", 2990),
		pedido(refOutro, "tx-bom", 500),
	}}
	p := &fakePonte{
		erro: map[string]error{"tx-ruim": errors.New("a processadora caiu")},
		resp: map[string]Transacao{"tx-bom": {
			Existe: true, Status: "completed", ValorCentavos: 500, Descricao: descricaoDe(refOutro),
		}},
	}
	c := &fakeCredito{}

	r := Nova(p, c, b, mudo()).Conferir(context.Background(), time.Hour)

	if len(c.creditados) != 1 || c.creditados[0] != refOutro {
		t.Fatalf("creditados = %v", c.creditados)
	}
	if r.Falhas != 1 || r.Creditados != 1 {
		t.Errorf("rodada = %+v", r)
	}
}

// A JANELA PEDIDA É A QUE O CHAMADOR MANDOU — é o que separa a passada rápida dos
// pedidos novos da passada rara que cobre as 48 h.
func TestAJanelaChegaNoBanco(t *testing.T) {
	b := &fakeBanco{}
	Nova(&fakePonte{}, &fakeCredito{}, b, mudo()).Conferir(context.Background(), 10*time.Minute)
	if b.janelaPedida != 10*time.Minute {
		t.Errorf("janela = %v", b.janelaPedida)
	}
}

// A LEITURA DA REFERÊNCIA NO TEXTO, que é a mesma regra do site: a ÚLTIMA sequência
// de 32 hex minúsculos.
//
// A última e não a primeira porque o nome do pacote vem antes e não é controlado por
// nós — um pacote que um dia se chame algo com 32 hex no meio roubaria a leitura.
func TestReferenciaNaDescricao(t *testing.T) {
	casos := map[string]string{
		descricaoDe(refUm):                     refUm,
		"Apoiador Ouro | ref:" + refOutro:      refOutro,
		refUm + " no comeco | ref:" + refOutro: refOutro,
		"sem referencia nenhuma":               "",
		"":                                     "",
		"curta demais: aaaaaaaabbbbbbbbcccccccdddddd": "",
		// Maiúsculas não valem: a referência é minúscula, e aceitar as duas formas
		// faria duas referências diferentes parecerem a mesma.
		"ref:AAAAAAAABBBBBBBBCCCCCCCCDDDDDDDD": "",
	}
	for texto, quer := range casos {
		if got := referenciaNaDescricao(texto); got != quer {
			t.Errorf("referenciaNaDescricao(%q) = %q, quero %q", texto, got, quer)
		}
	}
}

// O CONTADOR DOS SEM IDENTIFIER APARECE NA RODADA: é o termômetro de o site ter
// parado de entregar o id, e sem ele a rede embaixo sumiria sem ninguém notar.
func TestSemIdentifierEntraNaRodada(t *testing.T) {
	b := &fakeBanco{semID: 3}
	r := Nova(&fakePonte{}, &fakeCredito{}, b, mudo()).Conferir(context.Background(), time.Hour)
	if r.SemIdentifier != 3 {
		t.Errorf("sem_identifier = %d", r.SemIdentifier)
	}
}
