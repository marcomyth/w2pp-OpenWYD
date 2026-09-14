package content

import (
	"os"
	"path/filepath"
	"testing"
)

func escreve(t *testing.T, linhas string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "LevelItem.txt")
	if err := os.WriteFile(p, []byte(linhas), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLevelItemLeOsQuatroEixos: nível, classe, construção e item, mais os três
// pares de efeito. A sequência da primeira coluna é lida e jogada fora, como no
// legado — ela é número de linha, não chave.
func TestLevelItemLeOsQuatroEixos(t *testing.T) {
	t.Setenv("TZ", "UTC")
	tab, avisos, err := LoadLevelItems(escreve(t, `
# comentário
0   29    0       1   1147 43 3 2 20 42 40
1   29    3       2   1159 60 6 0 0 0 0
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(avisos) != 0 {
		t.Errorf("avisos inesperados: %v", avisos)
	}
	got := tab.Para(0, 1, 29) // TK, mais INT
	if got.Index != 1147 {
		t.Errorf("TK/INT/29 = %d, queria 1147", got.Index)
	}
	if got.Effects != [3][2]uint8{{43, 3}, {2, 20}, {42, 40}} {
		t.Errorf("efeitos = %v", got.Effects)
	}
	if x := tab.Para(3, 2, 29); x.Index != 1159 { // Huntress, mais DES
		t.Errorf("HT/DES/29 = %d, queria 1159", x.Index)
	}
	// as casas que a linha não cobre continuam vazias
	if x := tab.Para(0, 0, 29); !x.Empty() {
		t.Errorf("TK/FOR/29 devia estar vazio, veio %d", x.Index)
	}
	if x := tab.Para(0, 1, 30); !x.Empty() {
		t.Errorf("nível 30 devia estar vazio, veio %d", x.Index)
	}
}

// TestLevelItemCoringas: 4 na classe vale para as quatro profissões, 4 na
// construção vale para as quatro construções, e os dois juntos preenchem tudo.
func TestLevelItemCoringas(t *testing.T) {
	tab, _, err := LoadLevelItems(escreve(t, `
0   50    4       1   900 0 0 0 0 0 0
1   60    2       4   901 0 0 0 0 0 0
2   70    4       4   902 0 0 0 0 0 0
`))
	if err != nil {
		t.Fatal(err)
	}
	for c := 0; c < LevelItemClasses; c++ {
		if x := tab.Para(c, 1, 50); x.Index != 900 {
			t.Errorf("classe %d no 50 = %d, queria 900", c, x.Index)
		}
		if x := tab.Para(c, 0, 50); !x.Empty() {
			t.Errorf("classe %d construção 0 no 50 devia estar vazia", c)
		}
	}
	for b := 0; b < LevelItemBuilds; b++ {
		if x := tab.Para(2, b, 60); x.Index != 901 {
			t.Errorf("BM construção %d no 60 = %d, queria 901", b, x.Index)
		}
		if x := tab.Para(0, b, 60); !x.Empty() {
			t.Errorf("TK construção %d no 60 devia estar vazia", b)
		}
	}
	for c := 0; c < LevelItemClasses; c++ {
		for b := 0; b < LevelItemBuilds; b++ {
			if x := tab.Para(c, b, 70); x.Index != 902 {
				t.Errorf("classe %d construção %d no 70 = %d, queria 902", c, b, x.Index)
			}
		}
	}
}

// TestLevelItemConstrucaoMenosUmNaoEstouraOVetor é a divergência deliberada.
//
// O legado não trata -1: a linha cai no ramo final e escreve em
// LevelItem[classe][-1][nivel], fora do vetor. As duas linhas do arquivo que vem
// no conteúdo fazem exatamente isso, e são as que entregam a montaria do nível
// 149. Aqui -1 é lido como "todas as construções", que é o que o cabeçalho do
// próprio arquivo diz que ele significa.
func TestLevelItemConstrucaoMenosUmNaoEstouraOVetor(t *testing.T) {
	tab, _, err := LoadLevelItems(escreve(t, "30  149   0      -1   2368 15 15 15 15 15 15\n"))
	if err != nil {
		t.Fatal(err)
	}
	for b := 0; b < LevelItemBuilds; b++ {
		if x := tab.Para(0, b, 149); x.Index != 2368 {
			t.Errorf("TK construção %d no 149 = %d, queria a montaria 2368", b, x.Index)
		}
	}
	if tab.ConstrucaoIndefinida != 1 {
		t.Errorf("construções indefinidas = %d, queria 1", tab.ConstrucaoIndefinida)
	}
	// e não vazou para as outras classes: a linha diz classe 0
	if x := tab.Para(1, 0, 149); !x.Empty() {
		t.Errorf("a linha da classe 0 vazou para a classe 1: %d", x.Index)
	}
}

// TestLevelItemUltimaLinhaGanhaEConta: fidelidade ao legado (a última sobrescreve)
// mais a contagem, que é o que tira o desperdício da invisibilidade.
func TestLevelItemUltimaLinhaGanhaEConta(t *testing.T) {
	tab, _, err := LoadLevelItems(escreve(t, `
0   80    1       2   111 0 0 0 0 0 0
1   80    1       2   222 0 0 0 0 0 0
2   80    1       2   333 0 0 0 0 0 0
`))
	if err != nil {
		t.Fatal(err)
	}
	if x := tab.Para(1, 2, 80); x.Index != 333 {
		t.Errorf("ganhou %d, queria a última (333)", x.Index)
	}
	if tab.Linhas != 3 {
		t.Errorf("linhas = %d, queria 3", tab.Linhas)
	}
	if tab.Sobrescritas != 2 {
		t.Errorf("sobrescritas = %d, queria 2", tab.Sobrescritas)
	}
}

// TestLevelItemLinhaTortaNaoDerrubaOBoot: o arquivo é escrito à mão e tem 547
// linhas; perder o servidor por uma delas seria pior que perder aquela peça.
func TestLevelItemLinhaTortaNaoDerrubaOBoot(t *testing.T) {
	tab, avisos, err := LoadLevelItems(escreve(t, `
0   29    0       1   1147 43 3 2 20 42 40
isto nao e numero
2   30
3   31    0       1   1148 0 0 0 0 0 0
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(avisos) != 2 {
		t.Errorf("avisos = %d (%v), queria 2", len(avisos), avisos)
	}
	if x := tab.Para(0, 1, 29); x.Index != 1147 {
		t.Error("a linha boa antes da torta se perdeu")
	}
	if x := tab.Para(0, 1, 31); x.Index != 1148 {
		t.Error("a linha boa depois da torta se perdeu")
	}
}

// TestLevelItemArquivoDoJogo lê o arquivo que vem no Release e prende os números
// que o levantamento achou. Se alguém editar o conteúdo, este teste conta.
func TestLevelItemArquivoDoJogo(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Release", "TMsrv", "run", "LevelItem.txt")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("LevelItem.txt indisponível: %v", err)
	}
	tab, avisos, err := LoadLevelItems(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(avisos) != 0 {
		t.Errorf("o arquivo do jogo tem linha torta: %v", avisos)
	}
	if tab.Linhas != 318 {
		t.Errorf("entradas = %d, o levantamento achou 318", tab.Linhas)
	}
	if tab.ConstrucaoIndefinida != 2 {
		t.Errorf("linhas com construção -1 = %d, o levantamento achou 2", tab.ConstrucaoIndefinida)
	}
	// a montaria do 149 é a razão de a divergência do -1 existir
	for b := 0; b < LevelItemBuilds; b++ {
		if x := tab.Para(0, b, 149); x.Index != 2368 {
			t.Errorf("a montaria do nível 149 não chega ao TK de construção %d (veio %d)", b, x.Index)
		}
	}
	// O DESENCONTRO DE NUMERAÇÃO, preso aqui até alguém decidir o que fazer.
	//
	// O arquivo numera a construção pelo próprio cabeçalho — 1 = mais Força,
	// 2 = mais Int, 3 = mais Destreza, 0 = independe dos pontos — e as seções
	// dizem a mesma coisa em palavras: "#TK DN SET MALHA" (o set físico) vem com
	// construção 1, "#TK MAG SET MALHA" (o mágico) com 2.
	//
	// O código do legado numera outra coisa: 0 = Força, 1 = Int, 2 = Destreza,
	// 3 = o resto. Lendo o arquivo por essa régua, o set FÍSICO vai para quem tem
	// mais INT, e quem tem mais FORÇA não recebe quase nada.
	//
	// Este carregador é fiel ao CÓDIGO, e o teste prende o resultado disso para
	// que a conversa aconteça com o número na mão em vez de com uma impressão.
	niveis := 23
	conta := func(classe, construcao int) int {
		n := 0
		for nv := int32(0); nv < 400; nv++ {
			if !tab.Para(classe, construcao, nv).Empty() {
				n++
			}
		}
		return n
	}
	for _, c := range []struct {
		nome                  string
		classe, construcao, q int
	}{
		{"TK com mais Força", 0, 0, 1},
		{"TK com mais Int", 0, 1, 23},
		{"Foema com mais Força", 1, 0, 0},
		{"BM com mais Força", 2, 0, 0},
		{"Huntress com mais Destreza", 3, 2, 0},
		{"Huntress com mais Força", 3, 0, 20},
	} {
		if got := conta(c.classe, c.construcao); got != c.q {
			t.Errorf("%s recebe em %d dos %d níveis, o levantamento achou %d",
				c.nome, got, niveis, c.q)
		}
	}
}
