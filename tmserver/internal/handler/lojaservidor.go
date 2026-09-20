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
	var ofertas []protocol.LojaOferta
	w.ForEachSession(func(s *world.Session, e *world.Entity) {
		if s == nil || s.AutoTrade == nil || e == nil {
			return
		}
		if filtro == protocol.LojaFiltroMeus && (quem == nil || s.Conn != quem.Conn) {
			return
		}
		// O mercado é do servidor inteiro: barraca de qualquer cidade entra na
		// vitrine, e comprar não exige chegar perto (decisão da Josiel,
		// 19/09/2026). O que a distância ainda decide é o imposto, que se
		// reparte entre a cidade da barraca e a de quem compra — ver lojacompra.
		perto := uint8(1)
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
// --- o cache da vitrine -----------------------------------------------------
//
// Montar a lista varrendo todas as sessões a cada pedido não escala: o painel
// pergunta a cada poucos segundos, e o servidor cheio tem mil conexões com até
// doze prateleiras cada. A lista agora é montada UMA VEZ e reaproveitada até
// alguém mexer no mercado — abrir barraca, fechar, comprar ou trocar a moeda de
// um item. Quem mexe chama mercadoMudou.
//
// O cache guarda a lista sem filtro; filtrar por moeda é uma passada barata
// sobre ela. O que não entra no cache é o "meus itens", que depende de quem
// pergunta — esse continua sendo montado na hora, e é curto por natureza.
type mercadoCache struct {
	ofertas []protocol.LojaOferta
	valido  bool
}

// mercadoMudou invalida a vitrine e avisa quem esta com o painel aberto.
//
// É o contrário do que era: em vez de o cliente perguntar de poucos em poucos
// segundos — pergunta que, com mil jogadores, vira mil varreduras por segundo
// sem nada ter mudado —, o servidor manda a página nova no instante em que o
// mercado muda. Quem não está olhando não recebe nada.
//
// Chamado de onde o mercado muda de forma: lojaAbrir, closeAutoTrade,
// lojaCompra e lojaMoeda.
func (d *Dispatcher) mercadoMudou(w *world.World) {
	d.mercado.valido = false
	d.mercado.ofertas = nil
	if w == nil {
		return
	}
	w.ForEachSession(func(s *world.Session, e *world.Entity) {
		if s == nil || e == nil || !s.LojaAberta {
			return
		}
		d.mandaVitrine(w, s, e, s.LojaPagina, s.LojaFiltro)
	})
}

// lojaFecha atende MsgLojaFecha: o painel fechou e o servidor para de avisar
// aquela sessão. Sem isto o aviso continuaria indo para quem nem está olhando.
func (d *Dispatcher) lojaFecha(_ *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	s.LojaAberta = false
}

// mercadoOfertas devolve a lista do servidor inteiro, do cache quando ele ainda
// vale.
func (d *Dispatcher) mercadoOfertas(w *world.World) []protocol.LojaOferta {
	if d.mercado.valido {
		return d.mercado.ofertas
	}
	d.mercado.ofertas = lojaOfertasAbertas(w, nil, nil, protocol.LojaFiltroTodos)
	d.mercado.valido = true
	return d.mercado.ofertas
}

func (d *Dispatcher) lojaPede(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	var pede protocol.LojaPedeBody
	if err := pede.Decode(payload); err != nil {
		return
	}

	// Quem pede esta com o painel aberto: o servidor passa a avisa-lo quando o
	// mercado mudar, e ele nao precisa mais perguntar de tempos em tempos.
	s.LojaAberta = true
	s.LojaPagina = pede.Pagina
	s.LojaFiltro = pede.Filtro
	d.mandaVitrine(w, s, e, pede.Pagina, pede.Filtro)
}

// mandaVitrine monta e envia uma pagina da vitrine. Serve ao pedido do painel e
// ao aviso que o servidor manda quando o mercado muda.
func (d *Dispatcher) mandaVitrine(w *world.World, s *world.Session, e *world.Entity,
	qualPagina, filtro int16) {
	var ofertas []protocol.LojaOferta
	if filtro == protocol.LojaFiltroMeus {
		// A minha barraca é curta e só interessa a mim: não vale cache.
		ofertas = lojaOfertasAbertas(w, s, e, filtro)
	} else {
		for _, o := range d.mercadoOfertas(w) {
			if lojaPassaNoFiltro(filtro, o.Moeda) {
				ofertas = append(ofertas, o)
			}
		}
	}
	total := len(ofertas)
	paginas := (total + protocol.LojaPorPagina - 1) / protocol.LojaPorPagina
	if paginas < 1 {
		paginas = 1
	}
	pagina := int(qualPagina)
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
		Cash:    s.Cash,
		RMT:     s.Rmt,
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
	d.mercadoMudou(w)   // a oferta mudou de moeda
}
