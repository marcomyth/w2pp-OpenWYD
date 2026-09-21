package protocol

import (
	"encoding/binary"
	"fmt"
)

// Pontos em lote: gastar vários pontos de uma vez na janela de Personagem.
//
// O jogo só sabe gastar UM ponto por clique. Cada `+` da janela manda um
// MsgApplyBonus (0x0277) e o servidor soma um; para pôr 200 na Magia Branca são
// duzentos cliques, ou segurar o botão e torcer. Este pacote é o atalho: o
// jogador dá dois cliques no VALOR do campo (o "0/200", o "12"), digita quanto
// quer, e vem um pacote só.
//
// É nosso, na faixa 0x0Fxx que o WYD.exe não usa — o cliente que o manda é o
// nosso GamePatch.dll.
//
// O cliente não manda nada além do campo e da quantidade: quantos pontos o
// jogador tem, quanto cabe no campo e qual o teto do nível são contas do
// servidor. Um cliente remendado não tem o que mentir.
//
// TUDO OU NADA, decidido em 21/09/2026: se o número pedido não couber inteiro, o
// servidor não gasta nada e diz por quê — "não tem tantos pontos" é uma recusa
// diferente de "não cabe tudo isso no campo", e confundir as duas deixaria o
// jogador sem saber o que fazer. Aplicar o que coubesse seria gastar ponto que
// ele talvez quisesse pôr em outro lugar.
const (
	// PontosAtributo é o quadro de cima: FOR, INT, DES, CON.
	PontosAtributo = 1
	// PontosAprendizagem é o de baixo: Aprender Arma, Magia Branca, Magia Negra,
	// Magia Especial.
	PontosAprendizagem = 2
)

// PontosEmLoteMax é o maior pedido aceito numa mensagem. Não é regra de jogo —
// os tetos de verdade são os do personagem —, é só o limite que impede um corpo
// absurdo de virar um laço longo dentro do laço do mundo.
const PontosEmLoteMax = 1000

// pontosEmLoteSize é HEADER + Tipo + Campo + Quantidade = 16 bytes.
const pontosEmLoteSize = HeaderSize + 4

// MsgPontosEmLoteBody é o corpo de MsgPontosEmLote (C→S).
type MsgPontosEmLoteBody struct {
	Tipo       uint8 // PontosAtributo ou PontosAprendizagem
	Campo      uint8 // 0..3 dentro do quadro
	Quantidade int16
}

// Encode monta o corpo.
func (b MsgPontosEmLoteBody) Encode() []byte {
	out := make([]byte, pontosEmLoteSize-HeaderSize)
	out[0] = b.Tipo
	out[1] = b.Campo
	binary.LittleEndian.PutUint16(out[2:], uint16(b.Quantidade))
	return out
}

// Decode lê o corpo. Um corpo curto é recusado aqui, e não adivinhado.
func (b *MsgPontosEmLoteBody) Decode(payload []byte) error {
	if len(payload) < pontosEmLoteSize-HeaderSize {
		return fmt.Errorf("protocol: pontos em lote: corpo de %d bytes, quero %d",
			len(payload), pontosEmLoteSize-HeaderSize)
	}
	b.Tipo = payload[0]
	b.Campo = payload[1]
	b.Quantidade = int16(binary.LittleEndian.Uint16(payload[2:]))
	return nil
}
