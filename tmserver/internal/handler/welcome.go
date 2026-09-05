package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The login greeting: who you are, and what the server's rates are right now.
//
// This has NO legacy counterpart — the original tells a player nothing on entry,
// and whether an EXP event is running is something you could only infer by
// grinding and noticing. That is the reason for the divergence: the rates are
// server-wide switches an operator flips (-double-exp, -newbie-event,
// -kefra-live), and a player who cannot see them cannot tell a bonus weekend
// from a normal one.
//
// It goes out as MSG_MessageBoxOk rather than the message panel because the
// panel shares the chat area and scrolls away behind the first few lines of
// chatter; the box stays until the player dismisses it.

// expMultiplierText renders the PvE experience multiplier now in effect.
//
// The factors are the ones SoloExpReward actually applies, in its order: the
// newbie event's +25% (which only reaches characters under 100, so it is
// reported as "até 100"), DoubleMode's ×2, and the Kefra halving. The ±15%
// newbie swing is deliberately left out: it is noise around the mean, not a rate
// anyone can plan around.
func expMultiplierText(ev expEventsView) string {
	mult := 1.0
	if ev.DoubleMode {
		mult *= 2
	}
	// KefraLive false is the legacy default and halves PvE exp: the flag reads as
	// "the Kefra has been put down", not "a Kefra is walking around"
	// (_MSG_MessageWhisper.cpp:837 sends "ainda esta vivo" when it is 0).
	if !ev.KefraLive {
		mult /= 2
	}
	if mult == float64(int(mult)) {
		return fmt.Sprintf("%dx", int(mult))
	}
	return fmt.Sprintf("%.1fx", mult)
}

// expEventsView is the subset of level.ExpEvents this file reads, so the text
// builders can be tested without standing up a dispatcher.
type expEventsView struct {
	DoubleMode  bool
	NewbieEvent bool
	KefraLive   bool
}

// kefraText reports the Kefra event in the words the command uses.
func kefraText(kefraLive bool) string {
	if kefraLive {
		return "Kefra: derrotado"
	}
	return "Kefra: vivo"
}

// welcomeText builds the greeting as ONE line, because the box carries one
// string: MESSAGE_LENGTH is 96 bytes, and name (12) plus the rates leaves room
// to spare. It is split from the sending so the wording is testable.
func welcomeText(name string, ev expEventsView) string {
	line := fmt.Sprintf("Bem vindo de volta, %s! %s | EXP: %s",
		name, kefraText(ev.KefraLive), expMultiplierText(ev))
	if ev.NewbieEvent {
		line += " (+25% ate 100)"
	}
	return line
}

// sendWelcome greets a character that just entered the world.
func (d *Dispatcher) sendWelcome(w *world.World, s *world.Session) {
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	ev := expEventsView{
		DoubleMode:  d.expEvents.DoubleMode,
		NewbieEvent: d.expEvents.NewbieEvent,
		KefraLive:   d.expEvents.KefraLive,
	}
	// The message panel, NOT MSG_MessageBoxOk.
	//
	// The box was the obvious fit — it stays until dismissed, where the panel
	// scrolls away with chat — and its layout is in Basedef.h:1535, so the frame
	// was correct. The client disagreed: a capture shows 25 MALFORMED boxes (the
	// 4-byte notify placeholder) passing harmlessly through a 594-frame session,
	// and then a single WELL-FORMED one as the last frame before the client closed
	// the socket. It ignores the frame it cannot parse and dies on the one it can,
	// which points at what the box DOES — a modal, raised while the world is still
	// being built — rather than at the bytes.
	//
	// So the greeting rides the channel that demonstrably works. Making it stay on
	// screen needs a different mechanism, and one confirmed against the client
	// before it goes near the login path again.
	sendClientMessage(w, s, welcomeText(e.Name, ev))
}
