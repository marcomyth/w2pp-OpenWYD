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
//   MsgLojaCargo (C→S): "o que eu tenho no cofre?" — para montar a barraca.
//   MsgLojaCargoLista (S→C): o cofre, item a item.
//   MsgLojaAbrir (C→S): "abre a minha barraca com estes itens, preços e moedas".
//   MsgLojaAbriu (S→C): "subiu, e o id dela é este".
//
// Nada disso é persistido: a vitrine é montada na hora a partir das barracas
// abertas, então fechar a lojinha tira as ofertas do ar no mesmo instante.

// LojaPorPagina é quantas ofertas cabem numa página — a grade do painel do
// cliente tem 5 colunas por 7 linhas, e é ela quem manda: com 32 aqui, toda
// página cheia chegava com três quadrados vazios no fim.
const LojaPorPagina = 35

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

// LojaEfeitosSize são os três pares de efeito de um item: 6 bytes.
const LojaEfeitosSize = 6

// LojaEfeitos são os três pares (efeito, valor) do STRUCT_ITEM, na ordem dele.
//
// É ISTO QUE O JOGADOR CHAMA DE "OS ADDS", e era o que faltava: a lojinha mostrava o
// item certo, com refino e quantidade certos, e nenhum add — porque eles nunca saíam
// do servidor. Quem comprava só descobria o que tinha comprado depois de pagar.
//
// OS BYTES VÃO NUM BLOCO NO FIM DO PACOTE, e não dentro de cada linha, e isso é o
// contrato combinado com quem cuida do cliente: o cliente antigo mede o pacote e
// ignora o que sobra, então nada do que já existe muda de posição nem de valor, e o
// cliente novo reconhece o bloco pelo tamanho exato. Mexer na linha de 32 bytes
// quebraria o cliente que está na rua hoje.
//
// MAS OS CAMPOS MORAM DENTRO DO ITEM, e não numa lista paralela. O contrato exige "na
// mesma ordem das ofertas", e uma lista paralela é exatamente o jeito de essa ordem se
// perder: basta alguém filtrar, ordenar ou pular um slot num lugar e não no outro, e
// cada item passa a mostrar os adds do vizinho. Guardados no item, eles não têm como
// se separar dele; a separação existe só na hora de escrever os bytes.
type LojaEfeitos struct {
	Ef1, V1 uint8
	Ef2, V2 uint8
	Ef3, V3 uint8
}

func (e *LojaEfeitos) encode(b []byte) {
	b[0], b[1] = e.Ef1, e.V1
	b[2], b[3] = e.Ef2, e.V2
	b[4], b[5] = e.Ef3, e.V3
}

func (e *LojaEfeitos) decode(b []byte) {
	e.Ef1, e.V1 = b[0], b[1]
	e.Ef2, e.V2 = b[2], b[3]
	e.Ef3, e.V3 = b[4], b[5]
}

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
	// Efeitos são os adds. Eles NÃO entram nos 32 bytes desta linha: viajam num
	// bloco no fim do pacote (ver LojaEfeitos). Moram aqui para não se separarem do
	// item que descrevem.
	Efeitos LojaEfeitos
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

// --- montar a barraca pelo painel -------------------------------------------
//
// A janela de barraca do cliente está aposentada: ela só sabia de ouro e o
// jogador escolhia os itens por lá. Agora quem monta é o painel, e para isso
// precisa ver o cofre — que mora no servidor, não no cliente.

// LojaCargoMax é MaxCargo, os slots do cofre da conta.
const LojaCargoMax = 128

// LojaCargoItemSize é uma linha do cofre: 8 bytes.
const LojaCargoItemSize = 8

// LojaCargoItem é um item do cofre como o painel precisa vê-lo.
type LojaCargoItem struct {
	Slot   int16
	Indice int16
	Refino uint8
	Qtd    uint8
	// Efeitos são os adds, fora dos 8 bytes desta linha — ver LojaEfeitos.
	Efeitos LojaEfeitos
}

// lojaCofreSemEfeitos é o corpo do cofre como ele era antes do bloco de adds.
const lojaCofreSemEfeitos = 4 + LojaCargoMax*LojaCargoItemSize

// LojaCargoListaBodySize é o corpo da resposta do cofre, com o bloco de adds.
//
// 4 + 128*8 + 128*6 = 1796, e com os 12 do cabeçalho dá 1808 no fio. Cabe nos 8192 do
// ReadMessage, e há teste que falha se deixar de caber.
const LojaCargoListaBodySize = lojaCofreSemEfeitos + LojaCargoMax*LojaEfeitosSize

// LojaCargoListaBody é o cofre inteiro, só com os slots ocupados.
type LojaCargoListaBody struct {
	Qtd   int16
	Itens [LojaCargoMax]LojaCargoItem
}

func (m *LojaCargoListaBody) Encode() []byte {
	b := make([]byte, LojaCargoListaBodySize)
	binary.LittleEndian.PutUint16(b[0:], uint16(m.Qtd))
	for i := 0; i < LojaCargoMax; i++ {
		o := 4 + i*LojaCargoItemSize
		binary.LittleEndian.PutUint16(b[o+0:], uint16(m.Itens[i].Slot))
		binary.LittleEndian.PutUint16(b[o+2:], uint16(m.Itens[i].Indice))
		b[o+4] = m.Itens[i].Refino
		b[o+5] = m.Itens[i].Qtd
		m.Itens[i].Efeitos.encode(b[lojaCofreSemEfeitos+i*LojaEfeitosSize:])
	}
	return b
}

func (m *LojaCargoListaBody) Decode(b []byte) error {
	if len(b) < LojaCargoListaBodySize {
		return fmt.Errorf("loja: cofre com %d bytes, esperado %d", len(b), LojaCargoListaBodySize)
	}
	m.Qtd = int16(binary.LittleEndian.Uint16(b[0:]))
	for i := 0; i < LojaCargoMax; i++ {
		o := 4 + i*LojaCargoItemSize
		m.Itens[i].Slot = int16(binary.LittleEndian.Uint16(b[o+0:]))
		m.Itens[i].Indice = int16(binary.LittleEndian.Uint16(b[o+2:]))
		m.Itens[i].Refino = b[o+4]
		m.Itens[i].Qtd = b[o+5]
		m.Itens[i].Efeitos.decode(b[lojaCofreSemEfeitos+i*LojaEfeitosSize:])
	}
	return nil
}

// LojaAbrirSlot é uma prateleira da barraca: de onde sai o item, por quanto e em
// que moeda. CargoPos -1 é prateleira vazia.
type LojaAbrirSlot struct {
	CargoPos int8
	Moeda    uint8
	Preco    int32
}

// LojaAbrirSlotSize e LojaAbrirBodySize fecham o corpo do pedido.
const (
	LojaAbrirSlotSize = 8
	LojaAbrirBodySize = autoTradeTitleLen + MaxAutoTradeWire*LojaAbrirSlotSize
)

// LojaAbrirBody é o pedido de montar a barraca. Repare que ele NÃO leva o item:
// o servidor lê do cofre pela posição, que é o único lugar em que o item de
// verdade existe.
type LojaAbrirBody struct {
	Titulo string
	Slots  [MaxAutoTradeWire]LojaAbrirSlot
}

func (m *LojaAbrirBody) Encode() []byte {
	b := make([]byte, LojaAbrirBodySize)
	titulo := m.Titulo
	if len(titulo) >= autoTradeTitleLen {
		titulo = titulo[:autoTradeTitleLen-1]
	}
	copy(b[0:autoTradeTitleLen], titulo)
	for i := 0; i < MaxAutoTradeWire; i++ {
		o := autoTradeTitleLen + i*LojaAbrirSlotSize
		b[o+0] = byte(m.Slots[i].CargoPos)
		b[o+1] = m.Slots[i].Moeda
		binary.LittleEndian.PutUint32(b[o+4:], uint32(m.Slots[i].Preco))
	}
	return b
}

func (m *LojaAbrirBody) Decode(b []byte) error {
	if len(b) < LojaAbrirBodySize {
		return fmt.Errorf("loja: abrir com %d bytes, esperado %d", len(b), LojaAbrirBodySize)
	}
	m.Titulo = cTrimNUL(b[0:autoTradeTitleLen])
	for i := 0; i < MaxAutoTradeWire; i++ {
		o := autoTradeTitleLen + i*LojaAbrirSlotSize
		m.Slots[i].CargoPos = int8(b[o+0])
		m.Slots[i].Moeda = b[o+1]
		m.Slots[i].Preco = int32(binary.LittleEndian.Uint32(b[o+4:]))
	}
	return nil
}

// LojaAbriuBody confirma que a barraca subiu e diz por qual id ela é comprada —
// o mesmo que a vitrine mostra nas ofertas.
type LojaAbriuBody struct {
	Barraca int32
}

// LojaAbriuBodySize é o tamanho do corpo.
const LojaAbriuBodySize = 4

func (m *LojaAbriuBody) Encode() []byte {
	b := make([]byte, LojaAbriuBodySize)
	binary.LittleEndian.PutUint32(b[0:], uint32(m.Barraca))
	return b
}

func (m *LojaAbriuBody) Decode(b []byte) error {
	if len(b) < LojaAbriuBodySize {
		return fmt.Errorf("loja: abriu com %d bytes, esperado %d", len(b), LojaAbriuBodySize)
	}
	m.Barraca = int32(binary.LittleEndian.Uint32(b[0:]))
	return nil
}

// lojaListaSemEfeitos é o corpo como ele era antes do bloco de adds.
//
// FICA NOMEADO, e não embutido na conta abaixo, porque é ele que o teste usa para
// provar que o prefixo não mudou um byte. Um número solto no teste seria um número
// que alguém atualiza junto com o código, e aí ele deixa de provar qualquer coisa.
const lojaListaSemEfeitos = lojaCabecalhoLista + LojaPorPagina*LojaOfertaSize

// LojaListaBodySize é o corpo da resposta: o cabeçalho, as ofertas e o bloco de adds.
//
// 20 + 35*32 + 35*6 = 1350, e com os 12 do cabeçalho de rede dá 1362 no fio. O
// ReadMessage do cliente aceita de 12 a 8192 bytes (medido no WYD.exe 384eaeac, na
// checagem em 0x4251E2), então cabe com folga — e há teste que falha se deixar de
// caber.
const LojaListaBodySize = lojaListaSemEfeitos + LojaPorPagina*LojaEfeitosSize

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
		// O BLOCO VEM DEPOIS DE TODAS AS OFERTAS, e não intercalado: é o que deixa o
		// prefixo byte a byte igual ao de antes para o cliente que está na rua.
		m.Ofertas[i].Efeitos.encode(b[lojaListaSemEfeitos+i*LojaEfeitosSize:])
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
		m.Ofertas[i].Efeitos.decode(b[lojaListaSemEfeitos+i*LojaEfeitosSize:])
	}
	return nil
}

// LojaMudouBody é o bilhete que avisa o painel de que o mercado mudou. São
// quatro bytes: a versão nova. O painel compara com a que ele tem e decide se
// vale pedir a página de novo - quem guarda a vitrine é ele, não o servidor.
type LojaMudouBody struct {
	Versao int32
}

const LojaMudouBodySize = 4

func (m LojaMudouBody) Encode() []byte {
	b := make([]byte, LojaMudouBodySize)
	binary.LittleEndian.PutUint32(b, uint32(m.Versao))
	return b
}

func (m *LojaMudouBody) Decode(b []byte) error {
	if len(b) < LojaMudouBodySize {
		return fmt.Errorf("loja: bilhete de %d bytes, esperava %d", len(b), LojaMudouBodySize)
	}
	m.Versao = int32(binary.LittleEndian.Uint32(b))
	return nil
}
