package panel

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/mapzones"
)

// The guild and city pages are READ ONLY, and that is a decision rather than an
// unfinished half.
//
// The game keeps guild and city state in the loop and writes its own copy back:
// a player changing the city tax, a siege resolving, somebody joining a guild —
// each of those persists the loop's version (handler/guild.go persistGuildZone
// and siblings). A panel write would land in the database and be overwritten by
// the next in-game change without a word, which is worse than not offering it:
// the moderator would watch their edit vanish and have no way to tell why.
//
// So this shows what exists, and the writes that a moderator genuinely needs
// already have a path the game respects: /gm guildname and /gm guildfame go
// through the loop, which is the owner.

// nomeDeCidade turns a guild-zone index into the city everybody calls it.
//
// guild_zone is keyed 0..4 in the same order as the world's city table, which is
// the order mapzones froze its first five ids in.
func nomeDeCidade(zona int) string {
	if n := mapzones.Name(int32(zona)); n != "" {
		return n
	}
	return "Zona " + strconv.Itoa(zona)
}

// guildaView is one guild on the list.
type guildaView struct {
	domain.Guild
	Membros  int
	Aliada   string
	EmGuerra string
}

// cidadeView is one city and who holds it.
type cidadeView struct {
	domain.GuildZone
	Nome string
	// Dono is the guild that holds the city, by name; empty when nobody does.
	Dono string
	// Desafiante is the guild with a standing challenge bid, by name.
	Desafiante string
}

// guildas lists the guilds and the five cities.
func (h *Handler) guildas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	guildas, err := h.cfg.Guildas.ListGuilds(ctx)
	if err != nil {
		h.cfg.Logger.Error("guild list failed", "err", err)
		http.Error(w, "Erro ao ler as guildas.", http.StatusInternalServerError)
		return
	}
	nomes := make(map[uint16]string, len(guildas))
	for _, g := range guildas {
		nomes[g.ID] = g.Name
	}

	// A failure in any of the three extras leaves the list standing: the roster
	// of guilds is the page, and the counts and relations are what it says about
	// each one.
	// Each failure is marked rather than swallowed: a zero member count and a
	// guild with no members look identical, and "Nenhuma cidade registrada" is
	// a claim the page cannot make when the read is what failed.
	var naoLeu falhas
	contagem, err := h.cfg.Guildas.CountGuildMembers(ctx)
	if err != nil {
		h.cfg.Logger.Error("guild member count failed", "err", err)
		contagem = map[uint16]int{}
		naoLeu.nao("membros")
	}
	relacoes, err := h.cfg.Guildas.ListGuildRelations(ctx)
	if err != nil {
		h.cfg.Logger.Error("guild relations failed", "err", err)
		relacoes = nil
		naoLeu.nao("relacoes")
	}
	zonas, err := h.cfg.Guildas.LoadGuildZones(ctx)
	if err != nil {
		h.cfg.Logger.Error("guild zones failed", "err", err)
		zonas = nil
		naoLeu.nao("cidades")
	}

	aliada := map[uint16]uint16{}
	guerra := map[uint16]uint16{}
	for _, rel := range relacoes {
		if rel.Kind == domain.GuildRelationAlly {
			aliada[rel.GuildID] = rel.TargetGuildID
		} else {
			guerra[rel.GuildID] = rel.TargetGuildID
		}
	}

	vistas := make([]guildaView, 0, len(guildas))
	for _, g := range guildas {
		vistas = append(vistas, guildaView{
			Guild: g, Membros: contagem[g.ID],
			Aliada: nomes[aliada[g.ID]], EmGuerra: nomes[guerra[g.ID]],
		})
	}
	// Biggest first: a list of guilds is read to find the ones that matter.
	sort.SliceStable(vistas, func(i, j int) bool {
		if vistas[i].Membros != vistas[j].Membros {
			return vistas[i].Membros > vistas[j].Membros
		}
		return vistas[i].Name < vistas[j].Name
	})

	cidades := make([]cidadeView, 0, len(zonas))
	for _, z := range zonas {
		cidades = append(cidades, cidadeView{
			GuildZone: z, Nome: nomeDeCidade(z.Zone),
			Dono: nomes[z.ChargeGuild], Desafiante: nomes[z.ChallengeGuild],
		})
	}
	sort.Slice(cidades, func(i, j int) bool { return cidades[i].Zone < cidades[j].Zone })

	h.render(w, "guildas.html", struct {
		page
		Guildas []guildaView
		Cidades []cidadeView
		NaoLeu  falhas
	}{h.pageFor(r, "guildas"), vistas, cidades, naoLeu})
}

// guilda shows one guild and its roster.
func (h *Handler) guilda(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("guilda"), 10, 16)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	guildas, err := h.cfg.Guildas.ListGuilds(ctx)
	if err != nil {
		h.cfg.Logger.Error("guild list failed", "err", err)
		http.Error(w, "Erro ao ler as guildas.", http.StatusInternalServerError)
		return
	}
	nomes := make(map[uint16]string, len(guildas))
	var alvo domain.Guild
	var achou bool
	for _, g := range guildas {
		nomes[g.ID] = g.Name
		if g.ID == uint16(id) {
			alvo, achou = g, true
		}
	}
	if !achou {
		http.NotFound(w, r)
		return
	}

	membros, err := h.cfg.Guildas.ListGuildMembers(ctx, uint16(id))
	if err != nil {
		h.cfg.Logger.Error("guild roster failed", "guilda", id, "err", err)
		http.Error(w, "Erro ao ler os membros.", http.StatusInternalServerError)
		return
	}
	var naoLeu falhas
	relacoes, err := h.cfg.Guildas.ListGuildRelations(ctx)
	if err != nil {
		h.cfg.Logger.Error("guild relations failed", "guilda", id, "err", err)
		relacoes = nil
		naoLeu.nao("relacoes")
	}

	v := guildaView{Guild: alvo, Membros: len(membros)}
	for _, rel := range relacoes {
		if rel.GuildID != alvo.ID {
			continue
		}
		if rel.Kind == domain.GuildRelationAlly {
			v.Aliada = nomes[rel.TargetGuildID]
		} else {
			v.EmGuerra = nomes[rel.TargetGuildID]
		}
	}

	h.render(w, "guilda.html", struct {
		page
		Guilda  guildaView
		Membros []domain.GuildMember
		NaoLeu  falhas
	}{h.pageFor(r, "guildas"), v, membros, naoLeu})
}
