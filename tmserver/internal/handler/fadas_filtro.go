package handler

import (
	"context"
	"slices"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/droprule"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O FILTRO DE DROP DAS FADAS (pedido da dona em 02/10/2026).
//
// Com a Fada Azul ou a Vermelha vestida e o filtro LIGADO, só entra na mochila o
// saque de monstro que está na lista de Itens Protegidos do personagem. O resto é
// descartado antes de ocupar espaço. Com o filtro desligado, ou sem a fada, tudo
// é como sempre foi.
//
// ESTA É A ÚNICA REGRA DO SERVIDOR QUE APAGA SAQUE. Até aqui a fada não apagava
// nada (só juntava pilhas, carry.go). Por isso as travas:
//
//   - só saque de monstro: a porta é putMobDrop com o monstro na mão. Baú, quest,
//     evento mundial, Kefra, combinação, compra e troca não passam por ela;
//   - só o que o painel MOSTRA para aquele monstro (dropsVisiveis). Um item que o
//     jogador não tinha como ver, e portanto não tinha como proteger, nunca some;
//   - depois de todos os sorteios: quem decide é a entrega, então o filtro não
//     muda a sequência de w.Rand() de ninguém;
//   - ligado com a lista vazia não existe (descartaria tudo): ligar é recusado, e
//     tirar o último item desliga.
//
// O saque de chefe entra na regra como qualquer outro (decisão da dona).

// fadaTemFiltro diz se a fada vestida é uma das duas que filtram: a Azul e a
// Vermelha. A do Vale junta pilhas e não filtra.
func fadaTemFiltro(e *world.Entity) bool {
	if e == nil {
		return false
	}
	switch e.Equip[fairyEquipSlot].Index {
	case fadaAzul3Dias, fadaAzul5Dias, fadaAzul7Dias,
		fadaVermelha3Dias, fadaVermelha5Dias, fadaVermelha7Dias:
		return true
	}
	return false
}

// fadaItemValido diz se um índice pode entrar na lista: a mesma faixa que o laço
// de saque de mobKilled aceita.
func fadaItemValido(idx int16) bool {
	return idx > 390 && int(idx) < maxItemList && idx != 454
}

// fadaFiltroDoBanco limpa o que veio do banco no login: tira índice fora da faixa
// e repetido, põe em ordem e corta no teto. Ligado sem item vira desligado.
func fadaFiltroDoBanco(ligado bool, itens []int16) (bool, []int16) {
	var limpo []int16
	for _, idx := range itens {
		if fadaItemValido(idx) {
			limpo = append(limpo, idx)
		}
	}
	slices.Sort(limpo)
	limpo = slices.Compact(limpo)
	if len(limpo) > protocol.FadasFiltroMax {
		limpo = limpo[:protocol.FadasFiltroMax]
	}
	return ligado && len(limpo) > 0, limpo
}

// fadaDescarta diz se o filtro de quem recebe joga fora este item do saque de
// mob. Chamada na entrega, com os sorteios todos já feitos.
func (d *Dispatcher) fadaDescarta(w *world.World, reward, mob *world.Entity, idx int16) bool {
	if reward == nil || mob == nil || !reward.FadaFiltroLigado || len(reward.FadaFiltro) == 0 || !fadaTemFiltro(reward) {
		return false
	}
	if _, protegido := slices.BinarySearch(reward.FadaFiltro, idx); protegido {
		return false
	}
	// A rede de segurança: só some o que o painel mostra para este monstro.
	cat := d.fadasCatalogo(w)
	i, ok := cat.porMolde[droprule.Canonical(mob.TemplateName)]
	if !ok {
		return false
	}
	_, mostrado := slices.BinarySearch(d.dropsVisiveis(w, &cat.lista[i]), idx)
	return mostrado
}

// fadasMandaFiltro manda o 0x0F74: o estado do filtro do personagem.
func (d *Dispatcher) fadasMandaFiltro(w *world.World, s *world.Session, e *world.Entity, motivo uint8) {
	w.Send(s, protocol.MsgFadasFiltro,
		protocol.EncodeFadasFiltro(e.FadaFiltroLigado, fadaTemFiltro(e), motivo, e.FadaFiltro))
}

// fadasMuda atende o 0x0F73: pôr e tirar item da lista, ligar e desligar.
//
// A mudança vale NA HORA na memória, que é onde o saque é decidido, e vai ao banco
// em seguida. O servidor responde sempre, uma vez, com o estado depois do pedido.
func (d *Dispatcher) fadasMuda(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || s.Mode != world.UserPlay {
		return
	}
	corpo, err := protocol.DecodeFadasMuda(payload)
	if err != nil {
		d.log.Info("painel das fadas: mudança recusada", "conn", s.Conn, "erro", err)
		return
	}
	motivo, mudou := fadaAplica(e, corpo)
	if mudou {
		d.log.Info("filtro da fada", "conn", s.Conn, "char", e.Name, "acao", corpo.Acao,
			"item", corpo.Item, "ligado", e.FadaFiltroLigado, "itens", len(e.FadaFiltro))
		d.fadasGrava(w, s, e)
	}
	d.fadasMandaFiltro(w, s, e, motivo)
}

// fadaAplica faz a mudança no personagem e diz o motivo da recusa, se houve, e
// se algo mudou de fato (pôr o que já está e tirar o que não está não gravam).
func fadaAplica(e *world.Entity, corpo protocol.FadasMudaBody) (motivo uint8, mudou bool) {
	switch corpo.Acao {
	case protocol.FadasMudaPoe:
		if !fadaItemValido(corpo.Item) {
			return protocol.FadasMotivoItemRuim, false
		}
		pos, ja := slices.BinarySearch(e.FadaFiltro, corpo.Item)
		if ja {
			return protocol.FadasMotivoNenhum, false
		}
		if len(e.FadaFiltro) >= protocol.FadasFiltroMax {
			return protocol.FadasMotivoListaCheia, false
		}
		// Cópia: a gravação em curso pode estar lendo o vetor antigo.
		e.FadaFiltro = slices.Insert(slices.Clone(e.FadaFiltro), pos, corpo.Item)
		return protocol.FadasMotivoNenhum, true
	case protocol.FadasMudaTira:
		pos, tem := slices.BinarySearch(e.FadaFiltro, corpo.Item)
		if !tem {
			return protocol.FadasMotivoNenhum, false
		}
		e.FadaFiltro = slices.Delete(slices.Clone(e.FadaFiltro), pos, pos+1)
		if len(e.FadaFiltro) == 0 {
			e.FadaFiltroLigado = false
		}
		return protocol.FadasMotivoNenhum, true
	case protocol.FadasMudaTiraTudo:
		if len(e.FadaFiltro) == 0 && !e.FadaFiltroLigado {
			return protocol.FadasMotivoNenhum, false
		}
		e.FadaFiltro, e.FadaFiltroLigado = nil, false
		return protocol.FadasMotivoNenhum, true
	case protocol.FadasMudaLiga:
		if e.FadaFiltroLigado {
			return protocol.FadasMotivoNenhum, false
		}
		if !fadaTemFiltro(e) {
			return protocol.FadasMotivoSemFada, false
		}
		if len(e.FadaFiltro) == 0 {
			return protocol.FadasMotivoListaVazia, false
		}
		e.FadaFiltroLigado = true
		return protocol.FadasMotivoNenhum, true
	case protocol.FadasMudaDesliga:
		if !e.FadaFiltroLigado {
			return protocol.FadasMotivoNenhum, false
		}
		e.FadaFiltroLigado = false
		return protocol.FadasMotivoNenhum, true
	}
	return protocol.FadasMotivoNenhum, false
}

const fadasGravaTimeout = 5 * time.Second

// fadasGrava leva o filtro do personagem ao banco, fora do laço.
//
// UMA GRAVAÇÃO POR VEZ por sessão. Cliques seguidos virariam várias gravações no
// ar, e a que chegasse por último ao banco mandaria, mesmo sendo a mais velha. Com
// uma só, a mudança que chega durante a gravação fica guardada na sessão, e a
// volta grava esse estado, o mais novo.
func (d *Dispatcher) fadasGrava(w *world.World, s *world.Session, e *world.Entity) {
	d.fadasGravaEstado(w, s, world.FadaFiltroSalvo{
		Conta: s.AccountID, Slot: s.Slot, Ligado: e.FadaFiltroLigado, Itens: e.FadaFiltro,
	})
}

func (d *Dispatcher) fadasGravaEstado(w *world.World, s *world.Session, estado world.FadaFiltroSalvo) {
	p := w.Persistence()
	if p == nil {
		return
	}
	if s.FadasGravando {
		s.FadasPendente = &estado
		return
	}
	s.FadasGravando, s.FadasPendente = true, nil
	w.Go(s, func() func(*world.World, *world.Session) {
		ctx, cancel := context.WithTimeout(context.Background(), fadasGravaTimeout)
		defer cancel()
		err := p.SaveFadaFiltro(ctx, estado.Conta, estado.Slot, estado.Ligado, estado.Itens)
		return func(w *world.World, s *world.Session) {
			s.FadasGravando = false
			if err != nil {
				d.log.Error("filtro da fada: não gravou", "conn", s.Conn,
					"conta", estado.Conta, "slot", estado.Slot, "err", err)
				// Só avisa quem ainda é o mesmo personagem em jogo.
				if e := w.Entity(s.Conn); e != nil && s.Mode == world.UserPlay &&
					s.Slot == estado.Slot && s.AccountID == estado.Conta {
					d.fadasMandaFiltro(w, s, e, protocol.FadasMotivoNaoGravou)
				}
			}
			if prox := s.FadasPendente; prox != nil {
				s.FadasPendente = nil
				d.fadasGravaEstado(w, s, *prox)
			}
		}
	})
}
