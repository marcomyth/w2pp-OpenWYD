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
	// SendNotice (SendFunc.cpp:139): the message panel, to everyone in world.
	// HEADER.ID is ZERO — the id names who said the line, so the receiver's own
	// conn made the client attribute the restart warning to the player reading it
	// ("[Nick]> O servidor esta sendo reiniciado"). Zero means the server.
	payload := protocol.EncodeMessagePanelBody(shutdownNotice)
	sent := 0
	w.ForEachPlaying(-1, func(s *Session, _ *Entity) {
		w.SendTo(s, protocol.Header{Type: protocol.MsgMessagePanel, ID: 0}, payload)
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
