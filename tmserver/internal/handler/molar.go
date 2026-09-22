package handler

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/refine"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O Molar de Gárgula (item 4122, EF_VOLATILE 194) sobe para +7 o refino das
// cinco peças que o personagem está vestindo, UMA vez por personagem.
//
// O item já existia no catálogo do legado com esse EF_VOLATILE e sem nenhum uso
// do lado do servidor: a quest do NPC que o anuncia ("Pegue o molar do
// Gargula!") é um teleporte puro, que nunca pediu o item (molarGargula, em
// misc.go). Aqui ele ganha a função que o nome sempre prometeu.
//
// A marca de "já usei" é persistida (Entity.MolarGargula, coluna molar_gargula
// da 0093) e não um estado de sessão: o molar cai a 35,6% de uma única Gárgula
// Inf do 2º andar da Dungeon, e sem a marca bastaria trocar de set e usar outro.
const (
	volMolarGargula  = 194 // EF_VOLATILE do Molar; exclusivo dele no catálogo
	molarSancAlvo    = 7   // o refino que o molar garante
	molarJaUsado     = 1   // valor gravado em Entity.MolarGargula
	molarPecasDoSet  = 5   // elmo, armadura, calça, luvas e botas
	molarPrimeiraPec = 1   // Equip[0] é o corpo, não é peça de equipamento
)

// useMolarGargula é o uso do item. Só refina para CIMA: peça já em +7 ou mais
// fica como está, e o molar só é consumido se pelo menos uma peça subiu — quem
// já tem o set inteiro acima de +7 recebe o item de volta em vez de perdê-lo.
func (d *Dispatcher) useMolarGargula(w *world.World, s *world.Session, e *world.Entity, src int) {
	if e.MolarGargula != 0 {
		// _NN_Youve_Done_It_Already: a mesma linha que o Cristal Arch usa quando o
		// estágio já foi entregue.
		d.notify(w, s, NoticeAlreadyDone)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	subiram := make([]int, 0, molarPecasDoSet)
	for slot := molarPrimeiraPec; slot < molarPrimeiraPec+molarPecasDoSet; slot++ {
		peca := &e.Equip[slot]
		if peca.Empty() {
			continue
		}
		// EF_NOSANC marca o item que nunca pode ser refinado; o molar respeita a
		// mesma trava que o pó e o Acelerador respeitam.
		if d.itemAbility(*peca, efNoSanc) != 0 {
			continue
		}
		if refine.Level(*peca) >= molarSancAlvo {
			continue
		}
		// Uma peça nunca refinada não tem onde guardar o nível, e refine.Set é um
		// no-op nela. Bootstrap planta o par EF_SANC e falha só quando os três
		// espaços de efeito já têm efeito real — que é a recusa do próprio legado.
		if refine.Level(*peca) == 0 && !refine.Bootstrap(peca) {
			continue
		}
		// O aux vai a 0 de propósito: abaixo de +10 ele é o contador de azar do
		// refino, e o molar é um refino ganho, não um atalho por cima do sistema.
		refine.Set(peca, molarSancAlvo, 0)
		subiram = append(subiram, slot)
	}

	if len(subiram) == 0 {
		// Nada para fazer: o set inteiro já está em +7 ou acima, vazio, ou é
		// irrefinável. Recusa sem consumir — perder o molar aqui seria o pior
		// resultado possível para quem clicou.
		d.notify(w, s, NoticeCantUseHere)
		d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
		return
	}

	e.MolarGargula = molarJaUsado
	consumeOneItem(&e.Carry[src])
	d.sendSlot(w, s, world.ItemPlaceCarry, src, e.Carry[src])
	for _, slot := range subiram {
		d.sendSlot(w, s, world.ItemPlaceEquip, slot, e.Equip[slot])
	}
	// O refino entra no score (AC e dano saem do sanc de cada peça), então a
	// ficha é recalculada e reenviada antes de o personagem ser salvo.
	d.refreshScore(e)
	d.sendScore(w, s, e)
	d.notify(w, s, NoticeRefineSuccess)
	w.SaveCharacterThen(s, func(*world.World, *world.Session) {})
	d.log.Info("molar de gargula usado",
		"conn", s.Conn, "name", e.Name, "pecas", len(subiram), "sanc", molarSancAlvo)
}
