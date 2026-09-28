package handler

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

type rawFrame struct {
	h protocol.Header
	p []byte
}

// collectRaw reads every frame until the stream goes quiet, visibility frames
// included — they are what this feature is about.
func collectRaw(t *testing.T, c net.Conn) []rawFrame {
	t.Helper()
	var out []rawFrame
	for {
		h, p, ok := readMaybeHeaderRaw(t, c)
		if !ok {
			return out
		}
		out = append(out, rawFrame{h, p})
	}
}

// dialAndLoginRaw is enterWorldAs without drainLoginScore, which reads through
// readMaybe and would throw away the login's CreateMob frames.
func dialAndLoginRaw(t *testing.T, addr, account string) net.Conn {
	t.Helper()
	c := dial(t, addr)
	send(t, c, protocol.MsgAccountLogin, loginBody(account, "secret", protocol.AppVersion))
	if ty, _ := read(t, c); ty != protocol.MsgCNFAccountLogin {
		t.Fatalf("login %s failed: %#x", account, ty)
	}
	var body protocol.MsgCharacterLoginBody
	send(t, c, protocol.MsgCharacterLogin, body.Encode())
	if ty, _ := read(t, c); ty != protocol.MsgCNFCharacterLogin {
		t.Fatalf("char login %s failed: %#x", account, ty)
	}
	return c
}

// about returns the entity a frame describes, the same way World.hiddenFrom
// reads it: MobID for CreateMob/CreateMobTrade, HEADER.ID for the rest.
func (f rawFrame) about() int {
	if (f.h.Type == protocol.MsgCreateMob || f.h.Type == protocol.MsgCreateMobTrade) && len(f.p) >= 6 {
		return int(binary.LittleEndian.Uint16(f.p[4:6]))
	}
	return int(f.h.ID)
}

func hasFrame(fs []rawFrame, ty protocol.Type, id int) bool {
	for _, f := range fs {
		if f.h.Type == ty && f.about() == id {
			return true
		}
	}
	return false
}

func anyAbout(fs []rawFrame, id int) (rawFrame, bool) {
	for _, f := range fs {
		if f.about() == id {
			return f, true
		}
	}
	return rawFrame{}, false
}

// TestGMInvisibleHidesFromPlayers walks the whole switch with one player and a
// second GM in view — the second GM is the case that failed in game, where every
// account in the room was admin and the first version let staff see through it. Every "nothing arrives" below has a positive control right
// before it — the same frame reaching someone — so a filter that swallowed
// everything, or a test that listened on the wrong connection, would fail.
func TestGMInvisibleHidesFromPlayers(t *testing.T) {
	addr, stop, _ := startServerClock(t, gmDB())
	defer stop()
	mod := enterWorldAs(t, addr, "mod") // conn 1
	defer mod.Close()
	player := enterWorldAs(t, addr, "player") // conn 2
	defer player.Close()
	mod2 := enterWorldAs(t, addr, "mod2") // conn 3
	defer mod2.Close()
	const modID = 1
	collectRaw(t, mod)
	collectRaw(t, player)
	collectRaw(t, mod2)

	// Control: while visible, both the player and the other GM hear the GM.
	chatFrame(t, mod, "antes")
	if !hasFrame(collectRaw(t, player), protocol.MsgMessageChat, modID) {
		t.Fatal("control failed: a visible GM's chat never reached the player")
	}
	if !hasFrame(collectRaw(t, mod2), protocol.MsgMessageChat, modID) {
		t.Fatal("control failed: a visible GM's chat never reached the other GM")
	}

	gmFrame(t, mod, "invisivel")
	if !hasFrame(collectRaw(t, player), protocol.MsgRemoveMob, modID) {
		t.Error("going invisible must RemoveMob the GM from a player already looking at them")
	}
	if !hasFrame(collectRaw(t, mod2), protocol.MsgRemoveMob, modID) {
		t.Error("going invisible must RemoveMob the GM from another GM too")
	}
	if !hasFrame(collectRaw(t, mod), protocol.MsgMessagePanel, 0) {
		t.Error("the GM was not told the switch went through")
	}

	chatFrame(t, mod, "depois")
	if f, ok := anyAbout(collectRaw(t, player), modID); ok {
		t.Errorf("player received %#x about the invisible GM", f.h.Type)
	}
	if f, ok := anyAbout(collectRaw(t, mod2), modID); ok {
		t.Errorf("another GM received %#x about the invisible GM", f.h.Type)
	}

	// A whisper is addressed by name, so hiding it hides nothing — it goes through.
	whisperFrame(t, mod, "Player", "psiu")
	if !hasFrame(collectRaw(t, player), protocol.MsgMessageWhisper, modID) {
		t.Error("an invisible GM's whisper must still reach the player")
	}

	gmFrame(t, mod, "invisivel off")
	if !hasFrame(collectRaw(t, player), protocol.MsgCreateMob, modID) {
		t.Error("becoming visible must CreateMob the GM for the player in view")
	}
	if !hasFrame(collectRaw(t, mod2), protocol.MsgCreateMob, modID) {
		t.Error("becoming visible must CreateMob the GM for the other GM in view")
	}
}

// TestGMInvisibleLateArrival: a player who logs in next to an invisible GM gets
// no CreateMob for them — and still gets one when the GM reappears, although the
// login marked the GM as seen for that player while the CreateMob was swallowed.
func TestGMInvisibleLateArrival(t *testing.T) {
	addr, stop, _ := startServerClock(t, gmDB())
	defer stop()
	mod := enterWorldAs(t, addr, "mod") // conn 1
	defer mod.Close()
	victim := enterWorldAs(t, addr, "victim") // conn 2
	defer victim.Close()
	const modID, victimID = 1, 2

	gmFrame(t, mod, "snoop")
	collectRaw(t, mod)
	collectRaw(t, victim)

	player := dialAndLoginRaw(t, addr, "player")
	defer player.Close()
	login := collectRaw(t, player)
	if !hasFrame(login, protocol.MsgCreateMob, victimID) {
		t.Fatal("control failed: the arriving player was not shown the visible player")
	}
	if hasFrame(login, protocol.MsgCreateMob, modID) {
		t.Error("the arriving player was shown the invisible GM")
	}

	gmFrame(t, mod, "invisivel off")
	if !hasFrame(collectRaw(t, player), protocol.MsgCreateMob, modID) {
		t.Error("the GM reappeared but the late arrival never got a CreateMob for them")
	}
}

// TestGMInvisibleExplicitState: "on" twice leaves it on (no toggle back), and a
// plain player cannot reach the command at all.
func TestGMInvisibleExplicitState(t *testing.T) {
	addr, stop, _ := startServerClock(t, gmDB())
	defer stop()
	mod := enterWorldAs(t, addr, "mod") // conn 1
	defer mod.Close()
	player := enterWorldAs(t, addr, "player") // conn 2
	defer player.Close()
	const modID, playerID = 1, 2
	collectRaw(t, mod)
	collectRaw(t, player)

	gmFrame(t, mod, "invisivel on")
	gmFrame(t, mod, "invisivel on")
	collectRaw(t, mod)
	collectRaw(t, player)
	chatFrame(t, mod, "ainda escondido")
	if f, ok := anyAbout(collectRaw(t, player), modID); ok {
		t.Errorf("a second \"on\" toggled the GM back: player received %#x", f.h.Type)
	}

	// The command is on the moderator bus: a player asking for it stays visible.
	gmFrame(t, player, "invisivel")
	collectRaw(t, player)
	chatFrame(t, player, "estou aqui")
	if !hasFrame(collectRaw(t, mod), protocol.MsgMessageChat, playerID) {
		t.Error("a plain player made themselves invisible")
	}
}

// TestGMInvisibleOutOfAggro: a monster neither takes an invisible GM into its
// enemy list nor keeps one it already had.
func TestGMInvisibleOutOfAggro(t *testing.T) {
	gm := &world.Entity{ID: 1, HP: 100}
	mob := &world.Entity{ID: world.MaxUser}

	addEnemyList(mob, gm)
	if mob.EnemyList[0] != gm.ID {
		t.Fatal("control failed: a visible player was not added to the enemy list")
	}
	clearEnemyList(mob)

	gm.GMInvisible = true
	addEnemyList(mob, gm)
	if hasEnemyList(mob) {
		t.Error("an invisible GM went into a monster's enemy list")
	}
}
