package handler

import (
	"sort"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Painel de up da montaria (client/gamepatch/montaria.cpp): o jogador escolhe
// uma pilha de âmago e quantas pilhas gastar, e o servidor dá âmago após âmago na
// montaria ADULTA vestida até as pilhas acabarem ou ela chegar no 120.
//
// Cada âmago é exatamente uma tentativa do arrasto (amagoTentativa): os mesmos
// sorteios na mesma ordem. O que muda é só o que vai para o cliente — nada por
// âmago, nenhuma linha de resumo no chat, tudo uma vez no fim: os slots mexidos e
// a contagem, que o painel mostra.
//
// UM PAC É UMA PILHA, gasta inteira (decisão da dona): o tamanho da pilha é
// configuração do servidor, e quem pede 3 Pac's quer as 3 pilhas vazias, tenham
// elas 35 ou 200.

// montariaTetoServidor é o máximo de âmagos que um pedido gasta. Não é regra de
// jogo — 60 pilhas de 255 ficam abaixo — é só a garantia de que um pedido nunca
// segura o laço do mundo sem fim.
const montariaTetoServidor = 60 * 255

// montariaLote atende MsgMontariaPede.
func (d *Dispatcher) montariaLote(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	var pedido protocol.MontariaPedeBody
	if err := pedido.Decode(payload); err != nil {
		return
	}
	r := d.montariaExecuta(w, s, e, pedido)
	w.Send(s, protocol.MsgMontariaResultado, r.Encode())
}

// montariaExecuta é o lote inteiro: valida, gasta os âmagos, manda os slots e
// devolve a contagem que vai para o painel.
func (d *Dispatcher) montariaExecuta(w *world.World, s *world.Session, e *world.Entity, pedido protocol.MontariaPedeBody) (r protocol.MontariaResultadoBody) {
	dst := &e.Equip[mountEquipSlot]
	r.Montaria = dst.Index
	nivel := dst.Effects[1].Effect
	r.NivelInicial, r.NivelFinal = nivel, nivel

	// Troca e loja aberta travam o inventário, como no painel de refino.
	if s.Trade.Active || shopPinsOwner(s) {
		r.Motivo = protocol.MontariaOcupado
		return
	}
	// Só a adulta: a cria cresce sem sorteio e vira adulta no meio do caminho, o
	// que o arrasto já faz bem, e o ovo não come âmago.
	if dst.Index < criaHi || dst.Index >= mountHi {
		r.Motivo = protocol.MontariaNaoAdulta
		return
	}
	// stEffect[0] é o HP da montaria: morta não come, como no arrasto.
	if amagoHunger(*dst) <= 0 {
		r.Motivo = protocol.MontariaMorta
		return
	}
	if int(nivel) >= adultMaxLevel {
		r.Motivo = protocol.MontariaNoMaximo
		return
	}
	slot := int(pedido.SlotAmago)
	if !carrySlotAccessible(e, slot) || e.Carry[slot].Empty() {
		r.Motivo = protocol.MontariaInvalido
		return
	}
	indice := e.Carry[slot].Index
	// Cada âmago é de UMA linhagem (mountAmagoSlot): o de outra montaria não serve.
	if indice < amagoBase || indice >= amagoBase+mountRowSize ||
		mountAmagoSlot(dst.Index) != int(indice)-amagoBase {
		r.Motivo = protocol.MontariaInvalido
		return
	}

	pilhas := int(pedido.Pilhas) // 0 = todas
	mexidos := map[int]bool{}
	for {
		if int(dst.Effects[1].Effect) >= adultMaxLevel {
			r.Motivo = protocol.MontariaNoMaximo
			break
		}
		if (pilhas > 0 && int(r.Pilhas) >= pilhas) || int(r.Usados) >= montariaTetoServidor {
			r.Motivo = protocol.MontariaAcabouPacs
			break
		}
		src := montariaAchaAmago(e, indice, slot)
		if src < 0 {
			r.Motivo = protocol.MontariaSemAmago
			break
		}
		subiu, caiu := d.amagoTentativa(w, e, dst, src)
		mexidos[src] = true
		r.Usados++
		if subiu {
			r.Sucessos++
		} else {
			r.Falhas++
		}
		if caiu {
			r.Quedas++
		}
		if e.Carry[src].Empty() {
			r.Pilhas++
		}
	}
	r.NivelFinal = dst.Effects[1].Effect

	if r.Usados > 0 {
		d.montariaMandaSlots(w, s, e, *dst, mexidos)
	}
	if r.Motivo == protocol.MontariaNoMaximo {
		d.notify(w, s, NoticeCantUpgradeMore)
	}

	// O lote gasta muito âmago de uma vez; esta linha é o que responde "sumiu meu
	// âmago".
	d.log.Info("painel de montaria",
		"conta", s.AccountName, "personagem", e.Name,
		"montaria", dst.Index, "amago", indice, "pilhasPedidas", pilhas,
		"inicial", r.NivelInicial, "final", r.NivelFinal,
		"usados", r.Usados, "sucessos", r.Sucessos, "falhas", r.Falhas,
		"quedas", r.Quedas, "pilhas", r.Pilhas, "motivo", r.Motivo)
	return r
}

// montariaAchaAmago devolve o slot do próximo âmago do lote: a pilha que o
// jogador escolheu enquanto durar, depois as outras do mesmo índice, em ordem.
// -1 quando acabou.
func montariaAchaAmago(e *world.Entity, indice int16, escolhida int) int {
	if carrySlotAccessible(e, escolhida) && e.Carry[escolhida].Index == indice && !e.Carry[escolhida].Empty() {
		return escolhida
	}
	limite := activeCarryLimit(e)
	for i := 0; i < limite; i++ {
		if e.Carry[i].Index == indice && !e.Carry[i].Empty() {
			return i
		}
	}
	return -1
}

// montariaMandaSlots reenvia a montaria e cada pilha de âmago mexida. O cliente
// não tirou nada sozinho, então sem isso a pilha fica errada na tela.
func (d *Dispatcher) montariaMandaSlots(w *world.World, s *world.Session, e *world.Entity, montaria world.Item, mexidos map[int]bool) {
	pilhas := make([]int, 0, len(mexidos))
	for i := range mexidos {
		pilhas = append(pilhas, i)
	}
	sort.Ints(pilhas)
	for _, i := range pilhas {
		d.sendSlot(w, s, world.ItemPlaceCarry, i, e.Carry[i])
	}
	d.sendSlot(w, s, world.ItemPlaceEquip, mountEquipSlot, montaria)
}
