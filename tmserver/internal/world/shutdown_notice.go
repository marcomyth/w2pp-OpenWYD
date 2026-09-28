package world

import (
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// shutdownNotice is the line every player sees when the server is going down.
// Encoded for the client's Windows-1252 rendering — a Go literal is UTF-8, and
// its accents would arrive as mojibake (protocol.ClientText).
const shutdownNotice = "O servidor esta sendo reiniciado. Voce sera desconectado em instantes."

// announceShutdown tells every player in-world that the server is stopping,
// before shutdown() saves them and closes their sessions, and reports how many
// it told.
//
// Without it a restart looks identical to a crash or a connection drop: the
// client simply freezes and times out, and the player has no way to tell an
// orderly deploy from something broken.
//
// IT DOES NOT WAIT. It used to sleep Config.ShutdownGrace here, before any save,
// and on 27/09/2026 that sleep was a third of the time the platform gave the
// process: the saves started 2 s after SIGTERM and the process died after the
// first of six (rollback-no-reinicio). The frame still gets its time on the
// wire — shutdown() closes the sockets only once ShutdownGrace has passed since
// this call, with the saves running in the meantime. Loop-only.
func (w *World) announceShutdown() int {
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
	if sent > 0 {
		w.log.Info("shutdown notice sent", "players", sent, "grace", w.cfg.ShutdownGrace)
	}
	return sent
}
