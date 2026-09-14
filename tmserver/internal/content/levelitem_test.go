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
	// a linha diz construção 1, que no cabeçalho do arquivo é "mais Força"
	got := tab.Para(0, 0, 29)
	if got.Index != 1147 {
		t.Errorf("TK/Força/29 = %d, queria 1147", got.Index)
	}
	if got.Effects != [3][2]uint8{{43, 3}, {2, 20}, {42, 40}} {
		t.Errorf("efeitos = %v", got.Effects)
	}
	if x := tab.Para(3, 1, 29); x.Index != 1159 { // construção 2 = mais Int
		t.Errorf("Huntress/Int/29 = %d, queria 1159", x.Index)
	}
	// as casas que a linha não cobre continuam vazias
	if x := tab.Para(0, 1, 29); !x.Empty() {
		t.Errorf("TK/Int/29 devia estar vazio, veio %d", x.Index)
	}
	if x := tab.Para(0, 0, 30); !x.Empty() {
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
	// classe 4 = todas as profissões; construção 1 = mais Força
	for c := 0; c < LevelItemClasses; c++ {
		if x := tab.Para(c, 0, 50); x.Index != 900 {
			t.Errorf("classe %d de Força no 50 = %d, queria 900", c, x.Index)
		}
		if x := tab.Para(c, 1, 50); !x.Empty() {
			t.Errorf("classe %d de Int no 50 devia estar vazia", c)
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
	// -1 é "independe da CLASSE" pelo cabeçalho, então alcança as quatro
	for c := 0; c < LevelItemClasses; c++ {
		for b := 0; b < LevelItemBuilds; b++ {
			if x := tab.Para(c, b, 149); x.Index != 2368 {
				t.Errorf("classe %d construção %d no 149 = %d, queria a montaria 2368", c, b, x.Index)
			}
		}
	}
	// -1 é tratado, não cai no balde de "não sei o que é isto"
	if tab.ConstrucaoIndefinida != 0 {
		t.Errorf("construções indefinidas = %d, queria 0: o -1 tem significado", tab.ConstrucaoIndefinida)
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
	if x := tab.Para(1, 1, 80); x.Index != 333 {
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
	if x := tab.Para(0, 0, 29); x.Index != 1147 {
		t.Error("a linha boa antes da torta se perdeu")
	}
	if x := tab.Para(0, 0, 31); x.Index != 1148 {
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
	// a montaria do 149 é a razão de a divergência do -1 existir
	for b := 0; b < LevelItemBuilds; b++ {
		if x := tab.Para(0, b, 149); x.Index != 2368 {
			t.Errorf("a montaria do nível 149 não chega ao TK de construção %d (veio %d)", b, x.Index)
		}
	}
	if tab.ConstrucaoIndefinida != 0 {
		t.Errorf("construções indefinidas = %d, queria 0: o -1 tem significado", tab.ConstrucaoIndefinida)
	}

	// A LEITURA ESCOLHIDA, presa aqui.
	//
	// O arquivo numera a construção pelo próprio cabeçalho — 1 = mais Força,
	// 2 = mais Int, 3 = mais Destreza, 0 = independe dos pontos, -1 = independe da
	// classe — e as seções confirmam em palavras: "#TK DN SET MALHA" (o físico)
	// vem com 1, "#TK MAG SET MALHA" (o mágico) com 2.
	//
	// Lendo pela régua do DoItemLevel (0 = Força, 1 = Int, 2 = Destreza) o
	// resultado seria outro, e claramente acidental: TK de Força 1 nível de 23,
	// Foema e BM de Força ZERO, e a Huntress de Destreza ZERO — a classe de
	// Destreza sem receber nada. Fica registrado para quem vier conferir.
	quer := [LevelItemClasses][LevelItemBuilds]int{
		{23, 23, 1, 1},   // TK
		{23, 23, 1, 1},   // Foema
		{23, 23, 1, 1},   // BM
		{23, 21, 23, 21}, // Huntress
	}
	nomes := [LevelItemClasses]string{"TK", "Foema", "BM", "Huntress"}
	cons := [LevelItemBuilds]string{"Força", "Int", "Destreza", "resto"}
	for c := 0; c < LevelItemClasses; c++ {
		for b := 0; b < LevelItemBuilds; b++ {
			n := 0
			for nv := int32(0); nv < 400; nv++ {
				if !tab.Para(c, b, nv).Empty() {
					n++
				}
			}
			if n != quer[c][b] {
				t.Errorf("%s de %s recebe em %d níveis, esperado %d", nomes[c], cons[b], n, quer[c][b])
			}
		}
	}
}
