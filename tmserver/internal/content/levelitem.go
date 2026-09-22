package content

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// A tabela de itens por nível (TMsrv/run/LevelItem.txt, lida por
// ReadLevelItemConfig em Server.cpp:9669 para LevelItem[4][4][400]).
//
// O jogo entrega uma peça ao subir de nível, escolhida por três coisas: o nível
// alcançado, a classe do personagem e a CONSTRUÇÃO dele — qual atributo base é o
// maior. O jogador não escolhe nada; quem decide é a ficha.
const (
	// LevelItemClasses são as quatro profissões (0=TK, 1=Foema, 2=BM, 3=Huntress).
	LevelItemClasses = 4
	// LevelItemBuilds são as quatro construções: 0 mais Força, 1 mais Int,
	// 2 mais Destreza, 3 o resto (Const, ou empate).
	LevelItemBuilds = 4
	// levelItemMaxLevel é o tamanho do vetor do legado (LevelItem[...][400]).
	levelItemMaxLevel = 400

	// levelItemTodas é o coringa da classe: 4 vale para as quatro profissões.
	levelItemTodas = 4
	// levelItemIndependeDosPontos é o 0 da construção, pelo cabeçalho do arquivo:
	// a peça vale para qualquer construção.
	levelItemIndependeDosPontos = 0
	// levelItemIndependeDaClasse é o -1 da construção, também pelo cabeçalho: a
	// peça vale para qualquer classe e qualquer construção.
	levelItemIndependeDaClasse = -1
)

// LevelItem é a peça que um nível entrega: o índice do catálogo e os três pares
// de (efeito, valor) que o arquivo escreve junto.
type LevelItem struct {
	Index   int16
	Effects [3][2]uint8
}

// Empty relata que aquele (classe, construção, nível) não entrega nada.
func (li LevelItem) Empty() bool { return li.Index == 0 }

// LevelItems é a tabela inteira, mais o que se aprendeu lendo o arquivo.
type LevelItems struct {
	itens [LevelItemClasses][LevelItemBuilds][levelItemMaxLevel]LevelItem
	// Sobrescritas conta quantas CASAS (classe, construção, nível) uma linha
	// posterior apagou — é por casa, não por linha: uma linha de "todas as
	// construções" cobre quatro casas de uma vez. No arquivo que veio no
	// conteúdo, lido pelo cabeçalho, são 236 casas sobrescritas por 318 linhas
	// (conferido pelo próprio carregador em 14/09/2026). O legado tem o
	// mesmo comportamento e não conta; contar aqui é o que tira o desperdício da
	// invisibilidade, sem mudar o que o jogo entrega.
	Sobrescritas int
	// Linhas é quantas linhas viraram entrada.
	Linhas int
	// ConstrucaoIndefinida conta as linhas com a construção fora de 0..4 — no
	// arquivo de hoje são as duas do -1. Ver o comentário em aplicar.
	ConstrucaoIndefinida int
}

// Para devolve a peça de um (classe, construção, nível), ou uma vazia.
func (t *LevelItems) Para(classe, construcao int, nivel int32) LevelItem {
	if t == nil || classe < 0 || classe >= LevelItemClasses ||
		construcao < 0 || construcao >= LevelItemBuilds ||
		nivel < 0 || int(nivel) >= levelItemMaxLevel {
		return LevelItem{}
	}
	return t.itens[classe][construcao][nivel]
}

// aplicar grava uma linha nas casas que ela cobre, contando as sobreposições.
//
// DUAS DIVERGÊNCIAS DELIBERADAS, e são da mesma família: o arquivo diz uma
// coisa, o código do legado faz outra, e aqui se segue o arquivo quando o
// resultado do código é claramente acidental.
//
// A PRIMEIRA é a numeração da construção. O cabeçalho do arquivo diz 1 = mais
// Força, 2 = mais Int, 3 = mais Destreza, 0 = independe dos pontos — e as seções
// confirmam em palavras: "#TK DN SET MALHA" (o set físico) vem com 1, "#TK MAG
// SET MALHA" (o mágico) com 2. O DoItemLevel do legado numera outra coisa
// (0 = Força, 1 = Int, 2 = Destreza, 3 = o resto), e lendo o arquivo por essa
// régua o set FÍSICO vai para quem tem mais Int, quem tem mais Força quase não
// recebe, e a Huntress — a classe de Destreza — fica com ZERO se for construída
// em Destreza. Isso não é a intenção de ninguém; é um desencontro entre quem
// escreveu o conteúdo e quem escreveu o código.
//
// A SEGUNDA é o -1. O original não o trata: a linha cai no ramo final e escreve
// em LevelItem[classe][-1][nivel] — FORA DO VETOR. O cabeçalho diz que -1
// significa "independe da classe", e é assim que se lê: reproduzir uma escrita em
// memória alheia não é fidelidade. O arquivo de hoje NÃO usa mais o -1 — as duas
// linhas que o usavam eram as do Cavalo Leve do nível 149, e a montaria de 149
// passou a ser uma por classe (21/09/2026). O tratamento fica: é a defesa contra
// uma linha futura, e o teste do arquivo cobre as duas leituras.
func (t *LevelItems) aplicar(classe, construcao int, nivel int32, item LevelItem) {
	if nivel < 0 || int(nivel) >= levelItemMaxLevel {
		return
	}
	todas := []int{0, 1, 2, 3}
	classes := []int{classe}
	// -1 é "independe da classe", pelo cabeçalho; 4 é o coringa que o código usa
	if classe == levelItemTodas || construcao == levelItemIndependeDaClasse {
		classes = todas
	}
	var construcoes []int
	switch construcao {
	case levelItemIndependeDosPontos, levelItemTodas, levelItemIndependeDaClasse:
		construcoes = todas
	case 1, 2, 3: // Força, Int, Destreza — o arquivo conta a partir de 1
		construcoes = []int{construcao - 1}
	default:
		t.ConstrucaoIndefinida++
		construcoes = todas
	}
	for _, c := range classes {
		if c < 0 || c >= LevelItemClasses {
			continue
		}
		for _, b := range construcoes {
			if !t.itens[c][b][nivel].Empty() {
				t.Sobrescritas++
			}
			t.itens[c][b][nivel] = item
		}
	}
}

// LoadLevelItems lê TMsrv/run/LevelItem.txt.
//
// Cada linha tem onze números: sequência, nível, classe, construção, item e três
// pares de (efeito, valor). A sequência o legado lê e joga fora — é número de
// linha, não serve para nada, e aqui também não. Linhas em branco e as que
// começam com '#' são comentário.
//
// Uma linha malformada é pulada com aviso em vez de derrubar o boot: o arquivo é
// escrito à mão, tem 547 linhas, e perder o servidor inteiro por uma vírgula
// trocada numa linha de recompensa seria pior do que perder aquela recompensa.
func LoadLevelItems(path string) (*LevelItems, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("content: open LevelItem: %w", err)
	}
	defer f.Close()

	t := &LevelItems{}
	var avisos []string
	sc := bufio.NewScanner(f)
	for linha := 1; sc.Scan(); linha++ {
		texto := strings.TrimSpace(sc.Text())
		if texto == "" || strings.HasPrefix(texto, "#") || strings.HasPrefix(texto, "//") {
			continue
		}
		campos := strings.Fields(texto)
		if len(campos) < 5 {
			avisos = append(avisos, fmt.Sprintf("linha %d: %d números, precisa de pelo menos 5", linha, len(campos)))
			continue
		}
		nums := make([]int, 0, 11)
		ruim := false
		for i := 0; i < len(campos) && i < 11; i++ {
			n, err := strconv.Atoi(campos[i])
			if err != nil {
				avisos = append(avisos, fmt.Sprintf("linha %d: %q não é número", linha, campos[i]))
				ruim = true
				break
			}
			nums = append(nums, n)
		}
		if ruim {
			continue
		}
		for len(nums) < 11 { // o legado deixa o que faltar em zero (memset + sscanf parcial)
			nums = append(nums, 0)
		}
		item := LevelItem{Index: int16(nums[4])}
		for i := 0; i < 3; i++ {
			item.Effects[i] = [2]uint8{uint8(nums[5+2*i]), uint8(nums[6+2*i])}
		}
		t.aplicar(nums[2], nums[3], int32(nums[1]), item)
		t.Linhas++
	}
	if err := sc.Err(); err != nil {
		return nil, avisos, fmt.Errorf("content: read LevelItem: %w", err)
	}
	return t, avisos, nil
}
