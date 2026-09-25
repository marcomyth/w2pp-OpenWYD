package store

import "testing"

// O SEGURADO SÓ VALE PARA PENDENTE.
//
// Um baú cheio não muda nada sobre uma entrega que já aconteceu. Dizer "segurada" numa
// linha entregue seria afirmar que o jogador não recebeu o que ele tem no baú — e essa é
// a frase que faz a pessoa abrir ticket sobre um item que está na mão dela.
func TestSeguradoSoValeParaPendente(t *testing.T) {
	casos := []struct {
		status string
		cheio  bool
		quer   EstadoEntrega
	}{
		{"pending", false, EntregaEsperando},
		{"pending", true, EntregaSegurada},
		// As duas de baixo são o coração do teste: o baú cheio NÃO muda o que já
		// terminou.
		{"delivered", true, EntregaEntregue},
		{"lost", true, EntregaPerdida},
		{"delivered", false, EntregaEntregue},
		{"lost", false, EntregaPerdida},
	}
	for _, c := range casos {
		if got := EstadoDaEntrega(c.status, c.cheio); got != c.quer {
			t.Errorf("EstadoDaEntrega(%q, cheio=%v) = %q, queria %q",
				c.status, c.cheio, got, c.quer)
		}
	}
}

// STATUS DESCONHECIDO NÃO VIRA "A CAMINHO".
//
// Prometer entrega por causa de uma palavra que ninguém previu é o pior dos dois erros
// possíveis: o indefinido faz a tela calar, e calar não mente. Se amanhã alguém
// acrescentar um status ao vocabulário do banco sem passar por aqui, a página deixa de
// falar daquela linha em vez de garantir uma entrega que talvez não venha.
func TestStatusDesconhecidoNaoPrometeEntrega(t *testing.T) {
	for _, status := range []string{"", "held", "cancelado", "PENDING", "refunded"} {
		if got := EstadoDaEntrega(status, false); got != EntregaIndefinida {
			t.Errorf("status %q virou %q; queria indefinido", status, got)
		}
	}
}

// E O VOCABULÁRIO É O DO CONTRATO com o par do site, escrito à mão.
//
// Não é preciosismo: estes textos atravessam a fronteira entre dois repositórios, e do
// outro lado uma tela compara com a palavra literal. Renomear um valor aqui quebraria a
// página sem quebrar nenhum build.
func TestOVocabularioEhODoContrato(t *testing.T) {
	quer := map[EstadoEntrega]string{
		EntregaIndefinida: "",
		EntregaEsperando:  "WAITING",
		EntregaSegurada:   "HELD",
		EntregaEntregue:   "DELIVERED",
		EntregaPerdida:    "LOST",
	}
	for got, palavra := range quer {
		if string(got) != palavra {
			t.Errorf("valor = %q, o contrato diz %q", string(got), palavra)
		}
	}
}
