package handler

import (
	"context"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// As RCoin (EF_VOLATILE 184) são a moeda de apoio: usar uma soma o EF_DONATE
// dela à carteira de donate DA CONTA — a mesma que a loja do site gasta —, e o
// jogador confere o saldo com /donate.
//
// O legado tinha o volátil 184 e nunca foi portado: até aqui o item existia,
// tinha nome e descrição, e clicar nele não fazia nada.
//
// A carteira é da conta e o crédito é somado PELO BANCO (balance + amount), o
// que importa porque os quatro personagens de uma conta podem gastar uma moeda
// cada no mesmo instante.

const volDonate = 184 // RCoin: EF_DONATE vai para a carteira de donate da conta

// As cinco moedas. Estão aqui por índice porque elas empilham, e a tabela de
// empilhar (isSplittable) é por índice: o EF_VOLATILE sozinho não serve, já que
// nada impede um item futuro de carregar o 184 sem ser moeda negociável.
const (
	itemRCoin100 = 3393
	itemRCoin1K  = 3394
	itemRCoin3K  = 3395
	itemRCoin5K  = 3396
	itemRCoin10K = 3441
)

// useRCoin gasta uma RCoin e credita o donate dela.
//
// A moeda sai da bolsa ANTES da ida ao banco, e essa ordem é o ponto todo da
// função. O loop é dono único do mundo, então tirar a moeda aqui é atômico e
// nenhuma segunda chamada pode gastar a mesma — enquanto que consumir só na
// volta deixaria uma janela em que a moeda ainda está na bolsa para ser trocada,
// vendida ou usada de novo, com o crédito já a caminho. Item duplicado é o erro
// que este servidor mais teme, e pagar por ele com uma devolução na falha é o
// lado barato da troca.
func (d *Dispatcher) useRCoin(w *world.World, s *world.Session, e *world.Entity, src int) {
	item := e.Carry[src]
	valor := d.itemDonates[int(item.Index)]
	if valor <= 0 {
		// Um item Vol 184 que o catálogo não precifica. Recusa com o slot de
		// volta, como o rejectUnimplementedConsumable: um return seco deixaria o
		// cliente mostrando a moeda gasta enquanto o servidor ainda a tem.
		d.log.Warn("rcoin recusada", "conn", s.Conn, "motivo", "sem EF_DONATE", "item", item.Index)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, item)
		return
	}
	if s.AccountID == 0 {
		d.log.Warn("rcoin recusada", "conn", s.Conn, "motivo", "sessão sem conta", "item", item.Index)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, item)
		return
	}
	// Uma ida ao banco por vez, como a trava do /novato. Não é ela que impede o
	// crédito duplo — isso é o consumo logo abaixo, que tira a moeda antes da ida —,
	// e sim o que mantém a devolução simples: com um crédito só em voo, uma falha
	// devolve uma moeda para a pilha de onde ela saiu, e dez cliques numa pilha
	// não viram dez idas simultâneas ao dbServer.
	if s.DonateEmCurso {
		d.sendSlot(w, s, world.ItemPlaceCarry, src, item)
		return
	}
	s.DonateEmCurso = true

	consumeOneItem(&e.Carry[src])
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, src, itemToSel(e.Carry[src])))

	conta, nome, idx := s.AccountID, e.Name, item.Index
	motivo := fmt.Sprintf("rcoin %d usada por %s", idx, nome)
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.CreditDonate(context.Background(), conta, valor, nome, motivo)
		return func(w *world.World, s *world.Session) {
			s.DonateEmCurso = false
			if err != nil {
				d.log.Warn("rcoin: crédito falhou", "conta", conta, "item", idx, "valor", valor, "err", err)
				d.devolverRCoin(w, s, item, src)
				sendClientMessage(w, s, "Não foi possível creditar o donate agora. Sua moeda foi devolvida.")
				return
			}
			// A TELA ACOMPANHA A FRASE. O crédito já devolve o saldo do banco, e é
			// esse mesmo número que vai na mensagem — então aqui não se relê nada:
			// uma segunda ida ao banco custaria uma viagem para trazer o que já
			// está na mão, e ainda poderia voltar depois de outra mexida e mostrar
			// um número diferente do que o jogador acabou de ler.
			//
			// Sem esta linha, a mensagem dizia o saldo novo e o rodapé da Loja do
			// Servidor seguia com o do login: dois números, na mesma sessão, no
			// mesmo minuto.
			s.Cash = saldo
			sendClientMessage(w, s, fmt.Sprintf("Você recebeu %d de donate. Seu saldo é %d.", valor, saldo))
			d.log.Info("rcoin usada", "conta", conta, "personagem", nome, "item", idx, "valor", valor, "saldo", saldo)
			w.SaveCharacterAsync(s)
		}
	})
}

// devolverRCoin repõe UMA moeda depois de um crédito que não aconteceu.
//
// De volta ao slot de origem quando ele continua vazio, que é o caso comum; em
// qualquer vaga livre quando o jogador mexeu na bolsa nesse meio-tempo. Uma
// moeda que não coube em lugar nenhum vai para o log com a conta e o índice,
// porque item que some sem rastro é um chamado de suporte sem resposta.
func (d *Dispatcher) devolverRCoin(w *world.World, s *world.Session, item world.Item, src int) {
	e := w.Entity(s.Conn)
	if e == nil {
		d.log.Warn("rcoin: personagem saiu antes da devolução", "conn", s.Conn, "item", item.Index)
		return
	}
	// Uma unidade, e não a pilha que estava no slot: o que foi gasto foi uma.
	uma := item
	if itemAmount(item) > 1 {
		setItemAmount(&uma, 1)
	}
	// De volta PARA A PILHA quando o resto dela continua ali, que é o caso comum
	// desde que as moedas empilham: repor numa vaga separada deixaria o jogador
	// com 9 num espaço e 1 noutro, sem entender por quê.
	if src >= 0 && src < len(e.Carry) && sameStackClass(uma, e.Carry[src]) {
		if n := itemAmount(e.Carry[src]); n < maxStackAmount && canWriteItemAmount(e.Carry[src]) {
			setItemAmount(&e.Carry[src], n+1)
			w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, src, itemToSel(e.Carry[src])))
			return
		}
	}
	dst := src
	if src < 0 || src >= len(e.Carry) || e.Carry[src].Index != 0 {
		dst = firstEmptyAccessibleCarry(e)
	}
	if dst < 0 {
		d.log.Warn("rcoin: sem lugar na bolsa para devolver", "conn", s.Conn, "item", item.Index)
		return
	}
	e.Carry[dst] = uma
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, dst, itemToSel(uma)))
}

// mostrarSaldoDeDonate answers /donate: the account's donate wallet.
//
// Read from the database and not from a cached number, because the wallet
// belongs to the ACCOUNT: the other three characters can have spent or earned
// since this one logged in, and so can the web shop. Off the loop, like every
// other database read.
func (d *Dispatcher) mostrarSaldoDeDonate(w *world.World, s *world.Session) {
	if s.AccountID == 0 {
		return
	}
	conta := s.AccountID
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		saldo, err := p.DonateBalance(context.Background(), conta)
		return func(w *world.World, s *world.Session) {
			if err != nil {
				d.log.Warn("donate: leitura de saldo falhou", "conta", conta, "err", err)
				sendClientMessage(w, s, "Não foi possível consultar seu donate agora.")
				return
			}
			sendClientMessage(w, s, fmt.Sprintf("Você tem %d de donate.", saldo))
		}
	})
}
