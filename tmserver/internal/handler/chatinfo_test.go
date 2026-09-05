package handler

import (
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// Each of these used to answer "O jogador não está conectado.", because the
// keyword fell through to the whisper delivery path. The assertion that matters
// is therefore as much about what does NOT come back as about the text.
func TestChatInfoCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		want string
	}{
		{"online", "Somos 1 jogadore(s) online."},
		{"day", "!#11  2"},
	}
	for _, c := range cases {
		t.Run(c.cmd, func(t *testing.T) {
			addr, stop, _ := startServerClock(t, newDB())
			defer stop()
			conn := enterWorld(t, addr)
			defer conn.Close()

			whisperFrame(t, conn, c.cmd, "")
			got := decodePanel(expect(t, conn, protocol.MsgMessagePanel))
			if got != c.want {
				t.Errorf("/%s = %q, want %q", c.cmd, got, c.want)
			}
		})
	}
}

// /time carries the clock, so the test pins the shape rather than the value:
// the legacy strftime is "%d-%m-%Y %H:%M:%S".
func TestChatInfoTime(t *testing.T) {
	addr, stop, _ := startServerClock(t, newDB())
	defer stop()
	conn := enterWorld(t, addr)
	defer conn.Close()

	whisperFrame(t, conn, "time", "")
	got := decodePanel(expect(t, conn, protocol.MsgMessagePanel))
	if len(got) != len("02-01-2006 15:04:05") {
		t.Fatalf("/time = %q, want dd-mm-yyyy hh:mm:ss", got)
	}
	if strings.Count(got, "-") != 2 || strings.Count(got, ":") != 2 {
		t.Errorf("/time = %q, want dd-mm-yyyy hh:mm:ss", got)
	}
}
