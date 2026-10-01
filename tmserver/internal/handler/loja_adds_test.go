package handler

import (
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// OS ADDS NA LOJINHA, de ponta a ponta: do item no baú até os bytes que o painel lê.
//
// O teste do protocolo prova que o pacote carrega o que lhe entregam. Estes provam a
// outra metade, que é a que faltava de verdade: que alguém ENTREGA. O defeito original
// não era de codificação — a vitrine e o cofre simplesmente não levavam os três pares
// do item, e o jogador comprava sem ver o que estava comprando.

// itemDoBauComAdds é um item do baú e o que o painel tem de receber dele.
//
// O "quero" é ESCRITO À MÃO, par por par, e não tirado do item pela função do servidor:
// calculado por ela, o teste concordaria com qualquer ordem que ela escolhesse.
type itemDoBauComAdds struct {
	nome       string
	cargoPos   int
	prateleira int8
	item       world.Item
	quero      protocol.LojaEfeitos
}

// itensComAdds é o baú dos testes.
//
// AS POSIÇÕES NÃO SÃO VIZINHAS NEM ESTÃO NA MESMA ORDEM nos três lugares (baú,
// prateleira, lista do cofre), de propósito: com tudo em 0, 1, 2 um índice errado
// acertaria por coincidência. E nenhum item repete os adds de outro, que é o que faz
// "mostrou os adds do vizinho" aparecer.
//
// NEM TUDO É ARMA. O bloco é do item, qualquer item: o amuleto e o brinco estão aqui
// porque é em acessório que o add decide o preço.
var itensComAdds = []itemDoBauComAdds{
	{
		nome: "arma com 3 adds (Balmung +9)", cargoPos: 3, prateleira: 2,
		item: world.Item{Index: 811, Effects: [3]world.Effect{
			{Effect: efSanc, Value: 9}, {Effect: efDamage, Value: 45}, {Effect: efCritical, Value: 12}}},
		quero: protocol.LojaEfeitos{Ef1: 43, V1: 9, Ef2: 2, V2: 45, Ef3: 42, V3: 12},
	},
	{
		nome: "amuleto com 3 adds (Amuleto de Prata)", cargoPos: 7, prateleira: 1,
		item: world.Item{Index: 551, Effects: [3]world.Effect{
			{Effect: efSpecial1, Value: 4}, {Effect: efHpAdd, Value: 8}, {Effect: efResistAll, Value: 15}}},
		quero: protocol.LojaEfeitos{Ef1: 11, V1: 4, Ef2: 45, V2: 8, Ef3: 54, V3: 15},
	},
	{
		nome: "acessorio com 1 add so (Brinco de Athena)", cargoPos: 20, prateleira: 5,
		item: world.Item{Index: 591, Effects: [3]world.Effect{
			{Effect: efMpAdd, Value: 17}}},
		quero: protocol.LojaEfeitos{Ef1: 46, V1: 17},
	},
	{
		// O add único no TERCEIRO par: o lugar do par também é informação, e um
		// servidor que "compactasse" os pares o mandaria no primeiro.
		nome: "armadura com 1 add so, no terceiro par (Luvas)", cargoPos: 40, prateleira: 11,
		item: world.Item{Index: 1236, Effects: [3]world.Effect{
			{}, {}, {Effect: efAc, Value: 25}}},
		quero: protocol.LojaEfeitos{Ef3: 3, V3: 25},
	},
	{
		// Item liso ENTRE os outros: é ele que pega o add que sobra do vizinho.
		nome: "item sem add nenhum", cargoPos: 21, prateleira: 0,
		item:  world.Item{Index: 540},
		quero: protocol.LojaEfeitos{},
	},
}

func addsDB() *fakeDB {
	db := autotradeDB(0)
	var cargo world.CargoState
	for _, it := range itensComAdds {
		cargo.Items[it.cargoPos] = it.item
	}
	db.accounts["tester"].cargo = cargo
	return db
}

// abreBarracaComAdds põe cada item na sua prateleira, em ouro, e espera a barraca subir.
func abreBarracaComAdds(t *testing.T, c net.Conn) {
	t.Helper()
	corpo := protocol.LojaAbrirBody{Titulo: "Adds"}
	for i := range corpo.Slots {
		corpo.Slots[i].CargoPos = -1
	}
	for _, it := range itensComAdds {
		corpo.Slots[it.prateleira] = protocol.LojaAbrirSlot{
			CargoPos: int8(it.cargoPos), Moeda: protocol.LojaMoedaOuro,
			Preco: int32(1000 + it.cargoPos),
		}
	}
	send(t, c, protocol.MsgLojaAbrir, corpo.Encode())
	readUntil(t, c, protocol.MsgLojaAbriu)
}

// TESTE 6 (vitrine): a oferta de cada item chega ao painel com os três pares DELE.
//
// Quem pergunta é o comprador, outra conta, porque é ele quem decide pagar olhando
// para isto.
func TestVitrineMostraOsAddsDeCadaItem(t *testing.T) {
	addr, stop, _ := startServerClock(t, addsDB())
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	abreBarracaComAdds(t, vendedor)

	lista := pedeVitrine(t, comprador, 0, protocol.LojaFiltroTodos)
	if int(lista.Qtd) != len(itensComAdds) || int(lista.Total) != len(itensComAdds) {
		t.Fatalf("vitrine = qtd %d, total %d; queria %d ofertas", lista.Qtd, lista.Total,
			len(itensComAdds))
	}
	porPrateleira := map[int8]protocol.LojaOferta{}
	for i := 0; i < int(lista.Qtd); i++ {
		porPrateleira[lista.Ofertas[i].Slot] = lista.Ofertas[i]
	}
	for _, it := range itensComAdds {
		o, ok := porPrateleira[it.prateleira]
		if !ok {
			t.Errorf("%s: a prateleira %d nao veio na vitrine", it.nome, it.prateleira)
			continue
		}
		if o.Indice != it.item.Index {
			t.Errorf("%s: prateleira %d trouxe o item %d; queria %d", it.nome, it.prateleira,
				o.Indice, it.item.Index)
		}
		if o.Efeitos != it.quero {
			t.Errorf("%s: adds na vitrine = %+v; queria %+v", it.nome, o.Efeitos, it.quero)
		}
	}
	// O que já ia continua indo: o "+9" da linha é o mesmo EF_SANC que agora também
	// aparece no bloco.
	if o := porPrateleira[2]; o.Refino != 9 || o.Preco != 1003 {
		t.Errorf("Balmung = +%d por %d; queria +9 por 1003", o.Refino, o.Preco)
	}
	// Quadrado vazio da página não tem add: o painel desenharia um add inventado.
	for i := int(lista.Qtd); i < protocol.LojaPorPagina; i++ {
		if lista.Ofertas[i].Efeitos != (protocol.LojaEfeitos{}) {
			t.Errorf("a oferta vazia %d veio com adds: %+v", i, lista.Ofertas[i].Efeitos)
		}
	}
}

// TESTE 6 (cofre): o item do baú chega ao painel de montar a barraca com os adds dele.
func TestCofreMostraOsAddsDeCadaItem(t *testing.T) {
	addr, stop, _ := startServerClock(t, addsDB())
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	cofre := pedeCofre(t, vendedor)
	if int(cofre.Qtd) != len(itensComAdds) {
		t.Fatalf("cofre = %d itens; queria %d", cofre.Qtd, len(itensComAdds))
	}
	porSlot := map[int16]protocol.LojaCargoItem{}
	for i := 0; i < int(cofre.Qtd); i++ {
		porSlot[cofre.Itens[i].Slot] = cofre.Itens[i]
	}
	for _, it := range itensComAdds {
		c, ok := porSlot[int16(it.cargoPos)]
		if !ok {
			t.Errorf("%s: o slot %d do bau nao veio", it.nome, it.cargoPos)
			continue
		}
		if c.Indice != it.item.Index {
			t.Errorf("%s: slot %d trouxe o item %d; queria %d", it.nome, it.cargoPos, c.Indice,
				it.item.Index)
		}
		if c.Efeitos != it.quero {
			t.Errorf("%s: adds no cofre = %+v; queria %+v", it.nome, c.Efeitos, it.quero)
		}
	}
	for i := int(cofre.Qtd); i < protocol.LojaCargoMax; i++ {
		if cofre.Itens[i].Efeitos != (protocol.LojaEfeitos{}) {
			t.Errorf("a linha vazia %d do cofre veio com adds: %+v", i, cofre.Itens[i].Efeitos)
		}
	}
}

// O BAÚ CHEIO: 128 itens, cada um com adds que só ele tem, e cada um volta com os seus.
//
// É o caso que o baú pela metade não exercita: a lista do cofre só tem os slots
// ocupados, então com buracos a posição na lista e a posição no baú são números
// diferentes; cheio, qualquer deslocamento de um aparece em 128 lugares.
func TestCofreCheioCadaItemComOsSeusAdds(t *testing.T) {
	db := autotradeDB(0)
	var cargo world.CargoState
	quero := map[int]protocol.LojaEfeitos{}
	for pos := 0; pos < world.MaxCargo; pos++ {
		// Efeitos de 1 a 40: longe de EF_SANC e EF_AMOUNT, que mudariam a linha do
		// item, e todos diferentes entre os três pares.
		e1, e2, e3 := uint8(1+pos%13), uint8(14+pos%13), uint8(27+pos%13)
		v1, v2, v3 := uint8(1+pos), uint8(130+pos%120), uint8(255-pos)
		cargo.Items[pos] = world.Item{Index: int16(1200 + pos), Effects: [3]world.Effect{
			{Effect: e1, Value: v1}, {Effect: e2, Value: v2}, {Effect: e3, Value: v3}}}
		quero[pos] = protocol.LojaEfeitos{Ef1: e1, V1: v1, Ef2: e2, V2: v2, Ef3: e3, V3: v3}
	}
	db.accounts["tester"].cargo = cargo

	addr, stop, _ := startServerClock(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	cofre := pedeCofre(t, vendedor)
	if cofre.Qtd != world.MaxCargo {
		t.Fatalf("cofre = %d itens; queria %d", cofre.Qtd, world.MaxCargo)
	}
	for i := 0; i < int(cofre.Qtd); i++ {
		c := cofre.Itens[i]
		if c.Indice != int16(1200+int(c.Slot)) {
			t.Errorf("linha %d: slot %d com o item %d; queria %d", i, c.Slot, c.Indice,
				1200+int(c.Slot))
		}
		if c.Efeitos != quero[int(c.Slot)] {
			t.Errorf("linha %d (slot %d): adds = %+v; queria %+v", i, c.Slot, c.Efeitos,
				quero[int(c.Slot)])
		}
	}
}

// pedeCofre manda MsgLojaCargo e devolve o cofre que voltou.
func pedeCofre(t *testing.T, c net.Conn) protocol.LojaCargoListaBody {
	t.Helper()
	send(t, c, protocol.MsgLojaCargo, nil)
	payload, _ := readUntil(t, c, protocol.MsgLojaCargoLista)
	// O tamanho do que CHEGOU, e não o da constante: é o número que o cliente novo usa
	// para reconhecer que o bloco veio.
	if len(payload) != 1796 {
		t.Fatalf("o cofre chegou com %d bytes de corpo; o contrato diz 1796", len(payload))
	}
	var cofre protocol.LojaCargoListaBody
	if err := cofre.Decode(payload); err != nil {
		t.Fatalf("decodificando o cofre: %v", err)
	}
	return cofre
}

// A vitrine também chega com o tamanho do contrato, mesmo vazia: o cliente novo decide
// se há bloco pelo tamanho do pacote, e uma página sem ofertas não pode parecer um
// pacote do servidor antigo.
func TestVitrineVaziaChegaComOTamanhoDoBloco(t *testing.T) {
	addr, stop, _ := startServerClock(t, addsDB())
	defer stop()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	pede := protocol.LojaPedeBody{Pagina: 0, Filtro: protocol.LojaFiltroTodos}
	send(t, comprador, protocol.MsgLojaPede, pede.Encode())
	payload, _ := readUntil(t, comprador, protocol.MsgLojaLista)
	if len(payload) != 1350 {
		t.Fatalf("a vitrine vazia chegou com %d bytes de corpo; o contrato diz 1350", len(payload))
	}
	for i, b := range payload[1140:] {
		if b != 0 {
			t.Fatalf("vitrine vazia com o byte %d do bloco de adds = %#02x; queria zero", i, b)
		}
	}
}

// efeitosDaLoja leva os três pares NA ORDEM DO ITEM, efeito antes do valor.
//
// Valores todos diferentes: com dois iguais, trocar um par pelo outro passaria.
func TestEfeitosDaLojaGuardaAOrdemDosPares(t *testing.T) {
	casos := []struct {
		nome  string
		entra [3]world.Effect
		quero protocol.LojaEfeitos
	}{
		{"tres pares", [3]world.Effect{{Effect: 11, Value: 12}, {Effect: 21, Value: 22}, {Effect: 31, Value: 32}},
			protocol.LojaEfeitos{Ef1: 11, V1: 12, Ef2: 21, V2: 22, Ef3: 31, V3: 32}},
		{"so o primeiro", [3]world.Effect{{Effect: 11, Value: 12}},
			protocol.LojaEfeitos{Ef1: 11, V1: 12}},
		{"so o do meio", [3]world.Effect{{}, {Effect: 21, Value: 22}},
			protocol.LojaEfeitos{Ef2: 21, V2: 22}},
		{"so o ultimo", [3]world.Effect{{}, {}, {Effect: 31, Value: 32}},
			protocol.LojaEfeitos{Ef3: 31, V3: 32}},
		{"nenhum", [3]world.Effect{}, protocol.LojaEfeitos{}},
	}
	for _, c := range casos {
		if tem := efeitosDaLoja(c.entra); tem != c.quero {
			t.Errorf("%s: %+v; queria %+v", c.nome, tem, c.quero)
		}
	}
}
