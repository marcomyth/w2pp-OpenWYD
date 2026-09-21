package content

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// fadaDoVale é a Fada do Vale(7dias) do ItemList.csv, a única chave do Vale
// Escondido (handler/vale.go, GetFunc.cpp:924-931).
const fadaDoVale = 3916

// structMobCarry e shopSlot: a vitrine de um mercador é o Carry do template dele,
// lido em três abas de nove (protocol.ShopSlot). Repetidos aqui porque este
// pacote não importa protocol, e são dois números do formato do arquivo.
const structMobCarry = 268

func shopSlot(i int) int { return (i % 9) + (i/9)*27 }

// Nenhum comerciante oferece a Fada do Vale (decisão de 21/09/2026, migração
// 0088). O template é a fonte viva da vitrine: o dbServer ressemeia a loja de
// todo mercador a partir dele em cada boot, então apagar a linha do banco sem
// tirar do arquivo devolve o item no deploy seguinte. Um teste sobre o arquivo é
// o que fecha esse caminho de volta.
func TestNenhumaLojaVendeAFadaDoVale(t *testing.T) {
	dir := release(t, "TMsrv", "run", "npc")
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	vistos := 0
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || len(b) != BaseMobSize {
			continue
		}
		vistos++
		for i := 0; i < 27; i++ {
			off := structMobCarry + shopSlot(i)*8
			if binary.LittleEndian.Uint16(b[off:off+2]) == fadaDoVale {
				t.Errorf("%s oferece a Fada do Vale na vaga %d", e.Name(), i)
			}
		}
	}
	// Sem esta conta o teste passa com um diretório vazio, que é o resultado de
	// qualquer erro de caminho — e passar por não ter olhado nada é o modo de
	// falha que este teste existe para não ter.
	if vistos < 1000 {
		t.Fatalf("só %d templates lidos em %s; a varredura não olhou o conteúdo", vistos, dir)
	}
}
