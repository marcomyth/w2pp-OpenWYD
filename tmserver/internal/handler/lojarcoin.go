package handler

import (
	"context"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Loja de Rcoin: a janela do cliente que vende, dentro do jogo, as ofertas da loja
// do site (donate_shop_item), pagas com a mesma carteira de donate. O contrato do
// fio está em protocol/lojarcoin.go e é congelado com o lado do cliente.
//
// O SERVIDOR NÃO GUARDA O CATÁLOGO. Cada página é lida do banco na hora, com o
// saldo junto: a staff muda preço e liga ou desliga oferta pelo painel, e uma cópia
// na memória do jogo mostraria a vitrine de ontem.
//
// A COMPRA É DO BANCO, e o jogo só pede e entrega. O dbServer debita e enfileira o
// item na mesma transação (store.BuyRcoinOffer), e a entrega é a da caixa postal
// — o mesmo caminho do site, do login e do "entregar agora" —, feita na hora,
// para o baú da conta.

// msgRcoinEntregue é o aviso depois de uma compra que coube inteira no baú. A
// janela desenha o resultado, mas não diz ONDE o item foi parar, e é isso que a
// pessoa vai procurar em seguida.
const msgRcoinEntregue = "Compra concluída: o item está no baú da conta."

// msgRcoinProximoLogin é o aviso quando a compra saiu mas a entrega imediata não
// pôde ser feita: o item está na caixa postal e o login o entrega.
const msgRcoinProximoLogin = "Compra concluída. O item chega ao baú no próximo login."

// rcoinPede atende MsgRcoinPede: uma página de uma aba.
func (d *Dispatcher) rcoinPede(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if s.Mode != world.UserPlay || s.AccountID == 0 || w.Entity(s.Conn) == nil {
		return
	}
	var pede protocol.RcoinPedeBody
	if err := pede.Decode(payload); err != nil {
		return
	}
	// Categoria fora da faixa vira página vazia, e não silêncio: o painel sabe
	// desenhar "nada aqui" e ficaria esperando uma resposta que não vem.
	if pede.Categoria < 0 || pede.Categoria > protocol.RcoinCategorias {
		d.mandaPaginaRcoin(w, s, nil, 0, s.Cash)
		return
	}
	conta, categoria, pagina := s.AccountID, int32(pede.Categoria), int(pede.Pagina)
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		ofertas, saldo, err := p.ListRcoinOffers(context.Background(), conta, categoria)
		return func(w *world.World, s *world.Session) {
			if s.Mode != world.UserPlay {
				return
			}
			if err != nil {
				d.log.Warn("loja de rcoin: lista falhou", "conta", conta, "categoria", categoria, "err", err)
				d.mandaPaginaRcoin(w, s, nil, 0, s.Cash)
				return
			}
			s.Cash = saldo
			d.mandaPaginaRcoin(w, s, ofertas, pagina, saldo)
		}
	})
}

// mandaPaginaRcoin recorta a página pedida e a envia. A página começa em 0 e é
// presa à faixa, como na Loja do Servidor: pedir além da última devolve a última.
func (d *Dispatcher) mandaPaginaRcoin(w *world.World, s *world.Session, ofertas []world.RcoinOferta, pagina int, saldo int32) {
	paginas := (len(ofertas) + protocol.RcoinPorPagina - 1) / protocol.RcoinPorPagina
	if paginas < 1 {
		paginas = 1
	}
	if pagina < 0 {
		pagina = 0
	}
	if pagina >= paginas {
		pagina = paginas - 1
	}
	corpo := protocol.RcoinListaBody{
		Pagina:  uint8(pagina),
		Paginas: uint8(min(paginas, 255)),
		Saldo:   saldo,
	}
	inicio := pagina * protocol.RcoinPorPagina
	for i := inicio; i < len(ofertas) && int(corpo.Qtd) < protocol.RcoinPorPagina; i++ {
		corpo.Ofertas[corpo.Qtd] = rcoinOfertaNoFio(ofertas[i])
		corpo.Qtd++
	}
	w.Send(s, protocol.MsgRcoinLista, corpo.Encode())
}

func rcoinOfertaNoFio(o world.RcoinOferta) protocol.RcoinOferta {
	var efs [3]protocol.Effect
	for i, ef := range o.Effects {
		efs[i] = protocol.Effect{Effect: ef.Effect, Value: ef.Value}
	}
	return protocol.RcoinOferta{
		ID:        uint32(o.ID),
		ItemIndex: o.ItemIndex,
		Efeitos:   efs,
		Categoria: o.Category,
		Preco:     o.Price,
		Dias:      uint16(max(0, min(o.Days, 65535))),
		Titulo:    o.Title,
	}
}

// rcoinCompra atende MsgRcoinCompra.
//
// DUAS TRAVAS, e cada uma responde a um clique repetido diferente:
//
//   - o MESMO pedido de uma compra já respondida devolve a resposta guardada, sem
//     ir ao banco — o cliente reenviou porque não viu a resposta, e cobrar de novo
//     seria cobrar duas vezes por um clique;
//   - qualquer compra enquanto outra espera o banco recebe OCUPADO. A trava vale
//     até o banco responder, e não até um prazo: o cliente desiste de esperar em
//     5 s, mas a resposta atrasada ainda chega, e uma trava que vencesse antes
//     dela deixaria a segunda compra passar junto com a primeira.
//
// A trava é a mesma DonateEmCurso da RCoin: as duas mexem na mesma carteira.
func (d *Dispatcher) rcoinCompra(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	if s.Mode != world.UserPlay || s.AccountID == 0 || w.Entity(s.Conn) == nil {
		return
	}
	var compra protocol.RcoinCompraBody
	if err := compra.Decode(payload); err != nil {
		return
	}
	if s.RcoinResposta != nil && compra.Pedido == s.RcoinPedido {
		w.Send(s, protocol.MsgRcoinResultado, s.RcoinResposta)
		return
	}
	if s.DonateEmCurso {
		w.Send(s, protocol.MsgRcoinResultado, protocol.RcoinResultadoBody{
			Resultado: protocol.RcoinOcupado, OfertaID: compra.OfertaID, Pedido: compra.Pedido, Saldo: s.Cash,
		}.Encode())
		return
	}
	if compra.OfertaID == 0 {
		w.Send(s, protocol.MsgRcoinResultado, protocol.RcoinResultadoBody{
			Resultado: protocol.RcoinIndisponivel, OfertaID: compra.OfertaID, Pedido: compra.Pedido, Saldo: s.Cash,
		}.Encode())
		return
	}
	s.DonateEmCurso = true

	conta := s.AccountID
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		r, err := p.BuyRcoinOffer(context.Background(), conta, int64(compra.OfertaID), compra.PrecoVisto)
		return func(w *world.World, s *world.Session) {
			s.DonateEmCurso = false
			resp := protocol.RcoinResultadoBody{OfertaID: compra.OfertaID, Pedido: compra.Pedido}
			if err != nil {
				// A compra pode ter acontecido do outro lado de um erro de rede. O
				// ERRO fica guardado para este pedido, então um reenvio não tenta
				// de novo às cegas; se o item foi comprado, ele está na caixa
				// postal e chega no próximo login.
				d.log.Warn("loja de rcoin: compra falhou", "conta", conta, "oferta", compra.OfertaID,
					"pedido", compra.Pedido, "err", err)
				resp.Resultado, resp.Saldo = protocol.RcoinErro, s.Cash
			} else {
				resp.Resultado, resp.Saldo = r.Result, r.Balance
				s.Cash = r.Balance
			}
			corpo := resp.Encode()
			s.RcoinPedido, s.RcoinResposta = compra.Pedido, corpo
			w.Send(s, protocol.MsgRcoinResultado, corpo)
			d.log.Info("loja de rcoin: compra", "conta", conta, "oferta", compra.OfertaID,
				"pedido", compra.Pedido, "preco_visto", compra.PrecoVisto,
				"resultado", resp.Resultado, "saldo", resp.Saldo, "entrega", r.DeliveryID)
			if err == nil && r.Result == protocol.RcoinOK {
				d.entregaRcoinAgora(w, s, conta)
			}
		}
	})
}

// entregaRcoinAgora esvazia a caixa postal da conta no baú, sem esperar o login.
// É o mesmo dreno do login e do "entregar agora" do site: o que não couber fica
// na fila, e o jogador é avisado para abrir espaço.
func (d *Dispatcher) entregaRcoinAgora(w *world.World, s *world.Session, conta int64) {
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		pendentes, err := p.ListPendingDeliveries(context.Background(), conta)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("loja de rcoin: leitura da caixa postal falhou", "conta", conta, "err", err)
				sendClientMessage(w, s, msgRcoinProximoLogin)
				return
			}
			if s.AccountID != conta {
				return
			}
			entregues, presos := w.ApplyDeliveries(s, pendentes)
			switch {
			case presos > 0:
				sendClientMessage(w, s, world.MensagemEntregaPresa(presos))
			case entregues > 0:
				sendClientMessage(w, s, msgRcoinEntregue)
			default:
				// Nada saiu da caixa postal: o baú da conta não está carregado,
				// ou outro dreno chegou antes. O item continua na fila.
				sendClientMessage(w, s, msgRcoinProximoLogin)
			}
		}
	})
}
