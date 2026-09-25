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

// carteiraDoBanco é a carteira da conta como ela existe FORA desta sessão: o
// teste mexe nela para imitar o que a recarga pelo site, um ajuste da staff ou
// outra sessão fazem enquanto o jogador está online.
//
// O valor é atômico porque quem escreve é a goroutine do teste e quem lê é a do
// mundo. Um int simples passaria e depois falharia só no -race da CI, que é a
// pior hora para descobrir.
type carteiraDoBanco struct {
	*fakeDB
	saldo atomic.Int32
	erro  atomic.Pointer[error]
}

func (c *carteiraDoBanco) DonateBalance(context.Context, int64) (int32, error) {
	if e := c.erro.Load(); e != nil {
		return 0, *e
	}
	return c.saldo.Load(), nil
}

// servidorComCarteira sobe um mundo cuja carteira o teste controla.
func servidorComCarteira(t *testing.T) (string, func(), *carteiraDoBanco) {
	t.Helper()
	db := newDB()
	db.loadResult = world.CharacterState{
		Slot: 0, Name: "Hero", Level: 50, X: 5, Y: 5, HP: 1000, MaxHP: 1000,
	}
	carteira := &carteiraDoBanco{fakeDB: db}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log})
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

// TestAVitrineMostraOSaldoDoBancoENaoODoLogin.
//
// O defeito que este teste prende: o saldo de Cash era lido UMA VEZ, no login, e
// o rodapé da Loja do Servidor mostrava aquele número até o jogador relogar. Uma
// recarga feita pelo site com o personagem online não aparecia — o jogador pagava,
// via o dinheiro no site, entrava na loja e não tinha com que comprar.
//
// A recarga aqui é escrita direto na carteira, sem passar pela sessão, de
// propósito: é assim que ela acontece de verdade. O site credita a CONTA, no
// banco, e nunca fala com o tmServer.
func TestAVitrineMostraOSaldoDoBancoENaoODoLogin(t *testing.T) {
	addr, stop, carteira := servidorComCarteira(t)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	// Entrou sem nada. Agora a recarga cai na conta, por fora.
	carteira.saldo.Store(4_000)

	lista := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos)
	if lista.Cash != 4_000 {
		t.Errorf("a vitrine mostrou Cash = %d, queria 4000: o rodapé ficou no valor do login "+
			"e a recarga feita com o personagem online não chegou à tela", lista.Cash)
	}
}

// TestAVitrineAbreMesmoComOBancoMudo.
//
// A releitura é uma ida ao banco a mais no caminho de abrir uma tela, então ela
// não pode ser capaz de fechar a tela. Com o banco recusando, o jogador tem de
// continuar vendo a vitrine — com o último saldo conhecido, que é o do login.
//
// Sem esta garantia o conserto trocaria "saldo velho" por "loja que não abre", e
// a segunda é pior: o saldo velho ainda deixa comprar, porque quem decide a
// compra é o banco.
func TestAVitrineAbreMesmoComOBancoMudo(t *testing.T) {
	addr, stop, carteira := servidorComCarteira(t)
	defer stop()
	c := enterWorld(t, addr)
	defer c.Close()

	recusa := error(context.DeadlineExceeded)
	carteira.erro.Store(&recusa)

	lista := pedeVitrine(t, c, 0, protocol.LojaFiltroTodos)
	if lista.Paginas != 1 {
		t.Errorf("a vitrine não abriu com o banco mudo: paginas = %d", lista.Paginas)
	}
}
