package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The client sends some of its own key bindings as whispers, the same way the
// slash commands arrive (_MSG_MessageWhisper.cpp). A keyword the server does not
// know falls through to the delivery path and answers "O jogador não está
// conectado." — which is how pressing a key produced a message about a player
// nobody named, twice on every login.
//
// These four are the read-only informational ones. They change nothing, so the
// cost of implementing them is small next to leaving the client talking to a
// server that answers every question with "no such player".

// showTime is /time (_MSG_MessageWhisper.cpp:1057): the server's wall clock, in
// the legacy's own strftime layout.
func (d *Dispatcher) showTime(w *world.World, s *world.Session) {
	sendClientMessage(w, s, d.now().Format("02-01-2006 15:04:05"))
}

// showOnline is /online (:611): how many players are in world. The legacy counts
// USER_PLAY sessions and words it exactly like this, plural "s" and all.
func (d *Dispatcher) showOnline(w *world.World, s *world.Session) {
	n := 0
	w.ForEachPlaying(-1, func(*world.Session, *world.Entity) { n++ })
	sendClientMessage(w, s, fmt.Sprintf("Somos %d jogadore(s) online.", n))
}

// showDay is /day (:603). The legacy answers with the literal "!#11  2" — not a
// sentence but a directive the client parses, which is why it is copied verbatim
// rather than translated into something readable.
func (d *Dispatcher) showDay(w *world.World, s *world.Session) {
	sendClientMessage(w, s, "!#11  2")
}
