package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Uma entrada e uma saída por rodada do relógio das arenas (pedido da Hanna,
// 17/09). Regra NOSSA: o legado deixa entrar quantas vezes o jogador tiver
// bilhete ou paciência com o NPC.
//
// Por personagem (conta e posição do personagem, não a sessão), por rodada de
// questClearTicks: no máximo UMA entrada numa das cinco arenas da Quest 256,
// somando as três portas (Mestre Grifo, bilhete e NPC). Sair da arena por
// qualquer caminho (andar, recall, teleporte, morte, desconexão) fecha a rodada
// daquele personagem: toda porta recusa até o próximo pulso, com o tempo que
// falta. Relogar não abre de novo, de propósito.
//
// Fica em memória e zera no pulso, depois da expulsão, para a expulsão não contar
// contra a rodada seguinte. Um reinício do servidor zera tudo, o que é aceitável.

// donoDaEntrada é o personagem, e não a sessão: um relog cai na mesma chave.
type donoDaEntrada struct {
	conta int64
	slot  int
}

func donoDe(s *world.Session) donoDaEntrada {
	return donoDaEntrada{conta: s.AccountID, slot: s.Slot}
}

type entradaDaRodada struct {
	saiu  bool
	passo int // índice em quest256Steps da arena em que entrou
}

// entradaDaRodadaUsada diz se o personagem já gastou a entrada desta rodada.
func (d *Dispatcher) entradaDaRodadaUsada(s *world.Session) bool {
	_, usada := d.entradasDaRodada[donoDe(s)]
	return usada
}

// textoRodadaUsada é a recusa das três portas, com o tempo até o próximo pulso.
func (d *Dispatcher) textoRodadaUsada() string {
	seg := d.segundosAteALimpeza()
	return fmt.Sprintf("Você já usou a entrada desta rodada das arenas. A próxima começa em %d min %02d s.", seg/60, seg%60)
}

// recusaBilheteDaRodada responde ao bilhete recusado: o aviso e o espaço
// reenviado, para o cliente não mostrar o bilhete como gasto.
func (d *Dispatcher) recusaBilheteDaRodada(w *world.World, s *world.Session, e *world.Entity, src int) {
	sendClientMessage(w, s, d.textoRodadaUsada())
	w.Send(s, protocol.MsgSendItem, protocol.EncodeSendItemBody(protocol.ItemPlaceCarry, src, itemToSel(e.Carry[src])))
	d.log.Info("arena: entrada da rodada ja usada", "porta", "bilhete", "conta", s.AccountName, "conn", s.Conn)
}

// marcaEntradaDaRodada grava a entrada. Chamado depois do pulo, em
// teleportQuest256Step, por onde as três portas passam.
func (d *Dispatcher) marcaEntradaDaRodada(s *world.Session, step quest256Step) {
	if d.entradasDaRodada == nil {
		d.entradasDaRodada = make(map[donoDaEntrada]entradaDaRodada)
	}
	d.entradasDaRodada[donoDe(s)] = entradaDaRodada{passo: passoDaArena(step)}
}

// passoDaArena é o índice do passo em quest256Steps, achado pela bandeira.
func passoDaArena(step quest256Step) int {
	passo := 0
	for i := range quest256Steps {
		if quest256Steps[i].flag == step.flag {
			passo = i
		}
	}
	return passo
}

// vigiaSaidasDasArenas fecha a rodada de quem saiu. Roda no Tick, antes do guarda
// e do relógio: quem o pulso expulsa ainda está dentro aqui, e o pulso zera a
// tabela logo depois.
//
// Quem saiu e ainda tem a bandeira perde a bandeira assim que estiver vivo, para
// o guarda devolvê-lo à cidade se ele voltar andando ou reviver lá dentro. A
// bandeira não é tirada de um morto: quem morreu volta pela cidade no restart.
func (d *Dispatcher) vigiaSaidasDasArenas(w *world.World) {
	if len(d.entradasDaRodada) == 0 {
		return
	}
	vistos := make(map[donoDaEntrada]bool, len(d.entradasDaRodada))
	w.ForEachPlayer(func(s *world.Session, e *world.Entity) {
		k := donoDe(s)
		ent, ok := d.entradasDaRodada[k]
		if !ok {
			return
		}
		vistos[k] = true
		if !ent.saiu && (e.HP <= 0 || !quest256Steps[ent.passo].area.contains(e.X, e.Y)) {
			ent.saiu = true
			d.entradasDaRodada[k] = ent
			d.log.Info("arena: saiu, rodada fechada", "conta", s.AccountName, "conn", s.Conn, "morto", e.HP <= 0)
		}
		if ent.saiu && e.HP > 0 && e.QuestFlag != 0 {
			e.QuestFlag = 0
		}
	})
	for k, ent := range d.entradasDaRodada {
		if !ent.saiu && !vistos[k] {
			ent.saiu = true // desconectou
			d.entradasDaRodada[k] = ent
		}
	}
}

// zeraEntradasDaRodada abre a rodada nova para todo mundo. Chamado pelo pulso,
// depois da expulsão.
func (d *Dispatcher) zeraEntradasDaRodada() {
	d.entradasDaRodada = nil
}
