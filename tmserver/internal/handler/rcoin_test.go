package handler

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// carteiraDonate é a carteira de donate que os testes observam. O canal deixa o
// teste parar o crédito no meio, que é a única forma de provar o que acontece
// enquanto a ida ao banco está em voo — e é exatamente aí que mora o risco de
// gastar uma moeda duas vezes.
type carteiraDonate struct {
	*fakeDB
	creditos chan int32 // cada crédito pedido
	solta    chan struct{}
	erro     error
	saldo    int32
}

func novaCarteiraDonate(db *fakeDB) *carteiraDonate {
	return &carteiraDonate{fakeDB: db, creditos: make(chan int32, 8), solta: make(chan struct{}, 8)}
}

func (c *carteiraDonate) CreditDonate(_ context.Context, _ int64, amount int32, _, _ string) (int32, error) {
	c.creditos <- amount
	<-c.solta
	if c.erro != nil {
		return 0, c.erro
	}
	c.saldo += amount
	return c.saldo, nil
}

func (c *carteiraDonate) DonateBalance(context.Context, int64) (int32, error) {
	if c.erro != nil {
		return 0, c.erro
	}
	return c.saldo, nil
}

// servidorRCoin sobe um mundo com a moeda no primeiro slot da bolsa. O valor vai
// em ItemDonates, que é de onde o EF_DONATE do ItemList.csv chega ao jogo.
func servidorRCoin(t *testing.T, item world.Item, valor int32) (string, func(), *carteiraDonate) {
	t.Helper()
	db := newDB()
	st := world.CharacterState{Slot: 0, Name: "Hero", Level: 50, X: 5, Y: 5, HP: 1000, MaxHP: 1000}
	st.Carry[0] = item
	db.loadResult = st
	carteira := novaCarteiraDonate(db)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{
		Log:           log,
		ItemVolatiles: map[int]int{int(item.Index): volDonate},
		ItemDonates:   map[int]int32{int(item.Index): valor},
	})
	w := world.New(world.Config{GridDim: 16}, log, carteira, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}, carteira
}

// usaRCoin manda o MSG_UseItem do slot 0 e devolve o item que o servidor
// responde para aquele slot.
func usaRCoin(t *testing.T, c net.Conn, slot int) {
	t.Helper()
	body := protocol.MsgUseItemBody{SourType: world.ItemPlaceCarry, SourPos: int32(slot)}
	send(t, c, protocol.MsgUseItem, body.Encode())
}

// esperaSlotRCoin lê até o MsgSendItem do slot pedido e devolve o índice do item que
// ficou nele. Os outros pacotes são drenados: um socket que ninguém lê trava o
// servidor no meio do teste.
func esperaSlotRCoin(t *testing.T, c net.Conn, slot int) int {
	t.Helper()
	for range 8 {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem && int(le16(payload[2:4])) == slot {
			return int(le16(payload[4:6]))
		}
	}
	t.Fatalf("o servidor não respondeu o slot %d", slot)
	return -1
}

// TestRCoinCreditaODonateDoCatalogo é o caso feliz: a moeda paga o EF_DONATE que
// o catálogo diz e sai da bolsa.
func TestRCoinCreditaODonateDoCatalogo(t *testing.T) {
	addr, stop, carteira := servidorRCoin(t, world.Item{Index: 3393}, 100)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	select {
	case valor := <-carteira.creditos:
		// 100 escrito à mão, e não a constante: com a constante o teste
		// concordaria com qualquer valor que alguém pusesse nela.
		if valor != 100 {
			t.Fatalf("creditou %d de donate, queria 100", valor)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a moeda não creditou nada")
	}
	carteira.solta <- struct{}{}
	if idx := esperaSlotRCoin(t, c, 0); idx != 0 {
		t.Errorf("a moeda ficou na bolsa: slot 0 tem o item %d", idx)
	}
}

// TestRCoinNaoCreditaDuasVezesNoCliqueDuplo cobre a trava por sessão: com o
// banco parado no meio do primeiro crédito, o segundo clique na MESMA pilha não
// abre uma segunda ida ao banco.
//
// A pilha é de duas de propósito. Com uma moeda só o teste passaria sem a trava,
// porque o consumo antecipado já esvaziou o slot — e aí ele provaria o teste
// seguinte, não este. Sabotagem conferida: sem o if s.DonateEmCurso, este falha.
func TestRCoinNaoCreditaDuasVezesNoCliqueDuplo(t *testing.T) {
	moeda := world.Item{Index: 3393, Effects: [3]world.Effect{{Effect: efAmount, Value: 2}}}
	addr, stop, carteira := servidorRCoin(t, moeda, 100)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	select {
	case <-carteira.creditos:
	case <-time.After(2 * time.Second):
		t.Fatal("o primeiro clique não creditou")
	}
	// O banco ainda não respondeu. Segundo clique.
	usaRCoin(t, c, 0)
	select {
	case valor := <-carteira.creditos:
		t.Fatalf("o segundo clique creditou %d de donate com uma moeda só", valor)
	case <-time.After(250 * time.Millisecond):
	}
	carteira.solta <- struct{}{}
}

// TestRCoinVoltaParaABolsaQuandoOBancoFalha: o crédito não aconteceu, então a
// moeda não pode ter sumido. Sem isto, uma queda do dbServer come a moeda do
// jogador em silêncio.
func TestRCoinVoltaParaABolsaQuandoOBancoFalha(t *testing.T) {
	addr, stop, carteira := servidorRCoin(t, world.Item{Index: 3393}, 100)
	carteira.erro = context.DeadlineExceeded
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	select {
	case <-carteira.creditos:
	case <-time.After(2 * time.Second):
		t.Fatal("o crédito não foi tentado")
	}
	carteira.solta <- struct{}{}

	// O primeiro SendItem esvazia o slot (o consumo), o segundo o repõe.
	prazo := time.Now().Add(2 * time.Second)
	devolvida := false
	for time.Now().Before(prazo) && !devolvida {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem && le16(payload[2:4]) == 0 && le16(payload[4:6]) == 3393 {
			devolvida = true
		}
	}
	if !devolvida {
		t.Error("o banco falhou e a moeda não voltou para a bolsa")
	}
}

// TestRCoinSemDonateNoCatalogoRecusa: um item Vol 184 que o ItemList.csv não
// precifica não pode virar um crédito de zero nem sumir da bolsa.
func TestRCoinSemDonateNoCatalogoRecusa(t *testing.T) {
	addr, stop, carteira := servidorRCoin(t, world.Item{Index: 3393}, 0)
	// Sem entrada em ItemDonates: valor 0.
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	if idx := esperaSlotRCoin(t, c, 0); idx != 3393 {
		t.Errorf("slot 0 tem o item %d, queria a moeda 3393 de volta", idx)
	}
	select {
	case valor := <-carteira.creditos:
		t.Fatalf("creditou %d de donate por um item sem EF_DONATE", valor)
	case <-time.After(250 * time.Millisecond):
	}
}

// TestRCoinConsomeUmaDaPilha: usar uma moeda de uma pilha de cinco tira uma, não
// a pilha.
func TestRCoinConsomeUmaDaPilha(t *testing.T) {
	moeda := world.Item{Index: 3393, Effects: [3]world.Effect{{Effect: efAmount, Value: 5}}}
	addr, stop, carteira := servidorRCoin(t, moeda, 100)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	select {
	case <-carteira.creditos:
	case <-time.After(2 * time.Second):
		t.Fatal("a moeda não creditou")
	}
	carteira.solta <- struct{}{}
	if idx := esperaSlotRCoin(t, c, 0); idx != 3393 {
		t.Fatalf("slot 0 tem o item %d, queria a pilha de moedas", idx)
	}
}

// TestRCoinConsumidaAntesDoCreditoNaoPodeCairNoChao é o teste que justifica a
// ordem da useRCoin — consumir a moeda ANTES da ida ao banco.
//
// A trava por sessão não cobre isto: aqui o jogador não clica duas vezes, ele
// JOGA A MOEDA NO CHÃO enquanto o crédito está em voo. Se o consumo tivesse
// ficado para a volta, a moeda estaria na bolsa para ser largada, e o jogador
// terminaria com o donate creditado e a moeda no chão para pegar de novo.
//
// Sabotagem conferida: movendo o consumeOneItem para dentro da continuação,
// este teste falha e o do clique duplo continua passando.
func TestRCoinConsumidaAntesDoCreditoNaoPodeCairNoChao(t *testing.T) {
	addr, stop, carteira := servidorRCoin(t, world.Item{Index: 3393}, 100)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	select {
	case <-carteira.creditos:
	case <-time.After(2 * time.Second):
		t.Fatal("a moeda não creditou")
	}
	// O crédito está em voo. A moeda já não pode estar na bolsa.
	dropAt(t, c, 0, 0, 6, 5)

	// Um CNFDropItem do slot 0 significa que havia o que largar.
	prazo := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(prazo) {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgCNFDropItem && len(payload) >= 8 &&
			int32(binary.LittleEndian.Uint32(payload[4:8])) == 0 {
			t.Fatal("a moeda ainda estava na bolsa durante o crédito e foi largada no chão")
		}
	}
	carteira.solta <- struct{}{}
}

// servidorLojaRCoin sobe um NPC de loja com a RCoin na vaga 0, pelo mesmo
// caminho do startServerShopItems — aqui com ItemDonates, que é o que o guard lê.
func servidorLojaRCoin(t *testing.T, preco int32) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{
		Log:         log,
		ItemPrices:  map[int]int32{3393: preco},
		ItemDonates: map[int]int32{3393: 100},
	})
	w := world.New(world.Config{GridDim: 16}, log, shopDB(1234), d.Handle)

	tmpl := make([]byte, 816)
	copy(tmpl[0:16], "DonatesBars")
	tmpl[92+12] = 1
	binary.LittleEndian.PutUint32(tmpl[92+16:], 100)
	binary.LittleEndian.PutUint32(tmpl[92+24:], 100)
	binary.LittleEndian.PutUint16(tmpl[268:], 3393) // Carry[0] = RCoin_100
	if id := w.SpawnMob(tmpl, 5, 5); id != shopNPCID {
		t.Fatalf("NPC nasceu com o id %d, queria %d", id, shopNPCID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor não parou")
		}
	}
}

// TestRCoinNaoSeCompraEmLojaDeNPC fecha o buraco que portar o EF_VOLATILE 184
// abriu: o seed de 0006 põe as cinco RCoin na loja do DonatesBars e o PREÇO
// delas no catálogo é ZERO, e a compra aceita preço zero de propósito (o legado
// tem itens de graça). Sem este guard, qualquer jogador vira donate infinito a
// um clique.
//
// Os dois preços importam: zero é o do catálogo hoje, e um preço qualquer em
// ouro continua sendo ouro virando donate.
func TestRCoinNaoSeCompraEmLojaDeNPC(t *testing.T) {
	for _, preco := range []int32{0, 500} {
		t.Run(map[bool]string{true: "de graça", false: "por ouro"}[preco == 0], func(t *testing.T) {
			addr, stop := servidorLojaRCoin(t, preco)
			defer stop()
			c := enterWorld(t, addr)
			defer c.Close()

			buyFrame(t, c, shopNPCID, 0, 3)

			// A recusa responde o slot de destino VAZIO; uma compra aceita
			// mandaria o MsgBuy com o eco do ouro antes de qualquer coisa.
			prazo := time.Now().Add(time.Second)
			for time.Now().Before(prazo) {
				ty, payload, ok := readMaybe(t, c)
				if !ok {
					break
				}
				if ty == protocol.MsgBuy {
					t.Fatal("o servidor vendeu a RCoin: ouro virou donate")
				}
				if ty == protocol.MsgSendItem && le16(payload[2:4]) == 3 {
					if idx := le16(payload[4:6]); idx != 0 {
						t.Fatalf("a RCoin entrou na bolsa: slot 3 tem o item %d", idx)
					}
					return
				}
			}
			t.Fatal("o servidor não respondeu a recusa")
		})
	}
}

// TestRCoinEmpilha: as cinco moedas somam numa pilha só. Sem isto, cem moedas
// de 100 ocupam cem espaços da bolsa e negociar volume fica impraticável — e é
// negociar entre jogadores que elas existem para fazer.
func TestRCoinEmpilha(t *testing.T) {
	for _, idx := range []int16{itemRCoin100, itemRCoin1K, itemRCoin3K, itemRCoin5K, itemRCoin10K} {
		if !isSplittable(idx) {
			t.Errorf("a moeda %d não empilha", idx)
		}
	}
	// Duas pilhas da mesma moeda são da mesma classe; moedas de valor diferente
	// não se misturam, ou 10 de 100 viariam 10 de 10K.
	cem := world.Item{Index: itemRCoin100, Effects: [3]world.Effect{{Effect: efAmount, Value: 5}}}
	maisCem := world.Item{Index: itemRCoin100, Effects: [3]world.Effect{{Effect: efAmount, Value: 3}}}
	dezMil := world.Item{Index: itemRCoin10K, Effects: [3]world.Effect{{Effect: efAmount, Value: 3}}}
	if !sameStackClass(cem, maisCem) {
		t.Error("duas pilhas de RCoin_100 não se juntam")
	}
	if sameStackClass(cem, dezMil) {
		t.Error("RCoin_100 e RCoin_10K se juntariam na mesma pilha")
	}
	if tryMergeItemStacks(&maisCem, &cem); itemAmount(cem) != 8 || !maisCem.Empty() {
		t.Errorf("juntar 5+3 deu %d (origem vazia: %v), quer 8 e true", itemAmount(cem), maisCem.Empty())
	}
}

// TestRCoinDevolvidaVoltaParaAPilha: com o resto da pilha ainda no espaço, a
// moeda devolvida soma nela em vez de ocupar um espaço novo.
func TestRCoinDevolvidaVoltaParaAPilha(t *testing.T) {
	moeda := world.Item{Index: itemRCoin100, Effects: [3]world.Effect{{Effect: efAmount, Value: 5}}}
	addr, stop, carteira := servidorRCoin(t, moeda, 100)
	carteira.erro = context.DeadlineExceeded
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	usaRCoin(t, c, 0)
	select {
	case <-carteira.creditos:
	case <-time.After(2 * time.Second):
		t.Fatal("o crédito não foi tentado")
	}
	carteira.solta <- struct{}{}

	// O consumo manda a pilha em 4; a devolução tem de mandá-la de volta em 5,
	// no MESMO espaço.
	prazo := time.Now().Add(2 * time.Second)
	var ultimoSlot, ultimaQtd = -1, -1
	for time.Now().Before(prazo) {
		ty, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem && le16(payload[4:6]) == uint16(itemRCoin100) {
			ultimoSlot = int(le16(payload[2:4]))
			// Os três pares (efeito, valor) vêm logo depois do índice.
			for i := 0; i < 3; i++ {
				if payload[6+i*2] == efAmount {
					ultimaQtd = int(payload[7+i*2])
				}
			}
		}
	}
	if ultimoSlot != 0 {
		t.Errorf("a moeda voltou para o espaço %d, queria o 0 (a própria pilha)", ultimoSlot)
	}
	if ultimaQtd != 5 {
		t.Errorf("a pilha ficou com %d moedas, queria 5", ultimaQtd)
	}
}
