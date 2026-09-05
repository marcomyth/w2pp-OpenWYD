package handler

import (
	"context"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Character presence (character.online_since, 0024_admin_editor).
//
// Nothing in the game reads these marks. They exist for the staff panel, which
// must know whether the database is still the authority for a character's items:
// while someone is in-play the loop owns their inventory and rewrites it whole on
// every save, so an edit written underneath them is silently lost. The panel
// refuses to edit a character marked in-play, and the mark is what makes that
// refusal possible.
//
// Because it is bookkeeping and not gameplay, every failure here is logged and
// swallowed: a database that cannot record presence must not be able to block a
// login or hold up a disconnect.

// markPresence records that a character entered or left play. It runs off the
// loop (World.GoDetached) — the caller is inside the loop goroutine and must not
// wait on the database.
func (d *Dispatcher) markPresence(w *world.World, name string, online bool) {
	if name == "" {
		return
	}
	p := w.Persistence()
	w.GoDetached(func() func(*world.World) {
		if err := p.SetCharacterPresence(context.Background(), name, online); err != nil {
			return func(*world.World) {
				d.log.Warn("presence mark failed", "character", name, "online", online, "err", err)
			}
		}
		return nil
	})
}
