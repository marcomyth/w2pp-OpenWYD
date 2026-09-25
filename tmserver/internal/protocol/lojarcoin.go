package protocol

import (
	"encoding/binary"
	"errors"
)

// Loja de Rcoin: as ofertas do site vendidas DENTRO do jogo.
//
// São as mesmas linhas do `donate_shop_item` que a loja do site mostra, pagas com a
// mesma carteira (`account.donate_balance`). O jogador não precisa mais sair do jogo,
// abrir o navegador e voltar para gastar o que comprou.
//
// Faixa 0x0F0C-0x0F0F, vizinha da Loja do Servidor (0x0F01-0x0F0B) e da Loja de Honra
// (0x0F20+). Nenhuma delas existe no cliente original: o WYD.exe não usa a faixa 0x0Fxx,
// e é por isso que ela é nossa.
//
// O CONTRATO ESTÁ CONGELADO com a dupla do cliente, e o número de bytes de cada campo é
// parte dele. Um campo que mude de tamanho aqui desenha lixo lá, porque o outro lado lê
// por deslocamento fixo — não há nome de campo no fio.

// Tamanhos do pacote da vitrine de Rcoin.
const (
	// RcoinPorPagina é quantas ofertas cabem numa página.
	//
	// VINTE, e não dezesseis: com 16 as 72 ofertas davam cinco páginas, e as duas
	// maiores categorias (Cosméticos, 20, e Consumíveis, 17) não cabiam numa tela
	// só — quem quisesse ver as pinturas de arma tinha de paginar no meio da
	// categoria. Com 20 cada categoria cabe inteira.
	RcoinPorPagina = 20

	// RcoinOfertaSize é o tamanho de uma oferta no fio.
	RcoinOfertaSize = 64

	// rcoinCabecalho é o cabeçalho da página: página, páginas, quantas, reservado,
	// e o saldo.
	rcoinCabecalho = 8

	// RcoinListaBodySize é o corpo inteiro: 8 + 20*64 = 1288 bytes.
	RcoinListaBodySize = rcoinCabecalho + RcoinPorPagina*RcoinOfertaSize

	// RcoinTituloSize é o espaço do título no fio, com o zero do fim incluído.
	//
	// O TEXTO ÚTIL SÃO 41 BYTES, e o último tem de ser zero: o cliente desenha até
	// achar o zero, e um título que ocupasse os 42 sairia colado no campo seguinte.
	RcoinTituloSize = 42

	// RcoinCategorias é quantas categorias existem (1 a 6). Zero, no pedido, quer
	// dizer "todas".
	RcoinCategorias = 6
)

// Erros de decodificação. São previstos: um cliente remendado manda qualquer coisa, e o
// servidor responde com silêncio, não com pânico.
var (
	ErrRcoinPedeCurto   = errors.New("protocol: pedido da loja de rcoin curto")
	ErrRcoinCompraCurta = errors.New("protocol: compra da loja de rcoin curta")
)

// RcoinPedeBody é o 0x0F0C: qual categoria e qual página.
//
// Categoria 0 é "todas", e 1 a 6 são as do cadastro. O servidor é quem valida: uma
// categoria fora da faixa vira lista vazia, e não recusa — o painel do cliente sabe
// desenhar "nada aqui", e não sabe desenhar um erro.
type RcoinPedeBody struct {
	Categoria int16
	Pagina    int16
}

// RcoinPedeBodySize são os quatro bytes do pedido.
const RcoinPedeBodySize = 4

func (m RcoinPedeBody) Encode() []byte {
	b := make([]byte, RcoinPedeBodySize)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Categoria))
	binary.LittleEndian.PutUint16(b[2:], uint16(m.Pagina))
	return b
}

func (m *RcoinPedeBody) Decode(b []byte) error {
	if len(b) < RcoinPedeBodySize {
		return ErrRcoinPedeCurto
	}
	m.Categoria = int16(binary.LittleEndian.Uint16(b[0:]))
	m.Pagina = int16(binary.LittleEndian.Uint16(b[2:]))
	return nil
}

// RcoinOferta é uma linha da vitrine, do jeito que ela viaja.
//
// O ID É O DA LINHA NO BANCO, e volta inteiro na compra. O cliente não manda posição na
// página: entre a página que ele desenhou e o clique, uma oferta pode ter sido desligada
// pelo painel, e a posição apontaria para a oferta seguinte — quem clicasse em "Fada
// Azul 3 dias" compraria "Fada Azul 5 dias" pelo preço dela.
type RcoinOferta struct {
	ID        uint32
	ItemIndex int16
	Efeitos   [3]Effect
	Categoria uint8
	Preco     int32
	Dias      uint16
	Titulo    string
}

// Effect é um par efeito/valor do item, como o STRUCT_ITEM do jogo os guarda.
type Effect struct {
	Effect uint8
	Value  uint8
}

// encodeEm escreve a oferta nos 64 bytes a partir de b.
//
// Os deslocamentos estão escritos um a um, e não derivados de um tamanho de struct: o
// Go não promete a disposição dos campos na memória, e o fio promete. É a mesma regra do
// resto do protocolo.
func (o RcoinOferta) encodeEm(b []byte) {
	binary.LittleEndian.PutUint32(b[0:], o.ID)
	binary.LittleEndian.PutUint16(b[4:], uint16(o.ItemIndex))
	for i, ef := range o.Efeitos {
		b[6+i*2] = ef.Effect
		b[7+i*2] = ef.Value
	}
	b[12] = o.Categoria
	b[13] = 0 // reservado, sempre zero
	binary.LittleEndian.PutUint32(b[14:], uint32(o.Preco))
	binary.LittleEndian.PutUint16(b[18:], o.Dias)
	b[20], b[21] = 0, 0 // reservados, sempre zero
	poeTitulo(b[22:22+RcoinTituloSize], o.Titulo)
}

func (o *RcoinOferta) decodeDe(b []byte) {
	o.ID = binary.LittleEndian.Uint32(b[0:])
	o.ItemIndex = int16(binary.LittleEndian.Uint16(b[4:]))
	for i := range o.Efeitos {
		o.Efeitos[i] = Effect{Effect: b[6+i*2], Value: b[7+i*2]}
	}
	o.Categoria = b[12]
	o.Preco = int32(binary.LittleEndian.Uint32(b[14:]))
	o.Dias = binary.LittleEndian.Uint16(b[18:])
	o.Titulo = FromClientText(pegaAteOZero(b[22 : 22+RcoinTituloSize]))
}

// poeTitulo escreve o título em cp1252, cortado, e com o zero do fim garantido.
//
// O TÍTULO VAI COM ACENTO. Em Windows-1252 um acento ocupa UM byte, o cliente desenha os
// painéis com TEXT nativo do WYD.exe na fonte Tahoma, e a dupla do cliente mediu o
// acento na tela em 25/09/2026. Tirar os acentos aqui deixaria "Poção de Ração" virar
// "Pocao de Racao" em toda a loja, por precaução contra um problema que não existe — e o
// caminho contrário, um nome chegando quebrado por falta de decodificação, foi defeito
// de verdade neste mesmo dia, no catálogo.
//
// O CORTE É EM BYTES, e depois da codificação, porque é byte que o campo conta. Cortar a
// string em runas antes de codificar erraria para mais em qualquer título acentuado.
func poeTitulo(dst []byte, s string) {
	for i := range dst {
		dst[i] = 0
	}
	b := ClientText(s)
	if len(b) > len(dst)-1 {
		b = b[:len(dst)-1]
	}
	copy(dst, b)
}

func pegaAteOZero(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// RcoinListaBody é o 0x0F0D: uma página da vitrine de Rcoin.
//
// O SALDO VIAJA JUNTO, e é lido do banco na montagem desta página. O painel precisa dele
// para decidir o que o jogador consegue pagar, e mandar o da sessão traria o número do
// login — o mesmo defeito que a Loja do Servidor teve até hoje.
type RcoinListaBody struct {
	Pagina  uint8
	Paginas uint8
	Qtd     uint8
	Saldo   int32
	Ofertas [RcoinPorPagina]RcoinOferta
}

func (m RcoinListaBody) Encode() []byte {
	b := make([]byte, RcoinListaBodySize)
	b[0] = m.Pagina
	b[1] = m.Paginas
	b[2] = m.Qtd
	b[3] = 0 // reservado, sempre zero
	binary.LittleEndian.PutUint32(b[4:], uint32(m.Saldo))
	for i := 0; i < int(m.Qtd) && i < RcoinPorPagina; i++ {
		inicio := rcoinCabecalho + i*RcoinOfertaSize
		m.Ofertas[i].encodeEm(b[inicio : inicio+RcoinOfertaSize])
	}
	return b
}

func (m *RcoinListaBody) Decode(b []byte) error {
	if len(b) < RcoinListaBodySize {
		return ErrRcoinPedeCurto
	}
	m.Pagina, m.Paginas, m.Qtd = b[0], b[1], b[2]
	m.Saldo = int32(binary.LittleEndian.Uint32(b[4:]))
	for i := 0; i < int(m.Qtd) && i < RcoinPorPagina; i++ {
		inicio := rcoinCabecalho + i*RcoinOfertaSize
		m.Ofertas[i].decodeDe(b[inicio : inicio+RcoinOfertaSize])
	}
	return nil
}

// RcoinCompraBody é o 0x0F0E: comprar uma oferta.
//
// O PEDIDO é um número que o CLIENTE escolhe e repete se reenviar. Ele é o que separa
// "cliquei duas vezes" de "quero comprar duas": o servidor guarda o último pedido de
// cada sessão com o resultado dele, e um pedido repetido devolve o resultado guardado
// sem cobrar de novo.
//
// O PREÇO VISTO é o que estava na tela. A staff pode mudar o preço pelo painel entre a
// página e o clique, e cobrar o novo calado seria tirar dinheiro que a pessoa não
// concordou em gastar. Diferente do banco, a compra não acontece e o resultado é
// PRECO_MUDOU.
type RcoinCompraBody struct {
	OfertaID   uint32
	Pedido     uint32
	PrecoVisto int32
}

// RcoinCompraBodySize são os doze bytes da compra.
const RcoinCompraBodySize = 12

func (m RcoinCompraBody) Encode() []byte {
	b := make([]byte, RcoinCompraBodySize)
	binary.LittleEndian.PutUint32(b[0:], m.OfertaID)
	binary.LittleEndian.PutUint32(b[4:], m.Pedido)
	binary.LittleEndian.PutUint32(b[8:], uint32(m.PrecoVisto))
	return b
}

func (m *RcoinCompraBody) Decode(b []byte) error {
	if len(b) < RcoinCompraBodySize {
		return ErrRcoinCompraCurta
	}
	m.OfertaID = binary.LittleEndian.Uint32(b[0:])
	m.Pedido = binary.LittleEndian.Uint32(b[4:])
	m.PrecoVisto = int32(binary.LittleEndian.Uint32(b[8:]))
	return nil
}

// Resultados da compra. A ordem é a do contrato e os números são parte dele.
const (
	RcoinOK           uint8 = 0
	RcoinSemSaldo     uint8 = 1
	RcoinIndisponivel uint8 = 2
	RcoinPrecoMudou   uint8 = 3
	RcoinOcupado      uint8 = 4
	RcoinErro         uint8 = 5
)

// RcoinResultadoBody é o 0x0F0F: como acabou a compra.
//
// ELE SAI SEMPRE QUE O BANCO RESPONDE, mesmo atrasado. O cliente desiste de esperar em 5
// segundos e desenha o aviso, mas continua aceitando o resultado quando ele chegar — e é
// por isso que a trava de "uma compra em voo" vale até a resposta do banco, e NÃO até um
// tempo limite. Uma trava com prazo menor que a resposta é exatamente como se cobra duas
// vezes: o prazo estoura, o jogador clica de novo, e as duas compras acontecem.
//
// O SALDO volta em toda resposta, inclusive nas recusas, porque é a chance barata de
// corrigir a tela. Numa recusa por saldo curto, é justamente o número que o jogador
// precisa ver.
type RcoinResultadoBody struct {
	Resultado uint8
	OfertaID  uint32
	Pedido    uint32
	Saldo     int32
}

// RcoinResultadoBodySize são os dezesseis bytes do resultado.
const RcoinResultadoBodySize = 16

func (m RcoinResultadoBody) Encode() []byte {
	b := make([]byte, RcoinResultadoBodySize)
	b[0] = m.Resultado
	b[1], b[2], b[3] = 0, 0, 0 // reservados, sempre zero
	binary.LittleEndian.PutUint32(b[4:], m.OfertaID)
	binary.LittleEndian.PutUint32(b[8:], m.Pedido)
	binary.LittleEndian.PutUint32(b[12:], uint32(m.Saldo))
	return b
}

func (m *RcoinResultadoBody) Decode(b []byte) error {
	if len(b) < RcoinResultadoBodySize {
		return ErrRcoinCompraCurta
	}
	m.Resultado = b[0]
	m.OfertaID = binary.LittleEndian.Uint32(b[4:])
	m.Pedido = binary.LittleEndian.Uint32(b[8:])
	m.Saldo = int32(binary.LittleEndian.Uint32(b[12:]))
	return nil
}
