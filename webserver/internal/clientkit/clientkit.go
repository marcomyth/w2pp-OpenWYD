// Package clientkit escreve no ItemList.bin do cliente as variantes de item que
// só existem neste servidor — hoje, as duas do kit de novato (/novato).
//
// Por que isto é preciso: o cliente desenha a bolsa a partir do PRÓPRIO
// catálogo. Um índice que o servidor conhece e o cliente não vira um item sem
// nome e sem ícone na mão do jogador — o registro existe (o arquivo é um vetor
// fixo de 6500), mas está zerado. Copiar a entrada do item de origem resolve as
// duas coisas de uma vez, porque nome, malha e textura moram no mesmo registro.
//
// Irmão de clientmount, e com a mesma divisão de trabalho: aqui só se mexe no
// que o jogador LÊ. O que o servidor aplica vem do Release/Common/ItemList.csv.
package clientkit

import (
	"bytes"
	"fmt"
)

// O ItemList.bin do cliente: 6500 registros de 140 bytes sob um XOR 0x5A plano,
// mais 4 bytes finais que o cliente não confere. O registro é
// Name[64] + 8 shorts + 12 pares (efeito, valor) + Price int32 + 4 shorts.
const (
	tamanhoArquivo = 6500*140 + 4
	tamanhoReg     = 140
	offNome        = 0
	tamanhoNome    = 64
	offEfeitos     = 80
	slotsDeEfeito  = 12
	offPreco       = 128
	xor            = 0x5A
)

// Variante é uma cópia de Origem gravada em Destino, com nome próprio, preço
// próprio e efeitos acrescentados.
type Variante struct {
	Origem  int16
	Destino int16
	Nome    string // em Windows-1252, como o resto do arquivo
	Preco   int32  // 0 nas variantes do kit: item que não vira gold no NPC
	Efeitos [][2]int16
}

// KitDoNovato são as duas variantes do /novato, com os mesmos índices que o
// Release/Common/ItemList.csv e o handler usam. Mudar um lado sem o outro
// entrega ao jogador um item que o cliente dele não sabe desenhar.
func KitDoNovato() []Variante {
	const efNoTrade = 127
	return []Variante{
		{Origem: 3314, Destino: 5760, Nome: "Frango Assado (Novato)", Efeitos: [][2]int16{{efNoTrade, 1}}},
		{Origem: 4140, Destino: 5761, Nome: "Ba\xfa de Experi\xeancia (Novato)", Efeitos: [][2]int16{{efNoTrade, 1}}},
	}
}

// Aplicar devolve uma cópia do ItemList.bin com as variantes gravadas.
func Aplicar(il []byte, vars []Variante) ([]byte, error) {
	if len(il) != tamanhoArquivo {
		return nil, fmt.Errorf("clientkit: o ItemList.bin tem %d bytes, esperava %d", len(il), tamanhoArquivo)
	}
	out := bytes.Clone(il)
	leByte := func(off int) byte { return out[off] ^ xor }
	leShort := func(off int) int16 {
		return int16(uint16(leByte(off)) | uint16(leByte(off+1))<<8)
	}
	poeShort := func(off int, v int16) {
		out[off], out[off+1] = byte(uint16(v))^xor, byte(uint16(v)>>8)^xor
	}

	for _, v := range vars {
		if v.Origem < 0 || int(v.Origem) >= 6500 || v.Destino < 0 || int(v.Destino) >= 6500 {
			return nil, fmt.Errorf("clientkit: índice fora do catálogo (origem %d, destino %d)", v.Origem, v.Destino)
		}
		org, dst := int(v.Origem)*tamanhoReg, int(v.Destino)*tamanhoReg
		if leByte(org) == 0 {
			return nil, fmt.Errorf("clientkit: o item de origem %d não existe no ItemList.bin", v.Origem)
		}
		// O destino precisa estar VAZIO. Sobrescrever um item que o cliente já
		// conhece trocaria o desenho de algo que o jogador tem na bolsa — e o
		// erro só apareceria em jogo, no item errado.
		if leByte(dst) != 0 {
			return nil, fmt.Errorf("clientkit: o índice de destino %d já é o item %q", v.Destino, nomeDoRegistro(out, dst))
		}
		copy(out[dst:dst+tamanhoReg], out[org:org+tamanhoReg])

		if len(v.Nome) >= tamanhoNome {
			return nil, fmt.Errorf("clientkit: o nome %q não cabe em %d bytes", v.Nome, tamanhoNome-1)
		}
		for i := 0; i < tamanhoNome; i++ {
			b := byte(0)
			if i < len(v.Nome) {
				b = v.Nome[i]
			}
			out[dst+offNome+i] = b ^ xor
		}

		// Preço: escrito como dois shorts, que é como o resto do arquivo é lido.
		poeShort(dst+offPreco, int16(v.Preco))
		poeShort(dst+offPreco+2, int16(v.Preco>>16))

		for _, ef := range v.Efeitos {
			slot := -1
			for i := 0; i < slotsDeEfeito; i++ {
				c := leShort(dst + offEfeitos + 4*i)
				if c == ef[0] {
					slot = i
					break
				}
				if c == 0 && slot < 0 {
					slot = i
				}
			}
			if slot < 0 {
				return nil, fmt.Errorf("clientkit: o item %d não tem espaço de efeito livre", v.Destino)
			}
			poeShort(dst+offEfeitos+4*slot, ef[0])
			poeShort(dst+offEfeitos+4*slot+2, ef[1])
		}
	}
	return out, nil
}

func nomeDoRegistro(b []byte, reg int) string {
	nome := make([]byte, 0, tamanhoNome)
	for i := 0; i < tamanhoNome; i++ {
		c := b[reg+offNome+i] ^ xor
		if c == 0 {
			break
		}
		nome = append(nome, c)
	}
	return string(nome)
}
