package protocol

import "fmt"

// O PAINEL DE DROP DAS FADAS (0x0F70 a 0x0F74).
//
// O painel mostra os monstros perto do jogador (ou os que ele procurou pelo nome),
// os itens que cada um pode dar, e a lista de Itens Protegidos do filtro da fada.
//
// O QUE NUNCA VIAJA: a chance de drop, a casa do molde e a ordem de raridade. O
// pacote de drops leva só índices de item, em ordem de índice. A casa de um item no
// molde é a chance dele (casa 11 cai sempre, casa 20 quase nunca), e mandá-la, ou
// mandar os itens na ordem das casas, seria entregar a tabela de chances.

// Os pedidos do 0x0F70.
const (
	FadasPedeProximos uint8 = 1 // monstros em volta de mim
	FadasPedeBusca    uint8 = 2 // monstros pelo nome
	FadasPedeDrops    uint8 = 3 // o que um monstro pode dar
	FadasPedeFiltro   uint8 = 4 // o meu filtro
)

// As mudanças do 0x0F73.
const (
	FadasMudaPoe      uint8 = 1
	FadasMudaTira     uint8 = 2
	FadasMudaLiga     uint8 = 3
	FadasMudaDesliga  uint8 = 4
	FadasMudaTiraTudo uint8 = 5
)

// Os motivos do 0x0F74. Zero é "nada a dizer": o estado mudou como pedido.
const (
	FadasMotivoNenhum     uint8 = 0
	FadasMotivoSemFada    uint8 = 1 // ligar sem a Fada Azul ou a Vermelha vestida
	FadasMotivoListaVazia uint8 = 2 // ligar sem nenhum Item Protegido
	FadasMotivoListaCheia uint8 = 3 // pôr com a lista no teto
	FadasMotivoItemRuim   uint8 = 4 // índice fora do catálogo
	FadasMotivoNaoGravou  uint8 = 5 // o banco não respondeu; o estado é o anterior
)

const (
	// FadasFiltroMax é o teto da lista de Itens Protegidos (o da tabela fada_filtro).
	FadasFiltroMax = 60
	// FadasMonstrosPorPagina é quantas linhas a lista de monstros do painel tem.
	FadasMonstrosPorPagina = 10
	// FadasDropsMax é quantos itens cabem num 0x0F72. As 64 casas do molde, as
	// regras da Mesa e o saque de chefe de um monstro só ficam bem abaixo disto.
	FadasDropsMax = 240
	// FadasNome é o campo do nome: o do monstro (16, como no STRUCT_MOB) e o da busca.
	FadasNome = 16
)

// FadasPedeBody é o 0x0F70.
//
//	+0  tipo    u8   FadasPede*
//	+1  pagina  u8   página pedida, de zero (próximos e busca)
//	+2  versao  u16  a versão do catálogo que o cliente tem (drops)
//	+4  monstro u16  o número do monstro no catálogo (drops)
//	+6  (2)
//	+8  texto   [16] a busca, CP1252, com zero no fim
type FadasPedeBody struct {
	Tipo    uint8
	Pagina  uint8
	Versao  uint16
	Monstro uint16
	Texto   string // os bytes como vieram (CP1252)
}

const fadasPedeSize = 8 + FadasNome

// DecodeFadasPede lê o 0x0F70.
func DecodeFadasPede(b []byte) (FadasPedeBody, error) {
	if len(b) < fadasPedeSize {
		return FadasPedeBody{}, fmt.Errorf("protocol: fadas pede curto: %d", len(b))
	}
	return FadasPedeBody{
		Tipo:    b[0],
		Pagina:  b[1],
		Versao:  le.Uint16(b[2:]),
		Monstro: le.Uint16(b[4:]),
		Texto:   cstr16(b[8 : 8+FadasNome]),
	}, nil
}

// FadasMonstro é uma linha da lista de monstros. O nível do monstro NÃO viaja: a
// tela mostra o tipo do lugar no lugar dele (pedido da dona, 02/10/2026).
type FadasMonstro struct {
	Numero uint16 // posição no catálogo desta Versao
	Tipo   uint8  // regiao.Tipo: 0 mapa aberto, 1 quest, 2 masmorra
	Nome   string // os bytes do nome do molde (CP1252)
	Regiao string // o nome da região, em UTF-8; vai para o fio em CP1252
}

// FadasMonstrosBody é o 0x0F71.
//
//	+0  versao u16  a versão do catálogo; os números só valem nela
//	+2  tipo   u8   FadasPedeProximos ou FadasPedeBusca
//	+3  pagina u8
//	+4  total  u16  quantos monstros a lista inteira tem
//	+6  n      u8
//	+7  (1)
//	+8  n × { numero u16, tipo u8, (1), nome [16], regiao [24] }
type FadasMonstrosBody struct {
	Versao   uint16
	Tipo     uint8
	Pagina   uint8
	Total    uint16
	Monstros []FadasMonstro
}

// FadasRegiao é o campo do nome da região: 23 letras e o zero do fim.
const FadasRegiao = 24

const fadasMonstroSize = 4 + FadasNome + FadasRegiao

// Encode escreve o 0x0F71.
func (m *FadasMonstrosBody) Encode() []byte {
	n := min(len(m.Monstros), FadasMonstrosPorPagina)
	b := make([]byte, 8+n*fadasMonstroSize)
	le.PutUint16(b[0:], m.Versao)
	b[2], b[3] = m.Tipo, m.Pagina
	le.PutUint16(b[4:], m.Total)
	b[6] = uint8(n)
	for i := 0; i < n; i++ {
		p := 8 + i*fadasMonstroSize
		le.PutUint16(b[p:], m.Monstros[i].Numero)
		b[p+2] = m.Monstros[i].Tipo
		// 15 bytes e o zero do fim: o cliente lê como texto C.
		copy(b[p+4:p+4+FadasNome-1], m.Monstros[i].Nome)
		copy(b[p+4+FadasNome:p+4+FadasNome+FadasRegiao-1], paraCP1252(m.Monstros[i].Regiao))
	}
	return b
}

// EncodeFadasDrops escreve o 0x0F72: só índices, na ordem em que vieram (o
// tratador os manda em ordem de índice).
//
//	+0  versao  u16
//	+2  monstro u16
//	+4  n       u16
//	+6  (2)
//	+8  n × u16
func EncodeFadasDrops(versao, monstro uint16, itens []int16) []byte {
	n := min(len(itens), FadasDropsMax)
	b := make([]byte, 8+2*n)
	le.PutUint16(b[0:], versao)
	le.PutUint16(b[2:], monstro)
	le.PutUint16(b[4:], uint16(n))
	for i := 0; i < n; i++ {
		le.PutUint16(b[8+2*i:], uint16(itens[i]))
	}
	return b
}

// paraCP1252 leva um texto do servidor (UTF-8) aos bytes que o cliente desenha.
// As letras do português estão na faixa 0xA0..0xFF, igual em Latin-1 e CP1252; o
// que não couber num byte vira '?'.
func paraCP1252(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			r = '?'
		}
		out = append(out, byte(r))
	}
	return out
}

// FadasMudaBody é o 0x0F73.
//
//	+0  acao u8   FadasMuda*
//	+1  (1)
//	+2  item u16  o índice (pôr e tirar)
type FadasMudaBody struct {
	Acao uint8
	Item int16
}

// DecodeFadasMuda lê o 0x0F73.
func DecodeFadasMuda(b []byte) (FadasMudaBody, error) {
	if len(b) < 4 {
		return FadasMudaBody{}, fmt.Errorf("protocol: fadas muda curto: %d", len(b))
	}
	return FadasMudaBody{Acao: b[0], Item: int16(le.Uint16(b[2:]))}, nil
}

// EncodeFadasFiltro escreve o 0x0F74.
//
//	+0  ligado  u8
//	+1  temFada u8  a Fada Azul ou a Vermelha está vestida agora
//	+2  motivo  u8  FadasMotivo*
//	+3  n       u8
//	+4  n × u16     os Itens Protegidos, em ordem de índice
func EncodeFadasFiltro(ligado, temFada bool, motivo uint8, itens []int16) []byte {
	n := min(len(itens), FadasFiltroMax)
	b := make([]byte, 4+2*n)
	if ligado {
		b[0] = 1
	}
	if temFada {
		b[1] = 1
	}
	b[2], b[3] = motivo, uint8(n)
	for i := 0; i < n; i++ {
		le.PutUint16(b[4+2*i:], uint16(itens[i]))
	}
	return b
}

// FadasFaixa é o menor e o maior valor de um efeito adicional.
type FadasFaixa struct {
	Efeito, Min, Max uint8
}

// FadasFaixasItem são as faixas de adicional de um item.
type FadasFaixasItem struct {
	Item   int16
	Faixas []FadasFaixa
}

const (
	// FadasFaixasPorItem é quantas faixas um item leva: a luva, a peça com mais
	// efeitos possíveis, tem cinco.
	FadasFaixasPorItem = 6
	// FadasFaixasBytes é o teto do corpo do 0x0F76. O que não couber fica sem
	// faixa na dica, o que é só uma linha a menos.
	FadasFaixasBytes = 1200
)

// EncodeFadasFaixas escreve o 0x0F76: as faixas de adicional dos itens que um
// monstro pode dar. Vem logo depois do 0x0F72 do mesmo monstro. Só o menor e o
// maior valor de cada efeito: a chance de sair não viaja.
//
//	+0  versao  u16
//	+2  monstro u16
//	+4  n       u16  itens
//	+6  (2)
//	+8  n × { item u16, k u8, (1), k × { efeito u8, min u8, max u8 } }
func EncodeFadasFaixas(versao, monstro uint16, itens []FadasFaixasItem) []byte {
	b := make([]byte, 8, 8+len(itens)*8)
	le.PutUint16(b[0:], versao)
	le.PutUint16(b[2:], monstro)
	n := 0
	for _, it := range itens {
		k := min(len(it.Faixas), FadasFaixasPorItem)
		if k == 0 {
			continue
		}
		if len(b)+4+3*k > FadasFaixasBytes {
			break
		}
		b = append(b, byte(uint16(it.Item)), byte(uint16(it.Item)>>8), byte(k), 0)
		for _, f := range it.Faixas[:k] {
			b = append(b, f.Efeito, f.Min, f.Max)
		}
		n++
	}
	le.PutUint16(b[4:], uint16(n))
	return b
}
