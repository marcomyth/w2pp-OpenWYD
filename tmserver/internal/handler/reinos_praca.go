package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A praça do Dragão Dourado de Armia (Atlas de Quests, área "Reinos e Reis",
// 17/09/2026): o Dragão troca Fragmentos de Alma, que a Escolta do Trono dos
// Reinos solta, por uma Alma sorteada; e quatro Lendas montadas, uma por classe,
// ficam em volta dele só para ver, com uma fala ao clique.
//
// A troca é regra nova. O GOLD_DRAGON do legado (_MSG_Quest.cpp:1472) trocava
// mapas do tesouro e nunca foi portado.

const (
	// merchantDragaoDeArmia é o CurrentScore.Merchant do Dragao_Dourado (bloco
	// 3420). Nenhum outro template do NPCGener usa o 36 nesse byte; o 36 dos
	// Treinadores do campo é o outro byte, o 17 (treinador.go).
	merchantDragaoDeArmia = 36
	// gradeLendaPassada é o EF_GRADE0 das quatro Lendas (Merchant 100); nenhum
	// template do legado usa o 42, e o 40 e o 41 são os Xamãs do Orc e do Troll.
	gradeLendaPassada = 42

	itemFragmentoDeAlma int16 = 3224
	itemAlmaDoUnicornio int16 = 1740
	itemAlmaDaFenix     int16 = 1741
	fragmentosPorAlma         = 10
)

// As falas. Cabem no balão do NPC.
const (
	msgLendaPassada       = "Eu já fiz minha parte, você poderá ser a próxima lenda."
	msgDragaoSemFragmento = "Traga-me 10 Fragmentos de Alma e eu lhe darei uma Alma."
	msgDragaoSemEspaco    = "Abra espaço na bolsa para receber a Alma."
)

func ehDragaoDeArmia(npc *world.Entity) bool {
	return npc != nil && npc.Merchant == merchantDragaoDeArmia
}

func ehLendaPassada(npc *world.Entity) bool {
	return npc != nil && npc.Merchant == 100 && npc.Grade == gradeLendaPassada
}

// lendaPassada é o clique numa Lenda: só a fala.
func (d *Dispatcher) lendaPassada(w *world.World, s *world.Session, npc *world.Entity) {
	sendSay(w, npc, msgLendaPassada)
	sendClientMessage(w, s, msgLendaPassada)
}

// dragaoDeArmia troca, num clique, cada 10 Fragmentos de Alma da bolsa por uma
// Alma, sorteada meio a meio entre a do Unicórnio e a da Fênix. Faz quantas
// trocas os Fragmentos e o espaço permitirem; o que sobra fica na bolsa.
func (d *Dispatcher) dragaoDeArmia(w *world.World, s *world.Session, e, npc *world.Entity) {
	limit := activeCarryLimit(e)
	if countCarried(e, limit, itemFragmentoDeAlma) < fragmentosPorAlma {
		sendSay(w, npc, msgDragaoSemFragmento)
		sendClientMessage(w, s, msgDragaoSemFragmento)
		return
	}
	before := e.Carry
	var almas []int16
	for countCarried(e, limit, itemFragmentoDeAlma) >= fragmentosPorAlma {
		antes := e.Carry
		tirarDaBolsa(e, limit, itemFragmentoDeAlma, fragmentosPorAlma)
		slot := firstEmptyCarrySlot(e.Carry[:], limit)
		if slot < 0 {
			e.Carry = antes // sem espaço: os 10 Fragmentos voltam
			break
		}
		alma := itemAlmaDoUnicornio
		if w.Rand().Intn(2) == 1 {
			alma = itemAlmaDaFenix
		}
		e.Carry[slot] = world.Item{Index: alma}
		almas = append(almas, alma)
	}
	if len(almas) == 0 {
		sendSay(w, npc, msgDragaoSemEspaco)
		sendClientMessage(w, s, msgDragaoSemEspaco)
		return
	}
	for i := range limit {
		if e.Carry[i] != before[i] {
			d.sendSlot(w, s, world.ItemPlaceCarry, i, e.Carry[i])
		}
	}
	texto := fmt.Sprintf("Os Fragmentos se uniram: %d Alma(s) para você.", len(almas))
	sendSay(w, npc, texto)
	sendClientMessage(w, s, texto)
	d.log.Info("reinos: troca no dragão de armia", "conn", s.Conn, "account", s.AccountName,
		"personagem", e.Name, "almas", almas)
}

// tirarDaBolsa tira n unidades de item da bolsa, pilha por pilha. Quem chama
// garante que há n.
func tirarDaBolsa(e *world.Entity, limit int, item int16, n int) {
	for i := 0; i < limit && n > 0; i++ {
		it := &e.Carry[i]
		if it.Index != item {
			continue
		}
		if q := itemAmount(*it); q > n {
			setItemAmount(it, q-n)
			n = 0
		} else {
			*it = world.Item{}
			n -= q
		}
	}
}
