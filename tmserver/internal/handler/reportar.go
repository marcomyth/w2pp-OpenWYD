package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The /reportar command (issue: support snapshot).
//
// NO LEGACY COUNTERPART. The original server has no report command: a player
// with a problem took a screenshot and told the story in a forum, and the first
// thing staff had to do was ask them to reproduce it. What this adds is the
// server's own answer to "what was actually happening" — position, level, and
// who was in view — taken at the instant the player pressed enter.

// reportEspera is how long an account waits between reports.
//
// Not politeness: without it one annoyed player writes hundreds of rows in a
// minute and buries the queue, which turns the tool for handling grief into the
// thing being griefed. A minute is short enough that somebody with two real
// problems is not blocked, and long enough that flooding is pointless.
const reportEspera = 60 * time.Second

// reportar files one report and tells the player it went in.
//
// The text is NOT clamped here. internal/store is the last place before the
// database and clamps it there, with a test — a second limit in this file would
// be a second number to keep in step for no gain. Same for the nearby list: the
// view window is a screen, so what is collected is bounded by construction, and
// the store cuts it to its own maximum.
func (d *Dispatcher) reportar(w *world.World, s *world.Session, args []byte) {
	e := w.Entity(s.Conn)
	if e == nil {
		return
	}
	texto := strings.TrimSpace(cstr(args))
	if texto == "" {
		d.sendChatText(w, s, "Escreva o que houve. Exemplo: /reportar o Fulano está usando bot aqui")
		return
	}

	agora := d.now()
	if ultimo, ok := d.reportadoEm[s.AccountID]; ok && agora.Sub(ultimo) < reportEspera {
		falta := int((reportEspera - agora.Sub(ultimo)).Seconds()) + 1
		d.sendChatText(w, s, "Espere "+strconv.Itoa(falta)+"s para reportar de novo.")
		return
	}
	if d.reportadoEm == nil {
		d.reportadoEm = map[int64]time.Time{}
	}
	d.reportadoEm[s.AccountID] = agora

	// Who the server could see around them. Names only — no positions, no
	// accounts: this is what makes "somebody here is botting" checkable, and it
	// is also people who did not ask to be in anybody record. The row expires
	// (0028_player_report) for exactly that reason.
	var perto []string
	w.ForEachInView(s.Conn, func(_ *world.Session, outro *world.Entity) {
		if outro != nil && outro.Name != "" && world.IsPlayer(outro.ID) {
			perto = append(perto, outro.Name)
		}
	})

	r := world.PlayerReport{
		AccountID: s.AccountID, Account: s.AccountName,
		Character: e.Name, Level: e.Level, Text: texto,
		X: e.X, Y: e.Y, Nearby: perto,
	}

	// Told BEFORE the write lands, on purpose. The player asked for help and the
	// answer must not wait on Postgres; the write is best-effort and the failure
	// is ours to see in the log, not theirs to read on screen.
	d.sendChatText(w, s, "Reportado. A equipe vê o que estava acontecendo aqui agora.")
	d.log.Info("player report", "conn", s.Conn, "account", s.AccountName,
		"character", e.Name, "x", e.X, "y", e.Y, "nearby", len(perto))

	// Detached rather than bound to the session: the report belongs to the
	// server, not to whether this player is still connected when the write lands.
	p := w.Persistence()
	w.GoDetached(func() func(*world.World) {
		if err := p.RecordReport(context.Background(), r); err != nil {
			return func(*world.World) {
				d.log.Warn("player report: persistence failed",
					"account", r.Account, "character", r.Character, "err", err)
			}
		}
		return nil
	})
}
