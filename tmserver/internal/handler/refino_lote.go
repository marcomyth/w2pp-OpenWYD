package handler

import (
	"sort"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Painel de refino (client/gamepatch/refino.cpp): o jogador pede "refine este
// item com esta poeira até +N" e o servidor gasta poeira após poeira até chegar.
//
// Cada poeira é exatamente uma tentativa do arrasto (refineTentativa): as mesmas
// travas, os mesmos sorteios na mesma ordem, a mesma pity. O que muda é só o que
// vai para o cliente — nada por poeira, tudo uma vez no fim: os slots mexidos, o
// placar, um anúncio, uma emoção e a contagem.

// refinoTetoServidor é o máximo de poeiras que um pedido gasta, peça o jogador o
// que pedir. Um lote de +0 a +9 com Lac costuma ficar bem abaixo disso; o teto
// existe para um pedido não segurar o laço do mundo por tempo indefinido.
const refinoTetoServidor = 1000

// refinoAlvoMax é o nível mais alto que a poeira alcança (o passo +10→+11).
const refinoAlvoMax = sancHardCap

// refinoLote atende MsgRefinoPede.
func (d *Dispatcher) refinoLote(w *world.World, s *world.Session, _ protocol.Header, payload []byte) {
	e := w.Entity(s.Conn)
	if e == nil || e.HP <= 0 || s.Mode != world.UserPlay {
		return
	}
	var pedido protocol.RefinoPedeBody
	if err := pedido.Decode(payload); err != nil {
		return
	}
	r := d.refinoExecuta(w, s, e, pedido)
	w.Send(s, protocol.MsgRefinoResultado, r.Encode())
}

// refinoExecuta é o lote inteiro: valida, gasta as poeiras, manda os slots, o
// placar, o anúncio e a emoção, e devolve a contagem que vai para o painel.
func (d *Dispatcher) refinoExecuta(w *world.World, s *world.Session, e *world.Entity, pedido protocol.RefinoPedeBody) (r protocol.RefinoResultadoBody) {
	r = protocol.RefinoResultadoBody{Lugar: pedido.Lugar, Slot: pedido.Slot}

	// Troca e loja aberta travam o inventário, como no resto dos pacotes de item.
	// Aqui não se cancela a troca: o painel é um pedido, não um gesto no
	// inventário, e desfazer a troca do jogador por isso seria castigo demais.
	if s.Trade.Active || shopPinsOwner(s) {
		r.Motivo = protocol.RefinoOcupado
		return
	}

	lugar, slot, slotPoeira := int(pedido.Lugar), int(pedido.Slot), int(pedido.SlotPoeira)
	if lugar != world.ItemPlaceEquip && lugar != world.ItemPlaceCarry {
		r.Motivo = protocol.RefinoInvalido
		return
	}
	dst := d.itemSlot(w, s, e, lugar, slot)
	if dst == nil || dst.Empty() {
		r.Motivo = protocol.RefinoInvalido
		return
	}
	if !carrySlotAccessible(e, slotPoeira) || e.Carry[slotPoeira].Empty() ||
		(lugar == world.ItemPlaceCarry && slot == slotPoeira) {
		r.Motivo = protocol.RefinoInvalido
		return
	}
	indice := e.Carry[slotPoeira].Index
	vol := d.itemVolatiles[int(indice)]
	if vol != volDustOri && vol != volDustLac {
		r.Motivo = protocol.RefinoInvalido
		return
	}
	alvo := int(pedido.Alvo)
	if alvo < 1 || alvo > refinoAlvoMax {
		r.Motivo = protocol.RefinoInvalido
		return
	}
	// O que o arrasto converte ou choca em vez de refinar não entra no lote:
	// tintura vira feijão, a pedra Arch se transmuta, o ovo nasce montaria.
	if isTintura(*dst) || isPedraArch(dst.Index) || isEgg(*dst) || d.itemVolatiles[int(dst.Index)] != 0 {
		r.Motivo = protocol.RefinoNaoServe
		return
	}

	inicial := refine.Level(*dst)
	r.NivelInicial = uint8(inicial)
	r.NivelFinal = uint8(inicial)
	if alvo <= inicial {
		r.Motivo = protocol.RefinoInvalido
		return
	}

	teto := refinoTetoServidor
	if pedido.MaxPoeiras > 0 && int(pedido.MaxPoeiras) < teto {
		teto = int(pedido.MaxPoeiras)
	}

	mexidos := map[int]bool{}
	for {
		if refine.Level(*dst) >= alvo {
			r.Motivo = protocol.RefinoChegou
			break
		}
		if int(r.Usadas) >= teto {
			r.Motivo = protocol.RefinoTeto
			break
		}
		src := refinoAchaPoeira(e, indice, slotPoeira)
		if src < 0 {
			r.Motivo = protocol.RefinoSemPoeira
			break
		}
		res, _, ok := d.refineTentativa(w, e, dst, src, vol)
		if !ok {
			// A recusa não gasta poeira nem sorteia, igual à do arrasto.
			r.Motivo = protocol.RefinoMuro
			break
		}
		mexidos[src] = true
		r.Usadas++
		if res.sucesso {
			r.Sucessos++
		} else {
			r.Falhas++
		}
		if dst.Empty() {
			r.Motivo = protocol.RefinoQuebrou
			break
		}
	}
	final := refine.Level(*dst)
	r.NivelFinal = uint8(final)

	if r.Usadas > 0 {
		d.refinoMandaSlots(w, s, e, lugar, slot, *dst, mexidos)
		if r.Sucessos > 0 {
			d.refreshScore(e)
			d.sendScore(w, s, e)
			d.announceRefine(w, e.Name, dst.Index, final)
		}
		if final > inicial {
			sendEmotion(w, s, e, motionLevelUp, motionLevelUpParm)
		} else {
			sendEmotion(w, s, e, refineFailMotion(e), 0)
		}
	}

	// O lote gasta muita poeira de uma vez; esta linha é o que responde "sumiu
	// minha poeira".
	d.log.Info("painel de refino",
		"conta", s.AccountName, "personagem", e.Name,
		"lugar", lugar, "slot", slot, "item", dst.Index, "poeira", indice,
		"inicial", inicial, "final", final, "alvo", alvo,
		"usadas", r.Usadas, "sucessos", r.Sucessos, "falhas", r.Falhas, "motivo", r.Motivo)
	return r
}

// refinoAchaPoeira devolve o slot da próxima poeira do lote: a pilha que o
// jogador escolheu enquanto durar, depois as outras do mesmo índice, em ordem.
// -1 quando acabou.
func refinoAchaPoeira(e *world.Entity, indice int16, escolhida int) int {
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

// refinoMandaSlots reenvia o item e cada pilha de poeira mexida. Ao contrário do
// arrasto, aqui a poeira TEM de voltar: o cliente não tirou nada sozinho, e sem
// isso a pilha fica errada na tela.
func (d *Dispatcher) refinoMandaSlots(w *world.World, s *world.Session, e *world.Entity, lugar, slot int, item world.Item, mexidos map[int]bool) {
	d.sendSlot(w, s, lugar, slot, item)
	pilhas := make([]int, 0, len(mexidos))
	for i := range mexidos {
		pilhas = append(pilhas, i)
	}
	sort.Ints(pilhas)
	for _, i := range pilhas {
		d.sendSlot(w, s, world.ItemPlaceCarry, i, e.Carry[i])
	}
}
