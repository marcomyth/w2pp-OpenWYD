package handler

import (
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

// lojaOfertasAbertas é a vitrine DO JOGO: as mesmas prateleiras, estreitadas pelo
// filtro de moeda e pelo "só as minhas".
func lojaOfertasAbertas(w *world.World, quem *world.Session, filtro int16) []protocol.LojaOferta {
	var ofertas []protocol.LojaOferta
	for _, o := range world.OfertasDoMercado(w) {
		if filtro == protocol.LojaFiltroMeus &&
			(quem == nil || quem.AccountID != o.ContaVendedor) {
			continue
		}
		if !lojaPassaNoFiltro(filtro, o.Moeda) {
			continue
		}
		ofertas = append(ofertas, protocol.LojaOferta{
			Vendedor: o.Barraca,
			Nome:     o.Personagem,
			Indice:   o.Indice,
			Slot:     o.Slot,
			Refino:   o.Refino,
			Qtd:      o.Qtd,
			Moeda:    o.Moeda,
			Perto:    o.Perto,
			Preco:    o.Preco,
		})
	}
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
// --- quem guarda a vitrine é o painel ---------------------------------------
//
// O servidor não guarda vitrine nenhuma. A lista é montada na hora de cada
// pedido e jogada fora; o que fica guardado é o que o painel do jogador já tem
// na tela, e esse cache vive na sessão dele: acabou a sessão, acabou o cache
// (decisão da Josiel, 20/09/2026).
//
// O painel também não pergunta de tempos em tempos. Quando o mercado muda -
// abrir barraca, fechar, comprar, trocar a moeda de um item -, o servidor manda
// um bilhete de quatro bytes com a versão nova, e o painel decide se vale pedir
// de novo: só a página visível, e com freio de um segundo.
//
// A conta que justifica o bilhete, medida num processo cheio (999 barracas de
// doze prateleiras, doze mil ofertas): empurrar a página pronta custava 1,26 ms
// e 2,4 MB POR OBSERVADOR, isto é 1,25 s de laço parado e 2,4 GB de lixo em cada
// compra. O laço é de uma linha só - nesse tempo ninguém anda nem bate -, e a
// fila de saída tem 512 quadros: enchendo, o servidor derruba a conexão.
//
// Chamado de onde o mercado muda de forma: lojaAbrir, closeAutoTrade,
// lojaCompra e lojaMoeda.
func (d *Dispatcher) mercadoMudou(w *world.World) {
	d.mercadoVersao++
	if w == nil {
		return
	}
	corpo := protocol.LojaMudouBody{Versao: d.mercadoVersao}.Encode()
	w.ForEachSession(func(s *world.Session, e *world.Entity) {
		if s == nil || e == nil || !s.LojaAberta {
			return
		}
		w.SendTo(s, protocol.Header{Type: protocol.MsgLojaMudou, ID: protocol.IDScene}, corpo)
	})
}

// lojaFecha atende MsgLojaFecha: o painel fechou e o servidor para de avisar
// aquela sessão. Sem isto o aviso continuaria indo para quem nem está olhando.
func (d *Dispatcher) lojaFecha(_ *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	s.LojaAberta = false
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
	// A carteira é relida ANTES de a vitrine ir para a tela: é aqui que o jogador
	// olha o saldo para decidir o que consegue pagar, e era aqui que ele lia o
	// número do login. A entidade é buscada DE NOVO na volta — entre o pedido e a
	// resposta do banco o jogador pode ter morrido, trocado de personagem ou saído.
	pagina, filtro := pede.Pagina, pede.Filtro
	d.relerCash(w, s, func(w *world.World, s *world.Session) {
		e := w.Entity(s.Conn)
		if e == nil || s.Mode != world.UserPlay {
			return
		}
		d.mandaVitrine(w, s, e, pagina, filtro)
	})
}

// mandaVitrine monta e envia uma pagina da vitrine. Serve ao pedido do painel e
// ao aviso que o servidor manda quando o mercado muda.
func (d *Dispatcher) mandaVitrine(w *world.World, s *world.Session, e *world.Entity,
	qualPagina, filtro int16) {
	// Montada agora e jogada fora: o cache e do painel, na sessao dele.
	ofertas := lojaOfertasAbertas(w, s, filtro)

	// Duas passadas pela lista, sem copia-la: a primeira conta o que o filtro
	// deixa entrar, a segunda recorta a pagina pedida. Copiar a lista filtrada
	// para dela tirar 32 ofertas custava 1,26 ms e 2,4 MB por pedido; assim sao
	// 28 us e dois quilos de bytes.
	total := 0
	for i := range ofertas {
		if lojaPassaNoFiltro(filtro, ofertas[i].Moeda) {
			total++
		}
	}
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
	vistas := 0
	for i := range ofertas {
		if !lojaPassaNoFiltro(filtro, ofertas[i].Moeda) {
			continue
		}
		if vistas >= inicio && int(corpo.Qtd) < protocol.LojaPorPagina {
			corpo.Ofertas[corpo.Qtd] = ofertas[i]
			corpo.Qtd++
		}
		vistas++
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
	// DINHEIRO REAL NÃO SE ESCOLHE DEPOIS.
	//
	// Trocar a moeda aqui mexe só num campo da barraca viva. Virar uma prateleira
	// de ouro em dinheiro real por este caminho pularia tudo o que a montagem faz
	// por um anúncio de verdade: a fotografia do item, a marca do escrow no slot
	// do baú, a conferência da chave Pix. O resultado seria uma oferta em reais
	// sem anúncio nenhum atrás dela.
	//
	// Então a moeda de dinheiro real só se escolhe na montagem, que é onde aquilo
	// tudo acontece. Para trocar, o vendedor remonta a barraca.
	if corpo.Moeda == protocol.LojaMoedaRMT {
		sendClientMessage(w, s, msgMoedaRMTSoNaMontagem)
		return
	}
	sl := s.AutoTrade.Slots[corpo.Slot]
	if sl.CargoPos < 0 || sl.Item.Empty() {
		return
	}
	// E NEM SE DESESCOLHE.
	//
	// O outro lado da mesma regra, e o que a torna útil. Virar para ouro uma
	// prateleira que tem anúncio vivo deixaria o vendedor com uma oferta que
	// NUNCA VENDE: a compra confere o cadeado do escrow e recusa, sempre, e o
	// jogador não teria como saber por quê. Pior, ele pensaria que vendeu em
	// ouro uma coisa que ainda está prometida por Pix.
	//
	// A recusa é aqui e não na compra porque aqui existe alguém para avisar, e
	// existe o que fazer a respeito: fechar a barraca cancela o anúncio e devolve
	// o item.
	if cargo := w.Cargo(s.AccountID); cargo != nil && sl.CargoPos < world.MaxCargo &&
		cargo.Items[sl.CargoPos].AnuncioRMT != 0 {
		sendClientMessage(w, s, msgAnuncioVivoNaPrateleira)
		return
	}
	s.AutoTrade.Moeda[corpo.Slot] = corpo.Moeda
	d.mercadoMudou(w) // a oferta mudou de moeda
}

// msgMoedaRMTSoNaMontagem é o que o vendedor lê ao tentar virar uma prateleira
// para dinheiro real com a barraca já de pé.
const msgMoedaRMTSoNaMontagem = "Para vender por dinheiro real, monte a barraca de novo escolhendo essa moeda."

// msgAnuncioVivoNaPrateleira é o que o vendedor lê ao tentar tirar de dinheiro
// real uma prateleira que já tem anúncio de pé.
const msgAnuncioVivoNaPrateleira = "Esse item está anunciado por dinheiro real. Feche a barraca para cancelar o anúncio."
