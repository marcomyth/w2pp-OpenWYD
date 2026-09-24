package protocol

import (
	"encoding/binary"
	"fmt"
)

// Painel de refino: o jogador escolhe um item, uma poeira (Ori ou Lac) e um nível
// alvo, e o servidor refina de uma vez, poeira após poeira, até chegar.
//
// Por que pacotes nossos e não o MSG_UseItem (0x373) repetido: no arrasto o
// cliente tira a poeira da pilha por conta própria antes de mandar, e tem uma
// trava de 200 ms entre usos. Repetir 0x373 deixaria a pilha da tela descasada da
// do servidor e custaria 200 ms por poeira. Aqui o laço é do servidor
// (handler/refino_lote.go), e o cliente só relê o inventário quando os slots
// mexidos voltam.
//
//   MsgRefinoPede      (C→S): refine o item de lugar/slot com a poeira de
//                             slotPoeira até +alvo, gastando no máximo maxPoeiras.
//   MsgRefinoResultado (S→C): usadas, sucessos, falhas, nível inicial e final, e
//                             o motivo de ter parado.

// RefinoPedeBodySize é o tamanho do pedido: 12 bytes.
const RefinoPedeBodySize = 12

// RefinoPedeBody é o corpo de MsgRefinoPede.
type RefinoPedeBody struct {
	Lugar      int16 // 0 = equipado, 1 = carry
	Slot       int16
	SlotPoeira int16 // slot do carry com a poeira escolhida
	Alvo       uint8 // nível desejado, 1..11
	MaxPoeiras uint16
}

func (m *RefinoPedeBody) Encode() []byte {
	b := make([]byte, RefinoPedeBodySize)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Lugar))
	binary.LittleEndian.PutUint16(b[2:], uint16(m.Slot))
	binary.LittleEndian.PutUint16(b[4:], uint16(m.SlotPoeira))
	b[6] = m.Alvo
	// b[7] reservado
	binary.LittleEndian.PutUint16(b[8:], m.MaxPoeiras)
	// b[10:12] reservado
	return b
}

func (m *RefinoPedeBody) Decode(b []byte) error {
	if len(b) < RefinoPedeBodySize {
		return fmt.Errorf("refino: pedido com %d bytes, esperado %d", len(b), RefinoPedeBodySize)
	}
	m.Lugar = int16(binary.LittleEndian.Uint16(b[0:]))
	m.Slot = int16(binary.LittleEndian.Uint16(b[2:]))
	m.SlotPoeira = int16(binary.LittleEndian.Uint16(b[4:]))
	m.Alvo = b[6]
	m.MaxPoeiras = binary.LittleEndian.Uint16(b[8:])
	return nil
}

// Por que o lote parou. Os números TÊM de bater com refinorede.h no cliente.
const (
	RefinoChegou    uint8 = 0 // nível final >= alvo
	RefinoSemPoeira uint8 = 1 // acabou a poeira daquele tipo no carry
	RefinoTeto      uint8 = 2 // chegou em maxPoeiras ou no teto do servidor
	RefinoMuro      uint8 = 3 // o refino recusou continuar (Ori em +6, +9, +11, 769, EF_NOSANC, capa do Celestial)
	RefinoQuebrou   uint8 = 4 // o item sumiu na tentativa (nenhum ramo que quebra está portado hoje)
	RefinoInvalido  uint8 = 5 // pedido ruim: slot vazio ou fora do limite, poeira errada, alvo fora da faixa
	RefinoOcupado   uint8 = 6 // troca aberta ou loja aberta
	RefinoNaoServe  uint8 = 7 // o item é tintura, pedra Arch, ovo ou consumível
)

// RefinoResultadoBodySize é o tamanho da resposta: 16 bytes.
const RefinoResultadoBodySize = 16

// RefinoResultadoBody é o corpo de MsgRefinoResultado.
type RefinoResultadoBody struct {
	Motivo       uint8
	NivelInicial uint8
	NivelFinal   uint8
	Usadas       uint16 // = Sucessos + Falhas
	Sucessos     uint16
	Falhas       uint16
	Lugar        int16 // eco do pedido
	Slot         int16 // eco do pedido
}

func (m *RefinoResultadoBody) Encode() []byte {
	b := make([]byte, RefinoResultadoBodySize)
	b[0] = m.Motivo
	b[1] = m.NivelInicial
	b[2] = m.NivelFinal
	// b[3] reservado
	binary.LittleEndian.PutUint16(b[4:], m.Usadas)
	binary.LittleEndian.PutUint16(b[6:], m.Sucessos)
	binary.LittleEndian.PutUint16(b[8:], m.Falhas)
	binary.LittleEndian.PutUint16(b[10:], uint16(m.Lugar))
	binary.LittleEndian.PutUint16(b[12:], uint16(m.Slot))
	// b[14:16] reservado
	return b
}

func (m *RefinoResultadoBody) Decode(b []byte) error {
	if len(b) < RefinoResultadoBodySize {
		return fmt.Errorf("refino: resultado com %d bytes, esperado %d", len(b), RefinoResultadoBodySize)
	}
	m.Motivo = b[0]
	m.NivelInicial = b[1]
	m.NivelFinal = b[2]
	m.Usadas = binary.LittleEndian.Uint16(b[4:])
	m.Sucessos = binary.LittleEndian.Uint16(b[6:])
	m.Falhas = binary.LittleEndian.Uint16(b[8:])
	m.Lugar = int16(binary.LittleEndian.Uint16(b[10:]))
	m.Slot = int16(binary.LittleEndian.Uint16(b[12:]))
	return nil
}
