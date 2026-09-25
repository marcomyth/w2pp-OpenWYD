package handler

import (
	"bytes"
	"encoding/binary"
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

// TestOfertaQueSumiuFalaComQuemClicou.
//
// A vitrine é uma fotografia: entre ela e o clique, outra pessoa compra ou o vendedor
// recolhe o item. Antes disto o servidor não devolvia NADA — o botão não fazia coisa
// nenhuma —, e quem estava do outro lado clicava de novo achando que o clique tinha
// falhado.
func TestOfertaQueSumiuFalaComQuemClicou(t *testing.T) {
	const item = int16(1030)
	const preco = int32(300)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, preco, protocol.LojaMoedaOuro)
	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	o := lista.Ofertas[0]

	// A barraca desce depois de a vitrine ter sido tirada: é a corrida real. O
	// /fecharloja é o caminho que o dono tem para derrubá-la.
	whisperFrame(t, vendedor, "fecharloja", "")
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus) // garante a ordem entre os sockets

	drena(t, comprador)
	compra := protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	avisado, levou := false, false
	for {
		ty, payload, ok := readMaybe(t, comprador)
		if !ok {
			break
		}
		switch {
		case ty == protocol.MsgSendItem:
			levou = true
		case ty == protocol.MsgMessageBoxOk && len(payload) >= 4 &&
			Notice(binary.LittleEndian.Uint32(payload[0:4])) == NoticeItemSold:
			avisado = true
		}
	}
	if levou {
		t.Fatal("comprou de uma barraca que já tinha descido")
	}
	if !avisado {
		t.Fatal("a oferta sumiu e o jogador não foi avisado: o botão não fez nada")
	}
}

// TestAsFrasesNovasCabemECodificamEmCp1252.
//
// O cliente desenha texto de UM BYTE (Windows-1252), e uma string escrita em Go é
// UTF-8: o "ç" que o autor digita são DOIS bytes e chegam como lixo. O caminho já
// passa pelo ClientText, e este teste é o que garante que continue passando — e que
// a frase caiba, porque o que passa do limite é CORTADO em silêncio, no meio.
func TestAsFrasesNovasCabemECodificamEmCp1252(t *testing.T) {
	casos := []struct {
		nome  string
		frase string
		// acento é um caractere que tem de virar UM byte, escolhido dentro da frase.
		acento byte
	}{
		{"sem Rcoins", msgSemRcoins, 0xE3},         // "ã" de "não"
		{"preço mudou", msgPrecoMudou, 0xE7},       // "ç" de "preço"
		{"item reservado", msgItemReservado, 0xE1}, // "á" de "está"
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			b := protocol.ClientText(c.frase)
			if len(b) != len([]rune(c.frase)) {
				t.Errorf("%d bytes para %d letras: alguma coisa virou dois bytes",
					len(b), len([]rune(c.frase)))
			}
			if bytes.IndexByte(b, c.acento) < 0 {
				t.Errorf("o acento %#x não apareceu na frase codificada", c.acento)
			}
			if bytes.IndexByte(b, '?') >= 0 {
				t.Errorf("o ClientText trocou algum caractere por '?': %q", c.frase)
			}
			// O corte é silencioso, então a frase inteira tem de caber.
			corpo := protocol.EncodeMessagePanelBody(c.frase)
			fim := bytes.IndexByte(corpo, 0)
			if fim < 0 {
				fim = len(corpo)
			}
			if fim != len(b) {
				t.Errorf("a frase foi cortada: %d bytes de %d", fim, len(b))
			}
		})
	}
}

// TestOPrecoMudouNaoFalaDeAutoVenda: o texto velho dizia "Não é possível durante a
// auto venda", e o comprador não está em auto venda nenhuma. Mandar procurar um
// problema que não existe é pior que ficar calado.
func TestOPrecoMudouNaoFalaDeAutoVenda(t *testing.T) {
	for _, frase := range []string{msgPrecoMudou, msgItemReservado} {
		if strings.Contains(strings.ToLower(frase), "auto venda") {
			t.Errorf("a frase ainda fala em auto venda: %q", frase)
		}
	}
}
