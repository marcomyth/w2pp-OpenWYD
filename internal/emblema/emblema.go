// Package emblema confere se um BMP é o emblema de guilda que o jogo desenha.
//
// A CONFERÊNCIA É DO SERVIDOR, e não do site: um cliente remendado, ou um site
// futuro, não pode gravar imagem que o jogo não sabe desenhar. O formato foi medido
// no cliente 1.0.0 (gamepatch/emblema.cpp, LeBmpDoEmblema) pela dupla do cliente.
package emblema

import "encoding/binary"

// Tamanho é o BMP inteiro: 54 de cabeçalho e 576 de pixels.
//
// EXATO, e não "até": o teto de 632 do cliente é folga dele, e aceitar mais deixaria
// entrar bytes que ninguém sabe o que são. Um arquivo maior é erro de quem montou.
const Tamanho = 630

// Largura e Altura são as do emblema no cliente. A altura é positiva no cabeçalho,
// o que significa linhas de baixo para cima — o normal em BMP.
const (
	Largura = 16
	Altura  = 12
)

// Valido diz se estes bytes são o emblema que o jogo desenha.
//
// Cada campo é conferido por OFFSET, e não por "parece um BMP": a biblioteca de
// imagem aceitaria dezenas de variações que o cliente não lê — 32 bits, altura
// negativa, compressão RLE, paleta. O jogo não trata nenhuma delas; ele lê os bytes
// na posição em que espera.
func Valido(b []byte) bool {
	if len(b) != Tamanho {
		return false
	}
	if b[0] != 'B' || b[1] != 'M' {
		return false
	}
	u16 := func(i int) uint16 { return binary.LittleEndian.Uint16(b[i:]) }
	u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(b[i:]) }
	switch {
	case u32(2) != Tamanho: // bfSize
		return false
	case u32(10) != 54: // bfOffBits: os pixels começam logo depois dos dois cabeçalhos
		return false
	case u32(14) != 40: // biSize: BITMAPINFOHEADER, e não as variantes maiores
		return false
	case int32(u32(18)) != Largura: // biWidth
		return false
	// biHeight POSITIVO: negativo é BMP de cima para baixo, e o cliente leria a
	// imagem espelhada na vertical sem reclamar de nada.
	case int32(u32(22)) != Altura:
		return false
	case u16(26) != 1: // biPlanes
		return false
	case u16(28) != 24: // biBitCount
		return false
	case u32(30) != 0: // biCompression: BI_RGB, sem compressão
		return false
	}
	return true
}
