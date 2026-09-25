package control

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	gamev1 "github.com/jeanluca/w2pp-openwyd/api/game/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// bancoQueRecusa falha toda gravação de saída, que é o estado em que o dreno não
// pode dizer "pronto".
type bancoQueRecusa struct {
	world.NopPersistence
}

func (bancoQueRecusa) SaveOnShutdown(context.Context, world.CharacterSave, int64, int64, bool) error {
	return errors.New("o banco recusou")
}

func (bancoQueRecusa) SaveCargo(context.Context, world.CargoSave) error {
	return errors.New("o banco recusou")
}

func (bancoQueRecusa) SaveCargoWithDeliveries(context.Context, world.CargoSave, []int64, []int64) error {
	return errors.New("o banco recusou")
}

func (bancoQueRecusa) SalvarPersonagemComCarga(context.Context, world.CharacterSave, world.CargoSave, []int64, []int64, int64, int64, bool) error {
	return errors.New("o banco recusou")
}

// servidorComJogador sobe um mundo de verdade, com uma sessão em jogo e carga
// carregada, sobre a persistência dada.
func servidorComJogador(t *testing.T, p world.Persistence) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pronto := make(chan struct{}, 1)
	h := func(w *world.World, s *world.Session, _ protocol.Header, _ []byte) {
		s.Mode = world.UserPlay
		s.AccountID = 42
		// A carga carregada basta para o teste: a saída grava a carga da conta, e
		// é essa gravação que o banco recusa.
		w.SetCargo(42, &world.CargoState{Coin: 100})
		select {
		case pronto <- struct{}{}:
		default:
		}
	}
	w := world.New(world.Config{GridDim: 16}, log, p, h)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	feito := make(chan struct{})
	go func() { defer close(feito); _ = w.Serve(ctx, ln) }()
	t.Cleanup(func() { cancel(); <-feito })

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	// O INITCODE é obrigatório antes de qualquer quadro.
	var ic [4]byte
	binary.LittleEndian.PutUint32(ic[:], protocol.InitCode)
	if _, err := c.Write(ic[:]); err != nil {
		t.Fatal(err)
	}
	quadro, err := protocol.Encode(protocol.Header{Type: protocol.MsgAction}, nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(quadro); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pronto:
	case <-time.After(3 * time.Second):
		t.Fatal("o jogador não entrou em jogo")
	}

	srv, err := NewServer(w, tokenDeTeste, log, teleporteNulo, Overlays{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

// TestDrenoNaoDizProntoComGravacaoQueFalhou é o contrato da tela do painel.
//
// O dreno esperava as gravações TERMINAREM e respondia "pronto" — mas terminar
// não é ter dado certo. Uma gravação que voltava com erro só virava linha de log,
// e o painel, que promete não reiniciar sem tudo gravado, reiniciava por cima do
// que não foi salvo.
func TestDrenoNaoDizProntoComGravacaoQueFalhou(t *testing.T) {
	s := servidorComJogador(t, bancoQueRecusa{})
	_, err := s.Drain(context.Background(), &gamev1.DrainRequest{})
	if err == nil {
		t.Fatal("o dreno disse pronto com a gravação falhando — o painel reiniciaria por cima")
	}
	if status.Code(err) != codes.Unavailable {
		t.Errorf("código = %v, queria Unavailable", status.Code(err))
	}
}

// TestDrenoDizProntoQuandoGravou é o outro lado: sem falha, o dreno responde e o
// reinício pode seguir.
func TestDrenoDizProntoQuandoGravou(t *testing.T) {
	s := servidorComJogador(t, world.NopPersistence{})
	resp, err := s.Drain(context.Background(), &gamev1.DrainRequest{})
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if resp.GetKicked() == 0 {
		t.Error("o dreno não derrubou ninguém")
	}
}
