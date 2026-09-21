package content

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/savefmt"
)

// Itens que a limpeza de lojas do lançamento tirou de TODA vitrine.
const (
	coral          = 2443 // migração 0092: fechou a família que a 0086 começou
	lactolerium100 = 4141 // migração 0092: o refino garantido, fora do drop na 0088

	// Migração 0094: os dez da vitrine do Martin — as três poções de trinta
	// dias, as três Esferas da Sorte (que nenhuma linha de código lê), a Poção
	// Poderosa, as Ervas de Cura e as duas Caixas de Poção.
	//
	// As Ervas de Cura voltaram na 0095, só no Martin, a pedido: são a erva que
	// o jogador usa contra a lentidão. Por isso NÃO estão em foraDeTodaVitrine —
	// quem as afirma agora é TestVitrineDoMartin.
	pocaoDivina30  = 3381
	pocaoSephira30 = 3363
	pocaoSaude30   = 3366
	esferaDaSorteN = 4128
	esferaDaSorteM = 4129
	esferaDaSorteA = 4130
	pocaoPoderosa  = 3431
	ervasDeCura    = 415
	caixaPocaoCura = 3322
	caixaPocaoMana = 3323

	pedidoDeCaca    = 3432 // 3432-3437: Armia, Dung, SubM, Kult, Kefra, Nipple
	pedidoDeCacaFim = 3437
	pedidoMaximo    = 10
)

// foraDeTodaVitrine é a lista inteira, com o nome que a falha vai imprimir.
var foraDeTodaVitrine = map[int]string{
	coral:          "o Coral",
	lactolerium100: "o Lactolerium 100",
	pocaoDivina30:  "a Poção Divina(30dias)",
	pocaoSephira30: "a Poção Sephira(30dias)",
	pocaoSaude30:   "a Poção de Saúde(30dias)",
	esferaDaSorteN: "a Esfera da Sorte(N)",
	esferaDaSorteM: "a Esfera da Sorte(M)",
	esferaDaSorteA: "a Esfera da Sorte(A)",
	pocaoPoderosa:  "a Poção Poderosa",
	caixaPocaoCura: "a Caixa de Poção de Cura",
	caixaPocaoMana: "a Caixa de Poção de Mana",
}

// efAmount é o EF_AMOUNT do legado, o par de efeito que carrega o tamanho da
// pilha vendida.
const efAmount = 61

// shopSlot mapeia a vaga da janela de loja para o índice no Carry: três abas de
// nove, como protocol.ShopSlot.
func shopSlot(i int) int { return (i % 9) + (i/9)*27 }

// molde são os offsets de Carry e de Merchant de um layout de template.
//
// TRÊS tamanhos convivem em Release/TMsrv/run/npc/: 1.792 no canônico de 816,
// 207 no legado de 756 e 15 no mesmo 756 com 164 bytes de lixo no fim
// (savefmt.DetectMobVersion). Os offsets do legado são OUTROS, e uma varredura
// que só aceita 816 deixa 222 arquivos sem olhar — foi exatamente assim que a
// Fada do Vale continuou à venda no Utilidades depois do commit que dizia
// tê-la tirado de todas as lojas, e que o Coral e o Lactolerium 100 sobraram
// em quatro vitrines.
type molde struct{ carry, merchant int }

func moldeDe(b []byte) (molde, bool) {
	switch len(b) {
	case savefmt.MobSize:
		// CurrentScore em 92, Merchant em +12 (savefmt/codec.go:54).
		return molde{carry: 268, merchant: 104}, true
	case savefmt.MobSizeLegacy756, savefmt.MobSizeLegacy756Padded:
		// CurrentScore em 64, Merchant em +6 do score compacto de 28 bytes.
		return molde{carry: 220, merchant: 70}, true
	}
	return molde{}, false
}

// vitrine devolve as 27 vagas de loja de um template de mercador, ou nil quando
// o template não é mercador nem tem layout conhecido.
//
// Só Merchant 1 e 19 abrem loja (npcpanel.IsShop) — o Carry de quem tem
// Merchant 0 é tabela de DROP, e varrer isso como vitrine acusa o Coral que cai
// do Adamant Tauron.
func vitrine(b []byte) []int {
	m, ok := moldeDe(b)
	if !ok {
		return nil
	}
	if q := b[m.merchant]; q != 1 && q != 19 {
		return nil
	}
	vagas := make([]int, 27)
	for i := range vagas {
		off := m.carry + shopSlot(i)*8
		vagas[i] = int(b[off]) | int(b[off+1])<<8
	}
	return vagas
}

// quantidade lê o EF_AMOUNT da vaga i; sem o par, a loja vende a unidade.
func quantidade(b []byte, i int) int {
	m, ok := moldeDe(b)
	if !ok {
		return 1
	}
	off := m.carry + shopSlot(i)*8
	for k := 0; k < 3; k++ {
		if b[off+2+k*2] == efAmount {
			return int(b[off+3+k*2])
		}
	}
	return 1
}

// cadaTemplate roda fn sobre todo template de Release/TMsrv/run/npc/ com layout
// conhecido, e cobra um piso de leitura: sem ele o teste passa com um diretório
// vazio, que é o resultado de qualquer erro de caminho — e passar por não ter
// olhado nada é o modo de falha que estes testes existem para não ter.
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
		if err != nil {
			continue
		}
		if _, ok := moldeDe(b); !ok {
			continue
		}
		vistos++
		fn(e.Name(), b)
	}
	// São 2.014 arquivos nos três layouts. O piso descarta o diretório vazio e,
	// acima de 1.800, também a varredura que perdeu um layout inteiro.
	if vistos < 1800 {
		t.Fatalf("só %d templates lidos em %s; a varredura não olhou o conteúdo", vistos, dir)
	}
}

func TestNenhumaVitrineVendeOsItensRetirados(t *testing.T) {
	cadaTemplate(t, func(nome string, b []byte) {
		for i, item := range vitrine(b) {
			if oQue, fora := foraDeTodaVitrine[item]; fora {
				t.Errorf("%s (%d bytes) oferece %s na vaga %d", nome, len(b), oQue, i)
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

// O Martin (Armia 2116,2150 e também 1317,346) teve a vitrine refeita na 0095:
// os mesmos itens da Aki, vaga por vaga, mais as Ervas de Cura na vaga 0, que a
// limpeza da 0094 tinha deixado livre nas duas lojas. Afirmar a vitrine inteira
// é o que pega o item que alguém reponha no meio dela.
//
// A cópia é do conteúdo, não um espelho vivo: se a Aki mudar, este teste NÃO
// acompanha, e é de propósito — são duas lojas independentes a partir daqui.
func TestVitrineDoMartin(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(release(t, "TMsrv", "run", "npc"), "Martin"))
	if err != nil {
		t.Skipf("Release content unavailable: %v", err)
	}
	vagas := vitrine(b)
	if vagas == nil {
		t.Fatal("o Martin deixou de ser mercador")
	}
	esperado := map[int]int{
		0:  ervasDeCura, // a erva contra a lentidão, em pilha de dez
		2:  699,         // Pergaminho do Teleporte
		3:  410,         // Pergaminho Retorno
		10: 4038,        // Vela do Coveiro
		11: 4039,        // Colheita do Jardineiro
		12: 4040,        // Cura do Batedor
		13: 4041,        // Mana do Batedor
		18: 501,         // Anel de Hercules
		19: 503,         // Anel de Titã
		20: 502,         // Anel de Athena
		21: 506,         // Anel de Hecate
		22: 505,         // Anel de Zeus
	}
	for i, item := range vagas {
		if item != esperado[i] {
			t.Errorf("vaga %d do Martin tem %d, esperava %d", i, item, esperado[i])
		}
	}
	if q := quantidade(b, 0); q != pedidoMaximo {
		t.Errorf("as Ervas de Cura saem em pilha de %d, esperava %d", q, pedidoMaximo)
	}
}
