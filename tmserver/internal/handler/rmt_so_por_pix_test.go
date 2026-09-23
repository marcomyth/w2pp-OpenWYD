package handler

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// NÃO EXISTE CARTEIRA DE DINHEIRO REAL, e a compra em dinheiro real se recusa por
// isso — não por falta de saldo.
//
// O caminho antigo transferia `account.rmt_balance` entre contas, uma carteira
// que ninguém nunca alimentou: medido em produção em 23/09/2026, as 16 contas têm
// saldo ZERO. A carteira do jogo é a das Rcoins (Cash). Prateleira em dinheiro
// real é dinheiro que sai de um jogador e vai para OUTRO, e isso se paga por Pix,
// entre as duas pessoas.
//
// O teste prova as três metades de uma recusa que se pode confiar: o jogador
// SABE por quê, NADA de saldo se move, e o item NÃO sai do baú. Uma recusa que
// falha em qualquer das três é pior do que não recusar.
func TestCompraEmDinheiroRealERecusadaENadaSeMove(t *testing.T) {
	const item = int16(1030)
	const preco = int32(5000)
	var transferencias int
	UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error {
		transferencias++
		return nil // diz SIM de propósito: a recusa não pode depender do saldo
	}})
	defer UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error {
		return ErrSaldoNaoLigado
	}})

	db := autotradeDB(item)
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	// A prateleira nasce em ouro e vira dinheiro real pelo caminho de montagem —
	// a barraca em RMT criaria anúncio, e o que este teste mede é a COMPRA.
	barraca := abreBarraca(t, vendedor, "Loja", 0, preco, protocol.LojaMoedaOuro)
	noLacoDoMundo(t, w, func(w *world.World) {
		w.ForEachSession(func(s *world.Session, _ *world.Entity) {
			if s != nil && s.AutoTrade != nil && s.AccountID == 7 {
				s.AutoTrade.Moeda[0] = protocol.LojaMoedaRMT
			}
		})
	})
	drena(t, comprador)

	compra := protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaRMT}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	if !recebeu(t, comprador, msgRMTSoPorPix) {
		t.Error("recusou em silencio: o comprador clica e nada acontece, sem saber por que")
	}
	if transferencias != 0 {
		t.Errorf("mexeu no saldo %d vez(es) numa compra que devia ser recusada", transferencias)
	}
	noLacoDoMundo(t, w, func(w *world.World) {
		if c := w.Cargo(7); c == nil || c.Items[0].Empty() {
			t.Error("o item saiu do bau do vendedor sem ninguem ter pagado nada")
		}
	})
}

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
