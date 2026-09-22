// Package clientitemhelp edita o itemhelp.dat do cliente — o texto que o jogo
// mostra abaixo do nome do item, na bolsa e na loja.
//
// O arquivo é texto puro em Windows-1252, com quebra CRLF e sem cabeçalho: cada
// item é uma linha só com o índice, seguida das suas linhas de descrição, e os
// índices vêm em ordem crescente. Cada linha de descrição começa com a cor em
// oito dígitos hexadecimais (ARGB), um espaço, e o texto.
//
//	3343
//	FFFF00FF [Item_Premium]
//	FFFFFFFF Dispensa_os_pontos_caóticos_de_-50
//	FFFFFFFF para_0.
//
// O espaço dentro do texto é gravado como "_": é o próprio cliente que desenha
// o sublinhado como espaço, a mesma regra dos nomes de item no ItemList.
package clientitemhelp

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// As cores que os textos do cliente usam.
const (
	Branco   uint32 = 0xFFFFFFFF // o corpo da descrição
	Vermelho uint32 = 0xFFFF0000 // observações e avisos
	Premium  uint32 = 0xFFFF00FF // o selo "[Item_Premium]"
)

// Linha é uma linha da descrição: a cor e o texto, com espaços normais — o
// espaço vira "_" na gravação.
type Linha struct {
	Cor   uint32
	Texto string
}

// Set devolve o itemhelp.dat com a descrição do item trocada pela que vai em
// linhas, inserindo o bloco na ordem certa quando o item ainda não tem texto.
// Um bloco sem linhas remove a descrição.
//
// O arquivo é reescrito byte a byte fora do bloco tocado: os outros itens saem
// exatamente como entraram, porque um acento perdido ao decodificar e codificar
// de novo apareceria como pergunta na tela de todo mundo.
func Set(data []byte, item int, linhas []Linha) ([]byte, error) {
	if item <= 0 {
		return nil, fmt.Errorf("clientitemhelp: item %d inválido", item)
	}
	inicio, fim, err := bloco(data, item)
	if err != nil {
		return nil, err
	}
	var novo bytes.Buffer
	novo.Write(data[:inicio])
	// O itemhelp.dat do cliente termina SEM quebra de linha. Um bloco inserido
	// no fim — que é onde entra todo item de índice maior que os existentes —
	// colava no texto do último item: "..._Xp/Ouro_[Eterno]9999". Isso estraga
	// duas descrições de uma vez, a do vizinho e a do item novo, e só aparece
	// no item mais alto do arquivo.
	if len(linhas) > 0 && novo.Len() > 0 && !bytes.HasSuffix(novo.Bytes(), []byte("\n")) {
		novo.WriteString("\r\n")
	}
	if len(linhas) > 0 {
		novo.WriteString(strconv.Itoa(item))
		novo.WriteString("\r\n")
		for _, l := range linhas {
			fmt.Fprintf(&novo, "%08X %s\r\n", l.Cor, cp1252(sublinhado(l.Texto)))
		}
	}
	novo.Write(data[fim:])
	return novo.Bytes(), nil
}

// Get devolve a descrição que o item tem hoje, ou nil quando ele não tem bloco
// nenhum. É o par do Set, que grava o bloco INTEIRO: sem ler antes, acrescentar
// uma linha a um item apagaria o texto que ele já mostrava.
//
// O texto volta com espaços de verdade (o "_" do arquivo desfeito) e com os
// bytes lidos como Latin-1, que é o mesmo caminho de volta do cp1252 — assim o
// que sai do Get pode ser devolvido ao Set sem perder acento.
func Get(data []byte, item int) ([]Linha, error) {
	if item <= 0 {
		return nil, fmt.Errorf("clientitemhelp: item %d inválido", item)
	}
	inicio, fim, err := bloco(data, item)
	if err != nil {
		return nil, err
	}
	if inicio == fim {
		return nil, nil // o item não tem descrição
	}
	var out []Linha
	corpo := data[inicio:fim]
	for pos := 0; pos < len(corpo); {
		fimLinha := bytes.IndexByte(corpo[pos:], '\n')
		linha := corpo[pos:]
		proxima := len(corpo)
		if fimLinha >= 0 {
			linha = corpo[pos : pos+fimLinha]
			proxima = pos + fimLinha + 1
		}
		pos = proxima
		if _, ok := indice(linha); ok {
			continue // a linha do índice que abre o bloco
		}
		l, ok := parseLinha(linha)
		if !ok {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// parseLinha lê "AARRGGBB texto_com_sublinhado". Uma linha que não tenha essa
// forma é ignorada em vez de virar erro: o arquivo é editado à mão há anos e
// uma linha torta num item que ninguém vai tocar não pode impedir a edição de
// outro.
func parseLinha(linha []byte) (Linha, bool) {
	b := bytes.TrimRight(linha, "\r\n")
	espaco := bytes.IndexByte(b, ' ')
	if espaco != 8 {
		return Linha{}, false
	}
	cor, err := strconv.ParseUint(string(b[:espaco]), 16, 32)
	if err != nil {
		return Linha{}, false
	}
	return Linha{Cor: uint32(cor), Texto: strings.ReplaceAll(deLatin1(b[espaco+1:]), "_", " ")}, true
}

// deLatin1 é o inverso exato do cp1252: cada byte é o code point de mesmo
// valor. Tem de ser byte a byte, e não string(b), porque o arquivo NÃO é
// UTF-8 — um "ç" lá é o byte 0xE7 sozinho, que como UTF-8 é inválido e vira
// RuneError, e o cp1252 da volta o grava como "?". O acento sumiria da tela de
// todo mundo, e só no item que alguém tivesse editado.
func deLatin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// bloco acha onde começa e termina o bloco do item. Quando o item não tem
// bloco, os dois valores apontam para o lugar onde ele deve ser inserido para
// manter a ordem crescente dos índices.
//
// O arquivo inteiro é varrido atrás do item antes de escolher onde inserir: o
// itemhelp.dat real não está em ordem (blocos novos, como o da Chave do Rei Orc,
// foram acrescentados depois de índices maiores), e parar no primeiro índice maior
// punha um segundo bloco do mesmo item em vez de trocar o que existia.
func bloco(data []byte, item int) (int, int, error) {
	insere := -1
	pos := 0
	for pos < len(data) {
		fimLinha := bytes.IndexByte(data[pos:], '\n')
		linha := data[pos:]
		proxima := len(data)
		if fimLinha >= 0 {
			linha = data[pos : pos+fimLinha]
			proxima = pos + fimLinha + 1
		}
		idx, ok := indice(linha)
		if !ok {
			pos = proxima
			continue
		}
		switch {
		case idx == item:
			return pos, fimDoBloco(data, proxima), nil
		case idx > item && insere < 0:
			insere = pos // sem bloco próprio, o item entra aqui, antes deste
		}
		pos = proxima
	}
	if insere >= 0 {
		return insere, insere, nil
	}
	return len(data), len(data), nil
}

// fimDoBloco anda até a próxima linha que é só um índice, que é onde o bloco
// seguinte começa.
func fimDoBloco(data []byte, pos int) int {
	for pos < len(data) {
		fimLinha := bytes.IndexByte(data[pos:], '\n')
		linha := data[pos:]
		proxima := len(data)
		if fimLinha >= 0 {
			linha = data[pos : pos+fimLinha]
			proxima = pos + fimLinha + 1
		}
		if _, ok := indice(linha); ok {
			return pos
		}
		pos = proxima
	}
	return len(data)
}

// indice reconhece a linha que é só um número — a que abre o bloco de um item.
func indice(linha []byte) (int, bool) {
	s := strings.TrimRight(string(linha), "\r\n ")
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// sublinhado troca espaço por "_", que é como o cliente guarda o texto.
func sublinhado(s string) string { return strings.ReplaceAll(s, " ", "_") }

// cp1252 escreve o texto no código de página do cliente. Só a metade Latin-1 é
// usada pelos textos do jogo; um caractere fora dela vira "?" em vez de sair
// como dois bytes ilegíveis na tela.
func cp1252(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 256 {
			out = append(out, byte(r))
			continue
		}
		out = append(out, '?')
	}
	return out
}
