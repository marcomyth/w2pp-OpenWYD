package control

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	gamev1 "github.com/jeanluca/w2pp-openwyd/api/game/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const tokenDeTeste = "segredo-do-painel"

// mundoRodando starts a real world loop with no clients, which is enough to
// prove the loop crossing: every RPC here hands work to the loop and waits for
// it to answer, and that is the part that can deadlock.
func mundoRodando(t *testing.T) *world.World {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := world.New(world.Config{GridDim: 16}, log, world.NopPersistence{}, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return w
}

func servidor(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(mundoRodando(t), tokenDeTeste, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

func TestRecusaSubirSemToken(t *testing.T) {
	// secure.ServerCreds silently returns insecure credentials when TLS is not
	// configured, so "wire it like the other services" would have shipped an
	// open endpoint that kicks every player off the server. Refusing to start is
	// the only failure mode that cannot be missed.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, token := range []string{"", "   "} {
		if _, err := NewServer(nil, token, log); !errors.Is(err, ErrNoToken) {
			t.Errorf("NewServer(%q) = %v, want ErrNoToken", token, err)
		}
	}
}

func TestOInterceptadorExigeOToken(t *testing.T) {
	s := servidor(t)
	inter := s.Interceptor()
	chamou := false
	next := func(context.Context, any) (any, error) { chamou = true; return "ok", nil }

	casos := []struct {
		nome string
		ctx  context.Context
	}{
		{"sem metadata", context.Background()},
		{"sem o cabeçalho", metadata.NewIncomingContext(context.Background(), metadata.MD{})},
		{"token errado", metadata.NewIncomingContext(context.Background(),
			metadata.Pairs(gamev1.TokenHeader, "chute"))},
		{"token quase certo", metadata.NewIncomingContext(context.Background(),
			metadata.Pairs(gamev1.TokenHeader, tokenDeTeste+"x"))},
		{"vazio", metadata.NewIncomingContext(context.Background(),
			metadata.Pairs(gamev1.TokenHeader, ""))},
	}
	for _, c := range casos {
		chamou = false
		_, err := inter(c.ctx, nil, &grpc.UnaryServerInfo{}, next)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: código = %v, want Unauthenticated", c.nome, status.Code(err))
		}
		if chamou {
			t.Errorf("%s: a chamada passou mesmo assim", c.nome)
		}
	}

	// And the right token gets through.
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(gamev1.TokenHeader, tokenDeTeste))
	chamou = false
	if _, err := inter(ctx, nil, &grpc.UnaryServerInfo{}, next); err != nil {
		t.Fatalf("com o token certo: %v", err)
	}
	if !chamou {
		t.Error("o token certo não chegou ao handler")
	}
}

func TestListOnlineAtravessaOLaco(t *testing.T) {
	// With no clients the answer is empty — the point is that it ANSWERS. Every
	// call here hands work to the single-owner loop and blocks on a channel, and
	// getting that wrong deadlocks rather than failing.
	s := servidor(t)
	resp, err := s.ListOnline(context.Background(), &gamev1.ListOnlineRequest{})
	if err != nil {
		t.Fatalf("ListOnline: %v", err)
	}
	if resp.GetConnected() != 0 || resp.GetInPlay() != 0 || len(resp.GetPlayers()) != 0 {
		t.Errorf("mundo vazio respondeu %+v, want tudo zerado", resp)
	}
}

func TestKickDeContaAusenteNaoEErro(t *testing.T) {
	// Zero sessions is the answer, not a failure: the moderator asked whether
	// anyone was connected on that account and the answer is no.
	s := servidor(t)
	resp, err := s.Kick(context.Background(), &gamev1.KickRequest{AccountName: "ninguem"})
	if err != nil {
		t.Fatalf("Kick: %v", err)
	}
	if resp.GetSessions() != 0 {
		t.Errorf("sessões = %d, want 0", resp.GetSessions())
	}
}

func TestKickExigeUmaConta(t *testing.T) {
	s := servidor(t)
	for _, nome := range []string{"", "   "} {
		if _, err := s.Kick(context.Background(), &gamev1.KickRequest{AccountName: nome}); status.Code(err) != codes.InvalidArgument {
			t.Errorf("Kick(%q) = %v, want InvalidArgument", nome, status.Code(err))
		}
	}
}

func TestBroadcastRecusaOQueOClienteNaoCarrega(t *testing.T) {
	// The wire body is a fixed 96 bytes and the encoder truncates silently, so a
	// longer notice would arrive cut with nothing to say it had been.
	s := servidor(t)
	longa := strings.Repeat("a", maxBroadcast+1)
	err := func() error {
		_, e := s.Broadcast(context.Background(), &gamev1.BroadcastRequest{Message: longa})
		return e
	}()
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("mensagem longa = %v, want InvalidArgument", status.Code(err))
	}
	if !strings.Contains(err.Error(), "carries") {
		t.Errorf("a mensagem não diz qual é o limite: %v", err)
	}

	for _, m := range []string{"", "   "} {
		if _, e := s.Broadcast(context.Background(), &gamev1.BroadcastRequest{Message: m}); status.Code(e) != codes.InvalidArgument {
			t.Errorf("Broadcast(%q) = %v, want InvalidArgument", m, status.Code(e))
		}
	}
}

func TestBroadcastNoLimiteEAceito(t *testing.T) {
	s := servidor(t)
	resp, err := s.Broadcast(context.Background(), &gamev1.BroadcastRequest{
		Message: strings.Repeat("a", maxBroadcast),
	})
	if err != nil {
		t.Fatalf("mensagem no limite exato: %v", err)
	}
	if resp.GetRecipients() != 0 {
		t.Errorf("destinatários = %d num mundo vazio, want 0", resp.GetRecipients())
	}
}

func TestChamadaDesisteQuandoOLacoNaoResponde(t *testing.T) {
	// A wedged loop must fail the call rather than hold the connection open. The
	// world here is never Served, so nothing drains the callback queue.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	parado := world.New(world.Config{GridDim: 16}, log, world.NopPersistence{}, nil)
	s, err := NewServer(parado, tokenDeTeste, log)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := s.ListOnline(ctx, &gamev1.ListOnlineRequest{}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("laço parado = %v, want DeadlineExceeded", status.Code(err))
	}
}
