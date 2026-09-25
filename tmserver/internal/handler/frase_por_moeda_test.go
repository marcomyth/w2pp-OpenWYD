package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// TestSemRcoinsAFraseFalaDeRcoins.
//
// A recusa da compra em Rcoins usava o _NN_Not_Enough_Money do cliente, que é
// "Não possui gold suficiente." Quem tentava comprar em Rcoins lia isso com a bolsa
// cheia de ouro: a frase mandava conferir a moeda errada, e a pessoa ia atrás de
// ouro que ela já tinha.
func TestSemRcoinsAFraseFalaDeRcoins(t *testing.T) {
	const item = int16(1030)
	const preco = int32(300)
	UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error {
		return errors.New("saldo insuficiente")
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

	abreBarraca(t, vendedor, "Minha Loja", 0, preco, protocol.LojaMoedaOuro)
	moeda := protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}
	send(t, vendedor, protocol.MsgLojaMoeda, moeda.Encode())
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	o := lista.Ofertas[0]
	drena(t, comprador)
	compra := protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	linha, levouItem := "", false
	for {
		ty, payload, ok := readMaybe(t, comprador)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgMessagePanel:
			if linha == "" {
				linha = decodePanel(payload)
			}
		case protocol.MsgSendItem:
			levouItem = true
		}
	}
	if levouItem {
		t.Fatal("a compra passou sem saldo")
	}
	if linha != msgSemRcoins {
		t.Fatalf("a recusa foi %q, queria %q", linha, msgSemRcoins)
	}
}

// TestAFraseDeRcoinsNaoFalaDeGold: é o ponto inteiro. A frase tem de nomear a moeda
// que faltou, e nomear a outra é pior que não dizer nada — manda a pessoa procurar
// no lugar errado.
func TestAFraseDeRcoinsNaoFalaDeGold(t *testing.T) {
	if msgSemRcoins == "" {
		t.Fatal("a frase está vazia")
	}
	for _, proibida := range []string{"gold", "Gold", "ouro", "Ouro"} {
		if strings.Contains(msgSemRcoins, proibida) {
			t.Errorf("a frase de Rcoins fala em %q: %q", proibida, msgSemRcoins)
		}
	}
	if !strings.Contains(msgSemRcoins, "Rcoins") {
		t.Errorf("a frase não nomeia a moeda: %q", msgSemRcoins)
	}
}
