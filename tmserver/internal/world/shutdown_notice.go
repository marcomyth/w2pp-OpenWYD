package world

import (
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// shutdownNotice is the line every player sees when the server is going down.
// Encoded for the client's Windows-1252 rendering — a Go literal is UTF-8, and
// its accents would arrive as mojibake (protocol.ClientText).
const shutdownNotice = "O servidor esta sendo reiniciado. Voce sera desconectado em instantes."

// announceShutdown tells every player in-world that the server is stopping,
// before shutdown() drains and closes their sessions.
//
// Without it a restart looks identical to a crash or a connection drop: the
// client simply freezes and times out, and the player has no way to tell an
// orderly deploy from something broken.
//
// Loop-only, and it blocks the loop for Config.ShutdownGrace. That is safe here
// precisely because it is the last thing the loop does — nothing else is waiting
// on it, and the alternative (returning immediately) would close the sockets
// before the frame left the process.
func (w *World) announceShutdown() {
	payload := append(protocol.ClientText(shutdownNotice), 0) // NUL-terminated chat line
	sent := 0
	w.ForEachPlaying(-1, func(s *Session, _ *Entity) {
		// HEADER.ID = the receiver's own conn: the client renders the line as
		// coming from the server rather than from another player, the same shape
		// /gm notice uses.
		w.SendTo(s, protocol.Header{Type: protocol.MsgMessageChat, ID: uint16(s.Conn)}, payload)
		sent++
	})
	if sent == 0 {
		return // nobody in-world: nothing to wait for
	}
	w.log.Info("shutdown notice sent", "players", sent, "grace", w.cfg.ShutdownGrace)
	if w.cfg.ShutdownGrace > 0 {
		time.Sleep(w.cfg.ShutdownGrace)
	}
}
