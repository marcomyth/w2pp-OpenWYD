package content

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Itens que a limpeza de lojas do lançamento tirou de toda vitrine (migração
// 0092). O Coral fechou a família das quatro pedras que a 0086 começou; o
// Lactolerium 100 é o refino garantido, que a 0088 tirou do drop e deixou nas
// lojas de propósito, para um pedido depois.
const (
	coral           = 2443
	lactolerium100  = 4141
	pedidoDeCaca    = 3432 // 3432-3437: Armia, Dung, SubM, Kult, Kefra, Nipple
	pedidoDeCacaFim = 3437
	pedidoMaximo    = 10
)

// structMobMerchant é CurrentScore.Merchant, o byte que decide se o template
// abre loja — 92 (CurrentScore) + 12 (savefmt/codec.go:54). Não confundir com
// Mob.Merchant, no byte 17, que é outro campo: quem serve vitrine no port é
// este (dbserver/cmd/dbserver/main.go:188).
const structMobMerchant = 104

// efAmount é o EF_AMOUNT do legado, o par de efeito que carrega o tamanho da
// pilha vendida.
const efAmount = 61

// vitrine devolve as 27 vagas de loja de um template de mercador, ou nil quando
// o template não é mercador. Só 1 e 19 abrem loja (npcpanel.IsShop) — o Carry de
// um monstro é tabela de drop, e varrer isso como se fosse loja acusa o Coral
// que cai do Adamant Tauron.
func vitrine(b []byte) []int {
	if len(b) != BaseMobSize {
		return nil
	}
	if m := b[structMobMerchant]; m != 1 && m != 19 {
		return nil
	}
	vagas := make([]int, 27)
	for i := range vagas {
		off := structMobCarry + shopSlot(i)*8
		vagas[i] = int(binary.LittleEndian.Uint16(b[off : off+2]))
	}
	return vagas
}

// quantidade lê o EF_AMOUNT da vaga i; sem o par, a loja vende a unidade.
func quantidade(b []byte, i int) int {
	off := structMobCarry + shopSlot(i)*8
	for k := 0; k < 3; k++ {
		if b[off+2+k*2] == efAmount {
			return int(b[off+3+k*2])
		}
	}
	return 1
}

// cadaTemplate roda fn sobre todo template de Release/TMsrv/run/npc/ e cobra um
// piso de leitura: sem ele o teste passa com um diretório vazio, que é o
// resultado de qualquer erro de caminho — e passar por não ter olhado nada é o
// modo de falha que estes testes existem para não ter.
func cadaTemplate(t *testing.T, fn func(nome string, b []byte)) {
	t.Helper()
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
		fn(e.Name(), b)
	}
	if vistos < 1000 {
		t.Fatalf("só %d templates lidos em %s; a varredura não olhou o conteúdo", vistos, dir)
	}
}

func TestNenhumaVitrineVendeCoralNemLactolerium100(t *testing.T) {
	cadaTemplate(t, func(nome string, b []byte) {
		for i, item := range vitrine(b) {
			switch item {
			case coral:
				t.Errorf("%s oferece o Coral na vaga %d", nome, i)
			case lactolerium100:
				t.Errorf("%s oferece o Lactolerium 100 na vaga %d", nome, i)
			}
		}
	})
}

func TestPedidoDeCacaNaoPassaDeDez(t *testing.T) {
	cadaTemplate(t, func(nome string, b []byte) {
		for i, item := range vitrine(b) {
			if item < pedidoDeCaca || item > pedidoDeCacaFim {
				continue
			}
			if q := quantidade(b, i); q > pedidoMaximo {
				t.Errorf("%s vende o Pedido de Caça %d em pilha de %d na vaga %d, o teto é %d",
					nome, item, q, i, pedidoMaximo)
			}
		}
	})
}

// A Lucy é a loja de Azran (2550,1716) que o pedido de 21/09/2026 mandou
// organizar: as duas Classes que ficam, encostadas uma na outra, e as quatro
// Gemas juntas e em ordem. A vitrine inteira é afirmada, não só o que mudou —
// é uma loja pequena, e o que este teste precisa pegar é justamente o item que
// alguém reponha no meio dela.
func TestVitrineDaLucy(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(release(t, "TMsrv", "run", "npc"), "Lucy"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	vagas := vitrine(b)
	if vagas == nil {
		t.Fatal("a Lucy deixou de ser mercadora")
	}
	esperado := map[int]int{
		0:  4016, // Classe A
		1:  4017, // Classe B
		8:  411,  // Pergaminho Retorno 10x
		18: 3386, // Gema de Diamante
		19: 3387, // Gema de Esmeralda
		20: 3388, // Gema de Coral
		21: 3389, // Gema de Garnet
	}
	for i, item := range vagas {
		if item != esperado[i] {
			t.Errorf("vaga %d da Lucy tem %d, esperava %d", i, item, esperado[i])
		}
	}
}
