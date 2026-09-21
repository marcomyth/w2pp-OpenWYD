package protocol

import (
	"encoding/binary"
	"fmt"
)

// Loja de Honra: o God of War trocando itens por pontos de lojinha.
//
// O NPC já existia no mapa de Armia e não fazia nada: Merchant 104, que nenhum
// tratador do servidor atende (ver handler/loja_de_honra.go). Ele passou a ser a
// porta da loja onde se gasta o que a lojinha aberta rendeu — os pontos de
// shop_points, 3 por quinze minutos, que até agora só se acumulavam.
//
// Por que pacotes nossos e não a janela de loja do jogo: a janela do cliente
// mostra preço com o cifrão do ouro e não sabe dizer "pontos", nem mostrar o
// saldo de quem está olhando, e tem 27 casas fixas. O painel é nosso
// (client/gamepatch/honra.cpp), então o preço aparece em pontos, o saldo fica no
// alto e o item que o jogador não pode pagar fica apagado.
//
//   MsgHonraAbre   (S→C): o estoque e o saldo de quem clicou.
//   MsgHonraCompra (C→S): "quero a casa N" — e nada mais; preço e item são lidos
//                         do estoque do servidor.
//   MsgHonraSaldo  (S→C): o saldo depois de uma compra.
//   MsgHonraFecha  (C→S): fechei o painel.
//
// Como a compra da vitrine (lojaservidor.go), o cliente não manda valor nenhum:
// ele aponta a casa, o servidor lê o preço. Um cliente remendado não tem o que
// mentir.

// HonraMaxItens é quanto o estoque pode ter. É a grade do painel que manda: 5
// colunas por 6 linhas. Igual à vitrine, são números que TÊM de bater com o
// cliente (kColunas/kLinhas em honra.cpp).
const HonraMaxItens = 30

// HonraItemSize é o tamanho de um item do estoque na linha: 12 bytes.
const HonraItemSize = 12

// HonraItem é uma troca oferecida: o item, e quantos pontos ele custa. Slot é a
// casa dentro do estoque — é por ele, e só por ele, que a compra se refere ao
// item.
type HonraItem struct {
	Slot   int16
	Indice int16
	Refino uint8
	Qtd    uint8
	Preco  int32
}

func (it *HonraItem) encode(b []byte) {
	binary.LittleEndian.PutUint16(b[0:], uint16(it.Slot))
	binary.LittleEndian.PutUint16(b[2:], uint16(it.Indice))
	b[4] = it.Refino
	b[5] = it.Qtd
	// b[6:8] é enchimento, para o preço cair alinhado em 8
	binary.LittleEndian.PutUint32(b[8:], uint32(it.Preco))
}

func (it *HonraItem) decode(b []byte) {
	it.Slot = int16(binary.LittleEndian.Uint16(b[0:]))
	it.Indice = int16(binary.LittleEndian.Uint16(b[2:]))
	it.Refino = b[4]
	it.Qtd = b[5]
	it.Preco = int32(binary.LittleEndian.Uint32(b[8:]))
}

// HonraAbreCabecalho é o que vem antes dos itens: saldo e quantos itens.
const HonraAbreCabecalho = 8

// HonraAbreBody é o corpo de MsgHonraAbre.
type HonraAbreBody struct {
	Saldo int32
	Itens []HonraItem
}

func (m *HonraAbreBody) Encode() []byte {
	n := len(m.Itens)
	if n > HonraMaxItens {
		n = HonraMaxItens
	}
	b := make([]byte, HonraAbreCabecalho+n*HonraItemSize)
	binary.LittleEndian.PutUint32(b[0:], uint32(m.Saldo))
	binary.LittleEndian.PutUint16(b[4:], uint16(n))
	// b[6:8] é enchimento
	for i := 0; i < n; i++ {
		m.Itens[i].encode(b[HonraAbreCabecalho+i*HonraItemSize:])
	}
	return b
}

func (m *HonraAbreBody) Decode(b []byte) error {
	if len(b) < HonraAbreCabecalho {
		return fmt.Errorf("honra: abertura com %d bytes, esperado ao menos %d", len(b), HonraAbreCabecalho)
	}
	m.Saldo = int32(binary.LittleEndian.Uint32(b[0:]))
	n := int(binary.LittleEndian.Uint16(b[4:]))
	if n > HonraMaxItens {
		n = HonraMaxItens
	}
	if len(b) < HonraAbreCabecalho+n*HonraItemSize {
		return fmt.Errorf("honra: abertura diz %d itens mas traz %d bytes", n, len(b))
	}
	m.Itens = make([]HonraItem, n)
	for i := 0; i < n; i++ {
		m.Itens[i].decode(b[HonraAbreCabecalho+i*HonraItemSize:])
	}
	return nil
}

// HonraCompraBodySize é o tamanho do pedido de compra: 4 bytes.
const HonraCompraBodySize = 4

// HonraCompraBody é o corpo de MsgHonraCompra: só a casa do estoque.
type HonraCompraBody struct {
	Slot int16
}

func (m *HonraCompraBody) Encode() []byte {
	b := make([]byte, HonraCompraBodySize)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Slot))
	return b
}

func (m *HonraCompraBody) Decode(b []byte) error {
	if len(b) < HonraCompraBodySize {
		return fmt.Errorf("honra: compra com %d bytes, esperado %d", len(b), HonraCompraBodySize)
	}
	m.Slot = int16(binary.LittleEndian.Uint16(b[0:]))
	return nil
}

// EncodeHonraSaldo monta o corpo de MsgHonraSaldo: o saldo, e nada mais.
func EncodeHonraSaldo(saldo int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b[0:], uint32(saldo))
	return b
}

// DecodeHonraSaldo lê o corpo de MsgHonraSaldo.
func DecodeHonraSaldo(b []byte) (int32, error) {
	if len(b) < 4 {
		return 0, fmt.Errorf("honra: saldo com %d bytes, esperado 4", len(b))
	}
	return int32(binary.LittleEndian.Uint32(b[0:])), nil
}
