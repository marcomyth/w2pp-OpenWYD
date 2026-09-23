package grpcsrv

import (
	"testing"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// O item tem de atravessar a fronteira INTEIRO, nos dois sentidos.
//
// Este teste nasce de um campo que não atravessava: o serial. A migração 0033 o
// criou para que duas cópias do mesmo item fossem PROVA de duplicação e não
// suspeita — e ele existia no banco, no domínio e no fio, mas `itemToProto` e
// `protoToItems` não o copiavam. Resultado: o dbServer devolvia serial 0 em toda
// carga e gravava 0 em toda gravação. O recurso estava montado de ponta a ponta
// e mesmo assim não funcionava.
//
// O QUE DEIXOU PASSAR foi a forma dos testes que existiam: o do store
// (serial_integration_test.go) provava que o banco guardava, e o do gRPC provava
// que a chamada respondia — e nenhum dos dois olhava o campo do outro lado da
// fronteira. Um campo esquecido aqui não dá erro em lugar nenhum: ele chega
// zerado, e zero é valor legítimo em todos eles.
//
// Por isso este teste compara o item INTEIRO, e não campo a campo escolhido a
// dedo: assim o próximo campo que alguém acrescentar ao domínio e esquecer aqui
// quebra este teste no dia em que for acrescentado, e não meses depois quando
// alguém reparar que o dado não está lá.
func TestItemAtravessaAFronteiraInteiro(t *testing.T) {
	original := domain.Item{
		Slot:  7,
		Index: 1030,
		Eff1:  43, EffV1: 11,
		Eff2: 2, EffV2: 250,
		Eff3: 60, EffV3: 3,
		ExpiresAt:  1758499200,
		Serial:     987654321,
		AnuncioRMT: 4242,
	}

	voltou := protoToItems([]*dbv1.Item{itemToProto(original)})

	if len(voltou) != 1 {
		t.Fatalf("voltaram %d itens, esperado 1", len(voltou))
	}
	if voltou[0] != original {
		t.Errorf("o item perdeu algo na travessia:\n  foi:    %+v\n  voltou: %+v", original, voltou[0])
	}
}

// E o caso que o de cima não pega sozinho: um item VAZIO tem de voltar vazio, e
// não ganhar valor nenhum pelo caminho.
func TestItemVazioAtravessaVazio(t *testing.T) {
	var vazio domain.Item

	voltou := protoToItems([]*dbv1.Item{itemToProto(vazio)})

	if len(voltou) != 1 || voltou[0] != vazio {
		t.Errorf("o item vazio voltou diferente: %+v", voltou)
	}
}
