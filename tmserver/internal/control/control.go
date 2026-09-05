// Package control is the tmServer's inbound control surface — the first one it
// has ever had. Before this the game server listened only on the client port,
// so nothing could ask it who was playing or tell it to do anything.
//
// It exists for the admin panel: see who is connected, kick an account, send a
// global notice.
//
// Two properties shape everything here.
//
// The world is a single-owner loop. Every operation crosses into it through
// World.GoDetached and waits for the loop to answer on a channel, so nothing in
// this package ever touches world state from its own goroutine. The callbacks
// queue is drained ahead of player input, which is why these are one-shot calls
// and why the panel must not poll them on a timer.
//
// The tmServer cannot authorize a moderator. It has no database and no way to
// read account.role, so it does not try: the listener refuses to start without a
// shared token, and whoever presents it is treated as the panel. The panel is
// where the role was checked and where the audit row was written.
package control

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	gamev1 "github.com/jeanluca/w2pp-openwyd/api/game/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// TokenHeader carries the shared secret. Lower-case because gRPC metadata keys
// are normalised that way and a capitalised constant would silently never match.
const TokenHeader = "x-w2pp-control-token"

// loopTimeout bounds how long a call waits for the game loop to answer. The loop
// drains callbacks first, so it should be immediate; the bound exists so a
// wedged loop fails the call instead of holding the connection open forever.
const loopTimeout = 5 * time.Second

// ErrNoToken is returned by NewServer when no token was configured. It is an
// error rather than a warning on purpose: secure.ServerCreds silently falls back
// to insecure credentials when TLS is unconfigured, so "wire it like the other
// services" would otherwise ship an unauthenticated kick endpoint.
var ErrNoToken = errors.New("control: refusing to serve without a token")

// Server implements gamev1.GameControlServiceServer over a running world.
type Server struct {
	gamev1.UnimplementedGameControlServiceServer
	world *world.World
	token string
	log   *slog.Logger
}

// NewServer builds the control service. It fails when the token is empty.
func NewServer(w *world.World, token string, log *slog.Logger) (*Server, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrNoToken
	}
	return &Server{world: w, token: token, log: log}, nil
}

// Interceptor authenticates every call against the shared token.
func (s *Server) Interceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		vals := md.Get(TokenHeader)
		// Constant time, and only after the length is known to match: comparing
		// with == would leak the token one byte at a time to a caller who can
		// measure, and this endpoint can kick every player off the server.
		if len(vals) != 1 || subtle.ConstantTimeCompare([]byte(vals[0]), []byte(s.token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "bad control token")
		}
		return handler(ctx, req)
	}
}

// noLoop runs fn inside the game loop and returns its value.
//
// GoDetached runs the outer function off the loop and applies the returned
// callback inside it, so the callback is where the world may be touched. The
// channel is buffered so the loop never blocks on a caller that has already
// given up.
func noLoop[T any](ctx context.Context, w *world.World, fn func(*world.World) T) (T, error) {
	ch := make(chan T, 1)
	w.GoDetached(func() func(*world.World) {
		return func(w *world.World) { ch <- fn(w) }
	})
	ctx, cancel := context.WithTimeout(ctx, loopTimeout)
	defer cancel()
	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		var zero T
		return zero, status.Error(codes.DeadlineExceeded, "the game loop did not answer")
	}
}

// ListOnline reports the sessions the server holds.
func (s *Server) ListOnline(ctx context.Context, _ *gamev1.ListOnlineRequest) (*gamev1.ListOnlineResponse, error) {
	out, err := noLoop(ctx, s.world, func(w *world.World) *gamev1.ListOnlineResponse {
		resp := &gamev1.ListOnlineResponse{}
		w.ForEachSession(func(sess *world.Session, e *world.Entity) {
			p := &gamev1.OnlinePlayer{
				AccountName: sess.AccountName,
				Ip:          sess.IP,
				InPlay:      e != nil,
			}
			if e != nil {
				p.CharacterName = e.Name
				p.Level = e.Level
				p.MapX, p.MapY = int32(e.X), int32(e.Y)
				resp.InPlay++
			}
			resp.Connected++
			resp.Players = append(resp.Players, p)
		})
		return resp
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Kick ends every session of one account.
//
// By account name, not character: the panel works in accounts, and a player
// still on the character screen has no character name to match on. That is
// exactly how the in-game /gm ban misses people — it resolves through the
// entity name and only for sessions already in play.
func (s *Server) Kick(ctx context.Context, req *gamev1.KickRequest) (*gamev1.KickResponse, error) {
	nome := strings.TrimSpace(req.GetAccountName())
	if nome == "" {
		return nil, status.Error(codes.InvalidArgument, "account name is required")
	}
	n, err := noLoop(ctx, s.world, func(w *world.World) int32 {
		var alvos []*world.Session
		w.ForEachSession(func(sess *world.Session, _ *world.Entity) {
			if strings.EqualFold(sess.AccountName, nome) {
				alvos = append(alvos, sess)
			}
		})
		// Collected first, closed after: Close mutates the session table that
		// ForEachSession is walking.
		for _, sess := range alvos {
			w.Close(sess)
		}
		return int32(len(alvos))
	})
	if err != nil {
		return nil, err
	}
	if n > 0 {
		s.log.Info("control: account kicked", "account", nome, "sessions", n)
	}
	return &gamev1.KickResponse{Sessions: n}, nil
}

// maxBroadcast bounds the notice. The wire body is a fixed 96 bytes and the
// encoder truncates silently, so a longer message would arrive cut with no sign
// that it had been.
const maxBroadcast = protocol.MessageLength - 1

// Broadcast sends a notice to everyone in play.
func (s *Server) Broadcast(ctx context.Context, req *gamev1.BroadcastRequest) (*gamev1.BroadcastResponse, error) {
	msg := strings.TrimSpace(req.GetMessage())
	if msg == "" {
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}
	if len(msg) > maxBroadcast {
		return nil, status.Errorf(codes.InvalidArgument,
			"message is %d bytes; the client carries %d", len(msg), maxBroadcast)
	}
	n, err := noLoop(ctx, s.world, func(w *world.World) int32 {
		// The same encoder and the same ID the three existing broadcasts use
		// (towerNotice, broadcastWorldEventNotice, noticeArea). A fourth spelling
		// of the same packet is how they drift apart.
		body := protocol.EncodeMessageChatBody(msg)
		var n int32
		w.ForEachPlaying(-1, func(sess *world.Session, _ *world.Entity) {
			w.SendTo(sess, protocol.Header{Type: protocol.MsgMessageChat, ID: 0}, body)
			n++
		})
		return n
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("control: broadcast sent", "recipients", n)
	return &gamev1.BroadcastResponse{Recipients: n}, nil
}
