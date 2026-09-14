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

	// levelItemTodas é o coringa dos DOIS eixos: classe 4 vale para as quatro
	// profissões, construção 4 vale para as quatro construções.
	levelItemTodas = 4
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
	// Sobrescritas conta quantas linhas foram apagadas por uma linha posterior
	// que cai no mesmo (classe, construção, nível). No arquivo que veio no
	// conteúdo são 157 das 318 — quase metade dele não faz nada. O legado tem o
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
// DIVERGÊNCIA DELIBERADA DO LEGADO, e é a única aqui: o original trata a
// construção 4 como coringa e não olha mais nada, então uma linha com -1 cai no
// ramo final e escreve em LevelItem[classe][-1][nivel] — FORA DO VETOR. O
// arquivo que veio no conteúdo tem duas dessas, e são justamente as que entregam
// a montaria do nível 149 (item 2368). Reproduzir isso seria reproduzir uma
// escrita em memória alheia, que não é fidelidade, é defeito: o cabeçalho do
// próprio arquivo diz que -1 significa "independe", e é assim que se lê aqui.
// Sem essa leitura a montaria não chega a ninguém.
func (t *LevelItems) aplicar(classe, construcao int, nivel int32, item LevelItem) {
	if nivel < 0 || int(nivel) >= levelItemMaxLevel {
		return
	}
	classes := []int{classe}
	if classe == levelItemTodas {
		classes = []int{0, 1, 2, 3}
	}
	construcoes := []int{construcao}
	if construcao == levelItemTodas || construcao < 0 || construcao >= LevelItemBuilds {
		if construcao != levelItemTodas {
			t.ConstrucaoIndefinida++
		}
		construcoes = []int{0, 1, 2, 3}
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
