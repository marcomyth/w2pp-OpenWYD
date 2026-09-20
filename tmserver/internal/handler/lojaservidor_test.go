package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// abreBarraca monta a barraca pelo caminho novo — o painel — e espera o aviso de
// que ela subiu. A janela de barraca do cliente está aposentada.
func abreBarraca(t *testing.T, c net.Conn, titulo string, cargoPos int8, preco int32, moeda uint8) int32 {
	t.Helper()
	corpo := protocol.LojaAbrirBody{Titulo: titulo}
	for i := range corpo.Slots {
		corpo.Slots[i].CargoPos = -1
	}
	corpo.Slots[0] = protocol.LojaAbrirSlot{CargoPos: cargoPos, Moeda: moeda, Preco: preco}
	send(t, c, protocol.MsgLojaAbrir, corpo.Encode())
	payload, _ := readUntil(t, c, protocol.MsgLojaAbriu)
	var subiu protocol.LojaAbriuBody
	if err := subiu.Decode(payload); err != nil {
		t.Fatalf("decodificando a barraca que subiu: %v", err)
	}
	return subiu.Barraca
}

// mandaAbrirBarraca manda montar e NÃO espera o aviso: serve aos testes que
// precisam ler, eles mesmos, os quadros que a barraca gera ao subir.
func mandaAbrirBarraca(t *testing.T, c net.Conn, titulo string, cargoPos int8, preco int32,
	moeda uint8) {
	t.Helper()
	corpo := protocol.LojaAbrirBody{Titulo: titulo}
	for i := range corpo.Slots {
		corpo.Slots[i].CargoPos = -1
	}
	corpo.Slots[0] = protocol.LojaAbrirSlot{CargoPos: cargoPos, Moeda: moeda, Preco: preco}
	send(t, c, protocol.MsgLojaAbrir, corpo.Encode())
}

// pedeVitrine envia MsgLojaPede e devolve a página que voltou.
func pedeVitrine(t *testing.T, c net.Conn, pagina, filtro int16) protocol.LojaListaBody {
	t.Helper()
	pede := protocol.LojaPedeBody{Pagina: pagina, Filtro: filtro}
	send(t, c, protocol.MsgLojaPede, pede.Encode())
	payload, _ := readUntil(t, c, protocol.MsgLojaLista)
	// O servidor tambem EMPURRA a vitrine quando o mercado muda, entao pode
	// haver mais de uma lista na fila. Vale a ultima, que e a regra do painel
	// de verdade: lista nova substitui a anterior. Ler so a primeira faria o
	// teste comprar por uma pagina velha - foi o que aconteceu.
	for {
		ty, corpo, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgLojaLista {
			payload = corpo
		}
	}
	var lista protocol.LojaListaBody
	if err := lista.Decode(payload); err != nil {
		t.Fatalf("decodificando a vitrine: %v", err)
	}
	return lista
}

// A vitrine mostra o que as barracas abertas estão vendendo, para quem está
// longe delas — é essa a razão de existir da Loja do Servidor.
func TestVitrineMostraBarracaAberta(t *testing.T) {
	const item = int16(1030)
	const preco = int32(250_000)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, preco, protocol.LojaMoedaOuro)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	if lista.Total != 1 || lista.Qtd != 1 || lista.Paginas != 1 {
		t.Fatalf("vitrine = total %d, qtd %d, paginas %d; queria 1, 1, 1", lista.Total, lista.Qtd,
			lista.Paginas)
	}
	o := lista.Ofertas[0]
	if o.Indice != item || o.Preco != preco || o.Slot != 0 {
		t.Errorf("oferta = item %d, preco %d, slot %d; queria %d, %d, 0", o.Indice, o.Preco, o.Slot,
			item, preco)
	}
	if o.Nome != "Seller" {
		t.Errorf("vendedor = %q, queria %q", o.Nome, "Seller")
	}
	if o.Moeda != protocol.LojaMoedaOuro {
		t.Errorf("moeda = %d, queria ouro", o.Moeda)
	}
}

// Fechar a lojinha tira as ofertas do ar no mesmo instante: a vitrine é lida das
// barracas abertas, não de uma tabela.
func TestVitrineEsvaziaQuandoALojinhaFecha(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 1000, protocol.LojaMoedaOuro)
	if lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); lista.Total != 1 {
		t.Fatalf("antes de fechar, total = %d; queria 1", lista.Total)
	}

	vendedor.Close()

	if lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); lista.Total != 0 {
		t.Fatalf("depois de fechar, total = %d; queria 0", lista.Total)
	}
}

// O filtro "Meus itens" mostra só a barraca de quem pergunta.
func TestVitrineFiltraMinhasOfertas(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 1000, protocol.LojaMoedaOuro)

	if lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroMeus); lista.Total != 0 {
		t.Errorf("para quem nao tem barraca, meus itens = %d; queria 0", lista.Total)
	}
	if lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus); lista.Total != 1 {
		t.Errorf("para o dono, meus itens = %d; queria 1", lista.Total)
	}
}

// Cash e RMT ainda não existem numa barraca do jogo, então esses filtros voltam
// vazios — e é assim que o painel deve mostrá-los até a lojinha ser montada por
// ele.
func TestVitrineAindaNaoTemCashNemRMT(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 1000, protocol.LojaMoedaOuro)

	for _, filtro := range []int16{protocol.LojaFiltroCash, protocol.LojaFiltroRMT} {
		if lista := pedeVitrine(t, vendedor, 0, filtro); lista.Total != 0 {
			t.Errorf("filtro %d = %d ofertas; queria 0", filtro, lista.Total)
		}
	}
	if lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroOuro); lista.Total != 1 {
		t.Errorf("filtro ouro = %d ofertas; queria 1", lista.Total)
	}
}

// Uma oferta refinada e empilhada chega ao painel com o "+N" e a quantidade.
func TestVitrineLevaRefinoEQuantidade(t *testing.T) {
	const item = int16(1030)
	db := newDB()
	db.loads = map[int64]world.CharacterState{
		7:  {Slot: 0, Name: "Seller", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1000},
		11: {Slot: 0, Name: "Buyer", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1_000_000},
	}
	var cargo world.CargoState
	cargo.Items[0] = world.Item{
		Index:   item,
		Effects: [3]world.Effect{{Effect: efSanc, Value: 9}, {Effect: efAmount, Value: 20}},
	}
	db.accounts["tester"].cargo = cargo

	addr, stop, _ := startServerClock(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	// O pedido leva só a posição no cofre: o item de verdade quem lê é o
	// servidor, e é ele que carrega refino e quantidade para a vitrine.
	abreBarraca(t, vendedor, "Minha Loja", 0, 1000, protocol.LojaMoedaOuro)

	lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroTodos)
	if lista.Total != 1 {
		t.Fatalf("total = %d; queria 1", lista.Total)
	}
	if o := lista.Ofertas[0]; o.Refino != 9 || o.Qtd != 20 {
		t.Errorf("oferta = +%d x%d; queria +9 x20", o.Refino, o.Qtd)
	}
}

// O vendedor escolhe a moeda de cada item pelo painel, e a vitrine passa a
// mostrar aquele preço naquela moeda.
func TestVendedorEscolheAMoedaDoItem(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 300, protocol.LojaMoedaOuro)

	moeda := protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}
	send(t, vendedor, protocol.MsgLojaMoeda, moeda.Encode())
	// A troca vai por um socket e a pergunta vem por outro, e o servidor nao
	// promete ordem entre conexoes: uma ida e volta pelo socket do vendedor
	// garante que a moeda ja foi aplicada quando o comprador perguntar.
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus)

	lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroCash)
	if lista.Total != 1 {
		t.Fatalf("filtro cash = %d ofertas; queria 1", lista.Total)
	}
	if o := lista.Ofertas[0]; o.Moeda != protocol.LojaMoedaCash || o.Preco != 300 {
		t.Errorf("oferta = moeda %d preco %d; queria cash 300", o.Moeda, o.Preco)
	}
	if lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroOuro); lista.Total != 0 {
		t.Errorf("depois de virar cash, filtro ouro = %d; queria 0", lista.Total)
	}
}

// Ninguém muda a moeda da barraca alheia.
func TestMoedaSoValeNaPropriaBarraca(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	outro := enterWorldAs(t, addr, "tradeb")
	defer outro.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 300, protocol.LojaMoedaOuro)

	moeda := protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaRMT}
	send(t, outro, protocol.MsgLojaMoeda, moeda.Encode())

	lista := pedeVitrine(t, outro, 0, protocol.LojaFiltroTodos)
	if lista.Total != 1 || lista.Ofertas[0].Moeda != protocol.LojaMoedaOuro {
		t.Errorf("moeda = %d; queria continuar em ouro", lista.Ofertas[0].Moeda)
	}
}

// A barraca ao lado é marcada como perto; a vitrine é da cidade, e a compra do
// jogo exige proximidade.
func TestOfertaAoLadoVemMarcadaComoPerto(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 1000, protocol.LojaMoedaOuro)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	if lista.Total != 1 {
		t.Fatalf("total = %d; queria 1", lista.Total)
	}
	if lista.Ofertas[0].Perto != 1 {
		t.Errorf("oferta ao lado veio como longe; queria perto")
	}
}

// Comprar em ouro pelo painel: o item sai do Cargo do vendedor, o ouro sai do
// comprador e o cofre do vendedor recebe.
func TestCompraEmOuroPeloPainel(t *testing.T) {
	const item = int16(1030)
	const preco = int32(50_000)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, preco, protocol.LojaMoedaOuro)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	if lista.Total != 1 {
		t.Fatalf("vitrine = %d ofertas; queria 1", lista.Total)
	}
	o := lista.Ofertas[0]
	compra := protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	// O comprador recebe o item; o vendedor, o slot do cofre esvaziado.
	if payload, _ := readUntil(t, comprador, protocol.MsgSendItem); len(payload) == 0 {
		t.Fatalf("comprador nao recebeu o item")
	}
	if depois := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); depois.Total != 0 {
		t.Errorf("depois da compra a vitrine tem %d ofertas; queria 0", depois.Total)
	}
}

// Em Cash a compra é recusada enquanto o saldo não estiver ligado ao banco — e,
// o que mais importa, nada se move: o item continua à venda.
func TestCompraEmCashRecusadaSemBanco(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 300, protocol.LojaMoedaOuro)
	moeda := protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}
	send(t, vendedor, protocol.MsgLojaMoeda, moeda.Encode())
	// A troca vai por um socket e a pergunta vem por outro, e o servidor nao
	// promete ordem entre conexoes: uma ida e volta pelo socket do vendedor
	// garante que a moeda ja foi aplicada quando o comprador perguntar.
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	o := lista.Ofertas[0]
	if o.Moeda != protocol.LojaMoedaCash {
		t.Fatalf("oferta esta em moeda %d; queria cash", o.Moeda)
	}
	compra := protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())

	if depois := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); depois.Total != 1 {
		t.Errorf("a oferta sumiu apesar da recusa: total = %d; queria 1", depois.Total)
	}
}

// Com o saldo ligado (é o que a Hanna vai plugar), a compra em Cash acontece e
// o valor é transferido entre as contas.
func TestCompraEmCashComSaldoLigado(t *testing.T) {
	const item = int16(1030)
	const preco = int32(300)
	var movido struct {
		de, para int64
		moeda    uint8
		valor    int32
		vezes    int
	}
	UsaSaldoDeConta(saldoDeMentira{func(de, para int64, moeda uint8, valor int32) error {
		movido.de, movido.para, movido.moeda, movido.valor = de, para, moeda, valor
		movido.vezes++
		return nil
	}})
	defer UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error {
		return ErrSaldoNaoLigado
	}})

	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, preco, protocol.LojaMoedaOuro)
	moeda := protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}
	send(t, vendedor, protocol.MsgLojaMoeda, moeda.Encode())
	// A troca vai por um socket e a pergunta vem por outro, e o servidor nao
	// promete ordem entre conexoes: uma ida e volta pelo socket do vendedor
	// garante que a moeda ja foi aplicada quando o comprador perguntar.
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	o := lista.Ofertas[0]
	compra := protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())
	readUntil(t, comprador, protocol.MsgSendItem)

	if movido.vezes != 1 || movido.moeda != protocol.LojaMoedaCash || movido.valor != preco {
		t.Errorf("transferencia = %+v; queria uma de %d em cash", movido, preco)
	}
	if depois := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); depois.Total != 0 {
		t.Errorf("depois da compra a vitrine tem %d ofertas; queria 0", depois.Total)
	}
}

// O painel mostra o saldo de verdade, e ele acompanha a compra.
//
// Antes o servidor mandava Cash e RMT zerados no cabeçalho da vitrine, sempre.
// Quem tinha saldo via "Cash 0" e, ao comprar em cash, achava que tinha levado
// de graça — o dinheiro saía da conta em silêncio.
func TestVitrineMostraSaldoDaContaEODesconta(t *testing.T) {
	const item = int16(1030)
	const preco = int32(300)
	UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error { return nil }})
	defer UsaSaldoDeConta(saldoDeMentira{func(int64, int64, uint8, int32) error {
		return ErrSaldoNaoLigado
	}})

	db := autotradeDB(item)
	db.accounts["tester"].cash, db.accounts["tester"].rmt = 5000, 70
	db.accounts["tradeb"].cash, db.accounts["tradeb"].rmt = 2300, 0

	addr, stop, _ := startServerClock(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	if lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); lista.Cash != 2300 ||
		lista.RMT != 0 {
		t.Fatalf("saldo do comprador no painel = cash %d, rmt %d; queria 2300 e 0",
			lista.Cash, lista.RMT)
	}
	if lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroTodos); lista.Cash != 5000 ||
		lista.RMT != 70 {
		t.Fatalf("saldo do vendedor no painel = cash %d, rmt %d; queria 5000 e 70",
			lista.Cash, lista.RMT)
	}

	abreBarraca(t, vendedor, "Minha Loja", 0, preco, protocol.LojaMoedaOuro)
	moeda := protocol.LojaMoedaBody{Slot: 0, Moeda: protocol.LojaMoedaCash}
	send(t, vendedor, protocol.MsgLojaMoeda, moeda.Encode())
	// A troca vai por um socket e a pergunta vem por outro, e o servidor nao
	// promete ordem entre conexoes: uma ida e volta pelo socket do vendedor
	// garante que a moeda ja foi aplicada quando o comprador perguntar.
	pedeVitrine(t, vendedor, 0, protocol.LojaFiltroMeus)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	o := lista.Ofertas[0]
	compra := protocol.LojaCompraBody{Vendedor: o.Vendedor, Slot: o.Slot, Moeda: o.Moeda}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())
	readUntil(t, comprador, protocol.MsgSendItem)

	if depois := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos); depois.Cash != 2300-preco {
		t.Errorf("cash do comprador depois da compra = %d; queria %d", depois.Cash, 2300-preco)
	}
	if depois := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroTodos); depois.Cash != 5000+preco {
		t.Errorf("cash do vendedor depois da venda = %d; queria %d", depois.Cash, 5000+preco)
	}
}

// saldoDeMentira é o lugar da implementação de verdade nos testes.
type saldoDeMentira struct {
	fn func(de, para int64, moeda uint8, valor int32) error
}

func (s saldoDeMentira) Transfere(de, para int64, moeda uint8, valor int32) error {
	return s.fn(de, para, moeda, valor)
}

// Montar a barraca pelo painel: o cofre é lido no servidor e as moedas vêm
// escolhidas de saída.
func TestAbreBarracaPeloPainel(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	// O painel primeiro pergunta o que há no cofre.
	send(t, vendedor, protocol.MsgLojaCargo, nil)
	payload, _ := readUntil(t, vendedor, protocol.MsgLojaCargoLista)
	var cofre protocol.LojaCargoListaBody
	if err := cofre.Decode(payload); err != nil {
		t.Fatalf("decodificando o cofre: %v", err)
	}
	if cofre.Qtd != 1 || cofre.Itens[0].Indice != item || cofre.Itens[0].Slot != 0 {
		t.Fatalf("cofre = %d itens, primeiro %+v; queria 1 item %d no slot 0", cofre.Qtd,
			cofre.Itens[0], item)
	}

	// E então manda montar, com preço e moeda.
	abrir := protocol.LojaAbrirBody{Titulo: "Barraca do Painel"}
	for i := range abrir.Slots {
		abrir.Slots[i].CargoPos = -1
	}
	abrir.Slots[0] = protocol.LojaAbrirSlot{CargoPos: 0, Moeda: protocol.LojaMoedaRMT, Preco: 42}
	send(t, vendedor, protocol.MsgLojaAbrir, abrir.Encode())
	// Esperar o aviso antes de perguntar pela vitrine: são duas conexões, e sem
	// isto a pergunta do comprador pode chegar antes de a barraca subir.
	readUntil(t, vendedor, protocol.MsgLojaAbriu)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	if lista.Total != 1 {
		t.Fatalf("vitrine = %d ofertas; queria 1", lista.Total)
	}
	if o := lista.Ofertas[0]; o.Indice != item || o.Preco != 42 || o.Moeda != protocol.LojaMoedaRMT {
		t.Errorf("oferta = item %d, %d, moeda %d; queria %d, 42, RMT", o.Indice, o.Preco, o.Moeda,
			item)
	}
}

// Duas prateleiras apontando para o mesmo slot do cofre venderiam o item duas
// vezes — a barraca não sobe.
func TestBarracaRecusaSlotRepetido(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	abrir := protocol.LojaAbrirBody{Titulo: "Repetida"}
	for i := range abrir.Slots {
		abrir.Slots[i].CargoPos = -1
	}
	abrir.Slots[0] = protocol.LojaAbrirSlot{CargoPos: 0, Preco: 100}
	abrir.Slots[1] = protocol.LojaAbrirSlot{CargoPos: 0, Preco: 200}
	send(t, vendedor, protocol.MsgLojaAbrir, abrir.Encode())

	if lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroTodos); lista.Total != 0 {
		t.Errorf("a barraca subiu com slot repetido: %d ofertas", lista.Total)
	}
}

// A janela de barraca do cliente está aposentada: o pacote antigo não monta mais
// nada.
func TestJanelaAntigaDeBarracaNaoAbreMais(t *testing.T) {
	const item = int16(1030)
	addr, stop, _ := startServerClock(t, autotradeDB(item))
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	send(t, vendedor, protocol.MsgSendAutoTrade, openShopPayload("Antiga", item, 0, 1000))

	if lista := pedeVitrine(t, vendedor, 0, protocol.LojaFiltroTodos); lista.Total != 0 {
		t.Errorf("o fluxo antigo montou barraca: %d ofertas; queria 0", lista.Total)
	}
}
