package grpcsrv

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// TestApagarChavePixTraduzCadaRecusa.
//
// É um teste-tabela pelo mesmo motivo do irmão no SavePixKey: dividir um erro em
// dois no store obriga a percorrer TODOS os lugares que traduziam o antigo, e o que
// fica de fora não quebra o build — vira codes.Internal em produção, num caminho que
// funcionava. A tabela é o que faz o próximo erro novo aparecer aqui.
func TestApagarChavePixTraduzCadaRecusa(t *testing.T) {
	casos := []struct {
		nome string
		erro error
		quer webv1.PixKeyDeleteResult
	}{
		{"apagou", nil, webv1.PixKeyDeleteResult_PIX_KEY_DELETE_RESULT_OK},
		{"nao havia chave", store.ErrNotFound, webv1.PixKeyDeleteResult_PIX_KEY_DELETE_RESULT_NO_KEY},
		{"venda em curso", store.ErrVendaEmCurso, webv1.PixKeyDeleteResult_PIX_KEY_DELETE_RESULT_SALE_IN_PROGRESS},
		{"repasse a caminho", store.ErrRepasseEmCurso, webv1.PixKeyDeleteResult_PIX_KEY_DELETE_RESULT_PAYOUT_PENDING},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f := &fakePix{erroApagar: c.erro}
			s := NewRmt(f)
			resp, err := s.DeletePixKey(context.Background(), &webv1.DeletePixKeyRequest{AccountId: 42})
			if err != nil {
				t.Fatalf("erro de gRPC inesperado: %v", err)
			}
			if resp.GetResult() != c.quer {
				t.Errorf("result = %v, queria %v", resp.GetResult(), c.quer)
			}
			if f.apagouConta != 42 {
				t.Errorf("o store recebeu a conta %d", f.apagouConta)
			}
		})
	}
}

// TestApagarChavePixComErroNovoViraInternal: um erro que este handler não conhece
// NÃO pode virar "não havia chave". A tela confirmaria uma ação que não aconteceu, e
// a pessoa acharia que o cadastro dela saiu.
func TestApagarChavePixComErroNovoViraInternal(t *testing.T) {
	s := NewRmt(&fakePix{erroApagar: errors.New("o banco caiu")})
	resp, err := s.DeletePixKey(context.Background(), &webv1.DeletePixKeyRequest{AccountId: 42})
	if status.Code(err) != codes.Internal {
		t.Fatalf("código = %v, queria Internal (resp = %v)", status.Code(err), resp)
	}
}

// TestApagarChavePixNaoDizOKQuandoNaoHaviaNada é o par do "separado de propósito"
// escrito no contrato: OK e NO_KEY são coisas diferentes para quem está olhando a
// tela.
func TestApagarChavePixNaoDizOKQuandoNaoHaviaNada(t *testing.T) {
	s := NewRmt(&fakePix{erroApagar: store.ErrNotFound})
	resp, err := s.DeletePixKey(context.Background(), &webv1.DeletePixKeyRequest{AccountId: 42})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetResult() == webv1.PixKeyDeleteResult_PIX_KEY_DELETE_RESULT_OK {
		t.Error("disse OK sem ter apagado nada")
	}
}

// TestApagarChavePixUsaAContaDaSessao: o account_id vem da sessão, e o handler não
// pode ir buscá-lo em outro lugar — um pedido que escolhe de quem é a chave é um
// pedido que apaga a chave dos outros.
func TestApagarChavePixUsaAContaDaSessao(t *testing.T) {
	f := &fakePix{}
	s := NewRmt(f)
	if _, err := s.DeletePixKey(context.Background(), &webv1.DeletePixKeyRequest{AccountId: 777}); err != nil {
		t.Fatal(err)
	}
	if f.apagouConta != 777 {
		t.Errorf("apagou a conta %d, queria 777", f.apagouConta)
	}
	if f.apagouVezes != 1 {
		t.Errorf("chamadas ao store = %d, queria 1", f.apagouVezes)
	}
}
