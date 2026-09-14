// Package clientkit escreve no cliente as variantes de item que só existem
// neste servidor — hoje, as duas do kit de novato (/novato).
//
// Por que isto é preciso: o cliente desenha a bolsa a partir dos PRÓPRIOS
// arquivos. Um índice que o servidor conhece e o cliente não vira um item sem
// nome, sem ícone e sem descrição na mão do jogador. E são três arquivos, não
// um, todos indexados pelo número do item:
//
//   - ItemList.bin: nome, malha, textura, efeitos e preço (Aplicar);
//   - itemicon.bin: a célula do atlas que desenha o ícone (AplicarIcones);
//   - itemhelp.dat: o texto abaixo do nome (AplicarDescricoes).
//
// A primeira versão só escrevia o ItemList.bin, e o kit chegou à bolsa com
// quadrados vazios (14/09/2026). Copiar o ícone aponta a variante para a MESMA
// célula da origem, sem desenhar nada: o atlas não muda.
//
// Irmão de clientmount, e com a mesma divisão de trabalho: aqui só se mexe no
// que o jogador LÊ. O que o servidor aplica vem do Release/Common/ItemList.csv.
package clientkit

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/clientitemhelp"
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
//
// Os nomes levam "_" no lugar do espaço, byte a byte como no CSV: é a forma de
// todo nome do ItemList.bin, e o cliente desenha o sublinhado como espaço.
func KitDoNovato() []Variante {
	const efNoTrade = 127
	return []Variante{
		{Origem: 3314, Destino: 5760, Nome: "Frango_Assado_(Novato)", Efeitos: [][2]int16{{efNoTrade, 1}}},
		{Origem: 4140, Destino: 5761, Nome: "Ba\xfa_de_Experi\xeancia_(Novato)", Efeitos: [][2]int16{{efNoTrade, 1}}},
	}
}

// AplicarIcones devolve uma cópia do itemicon.bin com cada variante apontando
// para a célula do seu item de origem.
//
// A tabela é um int32 little-endian por item com o id 1-based da célula, e 0 é
// "sem ícone" (ver itemicons.SetIcon).
func AplicarIcones(tabela []byte, vars []Variante) ([]byte, error) {
	if len(tabela)%4 != 0 {
		return nil, fmt.Errorf("clientkit: o itemicon.bin tem %d bytes, que não é múltiplo de 4", len(tabela))
	}
	out := bytes.Clone(tabela)
	icone := func(item int16) (uint32, error) {
		if item < 0 || (int(item)+1)*4 > len(out) {
			return 0, fmt.Errorf("clientkit: o itemicon.bin tem %d bytes e não alcança o item %d", len(out), item)
		}
		return binary.LittleEndian.Uint32(out[int(item)*4:]), nil
	}
	for _, v := range vars {
		org, err := icone(v.Origem)
		if err != nil {
			return nil, err
		}
		dst, err := icone(v.Destino)
		if err != nil {
			return nil, err
		}
		// Copiar "nenhum ícone" entregaria o quadrado vazio de novo, calado.
		if org == 0 {
			return nil, fmt.Errorf("clientkit: o item de origem %d não tem ícone no itemicon.bin", v.Origem)
		}
		// Mesma regra do ItemList.bin: um destino com ícone próprio é um item que
		// o cliente já desenha. Igual ao da origem é o gerador rodado de novo.
		if dst != 0 && dst != org {
			return nil, fmt.Errorf("clientkit: o índice de destino %d já tem o ícone %d", v.Destino, dst)
		}
		binary.LittleEndian.PutUint32(out[int(v.Destino)*4:], org)
	}
	return out, nil
}

// AplicarDescricoes devolve o itemhelp.dat com a descrição de cada origem
// repetida na sua variante. Origem sem texto não é erro: o Baú de Experiência
// (4140) não tem bloco no cliente, e a variante dele fica igual ao original.
func AplicarDescricoes(help []byte, vars []Variante) ([]byte, error) {
	out := help
	for _, v := range vars {
		novo, _, err := clientitemhelp.Copiar(out, int(v.Origem), int(v.Destino))
		if err != nil {
			return nil, fmt.Errorf("clientkit: descrição do item %d: %w", v.Destino, err)
		}
		out = novo
	}
	return out, nil
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
