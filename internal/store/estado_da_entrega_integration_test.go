//go:build integration

// Testes de integração do estado da entrega na cobrança do comprador.
//
//	W2PP_TEST_DSN=postgres://postgres:dev@localhost:5432/postgres go test -tags=integration ./internal/store/
package store

import (
	"context"
	"testing"
)

// entregaNaCobranca pendura uma linha da caixa postal na cobrança, com o status
// pedido, como o pagamento faz.
func entregaNaCobranca(ctx context.Context, t *testing.T, s *Store, ref string, conta int64, status string) {
	t.Helper()
	var entregaID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO delivery_queue (account_id, kind, payload, status, source)
		VALUES ($1, 'item', '{"index":1030}'::jsonb, $2, 'teste')
		RETURNING id`, conta, status).Scan(&entregaID); err != nil {
		t.Fatalf("criando a entrega: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET entrega_id = $2 WHERE referencia_externa = $1`, ref, entregaID); err != nil {
		t.Fatalf("ligando a entrega à cobrança: %v", err)
	}
}

// encheOBau põe itens no baú da conta até o número pedido, para o teste poder dizer
// "sem espaço livre" sem inventar estado.
func encheOBau(ctx context.Context, t *testing.T, s *Store, conta int64, quantos int) {
	t.Helper()
	for i := 0; i < quantos; i++ {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO item (owner_kind, account_id, slot, item_index)
			VALUES ('account_cargo', $1, $2, 1030)`, conta, i); err != nil {
			t.Fatalf("enchendo o baú no slot %d: %v", i, err)
		}
	}
}

// cobrancaPagaComEntrega monta uma compra paga desta conta e devolve a referência.
func cobrancaPagaComEntrega(ctx context.Context, t *testing.T, s *Store, sufixo string) (vendedor, comprador int64, ref string) {
	t.Helper()
	vendedor = contaPix(ctx, t, s, "entrega_vend_"+sufixo)
	comprador = contaPix(ctx, t, s, "entrega_comp_"+sufixo)
	if err := s.SalvarChavePix(ctx, vendedor, "11111111111", ChavePixCPF, "11144477735"); err != nil {
		t.Fatal(err)
	}
	personagemDe(ctx, t, s, vendedor, 0, "Mercador"+sufixo)
	anuncio := anuncioComFoto(ctx, t, s, vendedor, "Mercador"+sufixo, 0, 9, 1)
	itemMarcado(ctx, t, s, vendedor, 0, anuncio)
	ref = "ref-entrega-" + sufixo
	if _, _, err := s.AbrirCobrancaRMT(ctx, anuncio, comprador, ref, 0); err != nil {
		t.Fatal(err)
	}
	return vendedor, comprador, ref
}

// TestEstadoDaEntregaSeparaACaminhoDePresa é o defeito que este campo conserta.
//
// A página dizia "o item está a caminho" para DUAS situações diferentes: a entrega
// que acontece no próximo login, e a que não acontece nenhuma vez até a pessoa
// esvaziar o baú. Quem estava na segunda esperava um dia que não chega.
func TestEstadoDaEntregaSeparaACaminhoDePresa(t *testing.T) {
	s, ctx := freshStore(t)

	t.Run("sem linha de entrega e NENHUMA", func(t *testing.T) {
		_, comprador, _ := cobrancaPagaComEntrega(ctx, t, s, "sem")
		tem, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil || !tem {
			t.Fatalf("tem=%v err=%v", tem, err)
		}
		if cob.Entrega != EntregaNenhuma {
			t.Errorf("entrega = %v, queria EntregaNenhuma — a cobrança nem foi paga", cob.Entrega)
		}
	})

	t.Run("na fila com espaco livre", func(t *testing.T) {
		_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "fila")
		entregaNaCobranca(ctx, t, s, ref, comprador, "pending")
		_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cob.Entrega != EntregaNaFila {
			t.Errorf("entrega = %v, queria EntregaNaFila", cob.Entrega)
		}
	})

	t.Run("presa quando o bau esta sem espaco", func(t *testing.T) {
		_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "presa")
		entregaNaCobranca(ctx, t, s, ref, comprador, "pending")
		encheOBau(ctx, t, s, comprador, maxCargoDoBau)
		_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cob.Entrega != EntregaPresa {
			t.Errorf("entrega = %v, queria EntregaPresa", cob.Entrega)
		}
	})

	t.Run("um espaco livre ja e na fila", func(t *testing.T) {
		// A FRONTEIRA É O PONTO: com 127 itens ainda há espaço, e dizer "presa" ali
		// mandaria a pessoa esvaziar um baú que tem lugar.
		_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "quase")
		entregaNaCobranca(ctx, t, s, ref, comprador, "pending")
		encheOBau(ctx, t, s, comprador, maxCargoDoBau-1)
		_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cob.Entrega != EntregaNaFila {
			t.Errorf("entrega = %v com %d itens, queria EntregaNaFila", cob.Entrega, maxCargoDoBau-1)
		}
	})

	t.Run("entregue", func(t *testing.T) {
		_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "feita")
		entregaNaCobranca(ctx, t, s, ref, comprador, "delivered")
		_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cob.Entrega != EntregaFeita {
			t.Errorf("entrega = %v, queria EntregaFeita", cob.Entrega)
		}
	})

	t.Run("perdida", func(t *testing.T) {
		_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "perdida")
		entregaNaCobranca(ctx, t, s, ref, comprador, "lost")
		_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cob.Entrega != EntregaPerdida {
			t.Errorf("entrega = %v, queria EntregaPerdida", cob.Entrega)
		}
	})

	t.Run("o bau de OUTRA conta nao conta", func(t *testing.T) {
		// O baú lido tem de ser o do COMPRADOR. Contar o de outra pessoa diria
		// "presa" para quem tem o baú vazio.
		_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "outro")
		entregaNaCobranca(ctx, t, s, ref, comprador, "pending")
		alheio := contaPix(ctx, t, s, "entrega_bau_alheio")
		encheOBau(ctx, t, s, alheio, maxCargoDoBau)
		_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
		if err != nil {
			t.Fatal(err)
		}
		if cob.Entrega != EntregaNaFila {
			t.Errorf("entrega = %v: contou o baú de outra conta", cob.Entrega)
		}
	})
}

// TestEstadoDaEntregaComStatusDesconhecido: um status que esta versão não conhece
// vira NENHUMA, e a página tem uma frase neutra para isso. Afirmar "entregue" por
// engano é o pior erro possível aqui.
func TestEstadoDaEntregaComStatusDesconhecido(t *testing.T) {
	s, ctx := freshStore(t)
	_, comprador, ref := cobrancaPagaComEntrega(ctx, t, s, "estranho")
	var entregaID int64
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO delivery_queue (account_id, kind, payload, status, source)
		VALUES ($1, 'item', '{"index":1030}'::jsonb, 'inventado', 'teste')
		RETURNING id`, comprador).Scan(&entregaID); err != nil {
		t.Fatalf("criando a entrega: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE rmt_cobranca SET entrega_id = $2 WHERE referencia_externa = $1`, ref, entregaID); err != nil {
		t.Fatal(err)
	}
	_, cob, err := s.CobrancaAtualDoComprador(ctx, comprador, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cob.Entrega != EntregaNenhuma {
		t.Errorf("entrega = %v com status desconhecido, queria EntregaNenhuma", cob.Entrega)
	}
}
