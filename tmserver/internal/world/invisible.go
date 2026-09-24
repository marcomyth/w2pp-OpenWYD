package world

import (
	"encoding/binary"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// hiddenFrom reports whether frame (h, payload) must NOT reach session s because
// it is about a GM who turned invisible (Entity.GMInvisible, "/gm invisivel").
//
// The legacy "+snoop" (imple.cpp:1567) only set MSV_SNOOP in MOB.Merchant, and
// the server read that bit in exactly one place: mob aggro (CMob.cpp:340,
// Server.cpp:5027). Nothing stopped GetCreateMob from announcing the GM, so every
// player in view still saw them. Hiding a character from other clients is new
// here, and it is done at the one place every S→C frame passes through, rather
// than in each broadcast: a GM walking, swinging, drinking or changing gear all
// reach other clients through a different sender, and a filter in one of them
// only would leave the GM visible through the others.
//
// A frame is "about" an entity when HEADER.ID names it (broadcasts carry the
// source there), except CreateMob/CreateMobTrade, which ride IDScene and carry
// the entity in MobID (body @4).
//
// Staff see each other: another moderator or admin is never filtered, which is
// what lets a second GM find the first.
func (w *World) hiddenFrom(s *Session, h protocol.Header, payload []byte) bool {
	if s.AccessLevel >= AccessModerator {
		return false
	}
	id := int(h.ID)
	if (h.Type == protocol.MsgCreateMob || h.Type == protocol.MsgCreateMobTrade) && len(payload) >= 6 {
		id = int(binary.LittleEndian.Uint16(payload[4:6]))
	}
	if id == s.Conn || id <= 0 || id >= MaxUser {
		return false
	}
	e := w.entities[id]
	if e == nil || !e.GMInvisible {
		return false
	}
	return !reachesPastInvisibility(h.Type)
}

// reachesPastInvisibility lists the frames that name the GM in HEADER.ID but are
// not about where the GM stands: a whisper the GM sends, and the party/guild
// bookkeeping between the GM and a member. Dropping them would not hide the GM
// any better — the recipient already knows the GM by name — and would only break
// talking to a player while invisible.
func reachesPastInvisibility(t protocol.Type) bool {
	switch t {
	case protocol.MsgMessageWhisper,
		protocol.MsgSendReqParty, protocol.MsgAcceptParty, protocol.MsgRemoveParty, protocol.MsgCNFAddParty,
		protocol.MsgInviteGuild, protocol.MsgGuildAlly:
		return true
	}
	return false
}
