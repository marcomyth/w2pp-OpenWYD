package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Kit de novato (/novato) — uma vez por conta, decidido com a equipe em
// 12/09/2026. Nada disso existe no legado: é regra deste servidor.
//
// O que entra na bolsa são três itens, não nove: o frango e o baú vão
// EMPILHADOS com EF_AMOUNT, do mesmo jeito que a loja entrega pilha. A Shire vai
// com o tempo nos efeitos de duração, como a montaria da loja cash — assim o
// relógio dela só começa quando o novato montar pela primeira vez, e não enquanto
// o kit espera no baú.
//
// Os três são intransferíveis: a Shire já traz EF_NOTRADE do catálogo e as duas
// variantes do kit (5760/5761) também. Não é decoração — sem isso o kit vira a
// forma mais barata de mover valor entre contas, que é justamente o contrário do
// que um presente de boas-vindas deveria ser.
const (
	itemFrangoNovato = 5760 // Frango Assado (Novato): mesmo ícone e efeito do 3314, preço 0
	itemBauXPNovato  = 5761 // Baú de Experiência (Novato): mesmo ícone e efeito do 4140, preço 0
	itemShireNovato  = 3980 // Shire da loja cash: +150 dano, +15 mágico, ABS PvE 20%, +3% de XP

	frangosNoKit   = 5
	bausNoKit      = 3
	diasDaMontaria = 3
)

// newbieKit é o kit montado, na ordem em que o jogador o vê chegar.
func newbieKit() []world.Item {
	return []world.Item{
		{Index: itemFrangoNovato, Effects: [3]world.Effect{{Effect: efAmount, Value: frangosNoKit}}},
		{Index: itemBauXPNovato, Effects: [3]world.Effect{{Effect: efAmount, Value: bausNoKit}}},
		{Index: itemShireNovato, Effects: durationEffects(diasDaMontaria * 24 * time.Hour)},
	}
}

// novatoKit executa o /novato: confere quem pode pedir, toma a trava de uma vez
// por conta no banco e entrega o kit.
//
// A ordem importa e é deliberada. A trava é tomada ANTES de qualquer item
// aparecer, e a entrega só acontece para quem a tomou. O inverso — entregar e
// depois marcar — abriria a janela em que dois canais entregam dois kits para a
// mesma conta, e essa janela é do tamanho de uma chamada gRPC.
func (d *Dispatcher) novatoKit(w *world.World, s *world.Session) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	// Só Mortal (decisão da equipe): Arch e Celestial vêm de uma conta que já
	// jogou, e o kit é para quem está começando.
	//
	// Mortal é ClassMaster 2 (Basedef.h:238, score_derive.go), NÃO zero — zero é
	// "nunca gravado", e o login o converte para Mortal justamente por isso.
	// Comparar com zero aqui recusava o kit para todo mundo, que é o tipo de erro
	// que só aparece quando se testa o caminho feliz.
	if e.ClassMaster != classMasterMortal {
		sendClientMessage(w, s, "O kit de novato é só para personagens Mortais.")
		return
	}
	if s.AccountID == 0 {
		return
	}
	// Uma trava local ANTES da chamada ao banco. Sem ela, dez /novato seguidos
	// viram dez chamadas em voo — o banco recusaria nove, mas a resposta de cada
	// uma volta para o loop e o log enche de ruído por um comando que o jogador
	// só pode usar uma vez.
	if s.NovatoEmCurso {
		return
	}
	s.NovatoEmCurso = true

	conta, nome := s.AccountID, e.Name
	p := w.Persistence()
	w.Go(s, func() func(*world.World, *world.Session) {
		granted, err := p.ClaimNewbieKit(context.Background(), conta, nome)
		return func(w *world.World, s *world.Session) {
			s.NovatoEmCurso = false
			if err != nil {
				d.log.Warn("kit de novato: trava falhou", "conta", conta, "err", err)
				sendClientMessage(w, s, "Não foi possível entregar o kit agora. Tente novamente em instantes.")
				return
			}
			if !granted {
				sendClientMessage(w, s, "Esta conta já recebeu o kit de novato.")
				return
			}
			d.entregarKitNovato(w, s, conta)
		}
	})
}

// entregarKitNovato põe o kit na bolsa e manda para o baú o que não couber.
//
// O baú é a rede de segurança combinada com a equipe: a trava já foi gasta
// quando chegamos aqui, então recusar por falta de espaço custaria o kit
// inteiro ao jogador. O baú é da CONTA e tem 128 espaços, de modo que "não coube
// em lugar nenhum" é quase impossível — mas se acontecer o item é registrado
// como perdido no log, porque um kit que some sem rastro é um ticket de suporte
// sem resposta.
func (d *Dispatcher) entregarKitNovato(w *world.World, s *world.Session, conta int64) {
	e := w.Entity(s.Conn)
	if e == nil {
		d.log.Warn("kit de novato: personagem saiu antes da entrega", "conta", conta)
		return
	}
	cargo := w.Cargo(conta)

	var naBolsa, noBau int
	for _, it := range newbieKit() {
		if dst := firstEmptyAccessibleCarry(e); dst >= 0 {
			e.Carry[dst] = it
			naBolsa++
			continue
		}
		if cargo != nil {
			if dst := firstEmptyCargoSlot(cargo); dst >= 0 {
				cargo.Items[dst] = it
				w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(world.ItemPlaceCargo, dst, itemToSel(it)))
				noBau++
				continue
			}
		}
		d.log.Warn("kit de novato: item sem lugar na bolsa nem no baú",
			"conta", conta, "personagem", e.Name, "item", it.Index)
	}

	// SendCarry fecha a entrega: é o pacote que descreve a bolsa inteira, e sem
	// ele o cliente só saberia dos itens novos no próximo relog.
	d.sendCarry(w, s, e)
	w.SaveCharacterAsync(s)

	sendClientMessage(w, s, fmt.Sprintf(
		"Kit de novato entregue: %d frangos, %d baús de experiência e uma Shire de %d dias.",
		frangosNoKit, bausNoKit, diasDaMontaria))
	if noBau > 0 {
		sendClientMessage(w, s, fmt.Sprintf(
			"Sua bolsa estava cheia: %d %s foram para o baú.", noBau, pluralItem(noBau)))
	}
	sendClientMessage(w, s, "Os itens do kit não podem ser negociados nem vendidos.")
	d.log.Info("kit de novato entregue", "conta", conta, "personagem", e.Name,
		"na_bolsa", naBolsa, "no_bau", noBau)
}

func pluralItem(n int) string {
	if n == 1 {
		return "item"
	}
	return "itens"
}

// firstEmptyCargoSlot devolve o primeiro espaço livre do baú da conta, ou -1.
func firstEmptyCargoSlot(c *world.CargoState) int {
	for i := range c.Items {
		if c.Items[i].Empty() {
			return i
		}
	}
	return -1
}
