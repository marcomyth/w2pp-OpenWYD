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

// FadasMonstro é uma linha da lista de monstros.
type FadasMonstro struct {
	Numero uint16 // posição no catálogo desta Versao
	Nivel  uint16
	Nome   string // os bytes do nome do molde (CP1252)
}

// FadasMonstrosBody é o 0x0F71.
//
//	+0  versao u16  a versão do catálogo; os números só valem nela
//	+2  tipo   u8   FadasPedeProximos ou FadasPedeBusca
//	+3  pagina u8
//	+4  total  u16  quantos monstros a lista inteira tem
//	+6  n      u8
//	+7  (1)
//	+8  n × { numero u16, nivel u16, nome [16] }
type FadasMonstrosBody struct {
	Versao   uint16
	Tipo     uint8
	Pagina   uint8
	Total    uint16
	Monstros []FadasMonstro
}

const fadasMonstroSize = 4 + FadasNome

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
		le.PutUint16(b[p+2:], m.Monstros[i].Nivel)
		// 15 bytes e o zero do fim: o cliente lê como texto C.
		copy(b[p+4:p+4+FadasNome-1], m.Monstros[i].Nome)
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
