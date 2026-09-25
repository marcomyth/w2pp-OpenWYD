package panel

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// O AJUSTE CHEGA AO BANCO COM O VALOR EM CENTAVOS E A NOTA.
//
// O caso que originou a tela: a venda de R$ 1,00 abriu um repasse de R$ 1,00 cheio
// porque o desconto da taxa ainda não estava no ar, e a ordem foi reenviar o líquido
// de R$ 0,20. "0,20" digitado tem de virar 20, e não 0 nem 2.
func TestAjusteChegaEmCentavosComANota(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	f.antigoFake = 100
	h := painelComRepasses(t, f)

	res := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao":    {"ajustar-valor"},
		"novo_valor": {"0,20"},
		"nota":       {"ordem da Hanna, liquido medido, net_amount 0.2"},
	})

	if res.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d, queria 303; corpo=%s", res.Code, res.Body.String())
	}
	if len(f.ajustes) != 1 {
		t.Fatalf("ajustes = %+v, queria um", f.ajustes)
	}
	a := f.ajustes[0]
	if a.Novo != 20 {
		t.Errorf("valor = %d centavos, queria 20", a.Novo)
	}
	if a.Nota == "" {
		t.Error("a nota nao chegou ao banco")
	}
	if a.Ator == "" {
		t.Error("o ator nao chegou; um numero de dinheiro que muda sem dono nao se explica")
	}
}

// O AVISO DIZ QUE A LINHA CONTINUA RECUSADA.
//
// Quem ajusta um valor costuma achar que já mandou de novo. Aqui não mandou — quem
// devolve à fila é o outro botão —, e uma tela que não diz isso deixa a pessoa
// esperando um pagamento que ninguém pediu.
func TestOAvisoDizQueContinuaRecusado(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	res := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"ajustar-valor"}, "novo_valor": {"0,20"}, "nota": {"porque sim"},
	})

	destino := res.Header().Get("Location")
	if !strings.Contains(destino, "recusada") && !strings.Contains(destino, "recusado") {
		t.Errorf("aviso = %q; tinha de dizer que a linha continua recusada", destino)
	}
}

// SEM NOTA NÃO AJUSTA, e a tela nem chega a chamar o banco.
func TestSemNotaNaoAjusta(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	res := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"ajustar-valor"}, "novo_valor": {"0,20"}, "nota": {""},
	})

	if len(f.ajustes) != 0 {
		t.Errorf("ajustou sem nota: %+v", f.ajustes)
	}
	if res.Code != http.StatusSeeOther {
		t.Errorf("codigo = %d, queria 303 com aviso", res.Code)
	}
}

// VALOR ILEGÍVEL NÃO AJUSTA. "abc" não pode virar zero calado — zero seria um repasse
// que manda a ponte transferir nada.
func TestValorIlegivelNaoAjusta(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	h := painelComRepasses(t, f)

	postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"ajustar-valor"}, "novo_valor": {"abc"}, "nota": {"tentando"},
	})

	if len(f.ajustes) != 0 {
		t.Errorf("ajustou com valor ilegivel: %+v", f.ajustes)
	}
}

// A RECUSA DO BANCO VIRA AVISO, e não erro de servidor.
//
// Valor fora da faixa é culpa do que foi DIGITADO, e a pessoa conserta digitando de
// novo. Um 500 aqui a mandaria avisar quem cuida do servidor por um engano dela.
func TestValorForaDaFaixaViraAvisoENaoErroDeServidor(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	f.erroAjuste = store.ErrAjusteInvalido
	h := painelComRepasses(t, f)

	res := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"ajustar-valor"}, "novo_valor": {"999,00"}, "nota": {"acima da cobranca"},
	})

	if res.Code == http.StatusInternalServerError {
		t.Fatal("valor fora da faixa virou erro de servidor")
	}
	if res.Code != http.StatusSeeOther {
		t.Errorf("codigo = %d, queria 303 com aviso", res.Code)
	}
}

// E a linha que mudou de estado enquanto a tela estava aberta continua sendo aviso.
func TestRepasseQueMudouDeEstadoViraAviso(t *testing.T) {
	f := novoFakeRepasses(umaFilaDeRepasse()...)
	f.erroAjuste = store.ErrRepasseInexistente
	h := painelComRepasses(t, f)

	res := postSigned(t, h, "/repasses/1/resolver", url.Values{
		"decisao": {"ajustar-valor"}, "novo_valor": {"0,20"}, "nota": {"ok"},
	})

	if res.Code != http.StatusSeeOther || !errors.Is(f.erroAjuste, store.ErrRepasseInexistente) {
		t.Errorf("codigo = %d, queria 303", res.Code)
	}
}
