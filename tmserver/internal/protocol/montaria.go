package protocol

import (
	"encoding/binary"
	"fmt"
)

// Painel de up da montaria adulta: o jogador escolhe a pilha de âmago e quantas
// pilhas gastar ("Pac's"), e o servidor dá âmago após âmago na montaria vestida,
// de uma vez.
//
// Por que pacotes nossos e não o MSG_UseItem (0x373) repetido: é o mesmo motivo do
// refino (protocol/refino.go) — no arrasto o cliente mexe na pilha sozinho e tem
// trava entre usos. Aqui o laço é do servidor (handler/montaria_lote.go).
//
// UM PAC É UMA PILHA DO INVENTÁRIO, gasta inteira, seja do tamanho que for (o
// tamanho máximo da pilha é configuração do servidor, até 255). Decisão da dona.
//
//   MsgMontariaPede      (C→S): dê à montaria vestida o âmago da pilha slotAmago,
//                               gastando pilhas pilhas inteiras (0 = todas).
//   MsgMontariaResultado (S→C): usados, sucessos, falhas, níveis perdidos, nível
//                               inicial e final, pilhas gastas e o motivo.
//
// 0x0F60 e 0x0F61 ficam reservados: a spec previa uma consulta das taxas, e a dona
// decidiu que o painel não mostra chance.

// MontariaPedeBodySize é o tamanho do pedido: 8 bytes.
const MontariaPedeBodySize = 8

// MontariaPedeBody é o corpo de MsgMontariaPede.
type MontariaPedeBody struct {
	Pilhas    uint8 // quantas pilhas gastar inteiras; 0 = todas
	SlotAmago int16 // slot do carry com a pilha escolhida: é gasta primeiro
}

func (m *MontariaPedeBody) Encode() []byte {
	b := make([]byte, MontariaPedeBodySize)
	b[0] = m.Pilhas
	// b[1] reservado
	binary.LittleEndian.PutUint16(b[2:], uint16(m.SlotAmago))
	// b[4:8] reservado
	return b
}

func (m *MontariaPedeBody) Decode(b []byte) error {
	if len(b) < MontariaPedeBodySize {
		return fmt.Errorf("montaria: pedido com %d bytes, esperado %d", len(b), MontariaPedeBodySize)
	}
	m.Pilhas = b[0]
	m.SlotAmago = int16(binary.LittleEndian.Uint16(b[2:]))
	return nil
}

// Por que o lote parou. Os números TÊM de bater com montariarede.h no cliente.
const (
	MontariaAcabouPacs uint8 = 0 // gastou as pilhas pedidas (ou o teto do servidor)
	MontariaSemAmago   uint8 = 1 // acabou o âmago da linhagem no carry
	MontariaNoMaximo   uint8 = 2 // nível 120
	MontariaMorta      uint8 = 3 // HP da montaria vestida é 0
	MontariaNaoAdulta  uint8 = 4 // nada em Equip[14], ou é cria/ovo
	MontariaInvalido   uint8 = 5 // slot ruim, vazio, ou âmago de outra linhagem
	MontariaOcupado    uint8 = 6 // troca aberta ou loja aberta
)

// MontariaResultadoBodySize é o tamanho da resposta: 16 bytes.
const MontariaResultadoBodySize = 16

// MontariaResultadoBody é o corpo de MsgMontariaResultado.
type MontariaResultadoBody struct {
	Motivo       uint8
	NivelInicial uint8
	NivelFinal   uint8
	Usados       uint16 // = Sucessos + Falhas
	Sucessos     uint16
	Falhas       uint16
	Quedas       uint16 // níveis perdidos nas falhas (1 em 5)
	Pilhas       uint8  // pilhas que acabaram no lote
	Montaria     int16  // índice da montaria vestida no fim
}

func (m *MontariaResultadoBody) Encode() []byte {
	b := make([]byte, MontariaResultadoBodySize)
	b[0] = m.Motivo
	b[1] = m.NivelInicial
	b[2] = m.NivelFinal
	b[3] = m.Pilhas
	binary.LittleEndian.PutUint16(b[4:], m.Usados)
	binary.LittleEndian.PutUint16(b[6:], m.Sucessos)
	binary.LittleEndian.PutUint16(b[8:], m.Falhas)
	binary.LittleEndian.PutUint16(b[10:], m.Quedas)
	binary.LittleEndian.PutUint16(b[12:], uint16(m.Montaria))
	// b[14:16] reservado
	return b
}

func (m *MontariaResultadoBody) Decode(b []byte) error {
	if len(b) < MontariaResultadoBodySize {
		return fmt.Errorf("montaria: resultado com %d bytes, esperado %d", len(b), MontariaResultadoBodySize)
	}
	m.Motivo = b[0]
	m.NivelInicial = b[1]
	m.NivelFinal = b[2]
	m.Pilhas = b[3]
	m.Usados = binary.LittleEndian.Uint16(b[4:])
	m.Sucessos = binary.LittleEndian.Uint16(b[6:])
	m.Falhas = binary.LittleEndian.Uint16(b[8:])
	m.Quedas = binary.LittleEndian.Uint16(b[10:])
	m.Montaria = int16(binary.LittleEndian.Uint16(b[12:]))
	return nil
}
