package handler

import (
	"strconv"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// sendSay is SendSay (SendFunc.cpp:1779): an NPC speaking, to everyone standing
// near it.
//
// It is MSG_MessageChat with HEADER.ID set to the NPC's id, which completes the
// id rule the rest of this port follows: zero is the server, a player's conn is
// that player, and a mob's id is that mob. The client resolves the id to a name
// and labels the line with it, which is why a quest NPC refusing you says
// "Coveiro> ..." rather than appearing to come from nowhere.
//
// Without this, every NPC refusal was silent: the quest handlers either fell
// back to a notice with no text or did nothing at all, so clicking a quest NPC
// you did not qualify for looked identical to clicking a decoration.
func sendSay(w *world.World, npc *world.Entity, text string) {
	if npc == nil || text == "" {
		return
	}
	payload := protocol.EncodeChatBody(text)
	// GridMulticast in the legacy: the NPC talks out loud, so bystanders hear it
	// too. exclude=-1 keeps the player who triggered it in the audience.
	w.ForEachInViewAt(npc.X, npc.Y, -1, func(s *world.Session, _ *world.Entity) {
		w.SendTo(s, protocol.Header{Type: protocol.MsgMessageChat, ID: uint16(npc.ID)}, payload)
	})
}

// say makes an NPC speak a Language.txt line, falling back to a compiled literal
// when the content tree is not mounted — the same contract noticeLine follows,
// so an unmounted server still talks.
func (d *Dispatcher) say(w *world.World, npc *world.Entity, key, fallback string) {
	text, ok := d.lang.Text(key)
	if !ok || formatVerb.MatchString(text) {
		text = fallback
	}
	sendSay(w, npc, text)
}

// itemName resolves an item index to its catalog name for NPC dialogue. Falls
// back to the index so a line missing its catalog still says something specific
// enough to act on.
func (d *Dispatcher) itemName(index int16) string {
	if n, ok := d.itemNames[int(index)]; ok && n != "" {
		return n
	}
	return "#" + strconv.Itoa(int(index))
}
