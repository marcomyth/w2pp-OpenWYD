package handler

import (
	"sort"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Loja do Servidor: a vitrine global das barracas abertas.
//
// A lojinha pessoal continua sendo a de sempre (autotrade.go): o jogador monta a
// barraca, ela fica de pé no mapa e vende do Cargo da conta. A vitrine não é
// outra loja — é uma leitura de todas as barracas abertas neste instante, para
// que ninguém precise andar pela cidade procurando.
//
// Por isso nada aqui é guardado: a lista é montada na hora, a cada pedido, e uma
// barraca que fecha some da vitrine no mesmo instante. Também não há custódia de
// item: o que está à venda continua no Cargo do vendedor, como sempre esteve.
//
// As barracas ficam todas na cidade (Armia, na prática), e é por isso que a
// vitrine é a lista da cidade de quem pergunta: um agrupamento do que já está
// aberto por perto, em vez de obrigar a andar de barraca em barraca. A compra
// continua sendo a do jogo (MSG_ReqBuy), com a checagem de distância de sempre;
// como a cidade é maior que esse alcance, cada oferta vai marcada com Perto, e o
// painel mostra o que dá para levar sem sair do lugar.
//
// A moeda é escolha do vendedor: a janela de barraca do cliente só sabe de ouro,
// então ele monta a barraca como sempre e depois marca, pelo painel, em que
// moeda cada item é cobrado (MsgLojaMoeda). Quem compra paga naquela moeda.

// lojaRefino e lojaQuantidade leem do item o que o painel mostra: o "+N" vem de
// EF_SANC e a pilha de EF_AMOUNT, com a mesma leitura do resto do servidor
// (itemAmount trata item sem EF_AMOUNT como um).
func lojaRefino(it world.Item) uint8 {
	for _, ef := range it.Effects {
		if ef.Effect == efSanc {
			return ef.Value
		}
	}
	return 0
}

func lojaQuantidade(it world.Item) uint8 {
	n := itemAmount(it)
	if n < 1 {
		n = 1
	}
	if n > 255 {
		n = 255
	}
	return uint8(n)
}

// lojaOfertasAbertas percorre as barracas abertas e devolve uma oferta por item
// à venda, em ordem estável (barraca, depois posição) para a paginação não
// embaralhar entre um pedido e outro.
func lojaOfertasAbertas(w *world.World, quem *world.Session, eu *world.Entity,
	filtro int16) []protocol.LojaOferta {
	cidade := world.Village(eu.X, eu.Y)
	var ofertas []protocol.LojaOferta
	w.ForEachSession(func(s *world.Session, e *world.Entity) {
		if s == nil || s.AutoTrade == nil || e == nil {
			return
		}
		if filtro == protocol.LojaFiltroMeus && s.Conn != quem.Conn {
			return
		}
		// Só a cidade de quem pergunta: a vitrine é o agrupamento das barracas
		// abertas ali, não um mercado entre cidades.
		if world.Village(e.X, e.Y) != cidade {
			return
		}
		perto := uint8(0)
		if autoTradeInRange(eu, e) {
			perto = 1
		}
		barraca := int32(shopStallID(s))
		for i := range s.AutoTrade.Slots {
			sl := s.AutoTrade.Slots[i]
			if sl.CargoPos < 0 || sl.Item.Empty() {
				continue
			}
			moeda := s.AutoTrade.Moeda[i]
			if !lojaPassaNoFiltro(filtro, moeda) {
				continue
			}
			ofertas = append(ofertas, protocol.LojaOferta{
				Vendedor: barraca,
				Nome:     e.Name,
				Indice:   sl.Item.Index,
				Slot:     int8(i),
				Refino:   lojaRefino(sl.Item),
				Qtd:      lojaQuantidade(sl.Item),
				Moeda:    moeda,
				Perto:    perto,
				Preco:    sl.Price,
			})
		}
	})
	sort.Slice(ofertas, func(a, b int) bool {
		if ofertas[a].Vendedor != ofertas[b].Vendedor {
			return ofertas[a].Vendedor < ofertas[b].Vendedor
		}
		return ofertas[a].Slot < ofertas[b].Slot
	})
	return ofertas
}

func lojaPassaNoFiltro(filtro int16, moeda uint8) bool {
	switch filtro {
	case protocol.LojaFiltroOuro:
		return moeda == protocol.LojaMoedaOuro
	case protocol.LojaFiltroCash:
		return moeda == protocol.LojaMoedaCash
	case protocol.LojaFiltroRMT:
		return moeda == protocol.LojaMoedaRMT
	default:
		return true
	}
}

// lojaPede responde MsgLojaPede com uma página da vitrine.
func (d *Dispatcher) lojaPede(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	var pede protocol.LojaPedeBody
	if err := pede.Decode(payload); err != nil {
		return
	}

	ofertas := lojaOfertasAbertas(w, s, e, pede.Filtro)
	total := len(ofertas)
	paginas := (total + protocol.LojaPorPagina - 1) / protocol.LojaPorPagina
	if paginas < 1 {
		paginas = 1
	}
	pagina := int(pede.Pagina)
	if pagina < 0 {
		pagina = 0
	}
	if pagina >= paginas {
		pagina = paginas - 1
	}

	corpo := protocol.LojaListaBody{
		Pagina:  int16(pagina),
		Paginas: int16(paginas),
		Total:   int16(total),
		Ouro:    e.Coin,
	}
	inicio := pagina * protocol.LojaPorPagina
	for i := 0; i < protocol.LojaPorPagina && inicio+i < total; i++ {
		corpo.Ofertas[i] = ofertas[inicio+i]
		corpo.Qtd++
	}
	w.SendTo(s, protocol.Header{Type: protocol.MsgLojaLista, ID: protocol.IDScene}, corpo.Encode())
}

// lojaMoeda atende MsgLojaMoeda: o dono da barraca diz em que moeda um item dela
// é cobrado. Mexe só na própria barraca, e só num slot que exista.
func (d *Dispatcher) lojaMoeda(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay || s.AutoTrade == nil {
		return
	}
	var corpo protocol.LojaMoedaBody
	if err := corpo.Decode(payload); err != nil {
		return
	}
	if corpo.Slot < 0 || int(corpo.Slot) >= len(s.AutoTrade.Slots) {
		return
	}
	if corpo.Moeda > protocol.LojaMoedaRMT {
		return
	}
	sl := s.AutoTrade.Slots[corpo.Slot]
	if sl.CargoPos < 0 || sl.Item.Empty() {
		return
	}
	s.AutoTrade.Moeda[corpo.Slot] = corpo.Moeda
}
