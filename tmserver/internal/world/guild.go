package world

import "time"

// GuildInfo is the minimal guild metadata not modeled on Entity itself: the
// name and fame score, keyed by guild id (issue #131). In-memory, filled from
// dbServer at boot (handler/guild_state.go), by /create as each guild is made,
// and by the /gm guildname|guildfame admin commands, which mirror the legacy
// GM-only "+guildfame set" tool (Source/Comandos GM.txt).
type GuildInfo struct {
	Name string
	Fame int32

	// O Painel de Guilda (0079_painel_de_guilda). Ficam aqui, e não só no banco,
	// porque o painel os mostra em toda abertura e nenhum deles vale uma ida ao
	// banco: o recado muda quando alguém escreve, e o teto quase nunca.
	//
	// NoticeAt zero significa que nunca houve recado, e é assim que o painel sabe
	// não desenhar a data.
	Notice    string
	NoticeBy  string
	NoticeAt  time.Time
	MemberCap int
}

// GuildInfo returns the registered name/fame for a guild id, or false if
// nothing has been registered for it yet. Loop-only.
func (w *World) GuildInfo(id uint16) (GuildInfo, bool) {
	gi, ok := w.guilds[id]
	return gi, ok
}

// SetGuildName sets (or creates) a guild's registered name. Loop-only.
func (w *World) SetGuildName(id uint16, name string) {
	gi := w.guilds[id]
	gi.Name = name
	w.guilds[id] = gi
}

// SetGuildNotice sets a guild's notice board, its author and when it was
// written. Loop-only.
//
// Writes the three together on purpose: the panel draws "Atualizado: ..." beside
// the text, so a stamp that outlived its notice would date the wrong words.
func (w *World) SetGuildNotice(id uint16, notice, by string, at time.Time) {
	gi := w.guilds[id]
	gi.Notice, gi.NoticeBy, gi.NoticeAt = notice, by, at
	w.guilds[id] = gi
}

// SetGuildMemberCap sets a guild's member ceiling. Loop-only.
func (w *World) SetGuildMemberCap(id uint16, cap int) {
	gi := w.guilds[id]
	gi.MemberCap = cap
	w.guilds[id] = gi
}

// SetGuildFame sets (or creates) a guild's registered fame. Loop-only.
func (w *World) SetGuildFame(id uint16, fame int32) {
	gi := w.guilds[id]
	gi.Fame = fame
	w.guilds[id] = gi
}

// GuildNameTaken reports whether a guild this process knows already has exactly
// this name — the same rule as the guild table's UNIQUE constraint, checked here
// so /create can say so instead of losing the answer inside dbServer's ok=false.
// Loop-only.
func (w *World) GuildNameTaken(name string) bool {
	for _, gi := range w.guilds {
		if gi.Name == name {
			return true
		}
	}
	return false
}
