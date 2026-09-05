package handler

import (
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// Server-wide announcements for the things worth interrupting everyone for: a
// rebirth, an Ancient item, a high refine.
//
// DELIBERATE DIVERGENCE: the legacy announces guild wars, the Kefra kill and
// event drops (SendNotice, MobKilled.cpp:1480 and CWarTower.cpp), but nothing
// for these four. They are announced here because on a live server they are the
// events players actually gather around, and a rebirth nobody sees is a rebirth
// that may as well not have happened.
//
// They ride SendNotice — the message panel to every player in world — so they
// arrive as the server speaking, not as a player talking (HEADER.ID zero, see
// sendClientMessage).

// anctFamilyName is the CombineFamily name of the Ancient combine, shared so the
// announcement and the family definition cannot drift apart.
const anctFamilyName = "Anct"

// announceRefineLevel is the refine level from which a success is worth telling
// the server about. Below it the announcements would be constant noise: +9 and
// under are routine.
const announceRefineLevel = 10

// announceArch says a Mortal completed the Arch rebirth.
func (d *Dispatcher) announceArch(w *world.World, name string) {
	if name == "" {
		return
	}
	broadcastNotice(w, fmt.Sprintf("[EVENTO] %s renasceu como Arch! Parabéns!", name))
	d.log.Info("announce arch", "name", name)
}

// announceCelestial says an Arch ascended to Celestial.
func (d *Dispatcher) announceCelestial(w *world.World, name string) {
	if name == "" {
		return
	}
	broadcastNotice(w, fmt.Sprintf("[EVENTO] %s ascendeu a Celestial! Parabéns!", name))
	d.log.Info("announce celestial", "name", name)
}

// announceAncient says a player pulled an Ancient item out of a combine.
func (d *Dispatcher) announceAncient(w *world.World, name string, item int16) {
	if name == "" {
		return
	}
	broadcastNotice(w, fmt.Sprintf("[EVENTO] %s conseguiu um item ANCIENTE: %s!", name, d.itemName(item)))
	d.log.Info("announce ancient", "name", name, "item", item)
}

// announceRefine says a player took an item to +10 or beyond.
func (d *Dispatcher) announceRefine(w *world.World, name string, item int16, level int) {
	if name == "" || level < announceRefineLevel {
		return
	}
	broadcastNotice(w, fmt.Sprintf("[EVENTO] %s levou %s ao +%d!", name, d.itemName(item), level))
	d.log.Info("announce refine", "name", name, "item", item, "level", level)
}
