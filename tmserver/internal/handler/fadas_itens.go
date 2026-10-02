package handler

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A ABA "ITENS" do Painel de Drop das Fadas: o caminho inverso da lista de
// monstros. O jogador procura um ITEM pelo nome e vê QUAIS MONSTROS o dão; dali,
// um clique num monstro abre os drops dele.
//
// Só entra na busca o item que algum monstro do catálogo pode dar. A ordem das
// duas listas é de nome, nunca de chance: a chance continua não saindo daqui.

// fadaItem é um item que algum monstro dá, com o nome para mostrar e para procurar.
type fadaItem struct {
	indice int16
	nome   string // o nome do catálogo de itens
	busca  string // sem acento e em minúsculas
}

// fadasMontaIndice refaz, junto com o catálogo de monstros, o índice inverso:
// para cada item, os monstros que o dão.
func (d *Dispatcher) fadasMontaIndice(w *world.World, c *fadaCatalogo) {
	c.quemDropa = map[int16][]int{}
	for i := range c.lista {
		m := &c.lista[i]
		molde, mesa, especial := d.fadasFontes(w, m)
		todos := slices.Concat(molde, mesa, especial)
		slices.Sort(todos)
		for _, idx := range slices.Compact(todos) {
			c.quemDropa[idx] = append(c.quemDropa[idx], i)
		}
	}
	c.itens = c.itens[:0]
	for idx := range c.quemDropa {
		nome := d.itemNames[int(idx)]
		if nome == "" {
			continue // sem nome no catálogo não há o que procurar
		}
		c.itens = append(c.itens, fadaItem{indice: idx, nome: nome, busca: fadasSemAcento(nome)})
	}
	slices.SortFunc(c.itens, func(a, b fadaItem) int {
		if n := strings.Compare(a.busca, b.busca); n != 0 {
			return n
		}
		return int(a.indice) - int(b.indice)
	})
}

// fadasBaseDoItem é o que o sorteio de adicionais precisa saber do item.
func (d *Dispatcher) fadasBaseDoItem(idx int16) refine.Base {
	i := int(idx)
	return refine.Base{
		Unique:  d.itemUnique[i],
		ReqLvl:  int(d.itemReqs[i].Lvl),
		Pos:     d.itemPos[i],
		Efeitos: d.itemEffects[i],
		Indice:  i,
	}
}

func (d *Dispatcher) fadasPossiveis() *refine.Possiveis {
	if d.possiveis == nil || d.possiveis.Tabelas() != d.dropBonus {
		d.possiveis = refine.NovoPossiveis(d.dropBonus)
	}
	return d.possiveis
}

// fadasFaixaLarga é a faixa de adicional de um item SEM monstro escolhido (aba
// Filtro, e o item recém-escolhido na aba Itens): a mais larga entre todos os
// monstros que o dão — o menor dos mínimos e o maior dos máximos de cada efeito.
func (d *Dispatcher) fadasFaixaLarga(w *world.World, c *fadaCatalogo, idx int16) []protocol.FadasFaixa {
	var out []protocol.FadasFaixa
	for _, i := range c.quemDropa[idx] {
		m := &c.lista[i]
		molde, mesa, especial := d.fadasFontes(w, m)
		out = uneFaixas(out, d.fadasFaixasDoPar(m, idx, molde, mesa, especial))
	}
	return out
}

// fadasMandaFaixasLargas manda o 0x0F76 das faixas largas de uma lista de itens.
func (d *Dispatcher) fadasMandaFaixasLargas(w *world.World, s *world.Session, itens []int16) {
	c := d.fadasCatalogo(w)
	var linhas []protocol.FadasFaixasItem
	for _, idx := range itens {
		if f := d.fadasFaixaLarga(w, c, idx); len(f) > 0 {
			linhas = append(linhas, protocol.FadasFaixasItem{Item: idx, Faixas: f})
		}
	}
	w.Send(s, protocol.MsgFadasFaixas, protocol.EncodeFadasFaixas(c.versao, protocol.FadasFaixasLargas, linhas))
}

// fadasMandaItens responde com uma página dos itens cujo nome bate com a busca.
func (d *Dispatcher) fadasMandaItens(w *world.World, s *world.Session, corpo protocol.FadasPedeBody) {
	c := d.fadasCatalogo(w)
	var achados []int
	if texto := fadasSemAcento(corpo.Texto); len(texto) >= fadasBuscaMin {
		for i := range c.itens {
			if strings.Contains(c.itens[i].busca, texto) {
				achados = append(achados, i)
			}
		}
	}
	resp := protocol.FadasItensBody{Versao: c.versao, Pagina: corpo.Pagina, Total: uint16(min(len(achados), 0xFFFF))}
	if ultima := max(0, (len(achados)-1)/protocol.FadasMonstrosPorPagina); int(resp.Pagina) > ultima {
		resp.Pagina = uint8(ultima)
	}
	ini := min(int(resp.Pagina)*protocol.FadasMonstrosPorPagina, len(achados))
	fim := min(ini+protocol.FadasMonstrosPorPagina, len(achados))
	for _, i := range achados[ini:fim] {
		it := &c.itens[i]
		resp.Itens = append(resp.Itens, protocol.FadasItem{Indice: it.indice, Nome: fadasParaOFio(it.nome)})
	}
	w.Send(s, protocol.MsgFadasItens, resp.Encode())
}

// fadasMandaQuemDropa responde com uma página dos monstros que dão um item, em
// ordem de nome, e com a faixa larga de adicional daquele item.
func (d *Dispatcher) fadasMandaQuemDropa(w *world.World, s *world.Session, corpo protocol.FadasPedeBody) {
	c := d.fadasCatalogo(w)
	item := int16(corpo.Monstro) // o campo do pedido leva o índice do item
	achados := c.quemDropa[item]
	resp := protocol.FadasMonstrosBody{
		Versao: c.versao, Tipo: corpo.Tipo, Pagina: corpo.Pagina,
		Total: uint16(min(len(achados), 0xFFFF)),
	}
	if ultima := max(0, (len(achados)-1)/protocol.FadasMonstrosPorPagina); int(resp.Pagina) > ultima {
		resp.Pagina = uint8(ultima)
	}
	ini := min(int(resp.Pagina)*protocol.FadasMonstrosPorPagina, len(achados))
	fim := min(ini+protocol.FadasMonstrosPorPagina, len(achados))
	for _, i := range achados[ini:fim] {
		m := &c.lista[i]
		resp.Monstros = append(resp.Monstros, protocol.FadasMonstro{
			Numero: uint16(i), Tipo: uint8(m.lugar.Tipo), Nome: m.nome, Regiao: m.lugar.Nome,
		})
	}
	w.Send(s, protocol.MsgFadasMonstros, resp.Encode())
	d.fadasMandaFaixasLargas(w, s, []int16{item})
}

// fadasParaOFio leva um nome do catálogo de itens aos bytes que o cliente
// desenha: o catálogo pode estar em UTF-8 ou já nos bytes CP1252 do arquivo.
func fadasParaOFio(s string) string {
	if !utf8.ValidString(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			r = '?'
		}
		out = append(out, byte(r))
	}
	return string(out)
}
