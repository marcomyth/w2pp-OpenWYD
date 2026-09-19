package protocol

import (
	"encoding/binary"
	"fmt"
)

// Loja do Servidor: a vitrine global (issue da loja, 18/09/2026).
//
// A lojinha pessoal do jogo continua igual — o jogador monta a barraca com
// MSG_SendAutoTrade (0x0397) e ela fica de pé no mapa. O que a vitrine faz é
// mostrar, num painel só, o que TODAS as barracas abertas estão vendendo, para
// quem estiver em qualquer canto do mundo.
//
// Por isso estes dois pacotes não existem no cliente original: eles são nossos,
// e ficam numa faixa que o WYD.exe não usa (0x0Fxx). Quem os envia e recebe do
// lado do cliente é o GamePatch.dll (client/gamepatch/loja.cpp).
//
//   MsgLojaPede  (C→S): "me dá a página N da vitrine, com este filtro".
//   MsgLojaLista (S→C): uma página de ofertas.
//   MsgLojaMoeda (C→S): "este item da MINHA barraca é vendido nesta moeda".
//   MsgLojaCompra(C→S): "quero este item desta barraca, nesta moeda".
//
// Nada disso é persistido: a vitrine é montada na hora a partir das barracas
// abertas, então fechar a lojinha tira as ofertas do ar no mesmo instante.

// LojaPorPagina é quantas ofertas cabem numa página — a grade do painel do
// cliente tem 8 colunas por 4 linhas.
const LojaPorPagina = 32

// lojaNomeLen é o buffer do nome do vendedor, como em MAX_NAME.
const lojaNomeLen = 16

// Moedas de uma oferta. Hoje toda barraca do jogo vende por ouro; Cash e RMT
// entram quando a lojinha passar a ser montada pelo nosso painel.
const (
	LojaMoedaOuro uint8 = 0
	LojaMoedaCash uint8 = 1
	LojaMoedaRMT  uint8 = 2
)

// Filtros aceitos no pedido, na ordem do menu do painel.
const (
	LojaFiltroTodos int16 = 0
	LojaFiltroOuro  int16 = 1
	LojaFiltroCash  int16 = 2
	LojaFiltroRMT   int16 = 3
	LojaFiltroMeus  int16 = 4
)

// LojaPedeBody é o corpo de MsgLojaPede: 4 bytes.
type LojaPedeBody struct {
	Pagina int16
	Filtro int16
}

// LojaPedeBodySize é o tamanho do corpo do pedido.
const LojaPedeBodySize = 4

func (m *LojaPedeBody) Decode(b []byte) error {
	if len(b) < LojaPedeBodySize {
		return fmt.Errorf("loja: pedido com %d bytes, esperado %d", len(b), LojaPedeBodySize)
	}
	m.Pagina = int16(binary.LittleEndian.Uint16(b[0:]))
	m.Filtro = int16(binary.LittleEndian.Uint16(b[2:]))
	return nil
}

func (m *LojaPedeBody) Encode() []byte {
	b := make([]byte, LojaPedeBodySize)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Pagina))
	binary.LittleEndian.PutUint16(b[2:], uint16(m.Filtro))
	return b
}

// LojaOfertaSize é o tamanho de uma oferta na linha: 32 bytes.
const LojaOfertaSize = 32

// LojaOferta é um item à venda numa barraca aberta. Vendedor é o id da barraca
// (o mesmo que o cliente usa para pedir a lista dela e para comprar), e Slot é a
// posição dentro da barraca — os dois juntos são o que a compra precisa.
type LojaOferta struct {
	Vendedor int32
	Nome     string
	Indice   int16
	Slot     int8
	Refino   uint8
	Qtd      uint8
	Moeda    uint8
	// Perto é 1 quando a barraca está ao alcance de compra de quem pediu a lista.
	// A vitrine junta a cidade inteira, mas a compra é a do jogo e exige
	// proximidade, então o painel precisa saber o que dá para levar agora.
	Perto uint8
	Preco int32
}

func (o *LojaOferta) encode(b []byte) {
	binary.LittleEndian.PutUint32(b[0:], uint32(o.Vendedor))
	nome := o.Nome
	if len(nome) >= lojaNomeLen {
		nome = nome[:lojaNomeLen-1]
	}
	copy(b[4:4+lojaNomeLen], nome)
	binary.LittleEndian.PutUint16(b[20:], uint16(o.Indice))
	b[22] = byte(o.Slot)
	b[23] = o.Refino
	b[24] = o.Qtd
	b[25] = o.Moeda
	b[26] = o.Perto
	// b[27] é enchimento, para a oferta fechar em 32 bytes
	binary.LittleEndian.PutUint32(b[28:], uint32(o.Preco))
}

func (o *LojaOferta) decode(b []byte) {
	o.Vendedor = int32(binary.LittleEndian.Uint32(b[0:]))
	o.Nome = cTrimNUL(b[4 : 4+lojaNomeLen])
	o.Indice = int16(binary.LittleEndian.Uint16(b[20:]))
	o.Slot = int8(b[22])
	o.Refino = b[23]
	o.Qtd = b[24]
	o.Moeda = b[25]
	o.Perto = b[26]
	o.Preco = int32(binary.LittleEndian.Uint32(b[28:]))
}

// LojaMoedaBody é o corpo de MsgLojaMoeda: o dono diz em que moeda um item da
// barraca dele é vendido. O preço continua sendo o que ele digitou ao montar a
// barraca — o que muda é em que moeda aquele número é cobrado.
type LojaMoedaBody struct {
	Slot  int8
	Moeda uint8
}

// LojaMoedaBodySize é o tamanho do corpo.
const LojaMoedaBodySize = 2

func (m *LojaMoedaBody) Decode(b []byte) error {
	if len(b) < LojaMoedaBodySize {
		return fmt.Errorf("loja: moeda com %d bytes, esperado %d", len(b), LojaMoedaBodySize)
	}
	m.Slot = int8(b[0])
	m.Moeda = b[1]
	return nil
}

func (m *LojaMoedaBody) Encode() []byte {
	return []byte{byte(m.Slot), m.Moeda}
}

// lojaCabecalhoLista é o cabeçalho da página: contagem e os três saldos de quem
// pediu, que o painel mostra na faixa de cima.
const lojaCabecalhoLista = 8 + 12

// LojaCompraBody é o corpo de MsgLojaCompra. Repare no que NÃO vai aqui: preço
// e item. O servidor lê os dois da barraca; o cliente só aponta qual oferta, e a
// moeda que ele estava mostrando ao jogador — se o vendedor tiver trocado a
// moeda nesse meio-tempo, a compra não sai.
type LojaCompraBody struct {
	Vendedor int32
	Slot     int8
	Moeda    uint8
}

// LojaCompraBodySize é o tamanho do corpo: 8 bytes com o enchimento.
const LojaCompraBodySize = 8

func (m *LojaCompraBody) Decode(b []byte) error {
	if len(b) < LojaCompraBodySize {
		return fmt.Errorf("loja: compra com %d bytes, esperado %d", len(b), LojaCompraBodySize)
	}
	m.Vendedor = int32(binary.LittleEndian.Uint32(b[0:]))
	m.Slot = int8(b[4])
	m.Moeda = b[5]
	return nil
}

func (m *LojaCompraBody) Encode() []byte {
	b := make([]byte, LojaCompraBodySize)
	binary.LittleEndian.PutUint32(b[0:], uint32(m.Vendedor))
	b[4] = byte(m.Slot)
	b[5] = m.Moeda
	return b
}

// LojaListaBodySize é o corpo da resposta: o cabeçalho da página e as ofertas.
const LojaListaBodySize = lojaCabecalhoLista + LojaPorPagina*LojaOfertaSize

// LojaListaBody é uma página da vitrine. Total é quantas ofertas existem no
// filtro pedido (não só nesta página), para o painel montar o "1/3".
//
// Os saldos viajam junto porque o painel precisa deles para decidir o que o
// jogador consegue pagar. Cash e RMT ainda vêm zerados: esses saldos vivem na
// conta, do lado do site (CreditDonateBalance, api/web), e o tmServer ainda não
// tem por onde lê-los nem debitá-los.
type LojaListaBody struct {
	Pagina  int16
	Paginas int16
	Total   int16
	Qtd     int16
	Ouro    int32
	Cash    int32
	RMT     int32
	Ofertas [LojaPorPagina]LojaOferta
}

func (m *LojaListaBody) Encode() []byte {
	b := make([]byte, LojaListaBodySize)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Pagina))
	binary.LittleEndian.PutUint16(b[2:], uint16(m.Paginas))
	binary.LittleEndian.PutUint16(b[4:], uint16(m.Total))
	binary.LittleEndian.PutUint16(b[6:], uint16(m.Qtd))
	binary.LittleEndian.PutUint32(b[8:], uint32(m.Ouro))
	binary.LittleEndian.PutUint32(b[12:], uint32(m.Cash))
	binary.LittleEndian.PutUint32(b[16:], uint32(m.RMT))
	for i := 0; i < LojaPorPagina; i++ {
		m.Ofertas[i].encode(b[lojaCabecalhoLista+i*LojaOfertaSize:])
	}
	return b
}

func (m *LojaListaBody) Decode(b []byte) error {
	if len(b) < LojaListaBodySize {
		return fmt.Errorf("loja: lista com %d bytes, esperado %d", len(b), LojaListaBodySize)
	}
	m.Pagina = int16(binary.LittleEndian.Uint16(b[0:]))
	m.Paginas = int16(binary.LittleEndian.Uint16(b[2:]))
	m.Total = int16(binary.LittleEndian.Uint16(b[4:]))
	m.Qtd = int16(binary.LittleEndian.Uint16(b[6:]))
	m.Ouro = int32(binary.LittleEndian.Uint32(b[8:]))
	m.Cash = int32(binary.LittleEndian.Uint32(b[12:]))
	m.RMT = int32(binary.LittleEndian.Uint32(b[16:]))
	for i := 0; i < LojaPorPagina; i++ {
		m.Ofertas[i].decode(b[lojaCabecalhoLista+i*LojaOfertaSize:])
	}
	return nil
}
