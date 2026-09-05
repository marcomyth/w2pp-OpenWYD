package handler

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

func TestWelcomeText(t *testing.T) {
	cases := []struct {
		name string
		who  string
		ev   expEventsView
		want string
	}{
		{
			// The legacy default: KefraLive=0 halves PvE exp, and the command calls
			// that state "ainda esta vivo" (_MSG_MessageWhisper.cpp:837).
			name: "legacy default",
			who:  "TestPErg",
			ev:   expEventsView{},
			want: "Bem vindo de volta, TestPErg! Kefra: vivo | EXP: 0.5x",
		},
		{
			name: "kefra down",
			who:  "TestPErg",
			ev:   expEventsView{KefraLive: true},
			want: "Bem vindo de volta, TestPErg! Kefra: derrotado | EXP: 1x",
		},
		{
			name: "double exp weekend",
			who:  "Hero",
			ev:   expEventsView{KefraLive: true, DoubleMode: true},
			want: "Bem vindo de volta, Hero! Kefra: derrotado | EXP: 2x",
		},
		{
			// Double and the Kefra halving cancel out — worth pinning, because a
			// player told "2x" while the halving is on would be told a lie.
			name: "double cancelled by kefra",
			who:  "Hero",
			ev:   expEventsView{DoubleMode: true},
			want: "Bem vindo de volta, Hero! Kefra: vivo | EXP: 1x",
		},
		{
			name: "newbie event",
			who:  "Hero",
			ev:   expEventsView{KefraLive: true, NewbieEvent: true},
			want: "Bem vindo de volta, Hero! Kefra: derrotado | EXP: 1x (+25% ate 100)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := welcomeText(c.who, c.ev); got != c.want {
				t.Errorf("welcomeText = %q, want %q", got, c.want)
			}
		})
	}
}

// The greeting must survive the wire: MESSAGE_LENGTH is 96 bytes, and a 12-char
// name with every event on is the worst case. If this ever truncates, the rates
// are what gets cut — the half the player actually needs.
func TestWelcomeTextFitsTheBox(t *testing.T) {
	longest := welcomeText("Abcdefghijkl", expEventsView{NewbieEvent: true, DoubleMode: true})
	if n := len(protocol.ClientText(longest)); n > protocol.MessageLength-1 {
		t.Errorf("greeting encodes to %d bytes, over the %d the box carries: %q",
			n, protocol.MessageLength-1, longest)
	}
}

func TestEncodeMessageBoxBody(t *testing.T) {
	body := protocol.EncodeMessageBoxBody("Bem vindo de volta, Hero!")
	// Two unused ints, then the text (Basedef.h:1535). A short body is rejected
	// outright by the size check, so the length is the load-bearing part.
	if len(body) != 8+protocol.MessageLength {
		t.Fatalf("body is %d bytes, want %d", len(body), 8+protocol.MessageLength)
	}
	for i := 0; i < 8; i++ {
		if body[i] != 0 {
			t.Errorf("Useless%d byte %d = %d, want 0", i/4+1, i, body[i])
		}
	}
	if got := protocol.FromClientText(body[8 : strings.IndexByte(string(body[8:]), 0)+8]); got != "Bem vindo de volta, Hero!" {
		t.Errorf("text = %q", got)
	}
}
