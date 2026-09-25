package grpcsrv

import (
	"testing"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// TestEntregaParaProtoMapeiaCadaEstado.
//
// A tradução é explícita, um case por valor, e não uma conversão de número. Os dois
// enums mudam por motivos diferentes: um deles ganhar um valor no meio deslocaria o
// outro em silêncio — e aqui o deslocamento mais provável seria dizer "entregue" para
// quem não recebeu.
func TestEntregaParaProtoMapeiaCadaEstado(t *testing.T) {
	casos := []struct {
		nome string
		de   store.EstadoEntrega
		quer webv1.DeliveryState
	}{
		{"nenhuma", store.EntregaNenhuma, webv1.DeliveryState_DELIVERY_STATE_UNSPECIFIED},
		{"na fila", store.EntregaNaFila, webv1.DeliveryState_DELIVERY_STATE_WAITING},
		{"presa", store.EntregaPresa, webv1.DeliveryState_DELIVERY_STATE_HELD},
		{"feita", store.EntregaFeita, webv1.DeliveryState_DELIVERY_STATE_DELIVERED},
		{"perdida", store.EntregaPerdida, webv1.DeliveryState_DELIVERY_STATE_LOST},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := entregaParaProto(c.de); got != c.quer {
				t.Errorf("entregaParaProto(%v) = %v, queria %v", c.de, got, c.quer)
			}
		})
	}
}

// TestEntregaDesconhecidaNaoViraEntregue: um valor que esta versão não conhece cai em
// UNSPECIFIED, para o qual a página tem frase neutra. Cair em DELIVERED diria à pessoa
// que o item chegou quando ninguém sabe onde ele está.
func TestEntregaDesconhecidaNaoViraEntregue(t *testing.T) {
	inventado := store.EstadoEntrega(99)
	if got := entregaParaProto(inventado); got != webv1.DeliveryState_DELIVERY_STATE_UNSPECIFIED {
		t.Errorf("estado desconhecido = %v, queria UNSPECIFIED", got)
	}
}
