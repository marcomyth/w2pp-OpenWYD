package protocol

import "fmt"

// A LIXEIRA EM LOTE (0x0F50 e 0x0F51).
//
// POR QUE UM PACOTE NOVO, E NÃO VINTE VEZES O 0x02E4, que é o apagar de um item só:
//
//   - O 0x02E4 NÃO SABE DIZER NÃO. Quando ele recusa, volta calado, e o jogador
//     ficaria olhando uma lista sem entender o que sobrou.
//   - O 0x02E4 NÃO CONFERE O ÍNDICE. Entre marcar e confirmar, o item pode ter
//     mudado de lugar — o jogador arrastou, caiu um drop, a fada empilhou. Aí o
//     apagado seria outro item, e não tem volta.
//   - O 0x02E4 CANCELA A TROCA em aberto. Vinte seguidos cancelariam na primeira e
//     não diriam nada das outras dezenove.
//
// O pacote novo leva o índice que o jogador VIU em cada slot, e o servidor recusa o
// que não bater.

// LixeiraMax é quantos itens cabem num lote: a mochila inteira.
const LixeiraMax = 60

// Os motivos de recusa. Os números são os do contrato com o cliente.
const (
	LixeiraMotivoSlotBloqueado uint8 = 1 // página da mochila sem a bolsa
	LixeiraMotivoVazio         uint8 = 2
	LixeiraMotivoOutroItem     uint8 = 3 // o índice mudou desde que ele marcou
	LixeiraMotivoRepetido      uint8 = 4 // o mesmo slot duas vezes no lote
	LixeiraMotivoEmTroca       uint8 = 5 // o lote inteiro; a troca NÃO é cancelada
	LixeiraMotivoNaoPode       uint8 = 6 // morto ou fora de jogo; o lote inteiro
	LixeiraMotivoPacoteRuim    uint8 = 7 // n fora de 1..60, ou tamanho que não fecha
)

// LixeiraPedido é uma linha do lote: o slot e o índice que o jogador viu nele.
type LixeiraPedido struct {
	Slot   int16
	Indice int16
}

// LixeiraApagaBody é o 0x0F50.
type LixeiraApagaBody struct {
	Itens []LixeiraPedido
}

// DecodeLixeiraApaga lê o lote.
//
// O TAMANHO TEM DE FECHAR COM O `n`, e um que não fecha recusa o pacote INTEIRO em
// vez de ler o que der. Um lote truncado lido pela metade apagaria itens que o
// jogador não chegou a marcar — e apagar item não tem volta.
func DecodeLixeiraApaga(b []byte) (LixeiraApagaBody, error) {
	var out LixeiraApagaBody
	if len(b) < 4 {
		return out, fmt.Errorf("protocol: lixeira curta: %d", len(b))
	}
	n := int(b[0])
	if n < 1 || n > LixeiraMax {
		return out, fmt.Errorf("protocol: lixeira com %d itens, fora de 1..%d", n, LixeiraMax)
	}
	if len(b) < 4+4*n {
		return out, fmt.Errorf("protocol: lixeira truncada: %d itens nao cabem em %d bytes", n, len(b))
	}
	out.Itens = make([]LixeiraPedido, 0, n)
	for i := 0; i < n; i++ {
		p := 4 + 4*i
		out.Itens = append(out.Itens, LixeiraPedido{
			Slot:   int16(le.Uint16(b[p:])),
			Indice: int16(le.Uint16(b[p+2:])),
		})
	}
	return out, nil
}

// LixeiraRecusa é um slot que ficou, e por quê.
type LixeiraRecusa struct {
	Slot   int16
	Motivo uint8
}

// EncodeLixeiraResultado escreve o 0x0F51.
//
// SEMPRE HÁ UMA RESPOSTA, E SÓ UMA, para cada pedido — inclusive quando nada foi
// apagado. A janela do cliente espera por ela e, sem resposta, diz "sem resposta do
// servidor" depois de cinco segundos: o silêncio vira um erro que não aconteceu.
func EncodeLixeiraResultado(apagados uint8, recusas []LixeiraRecusa) []byte {
	b := make([]byte, 4+4*len(recusas))
	b[0] = apagados
	b[1] = uint8(len(recusas))
	for i, r := range recusas {
		p := 4 + 4*i
		le.PutUint16(b[p:], uint16(r.Slot))
		b[p+2] = r.Motivo
	}
	return b
}
