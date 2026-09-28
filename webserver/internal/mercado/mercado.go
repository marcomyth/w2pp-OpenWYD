// Package mercado é a vitrine das barracas como uma página pública pode vê-la.
//
// TRÊS COISAS ACONTECEM AQUI, e nenhuma delas cabe em outro lugar:
//
//  1. A LISTA VEM DO JOGO, porque a barraca vive só na memória dele — na sessão do
//     vendedor, nunca persistida. Nenhuma leitura de banco a encontra.
//  2. O QUE JÁ TEM ALGUÉM PAGANDO SAI DA LISTA, e isso precisa do BANCO. O jogo não
//     sabe que existe cobrança aberta; o banco não enxerga a barraca. Juntar os dois
//     é o trabalho deste pacote.
//  3. A RESPOSTA FICA EM CACHE por alguns segundos, porque a página é pública e o
//     laço do jogo não pode ser interrompido uma vez por visitante.
//
// E o id da conta do vendedor morre aqui: ele entra na resposta do jogo, serve para
// achar o anúncio no banco, e não atravessa para o site.
package mercado

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// Oferta é uma prateleira, já sem nada que identifique conta.
type Oferta struct {
	Indice     int32
	Refino     int32
	Qtd        int32
	Moeda      int32
	Preco      int64
	Personagem string
	Cidade     string
	// AbertaHa é a duração desde que a barraca subiu, MEDIDA NA HORA DA RESPOSTA —
	// já somada com o tempo que a lista passou no cache.
	AbertaHa time.Duration
}

// Bruta é uma prateleira como o jogo a devolve, com o que não vai para o site.
type Bruta struct {
	ContaVendedor int64
	CargoPos      int32
	Personagem    string
	Indice        int32
	Refino        int32
	Qtd           int32
	Moeda         int32
	Preco         int64
	Cidade        string
	AbertaHa      time.Duration
}

// DoJogo é a chamada de controle ao servidor de jogo.
type DoJogo interface {
	ListarMercado(ctx context.Context) ([]Bruta, error)
}

// Banco é o que o filtro precisa saber.
type Banco interface {
	PrateleirasComCobrancaAberta(ctx context.Context) ([]store.PrateleiraOcupada, error)
}

// ValidadeDoCache é por quanto tempo a mesma lista serve a todo mundo.
//
// CINCO SEGUNDOS, e o número sai do que se perde de cada lado. Curto demais e cada
// visitante vira uma interrupção no laço do jogo, que é o processo que não pode
// esperar por nada. Longo demais e a página oferece o que já foi vendido — e o
// prejuízo aí não é do servidor, é da pessoa que clicou.
//
// Cinco segundos é menos do que alguém leva para escolher um item numa lista, então
// na prática a página mente por menos tempo do que o visitante gasta lendo.
const ValidadeDoCache = 5 * time.Second

// Mercado serve a vitrine, com cache.
type Mercado struct {
	jogo  DoJogo
	banco Banco
	log   *slog.Logger

	// O cache é protegido por mutex e NÃO pelo laço do jogo: quem espera aqui são
	// requisições HTTP do site, e elas não têm nada a ver com o laço.
	mu      sync.Mutex
	lista   []Oferta
	buscada time.Time
}

// Novo monta o serviço.
func Novo(jogo DoJogo, banco Banco, log *slog.Logger) *Mercado {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Mercado{jogo: jogo, banco: banco, log: log}
}

// Ofertas devolve a vitrine e há quanto tempo essa resposta foi buscada.
//
// A IDADE SAI JUNTO, e não é detalhe: a página tem o direito de ser alguns segundos
// velha, e o visitante não deve descobrir isso comprando algo que já foi. Quem mostra
// decide o que fazer com o número; quem responde não pode escondê-lo.
func (m *Mercado) Ofertas(ctx context.Context) ([]Oferta, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idade := time.Since(m.buscada)
	if m.lista != nil && idade < ValidadeDoCache {
		return m.comIdade(m.lista, idade), idade, nil
	}

	lista, err := m.buscar(ctx)
	if err != nil {
		// O CACHE VELHO É MELHOR DO QUE NADA quando o jogo não responde: uma página
		// de mercado vazia diz "ninguém está vendendo", que é uma afirmação falsa e
		// desanimadora. Servir cinco segundos a mais de uma lista que existiu não
		// engana ninguém do mesmo jeito.
		//
		// Mas só até um limite, e o limite é grosseiro de propósito: passado um
		// minuto, a lista velha vira mentira e o erro é a resposta honesta.
		if m.lista != nil && idade < time.Minute {
			m.log.WarnContext(ctx, "mercado: o jogo nao respondeu; servindo a lista velha",
				"idade_segundos", int(idade.Seconds()), "erro", err)
			return m.comIdade(m.lista, idade), idade, nil
		}
		return nil, 0, err
	}
	m.lista, m.buscada = lista, time.Now()
	return m.comIdade(lista, 0), 0, nil
}

// comIdade soma ao tempo de barraca o tempo que a lista passou guardada.
//
// SEM ISTO O CACHE CONGELA OS MINUTOS: a lista guardaria "montada há 3 minutos" e
// repetiria isso para todo visitante enquanto valesse. Somar a idade é o que
// transforma um número guardado num número verdadeiro na hora da leitura.
func (m *Mercado) comIdade(lista []Oferta, idade time.Duration) []Oferta {
	if idade <= 0 {
		return lista
	}
	fora := make([]Oferta, len(lista))
	copy(fora, lista)
	for i := range fora {
		fora[i].AbertaHa += idade
	}
	return fora
}

// buscar pergunta ao jogo, tira o que está sendo pago, e limpa o que não vai para o
// site.
func (m *Mercado) buscar(ctx context.Context) ([]Oferta, error) {
	brutas, err := m.jogo.ListarMercado(ctx)
	if err != nil {
		return nil, err
	}
	ocupadas, err := m.banco.PrateleirasComCobrancaAberta(ctx)
	if err != nil {
		// FALHOU A CONSULTA, NÃO RESPONDE. Sem ela, as prateleiras com alguém pagando
		// entrariam na lista, e a página convidaria um segundo comprador para uma
		// recusa. Uma página que falha é melhor do que uma que promete o que não pode
		// vender.
		return nil, err
	}
	fora := make(map[chavePrateleira]bool, len(ocupadas))
	for _, o := range ocupadas {
		fora[chavePrateleira{o.VendedorConta, int32(o.CargoSlot)}] = true
	}

	lista := make([]Oferta, 0, len(brutas))
	for _, b := range brutas {
		if b.Moeda == MoedaReal && fora[chavePrateleira{b.ContaVendedor, b.CargoPos}] {
			continue
		}
		lista = append(lista, Oferta{
			Indice:     b.Indice,
			Refino:     b.Refino,
			Qtd:        b.Qtd,
			Moeda:      b.Moeda,
			Preco:      b.Preco,
			Personagem: b.Personagem,
			Cidade:     b.Cidade,
			AbertaHa:   b.AbertaHa,
		})
	}
	return lista, nil
}

type chavePrateleira struct {
	conta int64
	slot  int32
}

// As moedas, com os números que o jogo usa.
const (
	MoedaOuro = 0
	MoedaCash = 1
	MoedaReal = 2
)

// Ordem é como a página pediu para ordenar.
type Ordem int

const (
	// OrdemDoJogo é a do servidor: barraca, depois slot. Estável e inútil para quem
	// compra, que é por isso que as outras existem.
	OrdemDoJogo Ordem = iota
	OrdemPrecoSobe
	OrdemPrecoDesce
	OrdemMaisNovas
)

// PorPagina é o tamanho de uma página, combinado com o site.
const PorPagina = 50

// Filtro é o que a página pediu.
type Filtro struct {
	Moeda  int32
	Item   int32
	Ordem  Ordem
	Pagina int32
}

// Filtrar aplica moeda, item, ordem e página sobre uma lista já pronta.
//
// FORA DO CACHE de propósito: o que é caro é perguntar ao jogo, e essa resposta serve
// a todas as combinações de filtro. Guardar uma lista por filtro multiplicaria as
// idas ao laço sem comprar nada.
//
// Devolve também o total que casou com os filtros ANTES do corte da página, que é o
// número de que a tela precisa para dizer "1 de 4".
func Filtrar(lista []Oferta, f Filtro) ([]Oferta, int) {
	casaram := make([]Oferta, 0, len(lista))
	for _, o := range lista {
		if o.Moeda != f.Moeda {
			continue
		}
		if f.Item != 0 && o.Indice != f.Item {
			continue
		}
		casaram = append(casaram, o)
	}

	// ORDENAÇÃO ESTÁVEL, e não é preciosismo: duas prateleiras do mesmo preço trocando
	// de lugar entre dois cliques fariam a paginação repetir uma e pular a outra.
	switch f.Ordem {
	case OrdemPrecoSobe:
		sort.SliceStable(casaram, func(a, b int) bool { return casaram[a].Preco < casaram[b].Preco })
	case OrdemPrecoDesce:
		sort.SliceStable(casaram, func(a, b int) bool { return casaram[a].Preco > casaram[b].Preco })
	case OrdemMaisNovas:
		sort.SliceStable(casaram, func(a, b int) bool { return casaram[a].AbertaHa < casaram[b].AbertaHa })
	}

	total := len(casaram)
	pagina := f.Pagina
	if pagina <= 0 {
		// Zero é lido como a primeira página: é a leitura indulgente de um campo que
		// alguém esqueceu de preencher, e está escrita no contrato.
		pagina = 1
	}
	inicio := int(pagina-1) * PorPagina
	if inicio >= total {
		// PÁGINA ALÉM DO FIM: lista vazia com o total de verdade, e nunca erro. A
		// lista encolhe entre dois cliques o tempo todo — uma barraca desce —, e quem
		// chegou aqui não fez nada errado.
		return nil, total
	}
	fim := inicio + PorPagina
	if fim > total {
		fim = total
	}
	return casaram[inicio:fim], total
}
