package world

import (
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// TestSendEchoKeepsTheClientTick pins the one thing that separates an echo from
// every other outbound frame. The original replies to an attack with the client's
// OWN frame, substituting server time only for the SKIPCHECKTICK sentinel
// (_MSG_Attack.cpp:1745), and the client uses that tick to tell the answer to its
// own swing from a bystander's — taking CurrentExp only out of its own. Stamping
// server time here is what left the exp bar frozen while damage still landed.
func TestSendEchoKeepsTheClientTick(t *testing.T) {
	const serverNow = uint32(999)
	w := New(Config{GridDim: 16, Now: func() uint32 { return serverNow }}, slogDiscard(), nil, nil)

	tests := []struct {
		name string
		tick uint32
		want uint32
	}{
		{"a real client tick survives", 1927501224, 1927501224},
		{"the SKIPCHECKTICK sentinel is replaced", SkipCheckTick, serverNow},
		{"an absent tick is replaced", 0, serverNow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Session{Conn: 1, Mode: UserPlay, out: make(chan outFrame, 4), closeCh: make(chan struct{})}
			w.SendEcho(s, protocol.Header{Type: protocol.MsgAttackOne, ID: protocol.IDScene, ClientTick: tc.tick}, nil)
			select {
			case f := <-s.out:
				if f.header.ClientTick != tc.want {
					t.Errorf("ClientTick = %d, want %d", f.header.ClientTick, tc.want)
				}
				if f.header.Type != protocol.MsgAttackOne {
					t.Errorf("Type = %#x, want the frame's own MsgAttackOne", f.header.Type)
				}
			default:
				t.Fatal("SendEcho queued nothing")
			}
		})
	}
}

// TestSendStampsServerTick is the contrast: everything that is not an echo keeps
// the server clock, so SendEcho stays the deliberate exception.
func TestSendStampsServerTick(t *testing.T) {
	const serverNow = uint32(4242)
	w := New(Config{GridDim: 16, Now: func() uint32 { return serverNow }}, slogDiscard(), nil, nil)
	s := &Session{Conn: 1, Mode: UserPlay, out: make(chan outFrame, 4), closeCh: make(chan struct{})}

	w.SendTo(s, protocol.Header{Type: protocol.MsgAttackOne, ClientTick: 1927501224}, nil)
	select {
	case f := <-s.out:
		if f.header.ClientTick != serverNow {
			t.Errorf("ClientTick = %d, want the server clock %d", f.header.ClientTick, serverNow)
		}
	default:
		t.Fatal("SendTo queued nothing")
	}
}
