package handler

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// O painel do cliente CORTA em 94 bytes, calado. Uma frase que passa disso chega pela
// metade na tela e ninguém do lado do servidor fica sabendo.
//
// A conta é em BYTES e em Windows-1252, não em letras: cada acento custa um byte, e é
// justamente nas frases em português que a diferença aparece.
const maxBytesNoPainel = 94

// TestAsFrasesDoRMTCabemNaTela.
//
// Estas quatro linhas são as regras que custam dinheiro se não forem lidas: quando o
// dinheiro chega, em que horário sai, quanto a casa fica e quais preços são aceitos.
// Cortada pela metade, "Preço mínimo: R$ 5,00. Preço máximo: R$ 5" é pior que nenhuma.
func TestAsFrasesDoRMTCabemNaTela(t *testing.T) {
	for _, linha := range regrasDaVendaRMT {
		if n := len(protocol.ClientText(linha)); n > maxBytesNoPainel {
			t.Errorf("%d bytes (o painel corta em %d): %q", n, maxBytesNoPainel, linha)
		}
	}
	if n := len(protocol.ClientText(msgEstornoSemMotivo)); n > maxBytesNoPainel {
		t.Errorf("a frase do estorno tem %d bytes", n)
	}
	if n := len(protocol.ClientText(msgPrecoMaximoRMT)); n > maxBytesNoPainel {
		t.Errorf("a frase do teto tem %d bytes", n)
	}
}

// TestAsFrasesDoRMTMantemOsAcentos.
//
// O caminho contrário foi defeito de verdade neste mesmo dia, no catálogo: nome chegando
// quebrado por falta de decodificação. Aqui o risco é alguém "consertar" a frase tirando
// os acentos para caber, e o que este teste diz é que não precisa — elas já cabem.
//
// Um '?' no lugar de um acento é o sinal de que o texto não passou pelo ClientText: é
// assim que o cp1252 falha quando a letra não existe na tabela.
func TestAsFrasesDoRMTMantemOsAcentos(t *testing.T) {
	todas := append([]string{msgEstornoSemMotivo, msgPrecoMaximoRMT, msgPrecoMinimoRMT},
		regrasDaVendaRMT...)
	for _, linha := range todas {
		if strings.ContainsRune(string(protocol.ClientText(linha)), '?') && !strings.Contains(linha, "?") {
			t.Errorf("algum acento virou '?': %q", linha)
		}
	}
	// E as que TÊM acento continuam tendo: uma frase que perdeu os acentos passaria
	// nas duas conferências acima sem nada reclamar.
	comAcento := map[string]string{
		"o prazo de 48 horas": regrasDaVendaRMT[0],
		"o horario":           regrasDaVendaRMT[1],
		"os precos":           regrasDaVendaRMT[3],
	}
	for oque, linha := range comAcento {
		if !strings.ContainsAny(linha, "áâãàéêíóôõúçÁÂÃÀÉÊÍÓÔÕÚÇ") {
			t.Errorf("%s perdeu os acentos: %q", oque, linha)
		}
	}
}

// TestATaxaDitaAoVendedorEhADoCodigo.
//
// A frase promete "5% + R$ 0,80 (acima de R$ 100: 6,99% + R$ 0,80)". Se alguém mudar a
// tabela sem mudar a frase, o jogo passa a prometer uma taxa e cobrar outra — e o
// vendedor só descobre quando o dinheiro cai menor.
//
// O teste lê os NÚMEROS do código e procura cada um na frase, em vez de comparar a frase
// inteira com um texto fixo: assim ele reprova a divergência sem reprovar quem só
// reescreveu a frase melhor.
func TestATaxaDitaAoVendedorEhADoCodigo(t *testing.T) {
	frase := regrasDaVendaRMT[2]
	for _, esperado := range []string{"5%", "0,80", "6,99%", "100"} {
		if !strings.Contains(frase, esperado) {
			t.Errorf("a frase da taxa nao diz %q: %q", esperado, frase)
		}
	}
}
