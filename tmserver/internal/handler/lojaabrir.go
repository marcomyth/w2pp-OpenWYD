package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Montar a barraca pelo painel da Loja do Servidor.
//
// A janela de barraca do cliente está aposentada: ela só sabia de ouro, e o
// preço e a escolha dos itens vinham dela. Agora quem monta é o painel, e por
// isso precisa de duas coisas que só existem aqui: ver o cofre da conta e
// mandar montar.
//
// O pedido de montar NÃO leva o item, só a posição no cofre. Era daí que vinha
// metade das checagens do fluxo antigo (o memcmp anti-troca): se o servidor lê o
// item do próprio cofre, não há o que conferir contra o cliente. O resto das
// regras continua igual à de sempre — cidade, itens proibidos, EF_NOTRADE, teto
// de preço e pelo menos um item à venda.

// lojaCargo atende MsgLojaCargo: manda o cofre da conta, só os slots ocupados.
func (d *Dispatcher) lojaCargo(w *world.World, s *world.Session, _ protocol.Header, _ []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		return
	}
	var corpo protocol.LojaCargoListaBody
	for pos := 0; pos < world.MaxCargo && corpo.Qtd < protocol.LojaCargoMax; pos++ {
		it := cargo.Items[pos]
		if it.Empty() {
			continue
		}
		corpo.Itens[corpo.Qtd] = protocol.LojaCargoItem{
			Slot:   int16(pos),
			Indice: it.Index,
			Refino: lojaRefino(it),
			Qtd:    lojaQuantidade(it),
		}
		corpo.Qtd++
	}
	w.SendTo(s, protocol.Header{Type: protocol.MsgLojaCargoLista, ID: protocol.IDScene},
		corpo.Encode())
}

// lojaAbrir atende MsgLojaAbrir: monta a barraca com os itens, preços e moedas
// que o painel escolheu.
func (d *Dispatcher) lojaAbrir(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	// Não se monta barraca no meio de uma troca, nem com uma já aberta.
	if s.Trade.Active || s.TradeMode != 0 {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}
	village := world.Village(e.X, e.Y)
	if village < 0 || village > 4 || inAutoTradeForbiddenRect(e.X, e.Y) {
		d.notify(w, s, NoticeOnlyVillage)
		return
	}
	var pedido protocol.LojaAbrirBody
	if err := pedido.Decode(payload); err != nil {
		return
	}
	cargo := w.Cargo(s.AccountID)
	if cargo == nil {
		return
	}

	// Sem título, a barraca leva o nome do dono: no painel não há onde digitar —
	// o cliente lê o teclado por conta dele, e cada tecla é um atalho do jogo.
	titulo := pedido.Titulo
	if titulo == "" {
		titulo = e.Name
	}

	// Valida tudo antes de mexer em qualquer coisa.
	barraca := &world.AutoTradeState{Title: titulo, Tax: world.CityTax(village)}
	usados := map[int]bool{}
	for i := 0; i < protocol.MaxAutoTradeWire; i++ {
		p := pedido.Slots[i]
		barraca.Slots[i].CargoPos = -1
		if p.CargoPos < 0 {
			continue
		}
		pos := int(p.CargoPos)
		if pos >= world.MaxCargo {
			return
		}
		// Duas prateleiras apontando para o mesmo slot venderiam o item duas
		// vezes; a segunda venda sairia do vazio.
		if usados[pos] {
			return
		}
		item := cargo.Items[pos]
		if item.Empty() {
			return
		}
		if p.Preco <= 0 || p.Preco > autoTradePriceMax {
			return
		}
		if p.Moeda > protocol.LojaMoedaRMT {
			return
		}
		if autoTradeBlacklist[item.Index] {
			d.notify(w, s, NoticeCantAutoTrade)
			return
		}
		if d.itemAbility(item, efNoTrade) != 0 {
			d.notify(w, s, NoticeCantMoveItem)
			return
		}
		usados[pos] = true
		barraca.Slots[i].Item = item
		barraca.Slots[i].CargoPos = pos
		barraca.Slots[i].Price = p.Preco
		barraca.Moeda[i] = p.Moeda
	}
	if !shopStocked(barraca) {
		d.notify(w, s, NoticeCantAutoTrade)
		return
	}

	s.AutoTrade = barraca
	s.TradeMode = 1
	barraca.OpenedAt = w.Now()
	barraca.PaidUntil = barraca.OpenedAt
	// A barraca sobe primeiro: é ela que dá o id pelo qual os outros compram.
	d.raiseShopStall(w, s, e)
	if barraca.CloneID >= world.MaxUser {
		// A barraca fica de pé sozinha e o dono volta a jogar (decisão do Marco,
		// 17/09). O cliente fecha a janela de loja ao receber isto; a nossa nem
		// abre mais, mas manter o aviso custa nada e cobre um cliente antigo que
		// ainda tenha aquela janela na tela.
		w.Send(s, protocol.MsgQuitTrade, nil)
	}
	// O painel espera este aviso para fechar a tela de montagem e já mostrar a
	// barraca na vitrine. O id é o mesmo pelo qual os outros compram.
	aviso := protocol.LojaAbriuBody{Barraca: int32(shopStallID(s))}
	w.SendTo(s, protocol.Header{Type: protocol.MsgLojaAbriu, ID: protocol.IDScene}, aviso.Encode())
	d.log.Info("loja: barraca montada pelo painel", "conn", s.Conn, "titulo", barraca.Title,
		"imposto", barraca.Tax, "clone", barraca.CloneID)
}
