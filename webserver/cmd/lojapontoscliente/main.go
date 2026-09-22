// Command lojapontoscliente escreve no itemhelp.dat do cliente o aviso de que
// um item é vendido por PONTOS DE LOJINHA, para o jogador ver a moeda na
// própria janela da loja em vez de só na linha que o servidor manda no chat.
//
//	lojapontoscliente -cliente "C:\...\WYD-Cliente-Pronto" -dsn "$W2PP_DB_DSN"
//	lojapontoscliente -cliente "C:\..." -item 3343 -item 3344   # sem banco
//
// Com -dsn ele descobre sozinho o que marcar: lê os slots de loja de NPC que
// têm preço em pontos (npc_shop_item.price_points) e marca esses itens. É para
// rodar DEPOIS de abastecer a loja no painel, e de novo a cada mudança de
// estoque — não de preço.
//
// O aviso NÃO traz o número de pontos, de propósito. O itemhelp é por ITEM e
// estático: o preço vive no painel e muda quando a equipe quiser, então um
// número gravado aqui começaria a mentir no primeiro ajuste. Quem diz o preço é
// a linha que o servidor manda ao abrir a loja, que sai do banco toda vez.
//
// Pela mesma razão ele avisa alto quando um item marcado também está à venda
// por ouro: o texto vale para o item em TODO lugar do jogo — na bolsa, no chão,
// na loja do vendedor comum — e não só na prateleira da Loja de Pontos.
//
// A pasta do cliente é só LIDA. O que muda vai para -saida (por padrão uma
// pasta "gerado-lojapontos" dentro dela), como no itemnovocliente: os originais
// sobrevivem, um jogo aberto não atrapalha, e publicar pelo launcher continua
// sendo um passo à parte e consciente.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/clientitemhelp"
)

// avisoDeLoja é a linha acrescentada à descrição do item. Em vermelho, que é a
// cor que o cliente já usa para observação, e sem número — ver o cabeçalho.
const avisoDeLoja = "Vendido por pontos de lojinha."

type itens []int

func (l *itens) String() string { return fmt.Sprint(*l) }
func (l *itens) Set(v string) error {
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return fmt.Errorf("item %q inválido", v)
	}
	*l = append(*l, n)
	return nil
}

func main() {
	cliente := flag.String("cliente", "", "pasta do cliente original (só é lida)")
	saida := flag.String("saida", "", `pasta onde gravar (padrão: <cliente>\gerado-lojapontos`+`)`)
	dsn := flag.String("dsn", os.Getenv("W2PP_DB_DSN"), "PostgreSQL DSN; os itens saem das lojas cadastradas")
	remover := flag.Bool("remover", false, "tira o aviso dos itens em vez de pôr")
	var lista itens
	flag.Var(&lista, "item", "índice de item a marcar à mão (repita a opção); dispensa o -dsn")
	flag.Parse()
	if *cliente == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *saida == "" {
		*saida = filepath.Join(*cliente, "gerado-lojapontos")
	}
	if err := run(*cliente, *saida, *dsn, lista, *remover); err != nil {
		log.Fatal(err)
	}
}

func run(cliente, saida, dsn string, lista itens, remover bool) error {
	alvos, err := alvos(dsn, lista)
	if err != nil {
		return err
	}
	if len(alvos) == 0 {
		return fmt.Errorf("nenhum item para marcar: abasteça a Loja de Pontos no painel, ou passe -item")
	}

	origem := filepath.Join(cliente, "itemhelp.dat")
	data, err := os.ReadFile(origem)
	if err != nil {
		return fmt.Errorf("ler %s: %w", origem, err)
	}

	marcados, jaTinha := 0, 0
	for _, item := range alvos {
		antes, err := clientitemhelp.Get(data, item)
		if err != nil {
			return fmt.Errorf("ler a descrição do item %d: %w", item, err)
		}
		depois, mudou := aplicar(antes, remover)
		if !mudou {
			jaTinha++
			continue
		}
		if data, err = clientitemhelp.Set(data, item, depois); err != nil {
			return fmt.Errorf("gravar a descrição do item %d: %w", item, err)
		}
		marcados++
	}

	if err := os.MkdirAll(saida, 0o755); err != nil {
		return fmt.Errorf("criar %s: %w", saida, err)
	}
	destino := filepath.Join(saida, "itemhelp.dat")
	if err := os.WriteFile(destino, data, 0o644); err != nil {
		return fmt.Errorf("gravar %s: %w", destino, err)
	}

	verbo := "marcados"
	if remover {
		verbo = "limpos"
	}
	fmt.Printf("%d itens %s, %d já estavam como se queria\n", marcados, verbo, jaTinha)
	fmt.Printf("gravado em %s\n", destino)
	fmt.Println("o cliente original não foi tocado; publicar pelo launcher é um passo à parte")
	return nil
}

// aplicar põe ou tira a linha do aviso, preservando o resto da descrição.
// Devolve mudou=false quando não havia nada a fazer, para o comando poder ser
// rodado de novo sem duplicar linha nem reescrever o arquivo à toa.
func aplicar(antes []clientitemhelp.Linha, remover bool) ([]clientitemhelp.Linha, bool) {
	tem := -1
	for i, l := range antes {
		if strings.EqualFold(strings.TrimSpace(l.Texto), avisoDeLoja) {
			tem = i
			break
		}
	}
	if remover {
		if tem < 0 {
			return antes, false
		}
		return append(append([]clientitemhelp.Linha{}, antes[:tem]...), antes[tem+1:]...), true
	}
	if tem >= 0 {
		return antes, false
	}
	// No fim da descrição: o que o item É vem primeiro, como se paga por ele
	// vem depois.
	return append(append([]clientitemhelp.Linha{}, antes...),
		clientitemhelp.Linha{Cor: clientitemhelp.Vermelho, Texto: avisoDeLoja}), true
}

// alvos resolve a lista de itens: a passada à mão tem prioridade, e sem ela o
// banco responde quais são.
func alvos(dsn string, lista itens) ([]int, error) {
	if len(lista) > 0 {
		return dedup(lista), nil
	}
	if dsn == "" {
		return nil, fmt.Errorf("sem -dsn e sem -item: não há de onde tirar a lista")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := store.Pool(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("conectar: %w", err)
	}
	defer pool.Close()

	emPontos, err := store.New(pool).ItensEmPontos(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(emPontos))
	for _, it := range emPontos {
		out = append(out, int(it.ItemIndex))
		if it.TambemEmOuro {
			// Alto e por item, porque a escolha é da equipe e não do programa:
			// ou o item sai da loja de ouro, ou a Loja de Pontos vende um
			// clone só dela, ou se aceita o texto aparecendo nos dois lugares.
			fmt.Printf("AVISO: o item %d também é vendido por OURO em alguma loja; "+
				"o aviso do tooltip vale para ele em todo o jogo\n", it.ItemIndex)
		}
		if len(it.Precos) > 1 {
			fmt.Printf("nota: o item %d tem preços diferentes em pontos (%v) — "+
				"mais uma razão para o tooltip não trazer número\n", it.ItemIndex, it.Precos)
		}
	}
	return out, nil
}

func dedup(l itens) []int {
	visto := map[int]bool{}
	out := make([]int, 0, len(l))
	for _, n := range l {
		if visto[n] {
			continue
		}
		visto[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}
