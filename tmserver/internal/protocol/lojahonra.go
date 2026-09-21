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
//   MsgHonraFechou (S→C): feche o painel — o NPC saiu de vista.
//
// Como a compra da vitrine (lojaservidor.go), o cliente não manda valor nenhum:
// ele aponta a casa, o servidor lê o preço. Um cliente remendado não tem o que
// mentir.

// HonraMaxItens é quanto o estoque pode ter. É a grade do painel que manda: 5
// colunas por 7 linhas, a mesma da vitrine, porque a janela tem a medida fixa da
// janela de inventário do jogo e é esse o tanto que cabe nela. São números que
// TÊM de bater com o cliente (kColunas/kLinhas em honra.cpp e kMaxItens em
// honrarede.cpp).
const HonraMaxItens = 35

// HonraItemSize é o tamanho de um item do estoque na linha: 12 bytes.
const HonraItemSize = 12

// Abas do painel, na ordem em que aparecem. A categoria viaja com o item porque
// só o servidor sabe em que aba ele vai: o cliente não tem catálogo nenhum, ele
// só desenha o ícone pelo índice. Zero é "sem categoria" e aparece apenas na aba
// Todos — é o que um item novo, ainda não classificado, faz por conta própria, em
// vez de cair na aba errada.
const (
	HonraCatNenhuma uint8 = 0
	HonraCatArmas   uint8 = 1
	HonraCatSet     uint8 = 2
	HonraCatConsumo uint8 = 3
)

// HonraItem é uma troca oferecida: o item, e quantos pontos ele custa. Slot é a
// casa dentro do estoque — é por ele, e só por ele, que a compra se refere ao
// item.
type HonraItem struct {
	Slot      int16
	Indice    int16
	Refino    uint8
	Qtd       uint8
	Categoria uint8
	Preco     int32
}

func (it *HonraItem) encode(b []byte) {
	binary.LittleEndian.PutUint16(b[0:], uint16(it.Slot))
	binary.LittleEndian.PutUint16(b[2:], uint16(it.Indice))
	b[4] = it.Refino
	b[5] = it.Qtd
	b[6] = it.Categoria
	// b[7] é enchimento, para o preço cair alinhado em 8
	binary.LittleEndian.PutUint32(b[8:], uint32(it.Preco))
}

func (it *HonraItem) decode(b []byte) {
	it.Slot = int16(binary.LittleEndian.Uint16(b[0:]))
	it.Indice = int16(binary.LittleEndian.Uint16(b[2:]))
	it.Refino = b[4]
	it.Qtd = b[5]
	it.Categoria = b[6]
	it.Preco = int32(binary.LittleEndian.Uint32(b[8:]))
}

// HonraAbreCabecalho é o que vem antes dos itens: saldo, quantos itens, e quanto
// a lojinha aberta rende.
const HonraAbreCabecalho = 12

// HonraAbreBody é o corpo de MsgHonraAbre.
//
// PorJanela e MinutosJanela existem para o painel escrever, em português, como se
// ganham os pontos que ele cobra — "a cada 15 min com a loja aberta: +3 pontos".
// Vão do servidor e não são escritos no cliente porque os dois números são regra
// do servidor: a janela é shopPointsWindowMs, e o ganho é 3, ou 7 para quem está
// com uma Fada Azul (shoppoints.go). Um cliente que escrevesse "3" fixo mentiria
// para metade dos jogadores no dia em que a regra mudasse.
type HonraAbreBody struct {
	Saldo         int32
	PorJanela     int16 // pontos por janela, para ESTE jogador, agora
	MinutosJanela int16
	Itens         []HonraItem
}

func (m *HonraAbreBody) Encode() []byte {
	n := len(m.Itens)
	if n > HonraMaxItens {
		n = HonraMaxItens
	}
	b := make([]byte, HonraAbreCabecalho+n*HonraItemSize)
	binary.LittleEndian.PutUint32(b[0:], uint32(m.Saldo))
	binary.LittleEndian.PutUint16(b[4:], uint16(n))
	binary.LittleEndian.PutUint16(b[6:], uint16(m.PorJanela))
	binary.LittleEndian.PutUint16(b[8:], uint16(m.MinutosJanela))
	// b[10:12] é enchimento, para os itens caírem alinhados em 4
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
	m.PorJanela = int16(binary.LittleEndian.Uint16(b[6:]))
	m.MinutosJanela = int16(binary.LittleEndian.Uint16(b[8:]))
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
