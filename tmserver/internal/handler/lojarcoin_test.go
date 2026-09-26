package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// lojaRcoinDB é o banco da Loja de Rcoin nos testes: 25 ofertas na aba 1, e uma
// compra que o teste pode segurar no meio, que é onde mora o risco de cobrar duas
// vezes.
type lojaRcoinDB struct {
	*fakeDB
	listas   atomic.Int32
	compras  atomic.Int32
	segura   chan struct{} // nil = responde na hora
	erro     error
	resposta world.RcoinCompra
	drenos   atomic.Int32
}

func (l *lojaRcoinDB) ListRcoinOffers(_ context.Context, conta int64, categoria int32) ([]world.RcoinOferta, int32, error) {
	l.listas.Add(1)
	if conta != 7 {
		return nil, 0, errors.New("conta errada")
	}
	var out []world.RcoinOferta
	if categoria == 1 {
		for i := range 25 {
			out = append(out, world.RcoinOferta{
				ID: int64(100 + i), ItemIndex: 3379, Category: 1, Price: 50,
				Title: fmt.Sprintf("Poção %d", i),
			})
		}
	}
	return out, 777, nil
}

func (l *lojaRcoinDB) BuyRcoinOffer(context.Context, int64, int64, int32) (world.RcoinCompra, error) {
	l.compras.Add(1)
	if l.segura != nil {
		<-l.segura
	}
	return l.resposta, l.erro
}

func (l *lojaRcoinDB) ListPendingDeliveries(context.Context, int64) ([]world.Delivery, error) {
	l.drenos.Add(1)
	return []world.Delivery{{ID: 900, Item: world.Item{Index: 3901}}}, nil
}

func servidorLojaRcoin(t *testing.T, db *lojaRcoinDB) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
	w := world.New(world.Config{GridDim: 16}, log, db, d.Handle)
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

func novoLojaRcoinDB() *lojaRcoinDB {
	db := newDB()
	db.loadResult = world.CharacterState{Slot: 0, Name: "Hero", Level: 50, X: 5, Y: 5, HP: 1000, MaxHP: 1000}
	return &lojaRcoinDB{fakeDB: db}
}

// esperaTipo lê até o pacote do tipo pedido, drenando os outros.
func esperaTipo(t *testing.T, c net.Conn, ty protocol.Type) []byte {
	t.Helper()
	for range 30 {
		got, payload, ok := readMaybe(t, c)
		if !ok {
			break
		}
		if got == ty {
			return payload
		}
	}
	t.Fatalf("o servidor não mandou %#x", ty)
	return nil
}

func TestLojaDeRcoinMandaAPaginaDaAbaComOSaldo(t *testing.T) {
	db := novoLojaRcoinDB()
	addr, stop := servidorLojaRcoin(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	// Segunda página da aba 1: 25 ofertas dão duas páginas, e a segunda tem 5.
	send(t, c, protocol.MsgRcoinPede, protocol.RcoinPedeBody{Categoria: 1, Pagina: 1}.Encode())
	var pag protocol.RcoinListaBody
	if err := pag.Decode(esperaTipo(t, c, protocol.MsgRcoinLista)); err != nil {
		t.Fatal(err)
	}
	if pag.Pagina != 1 || pag.Paginas != 2 || pag.Qtd != 5 || pag.Saldo != 777 {
		t.Fatalf("página = %d de %d, qtd %d, saldo %d; quero 1 de 2, 5, 777", pag.Pagina, pag.Paginas, pag.Qtd, pag.Saldo)
	}
	if o := pag.Ofertas[0]; o.ID != 120 || o.Titulo != "Poção 20" || o.Preco != 50 {
		t.Errorf("primeira oferta da página = %+v", o)
	}
	// O byte +12 de cada oferta é a aba pedida: o cliente descarta a página se a
	// primeira disser outra (lojarcoins.cpp:193).
	for i := range int(pag.Qtd) {
		if pag.Ofertas[i].Categoria != 1 {
			t.Errorf("oferta %d veio com categoria %d, pedi a aba 1", i, pag.Ofertas[i].Categoria)
		}
	}

	// Aba fora da faixa: página vazia, sem ir ao banco.
	antes := db.listas.Load()
	send(t, c, protocol.MsgRcoinPede, protocol.RcoinPedeBody{Categoria: 9}.Encode())
	if err := pag.Decode(esperaTipo(t, c, protocol.MsgRcoinLista)); err != nil {
		t.Fatal(err)
	}
	if pag.Qtd != 0 || pag.Paginas != 1 {
		t.Errorf("aba 9 = qtd %d, páginas %d; quero vazia", pag.Qtd, pag.Paginas)
	}
	if db.listas.Load() != antes {
		t.Error("a aba fora da faixa foi ao banco")
	}
}

func TestLojaDeRcoinCompraEntregaNoBauEORepetidoNaoCobraDeNovo(t *testing.T) {
	db := novoLojaRcoinDB()
	db.resposta = world.RcoinCompra{Result: protocol.RcoinOK, Balance: 40, DeliveryID: 900}
	addr, stop := servidorLojaRcoin(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	compra := protocol.RcoinCompraBody{OfertaID: 55, Pedido: 1, PrecoVisto: 60}.Encode()
	send(t, c, protocol.MsgRcoinCompra, compra)
	var r protocol.RcoinResultadoBody
	if err := r.Decode(esperaTipo(t, c, protocol.MsgRcoinResultado)); err != nil {
		t.Fatal(err)
	}
	if r.Resultado != protocol.RcoinOK || r.OfertaID != 55 || r.Pedido != 1 || r.Saldo != 40 {
		t.Fatalf("resultado = %+v", r)
	}
	aviso := esperaTipo(t, c, protocol.MsgMessagePanel)
	if !bytes.Contains(aviso, protocol.ClientText(msgRcoinEntregue)) || db.drenos.Load() != 1 {
		t.Errorf("a entrega imediata não aconteceu: drenos %d, aviso %q", db.drenos.Load(), aviso)
	}

	// O mesmo pedido de novo: a resposta guardada volta, e o banco não é chamado.
	send(t, c, protocol.MsgRcoinCompra, compra)
	var r2 protocol.RcoinResultadoBody
	if err := r2.Decode(esperaTipo(t, c, protocol.MsgRcoinResultado)); err != nil {
		t.Fatal(err)
	}
	if r2 != r {
		t.Errorf("reenvio = %+v, quero a resposta guardada %+v", r2, r)
	}
	if n := db.compras.Load(); n != 1 {
		t.Errorf("o banco recebeu %d compras, quero 1", n)
	}
}

// Sabotagem conferida: sem o if s.DonateEmCurso, a segunda compra vai ao banco e
// este teste falha na contagem.
func TestLojaDeRcoinCompraEmVooRespondeOcupado(t *testing.T) {
	db := novoLojaRcoinDB()
	db.segura = make(chan struct{})
	db.resposta = world.RcoinCompra{Result: protocol.RcoinOK, Balance: 40, DeliveryID: 900}
	addr, stop := servidorLojaRcoin(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgRcoinCompra, protocol.RcoinCompraBody{OfertaID: 55, Pedido: 1, PrecoVisto: 60}.Encode())
	send(t, c, protocol.MsgRcoinCompra, protocol.RcoinCompraBody{OfertaID: 55, Pedido: 2, PrecoVisto: 60}.Encode())
	var r protocol.RcoinResultadoBody
	if err := r.Decode(esperaTipo(t, c, protocol.MsgRcoinResultado)); err != nil {
		t.Fatal(err)
	}
	if r.Resultado != protocol.RcoinOcupado || r.Pedido != 2 {
		t.Fatalf("segunda compra = %+v, quero OCUPADO do pedido 2", r)
	}
	close(db.segura)
	if err := r.Decode(esperaTipo(t, c, protocol.MsgRcoinResultado)); err != nil {
		t.Fatal(err)
	}
	if r.Resultado != protocol.RcoinOK || r.Pedido != 1 {
		t.Errorf("primeira compra = %+v, quero OK do pedido 1", r)
	}
	if n := db.compras.Load(); n != 1 {
		t.Errorf("o banco recebeu %d compras, quero 1", n)
	}
}

func TestLojaDeRcoinFalhaDoBancoViraErroSemEntrega(t *testing.T) {
	db := novoLojaRcoinDB()
	db.erro = errors.New("dbserver fora")
	addr, stop := servidorLojaRcoin(t, db)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	send(t, c, protocol.MsgRcoinCompra, protocol.RcoinCompraBody{OfertaID: 55, Pedido: 3, PrecoVisto: 60}.Encode())
	var r protocol.RcoinResultadoBody
	if err := r.Decode(esperaTipo(t, c, protocol.MsgRcoinResultado)); err != nil {
		t.Fatal(err)
	}
	if r.Resultado != protocol.RcoinErro || r.Pedido != 3 {
		t.Errorf("resultado = %+v, quero ERRO do pedido 3", r)
	}
	time.Sleep(100 * time.Millisecond)
	if db.drenos.Load() != 0 {
		t.Error("uma compra que falhou tentou entregar")
	}
}
