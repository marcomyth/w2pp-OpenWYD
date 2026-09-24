package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// A TRANCA DO AMBIENTE DE TESTE.
//
// O cliente do teste já está na mão de gente, e a conta de qualquer um serve para
// entrar. Sem a tranca, o ambiente de teste vira um segundo servidor aberto sem
// ninguém ter decidido isso.
//
// O QUE ESTES TESTES PRECISAM PROVAR, e a primeira parte importa tanto quanto a
// segunda: SEM a variável, nada muda para ninguém. Uma tranca que vaza para a
// produção derruba o servidor de verdade, e o estrago é pior do que o do problema
// que ela resolve.

// servidorTrancado sobe um servidor com a tranca ligada ou desligada.
func servidorTrancado(t *testing.T, persist world.Persistence, trancado bool) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	d := New(Config{Log: log, CombatRules: regraSemEscala(), AcessoRestrito: trancado})
	w := world.New(world.Config{GridDim: 16}, log, persist, d.Handle)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Serve(ctx, ln); close(done) }()
	return ln.Addr().String(), func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("o servidor nao parou")
		}
	}
}

// leTextoAtePainel lê a próxima mensagem e exige que seja o painel de texto (0x101).
func leTextoAtePainel(t *testing.T, c net.Conn) string {
	t.Helper()
	ty, payload := read(t, c)
	if ty != protocol.MsgMessagePanel {
		t.Fatalf("mensagem = %#x, quero o painel de texto (%#x)", ty, protocol.MsgMessagePanel)
	}
	return strings.TrimRight(string(payload), string(rune(0)))
}

// SEM A VARIÁVEL, O JOGADOR ENTRA COMO SEMPRE.
//
// É o teste que protege a produção. A tranca desligada tem de ser indistinguível de
// ela não existir.
func TestSemATrancaOJogadorEntraComoHoje(t *testing.T) {
	db := newDB()
	addr, stop := servidorTrancado(t, db, false)
	defer stop()

	c := dial(t, addr)
	defer func() { _ = c.Close() }()
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))

	if h := readHeader(t, c); h.Type != protocol.MsgCNFAccountLogin {
		t.Fatalf("resposta = %#x, quero a tela de personagens (%#x)", h.Type, protocol.MsgCNFAccountLogin)
	}
}

// COM A TRANCA, O JOGADOR É RECUSADO — e a mensagem diz POR QUÊ.
//
// Dizer "senha inválida" para quem acertou a senha faz a pessoa passar a tarde
// tentando de novo. O texto tem de falar do servidor, não da conta.
func TestComATrancaOJogadorEhRecusadoComOTexto(t *testing.T) {
	db := newDB()
	addr, stop := servidorTrancado(t, db, true)
	defer stop()

	c := dial(t, addr)
	defer func() { _ = c.Close() }()
	send(t, c, protocol.MsgAccountLogin, loginBody("tester", "secret", protocol.AppVersion))

	texto := leTextoAtePainel(t, c)
	if texto != "Servidor de teste, acesso restrito." {
		t.Fatalf("texto = %q", texto)
	}
	// E a conexão fecha DEPOIS do texto: fechar antes descartaria a mensagem, e a
	// pessoa ficaria olhando uma janela que sumiu sem dizer nada.
	expectClosed(t, c)
}

// E A STAFF ENTRA, que é a razão de o ambiente existir.
func TestComATrancaAStaffEntra(t *testing.T) {
	db := newDB()
	db.accounts["mod"] = &fakeAccount{id: 42, pass: "secret", role: "moderator"}
	addr, stop := servidorTrancado(t, db, true)
	defer stop()

	c := dial(t, addr)
	defer func() { _ = c.Close() }()
	send(t, c, protocol.MsgAccountLogin, loginBody("mod", "secret", protocol.AppVersion))

	if h := readHeader(t, c); h.Type != protocol.MsgCNFAccountLogin {
		t.Fatalf("resposta = %#x; a staff foi recusada no proprio ambiente dela", h.Type)
	}
}

// OS DOIS NÍVEIS DE STAFF PASSAM. O EhStaff existe para esta pergunta não ser
// escrita de três jeitos em três lugares.
func TestQuemEhStaff(t *testing.T) {
	casos := map[string]bool{
		"admin": true, "moderator": true,
		"player": false, "": false, "coisa-nova": false,
	}
	for papel, quer := range casos {
		if got := world.ParseAccess(papel).EhStaff(); got != quer {
			t.Errorf("EhStaff(%q) = %v, quero %v", papel, got, quer)
		}
	}
}
