package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// A CARTEIRA INTERNA DE DINHEIRO REAL NÃO EXISTE, e a prova de que ela saiu do
// caminho é o Cash continuar sozinho no `case`.
//
// O caminho antigo transferia `account.rmt_balance` entre contas, uma carteira que
// ninguém nunca alimentou: medido em produção em 23/09/2026, as 16 contas têm saldo
// ZERO. A carteira do jogo é a das Rcoins (Cash). Prateleira em dinheiro real é
// dinheiro que sai de um jogador e vai para OUTRO, e isso se paga por Pix.
//
// O QUE ESTAVA AQUI ANTES E FOI REMOVIDO: um teste que montava uma prateleira em
// dinheiro real com o item SEM a marca do escrow, e conferia que a compra era
// recusada. Ele passava, e não provava nada — o jogo NUNCA produz esse estado,
// porque toda prateleira em dinheiro real tem o item marcado. A recusa que ele
// media era, na verdade, a trava do escrow pegando a compra legítima, e a mensagem
// que ele esperava era código morto.
//
// O caminho de verdade está em cobrancapix_test.go, com o item marcado.

// E CASH CONTINUA FUNCIONANDO, que é o que dá valor à recusa de cima.
//
// Cash é a carteira de verdade — `account.donate_balance`, que a recarga credita.
// Sem esta metade, um guard que recusasse toda moeda que não é ouro passaria no
// teste anterior.
func TestCompraEmCashContinuaFuncionando(t *testing.T) {
	const item = int16(1030)
	const preco = int32(300)
	var movido struct {
		moeda uint8
		valor int32
		vezes int
	}
	UsaSaldoDeConta(saldoDeMentira{func(_, _ int64, moeda uint8, valor int32) error {
		movido.moeda, movido.valor = moeda, valor
		movido.vezes++
		return nil
	}})
	defer UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error {
		return ErrSaldoNaoLigado
	}})

	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Loja", 0, preco, protocol.LojaMoedaOuro)
	send(t, vendedor, protocol.MsgLojaMoeda,
		(&protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}).Encode())
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus) // garante a ordem entre os sockets

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	o := lista.Ofertas[0]
	send(t, comprador, protocol.MsgLojaCompra,
		(&protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}).Encode())
	readUntil(t, comprador, protocol.MsgSendItem)

	if movido.vezes != 1 || movido.moeda != protocol.LojaMoedaCash || movido.valor != preco {
		t.Errorf("transferencia = %+v; queria uma de %d em cash", movido, preco)
	}
}
