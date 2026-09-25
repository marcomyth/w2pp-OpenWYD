package handler

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// As três recusas da barraca que ainda usavam o _NN_CantWhenAutoTrade do cliente.
//
// A frase dele é "Não é possível durante a auto venda.", e nos três casos o jogador
// NÃO está em auto venda: ele está tentando ABRIR uma. Dizer o contrário manda
// procurar um problema que não existe - e nos três o problema tinha conserto fácil,
// se alguém tivesse dito qual era.
func TestAsFrasesDaBarracaNaoFalamDeAutoVenda(t *testing.T) {
	for _, frase := range []string{msgBarracaSoPeloPainel, msgItemNaoVaiParaBarraca, msgBarracaSemItem} {
		if frase == "" {
			t.Fatal("frase vazia")
		}
		if strings.Contains(strings.ToLower(frase), "auto venda") {
			t.Errorf("a frase ainda fala em auto venda: %q", frase)
		}
	}
}

// O painel do cliente desenha texto de UM BYTE (Windows-1252) e corta em silêncio o
// que passa de 94. Uma string escrita em Go é UTF-8, então cada acento são dois
// bytes na fonte e tem de virar um na rede.
func TestAsFrasesDaBarracaCabemECodificamEmCp1252(t *testing.T) {
	casos := []struct {
		nome  string
		frase string
		// acento é um caractere que tem de virar UM byte; 0 diz que a frase não tem.
		acento byte
	}{
		{"só pelo painel", msgBarracaSoPeloPainel, 0xE3},  // "ã" de "não"
		{"item proibido", msgItemNaoVaiParaBarraca, 0xE3}, // "ã" de "não"
		{"barraca sem item", msgBarracaSemItem, 0x00},     // sem acento nenhum
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			b := protocol.ClientText(c.frase)
			if len(b) != len([]rune(c.frase)) {
				t.Errorf("%d bytes para %d letras: alguma coisa virou dois bytes",
					len(b), len([]rune(c.frase)))
			}
			if c.acento != 0 && bytes.IndexByte(b, c.acento) < 0 {
				t.Errorf("o acento %#x não apareceu na frase codificada", c.acento)
			}
			if bytes.IndexByte(b, '?') >= 0 {
				t.Errorf("o ClientText trocou algum caractere por '?': %q", c.frase)
			}
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

// TestAJanelaAntigaDeBarracaDizParaOndeIr.
//
// Quem cai nesta recusa tem um cliente velho: o GamePatch novo toma o clique do
// botão antes de ele virar pacote. Ele não está em auto venda, está tentando montar
// uma pela janela que se aposentou em 18/09/2026 - e sem dizer para onde ir, ele
// fica clicando no mesmo botão.
func TestAJanelaAntigaDeBarracaDizParaOndeIr(t *testing.T) {
	addr, stop, _ := startServerClock(t, autotradeDB(1030))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	drena(t, vendedor)
	send(t, vendedor, protocol.MsgSendAutoTrade, nil)

	linha, caixa := "", false
	for {
		ty, payload, ok := readMaybe(t, vendedor)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgMessagePanel:
			if linha == "" {
				linha = decodePanel(payload)
			}
		case protocol.MsgMessageBoxOk:
			caixa = true
		}
	}
	if linha != msgBarracaSoPeloPainel {
		t.Fatalf("a recusa foi %q, queria %q", linha, msgBarracaSoPeloPainel)
	}
	// A caixa do cliente é desenhada a partir da TABELA DELE: enquanto ela chegar, a
	// frase mentirosa continua na tela, junto com a nossa.
	if caixa {
		t.Error("a caixa do cliente ainda chegou: a frase velha continua aparecendo")
	}
}

// TestBarracaComItemProibidoDizQueEOItem: sem dizer que o problema é o item, o
// vendedor tenta de novo com a mesma prateleira.
func TestBarracaComItemProibidoDizQueEOItem(t *testing.T) {
	const proibido = int16(508) // autoTradeBlacklist
	addr, stop, _ := startServerClock(t, autotradeDB(proibido))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	drena(t, vendedor)
	mandaAbrirBarraca(t, vendedor, "Loja", 0, 1000, protocol.LojaMoedaOuro)

	linha, subiu, caixa := "", false, false
	for {
		ty, payload, ok := readMaybe(t, vendedor)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgMessagePanel:
			if linha == "" {
				linha = decodePanel(payload)
			}
		case protocol.MsgLojaAbriu:
			subiu = true
		case protocol.MsgMessageBoxOk:
			caixa = true
		}
	}
	if subiu {
		t.Fatal("a barraca subiu com um item da lista de proibidos")
	}
	if linha != msgItemNaoVaiParaBarraca {
		t.Fatalf("a recusa foi %q, queria %q", linha, msgItemNaoVaiParaBarraca)
	}
	if caixa {
		t.Error("a caixa do cliente ainda chegou: a frase velha continua aparecendo")
	}
}

// TestBarracaSemNadaDentroDizOQueFalta: alcançável de verdade - o painel deixa tirar
// a última prateleira antes de mandar montar, e o pedido chega com todos os CargoPos
// em -1.
func TestBarracaSemNadaDentroDizOQueFalta(t *testing.T) {
	addr, stop, _ := startServerClock(t, autotradeDB(1030))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	corpo := protocol.LojaAbrirBody{Titulo: "Loja"}
	for i := range corpo.Slots {
		corpo.Slots[i].CargoPos = -1
	}
	drena(t, vendedor)
	send(t, vendedor, protocol.MsgLojaAbrir, corpo.Encode())

	linha, subiu := "", false
	for {
		ty, payload, ok := readMaybe(t, vendedor)
		if !ok {
			break
		}
		switch ty {
		case protocol.MsgMessagePanel:
			if linha == "" {
				linha = decodePanel(payload)
			}
		case protocol.MsgLojaAbriu:
			subiu = true
		}
	}
	if subiu {
		t.Fatal("subiu uma barraca sem nada para vender")
	}
	if linha != msgBarracaSemItem {
		t.Fatalf("a recusa foi %q, queria %q", linha, msgBarracaSemItem)
	}
}
