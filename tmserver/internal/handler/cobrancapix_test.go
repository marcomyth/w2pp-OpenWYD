package handler

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// cobradorDeMentira é a ponte fingida: guarda o que pediram e devolve o que o
// teste mandar.
type cobradorDeMentira struct {
	mu         sync.Mutex
	anuncio    int64
	comprador  int64
	referencia string
	chamadas   int
	resposta   CobrancaAberta
	erro       error
}

func (c *cobradorDeMentira) AbrirCobranca(_ context.Context, anuncio, comprador int64,
	ref string,
) (CobrancaAberta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anuncio, c.comprador, c.referencia = anuncio, comprador, ref
	c.chamadas++
	return c.resposta, c.erro
}

func (c *cobradorDeMentira) pedido() (int64, int64, string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.anuncio, c.comprador, c.referencia, c.chamadas
}

// usaCobrador liga a ponte fingida e a desliga no fim do teste.
func usaCobrador(t *testing.T, c CobradorPix) {
	t.Helper()
	UsaCobradorPix(c)
	t.Cleanup(func() { UsaCobradorPix(pixNaoLigado{}) })
}

// prateleiraEmRMT monta o estado REAL de uma prateleira em dinheiro real: item no
// baú do vendedor MARCADO pelo anúncio, e a moeda do slot em RMT.
//
// Marcado é o ponto. O teste anterior deste caminho montava a prateleira em RMT
// com o item SEM marca, que é um estado que o jogo não produz — e foi por isso que
// ele não pegou a trava do escrow recusando a compra legítima.
func prateleiraEmRMT(t *testing.T, w *world.World, vendedorConta int64, anuncio int64) {
	t.Helper()
	noLacoDoMundo(t, w, func(w *world.World) {
		c := w.Cargo(vendedorConta)
		if c == nil {
			t.Fatal("o bau do vendedor nao esta carregado")
		}
		c.Items[0].AnuncioRMT = anuncio
		w.ForEachSession(func(s *world.Session, _ *world.Entity) {
			if s != nil && s.AutoTrade != nil && s.AccountID == vendedorConta {
				s.AutoTrade.Moeda[0] = protocol.LojaMoedaRMT
				s.AutoTrade.Slots[0].Item = c.Items[0]
			}
		})
	})
}

// compraRMT monta o cenário completo e devolve o socket do comprador.
func compraRMT(t *testing.T, anuncio int64) (*fakeDB, *world.World, net.Conn, int32) {
	t.Helper()
	db := autotradeDB(1030)
	addr, stop, w := startServerNovato(t, db)
	t.Cleanup(stop)
	vendedor := enterWorldAs(t, addr, "tester")
	t.Cleanup(func() { _ = vendedor.Close() })
	comprador := enterWorldAs(t, addr, "tradeb")
	t.Cleanup(func() { _ = comprador.Close() })

	barraca := abreBarraca(t, vendedor, "Loja", 0, 5000, protocol.LojaMoedaOuro)
	prateleiraEmRMT(t, w, 7, anuncio)
	drena(t, comprador)
	return db, w, comprador, barraca
}

// CLICAR EM COMPRAR ABRE A COBRANÇA E DIZ ONDE PAGAR.
//
// Nada se move neste instante: o item fica no baú do vendedor, preso pela marca, e
// o comprador não paga nada ainda. O que sai daqui é uma cobrança e um aviso.
func TestComprarEmDinheiroRealAbreACobrancaEAvisaOndePagar(t *testing.T) {
	const anuncio = int64(4242)
	cobrador := &cobradorDeMentira{resposta: CobrancaAberta{
		CobrancaID: 7, CodigoPix: "00020126BR", ExpiraEm: time.Now().Add(5 * time.Minute)}}
	usaCobrador(t, cobrador)

	_, w, comprador, barraca := compraRMT(t, anuncio)
	send(t, comprador, protocol.MsgLojaCompra,
		(&protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaRMT}).Encode())

	if !recebeu(t, comprador, msgPagueNoSite()) {
		t.Error("nao disse ao comprador onde pagar; o pagamento nao esta no jogo e ele nao adivinha")
	}
	pedidoAnuncio, pedidoComprador, ref, chamadas := cobrador.pedido()
	if chamadas != 1 {
		t.Errorf("chamou a ponte %d vez(es), quero 1", chamadas)
	}
	if pedidoAnuncio != anuncio {
		t.Errorf("cobrou o anuncio %d, quero %d — o id vem da MARCA do bau", pedidoAnuncio, anuncio)
	}
	if pedidoComprador != 11 {
		t.Errorf("comprador = %d, quero 11", pedidoComprador)
	}
	if len(ref) < 10 {
		t.Errorf("referencia = %q; ela e a ancora da idempotencia e nao pode ser adivinhavel", ref)
	}
	// O ITEM NÃO SE MOVEU. É a metade que importa: a venda ainda não aconteceu.
	noLacoDoMundo(t, w, func(w *world.World) {
		c := w.Cargo(7)
		if c == nil || c.Items[0].Empty() {
			t.Error("o item saiu do bau do vendedor antes de alguem pagar")
		}
		if c.Items[0].AnuncioRMT != anuncio {
			t.Error("a marca do escrow saiu do item")
		}
	})
}

// O AVISO DIZ PARA NÃO SAIR DO JOGO, e essa linha vale uma compra.
//
// Sair do jogo CANCELA a cobrança, e o movimento natural de quem vai pagar no
// celular é fechar o jogo. Sem isto, o primeiro comprador de verdade perde a
// compra fazendo exatamente o que parecia certo.
func TestOAvisoDizParaNaoSairDoJogo(t *testing.T) {
	msg := msgPagueNoSite()
	if !contemPedaco(msg, "NÃO saia do jogo") {
		t.Errorf("a mensagem %q nao avisa para ficar; sair do jogo cancela a cobranca", msg)
	}
	// O ENDEREÇO COM www, MEDIDO: `wydretry.com` sem o www não responde nada
	// (curl devolve 000); só `www.wydretry.com` atende. A frase anterior mandava o
	// jogador para um endereço morto.
	if !contemPedaco(msg, "www.wydretry.com/conta") {
		t.Errorf("a mensagem %q nao diz ONDE pagar, ou manda para o dominio sem www, "+
			"que nao responde", msg)
	}
}

// O PRAZO DA MENSAGEM SEGUE A CONFIGURAÇÃO.
//
// Com o número escrito na frase, mudar a janela deixaria a mensagem mentindo — e
// mentir sobre prazo de pagamento é a mentira mais cara que esta tela pode contar.
func TestOPrazoDaMensagemSegueAConfiguracao(t *testing.T) {
	antes := JanelaDeCobranca
	t.Cleanup(func() { JanelaDeCobranca = antes })

	DefineJanelaDeCobranca(9 * time.Minute)
	if msg := msgPagueNoSite(); !contemPedaco(msg, "9 minutos") {
		t.Errorf("a mensagem %q nao acompanhou a janela de 9 minutos", msg)
	}
	// E valor inválido não vira prazo zero, que faria toda cobrança nascer vencida.
	DefineJanelaDeCobranca(0)
	if JanelaDeCobranca != 9*time.Minute {
		t.Errorf("a janela virou %v com um valor invalido", JanelaDeCobranca)
	}
}

// AS TRÊS RECUSAS PREVISTAS chegam ao comprador com o motivo certo, lado a lado.
//
// O valor está na DIFERENÇA: "alguém já está pagando" e "esse anúncio é seu" levam
// a ações opostas, e uma recusa genérica faria as duas parecerem a mesma coisa.
func TestAsRecusasDaCompraEmDinheiroReal(t *testing.T) {
	casos := []struct {
		nome     string
		erro     error
		mensagem string
	}{
		{"ja tem comprador", ErrCobrancaJaAberta, msgItemJaTemComprador},
		{"comprando de si mesmo", ErrCompradorEOVendedor, msgNaoComprarDeSiMesmo},
		{"a ponte caiu", errors.New("timeout"), msgCobrancaNaoSaiu},
		{"ja esta pagando outra coisa", ErrCompradorJaTemCobranca, msgJaTemPagamentoAberto},
		// A PONTE DESLIGADA NÃO É "TENTE DE NOVO". Mandar repetir uma coisa que
		// nunca vai dar certo faz o jogador clicar até desistir.
		{"a ponte nao esta ligada", ErrPixNaoLigado, msgRMTNaoEstaAberta},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			cobrador := &cobradorDeMentira{erro: c.erro}
			usaCobrador(t, cobrador)
			_, w, comprador, barraca := compraRMT(t, 4242)

			send(t, comprador, protocol.MsgLojaCompra,
				(&protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaRMT}).Encode())

			if !recebeu(t, comprador, c.mensagem) {
				t.Errorf("nao recebeu %q", c.mensagem)
			}
			noLacoDoMundo(t, w, func(w *world.World) {
				if c := w.Cargo(7); c == nil || c.Items[0].Empty() {
					t.Error("o item saiu do bau numa compra recusada")
				}
			})
		})
	}
}

// PRATELEIRA EM DINHEIRO REAL SEM MARCA NÃO COBRA NADA.
//
// É o anúncio órfão, que a reconciliação limpa. Cobrar assim mesmo geraria dinheiro
// entrando sem nada para entregar — que é o PAGA_SEM_ITEM, dívida com uma pessoa.
func TestPrateleiraSemMarcaNaoCobra(t *testing.T) {
	cobrador := &cobradorDeMentira{}
	usaCobrador(t, cobrador)
	// anuncio = 0 significa item sem marca.
	_, _, comprador, barraca := compraRMT(t, 0)

	send(t, comprador, protocol.MsgLojaCompra,
		(&protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaRMT}).Encode())

	if !recebeu(t, comprador, msgCobrancaNaoSaiu) {
		t.Error("recusou em silencio")
	}
	if _, _, _, chamadas := cobrador.pedido(); chamadas != 0 {
		t.Errorf("foi a ponte %d vez(es) por um anuncio que nao existe", chamadas)
	}
}

// A COMPRA EM OURO DE UM ITEM MARCADO CONTINUA RECUSADA.
//
// É o buraco que o PR 82 fechou, e ele não pode ter sido reaberto ao deixar a
// compra em dinheiro real passar: a marca ainda protege o item de TODA outra
// moeda.
func TestCompraEmOuroDeItemMarcadoContinuaRecusada(t *testing.T) {
	usaCobrador(t, &cobradorDeMentira{})
	db := autotradeDB(1030)
	db.loads[11] = world.CharacterState{Slot: 0, Name: "Comprador", Level: 1, HP: 1000, MaxHP: 1000, Coin: 1_000_000}
	addr, stop, w := startServerNovato(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	// Barraca em OURO, e o item marcado por um anúncio que sobrou de antes.
	barraca := abreBarraca(t, vendedor, "Loja", 0, 1000, protocol.LojaMoedaOuro)
	noLacoDoMundo(t, w, func(w *world.World) {
		if c := w.Cargo(7); c != nil {
			c.Items[0].AnuncioRMT = 777
		}
	})
	drena(t, comprador)

	send(t, comprador, protocol.MsgLojaCompra,
		(&protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaOuro}).Encode())

	for {
		ty, _, ok := readMaybe(t, comprador)
		if !ok {
			break
		}
		if ty == protocol.MsgSendItem {
			t.Fatal("a compra em ouro levou um item preso num anuncio em dinheiro real")
		}
	}
}

func contemPedaco(todo, pedaco string) bool {
	for i := 0; i+len(pedaco) <= len(todo); i++ {
		if todo[i:i+len(pedaco)] == pedaco {
			return true
		}
	}
	return false
}
