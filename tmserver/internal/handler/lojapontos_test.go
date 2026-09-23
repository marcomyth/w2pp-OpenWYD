package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Os pontos de lojinha e a barraca do painel.
//
// shoppoints_test.go já prova a conta: quinze minutos inteiros, nem um a mais,
// e o dobro com a Fada Azul. O que faltava provar é a ligação — que a barraca
// montada pelo PAINEL (MsgLojaAbrir) entra nessa conta como a da janela antiga.
//
// A ligação é um par de linhas em lojaabrir.go: OpenedAt e PaidUntil postos no
// mesmo instante em que a barraca nasce. Sem elas o relógio começaria em zero e
// a primeira varredura pagaria de uma vez todos os quinze minutos que o SERVIDOR
// está de pé — não é o tipo de erro que se percebe olhando a tela.

// AddShopPoints é a carteira de pontos da conta. Sobrescreve o NopPersistence que
// o fakeDB embute.
//
// O piso é copiado do banco de verdade (CHECK balance >= 0 em 0060_shop_points):
// um gasto maior que o saldo não move nada e devolve a sentinela. Sem isso a Loja
// de Honra passaria no teste vendendo a crédito.
func (f *fakeDB) AddShopPoints(_ context.Context, _ int64, delta int32, _, _ string) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pontosLojinha+delta < 0 {
		return 0, world.ErrPontosInsuficientes
	}
	f.pontosLojinha += delta
	return f.pontosLojinha, nil
}

// ShopPoints lê a carteira, como o /pontos e a abertura da Loja de Honra fazem.
func (f *fakeDB) ShopPoints(_ context.Context, _ int64) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pontosLojinha, nil
}

func (f *fakeDB) pontosLojinha0() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pontosLojinha
}

// startServerPontos é o startServerClock com a varredura ligada: relógio na mão
// do teste, e o tique do jogo rápido o bastante para a varredura passar várias
// vezes por segundo. O relógio NÃO anda sozinho - quem anda com ele é o teste.
func startServerPontos(t *testing.T, persist world.Persistence) (string, func(), *atomic.Uint32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clock := &atomic.Uint32{}
	clock.Store(serverTime)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, Now: func() time.Time { return time.Unix(0, 0) }})
	w := world.New(world.Config{GridDim: 16, Now: clock.Load}, log, persist, d.Handle)
	w.SetSessionEndHandler(d.SessionEnd)
	w.SetTickHandler(2*time.Millisecond, d.Tick)
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
	}, clock
}

// esperaPontos espera a carteira passar de zero, até o prazo.
func esperaPontos(db *fakeDB, prazo time.Duration) int32 {
	limite := time.Now().Add(prazo)
	for time.Now().Before(limite) {
		if p := db.pontosLojinha0(); p > 0 {
			return p
		}
		time.Sleep(5 * time.Millisecond)
	}
	return db.pontosLojinha0()
}

func TestBarracaDoPainelRendePontosACadaQuinzeMinutos(t *testing.T) {
	db := autotradeDB(1030)
	addr, stop, clock := startServerPontos(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()

	abreBarraca(t, vendedor, "Minha Loja", 0, 250_000, protocol.LojaMoedaOuro)

	// Catorze minutos e meio: a janela não fechou, e quem paga por janela
	// incompleta paga cedo demais. A varredura roda dezenas de vezes aqui
	// dentro, de propósito - nenhuma delas pode creditar nada.
	clock.Add(shopPointsWindowMs - 30_000)
	time.Sleep(200 * time.Millisecond)
	if p := db.pontosLojinha0(); p != 0 {
		t.Fatalf("pagou %d pontos antes de a janela fechar; queria 0", p)
	}

	// Passa dos quinze: uma janela inteira, e só uma.
	clock.Add(60_000)
	if p := esperaPontos(db, 2*time.Second); p != shopPointsBase {
		t.Fatalf("depois de uma janela a conta tem %d pontos; queria %d", p, shopPointsBase)
	}

	// O relógio parado não rende: chamar a varredura de novo não pode pagar
	// duas vezes a mesma janela.
	time.Sleep(200 * time.Millisecond)
	if p := db.pontosLojinha0(); p != shopPointsBase {
		t.Fatalf("a mesma janela foi paga duas vezes: %d pontos", p)
	}
}

// Uma barraca sem nada à venda não rende - e a do painel também não. O painel
// não deixa montar vazio, mas vender a última peça esvazia a barraca de pé.
func TestBarracaDoPainelVaziaNaoRende(t *testing.T) {
	db := autotradeDB(1030)
	addr, stop, clock := startServerPontos(t, db)
	defer stop()
	vendedor := enterWorldAs(t, addr, "tester")
	defer vendedor.Close()
	comprador := enterWorldAs(t, addr, "tradeb")
	defer comprador.Close()

	barraca := abreBarraca(t, vendedor, "Minha Loja", 0, 250_000, protocol.LojaMoedaOuro)

	// O comprador leva a única peça: a partir daqui a prateleira está vazia.
	compra := protocol.LojaCompraBody{Vendedor: barraca, Slot: 0, Moeda: protocol.LojaMoedaOuro}
	send(t, comprador, protocol.MsgLojaCompra, compra.Encode())
	readUntil(t, comprador, protocol.MsgSendItem)

	clock.Add(2 * shopPointsWindowMs)
	time.Sleep(300 * time.Millisecond)
	if p := db.pontosLojinha0(); p != 0 {
		t.Fatalf("barraca vazia rendeu %d pontos; queria 0", p)
	}
}
