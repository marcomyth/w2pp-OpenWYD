package handler

import (
	"strings"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// gmInvisible toggles the caller's invisibility:
//
//	/gm invisivel          alterna
//	/gm invisivel on|off   força
//
// "snoop" is accepted as an alias because that is the word staff remember from
// the legacy "+snoop" (imple.cpp:1567). The hiding itself is World.hiddenFrom;
// this only flips the flag and squares the clients already looking at the GM,
// because the filter works on frames yet to be sent: a player who saw the GM
// before the switch keeps the model on screen until told to drop it, and one
// who comes into view while the GM is hidden never got a CreateMob to show it
// once the GM reappears.
func (d *Dispatcher) gmInvisible(w *world.World, s *world.Session, rest string) {
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	on := !e.GMInvisible
	switch strings.ToLower(firstToken(rest)) {
	case "on", "ligar", "1":
		on = true
	case "off", "desligar", "0":
		on = false
	}
	if on == e.GMInvisible {
		sendClientMessage(w, s, invisibleStatus(on))
		return
	}

	if on {
		// RemoveMob BEFORE the flag goes up: after it, hiddenFrom would swallow the
		// removal itself. Staff are skipped — they keep seeing the GM.
		rm := protocol.EncodeRemoveMobBody(0)
		w.ForEachInView(s.Conn, func(vs *world.Session, _ *world.Entity) {
			if vs.AccessLevel >= world.AccessModerator {
				return
			}
			w.SendTo(vs, protocol.Header{Type: protocol.MsgRemoveMob, ID: uint16(s.Conn)}, rm)
			w.UnmarkSeen(vs, s.Conn)
		})
		e.GMInvisible = true
	} else {
		e.GMInvisible = false
		// Announced to everyone in view whatever their view set says: while hidden,
		// the movement multicast still marked the GM as seen for players it walked
		// past, although the CreateMob it queued never left.
		ty, body := createMobViewPacket(w, e, 0)
		pk := protocol.EncodeStandardParm(pkInfoParm(e))
		w.ForEachInView(s.Conn, func(vs *world.Session, _ *world.Entity) {
			if vs.AccessLevel >= world.AccessModerator {
				return
			}
			w.MarkSeen(vs, s.Conn)
			w.SendTo(vs, protocol.Header{Type: ty, ID: protocol.IDScene}, body)
			w.SendTo(vs, protocol.Header{Type: protocol.MsgPKInfo, ID: uint16(s.Conn)}, pk)
		})
	}
	d.log.Info("gm invisible", "account", s.AccountName, "name", e.Name, "on", on,
		"x", e.X, "y", e.Y)
	sendClientMessage(w, s, invisibleStatus(on))
}

func invisibleStatus(on bool) string {
	if on {
		return "Invisível: LIGADO. Só a equipe vê você."
	}
	return "Invisível: DESLIGADO."
}
