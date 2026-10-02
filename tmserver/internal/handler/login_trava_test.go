package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// O QUE ESTE TESTE GUARDA: que a trava das três senhas erradas SOLTE SOZINHA.
//
// O DEFEITO, medido em produção: a conta errou a senha três vezes e ficou travada mais
// de 13 horas, mesmo depois de a senha ser trocada. O contador só era apagado num login
// que desse certo, e a trava recusa antes de o pedido chegar ao banco — depois de
// travar, nada mais o apagava. Só reiniciar o servidor soltava.
//
// O PRAZO ESTÁ ESCRITO AQUI EM NÚMERO, E NÃO PELA CONSTANTE DO CÓDIGO, de propósito: é
// a conta do legado — a lista zera a cada 10 passadas do ProcessMinTimer
// (ProcessSecMinTimer.cpp:2645-2646) e a passada é de 12 s (Server.cpp:4087). Um teste
// que lesse a constante passaria igual com ela errada.
const (
	passadaDoLegadoEmTicks  = 12 // TIMER_MIN = 12000 ms, e o tique do mundo é 1 s
	passadasAteSoltarATrava = 10 // MinCounter % 10
	ticksAteSoltarATrava    = passadasAteSoltarATrava * passadaDoLegadoEmTicks
)

// servidorDaTrava é um servidor em que o TESTE anda o relógio do jogo.
//
// O tique de verdade bate de 5 em 5 ms, mas só chama o Tick do jogo quando o teste
// pede, e quantas vezes ele pedir. O Tick continua rodando dentro da goroutine do laço,
// que é a única que pode tocar o Dispatcher — o teste só entrega o número pelo canal.
// Esperar 120 s de parede seria o outro jeito, e ninguém rodaria o teste.
type servidorDaTrava struct {
	addr   string
	avanca chan int
	feito  chan struct{}
}

func sobeServidorDaTrava(t *testing.T, persist world.Persistence) (*servidorDaTrava, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, CombatRules: regraSemEscala()})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)
	sv := &servidorDaTrava{addr: ln.Addr().String(), avanca: make(chan int), feito: make(chan struct{}, 1)}
	w.SetTickHandler(5*time.Millisecond, func(w *world.World) {
		select {
		case n := <-sv.avanca:
			for range n {
				d.Tick(w)
			}
			sv.feito <- struct{}{}
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return sv, func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor nao parou")
		}
	}
}

// anda faz o jogo dar n tiques e só volta quando o laço terminou de dá-los.
func (sv *servidorDaTrava) anda(t *testing.T, n int) {
	t.Helper()
	select {
	case sv.avanca <- n:
	case <-time.After(5 * time.Second):
		t.Fatal("o laco nao pegou o pedido de andar o relogio")
	}
	select {
	case <-sv.feito:
	case <-time.After(30 * time.Second):
		t.Fatalf("o laco nao terminou os %d tiques", n)
	}
}

// erraTresVezes trava a conta e confere que travou: a quarta tentativa, com a senha
// CERTA, é recusada pela trava. É o estado em que a conta de produção ficou.
func erraTresVezes(t *testing.T, c net.Conn, conta, senhaCerta string) {
	t.Helper()
	for range 3 {
		send(t, c, protocol.MsgAccountLogin, loginBody(conta, "errada", protocol.AppVersion))
		esperaRecusaDeLogin(t, c, msgLoginSenha)
	}
	send(t, c, protocol.MsgAccountLogin, loginBody(conta, senhaCerta, protocol.AppVersion))
	esperaRecusaDeLogin(t, c, msgLoginTresErros)
}

// exigeQueEntra manda a senha certa e exige a tela de personagens.
func exigeQueEntra(t *testing.T, c net.Conn, conta, senha string) {
	t.Helper()
	send(t, c, protocol.MsgAccountLogin, loginBody(conta, senha, protocol.AppVersion))
	ty, payload := read(t, c)
	if ty == protocol.MsgCNFAccountLogin {
		return
	}
	if ty == protocol.MsgMessagePanel {
		t.Fatalf("a conta %q continuou recusada depois do prazo, com a senha certa: %q. "+
			"A trava das tres senhas erradas nao soltou, e so reiniciar o servidor solta",
			conta, decodePanel(payload))
	}
	t.Fatalf("a conta %q recebeu %#x, queria a tela de personagens (%#x)",
		conta, ty, protocol.MsgCNFAccountLogin)
}

// TestATravaDeSenhaSoltaSozinha: três erros travam; um tique antes do prazo continua
// travado; no prazo, a senha certa entra.
//
// DUAS CONTAS, porque o que o legado zera é a LISTA INTEIRA e não uma conta: as duas
// travam em momentos diferentes e as duas soltam na mesma limpeza.
func TestATravaDeSenhaSoltaSozinha(t *testing.T) {
	sv, stop := sobeServidorDaTrava(t, newDB())
	defer stop()

	a := dial(t, sv.addr)
	defer func() { _ = a.Close() }()
	erraTresVezes(t, a, "tester", "secret")

	// Um tique antes de o prazo fechar: nada mudou, a senha certa ainda é recusada.
	// A segunda conta trava no meio do caminho, com o relógio já andando.
	sv.anda(t, ticksAteSoltarATrava/2)
	b := dial(t, sv.addr)
	defer func() { _ = b.Close() }()
	erraTresVezes(t, b, "tradeb", "secret")
	sv.anda(t, ticksAteSoltarATrava/2-1)
	send(t, a, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))
	esperaRecusaDeLogin(t, a, msgLoginTresErros)

	// O tique que fecha o prazo. As duas entram, e a segunda ficou travada só metade
	// do período: a limpeza é da lista, e não um prazo contado por conta.
	sv.anda(t, 1)
	exigeQueEntra(t, a, "tester", "secret")
	exigeQueEntra(t, b, "tradeb", "secret")
}

// TestATravaDeSenhaVoltaATravarDepoisDeSoltar: a limpeza não desliga a trava. Quem
// insiste depois dela tem outras três tentativas e trava de novo.
func TestATravaDeSenhaVoltaATravarDepoisDeSoltar(t *testing.T) {
	sv, stop := sobeServidorDaTrava(t, newDB())
	defer stop()

	c := dial(t, sv.addr)
	defer func() { _ = c.Close() }()
	erraTresVezes(t, c, "tester", "secret")
	sv.anda(t, ticksAteSoltarATrava)
	erraTresVezes(t, c, "tester", "secret")
}
