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

// Copiar devolve o itemhelp.dat com a descrição de origem repetida sob o índice
// destino, trocando a que o destino tivesse. O bool diz se a origem tinha texto:
// sem bloco, o arquivo volta intacto, porque um item sem descrição é um estado
// válido do cliente e a cópia dele é outro item sem descrição.
//
// As linhas vão como bytes, sem passar por Linha: decodificar e codificar de
// novo é justamente o caminho em que um acento se perde.
func Copiar(data []byte, origem, destino int) ([]byte, bool, error) {
	if origem <= 0 || destino <= 0 || origem == destino {
		return nil, false, fmt.Errorf("clientitemhelp: cópia de %d para %d inválida", origem, destino)
	}
	inicio, fim, err := bloco(data, origem)
	if err != nil {
		return nil, false, err
	}
	if inicio == fim {
		return data, false, nil
	}
	corpo := data[inicio:fim]
	if nl := bytes.IndexByte(corpo, '\n'); nl >= 0 {
		corpo = corpo[nl+1:]
	} else {
		corpo = nil // o índice é a última linha do arquivo: bloco sem texto
	}
	var linhas bytes.Buffer
	linhas.WriteString(strconv.Itoa(destino))
	linhas.WriteString("\r\n")
	linhas.Write(corpo)
	// O último bloco do arquivo pode acabar sem quebra; sem esta, a última linha
	// dele ficaria colada no índice do bloco seguinte.
	if len(corpo) > 0 && corpo[len(corpo)-1] != '\n' {
		linhas.WriteString("\r\n")
	}

	di, df, err := bloco(data, destino)
	if err != nil {
		return nil, false, err
	}
	var novo bytes.Buffer
	novo.Write(data[:di])
	// Inserir no fim de um arquivo que não termina em quebra colaria o índice
	// novo na última linha do bloco anterior.
	if di == len(data) && di > 0 && data[di-1] != '\n' {
		novo.WriteString("\r\n")
	}
	novo.Write(linhas.Bytes())
	novo.Write(data[df:])
	return novo.Bytes(), true, nil
}

// bloco acha onde começa e termina o bloco do item. Quando o item não tem
// bloco, os dois valores apontam para o lugar onde ele deve ser inserido: antes
// do primeiro índice maior.
//
// A busca pelo índice exato percorre o arquivo INTEIRO antes de decidir que ele
// não existe. O itemhelp.dat do cliente só é quase ordenado — os blocos
// 3310-3315 vêm depois do 3463, e há outras 15 quedas —, e parar no primeiro
// índice maior dava esses itens como sem descrição: o Set duplicava o bloco e a
// cópia do Frango do kit de novato saía sem texto.
func bloco(data []byte, item int) (int, int, error) {
	insercao := len(data)
	pos := 0
	for pos < len(data) {
		fimLinha := bytes.IndexByte(data[pos:], '\n')
		linha := data[pos:]
		proxima := len(data)
		if fimLinha >= 0 {
			linha = data[pos : pos+fimLinha]
			proxima = pos + fimLinha + 1
		}
		if idx, ok := indice(linha); ok {
			if idx == item {
				return pos, fimDoBloco(data, proxima), nil
			}
			if idx > item && insercao == len(data) {
				insercao = pos // o item entraria aqui, antes deste
			}
		}
		pos = proxima
	}
	return insercao, insercao, nil
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
