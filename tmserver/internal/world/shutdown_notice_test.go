package world

import (
	"strings"
	"testing"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// The notice must reach the client as single-byte Windows-1252, not UTF-8, or it
// renders as mojibake. Keeping the literal accent-free is the simplest guarantee,
// so this pins that it stays that way.
func TestShutdownNoticeIsClientSafe(t *testing.T) {
	encoded := protocol.ClientText(shutdownNotice)
	if len(encoded) != len(shutdownNotice) {
		t.Errorf("notice changes length when encoded (%d → %d): it carries characters outside the client's codepage",
			len(shutdownNotice), len(encoded))
	}
	if strings.ContainsRune(shutdownNotice, '?') {
		t.Error("notice contains '?', which is what ClientText emits for an unmappable rune")
	}
	// A chat line is NUL-terminated and must fit the client's message buffer.
	if len(encoded)+1 > protocol.MessageLength {
		t.Errorf("notice is %d bytes, over the %d the client reads", len(encoded)+1, protocol.MessageLength)
	}
}

// announceShutdown must not block when nobody is in-world: a restart on an empty
// server should not pay the grace period. The same zero-cost path is what keeps
// the test suites fast, since Config.ShutdownGrace defaults to zero.
func TestAnnounceShutdownNoPlayersReturnsImmediately(t *testing.T) {
	w := New(Config{GridDim: 16, ShutdownGrace: time.Minute}, slogDiscard(), nil, nil)
	done := make(chan struct{})
	go func() { w.announceShutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("announceShutdown waited the grace period with no players connected")
	}
}
