package siteapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/dungeon"
)

// The schedule a player actually asks about: what happens at what time.
//
// Only two things on this server have a clock that can be READ. Both are here,
// and nothing else is:
//
//   - Guerra de Torres. The hour is configuration (world_event, migration 0051,
//     editable in the panel); the minutes inside it are the legacy's
//     (CWarTower.cpp): the announce runs from :00, the tower opens at :06 and
//     the war ends at :30.
//   - Pesadelo. Each tier opens three four-minute windows an hour, twenty
//     minutes apart, starting at :00 (Normal), :05 (Místico) and :10 (Arcano).
//     Each tier has its own door, and staff can shut one without the others.
//
// The minutes are a COPY of tmserver's (worldevents/tower.go and
// handler/pesadelo.go), for the same reason eventos.go copies the item-rain
// rule: they live under tmserver/internal, which this package cannot import,
// and a site that guesses a schedule announces events that do not happen. A
// table test freezes the copy, so the day the game changes a minute, the test is
// where the two versions are caught disagreeing.
//
// What is deliberately NOT here:
//   - "Chefes sozinhos": they come back N hours after dying. That is a duration,
//     not a time of day, and a calendar cannot show it without inventing one.
//   - Água: entered with a scroll, at any time. It has no window.

// MasmorrasLeitura reads the dungeon doors. Same shape as the other readers
// here: one method, the one this endpoint needs.
type MasmorrasLeitura interface {
	DungeonGates(ctx context.Context) (domain.DungeonGateConfig, error)
}

// Minutes copied from tmserver, frozen by agenda_test.go.
const (
	// torreAviso is when the announce starts, torreAbre when the tower appears
	// and torreFim when the war ends — all within the configured hour.
	torreAviso = 0
	torreAbre  = 6
	torreFim   = 30
	// pesaJanelaMin is NigthTime in whole minutes, pesaPasso the gap between a
	// tier's three windows.
	pesaJanelaMin = 4
	pesaPasso     = 20
)

// pesaJanelas is each Pesadelo tier: the key the site matches on and the first
// of its three windows (the others follow at +20 and +40 —
// handler/pesadelo.go: openMinute).
//
// The key is WRITTEN HERE and never derived from the door's label. The label is
// display text, and rewording one is the most harmless-looking change there is;
// a key that followed it would change this contract by accident, the site would
// stop recognising the line, and the page would break with nobody having touched
// the site. The label keeps coming from Name(), which is exactly what may change
// without hurting anyone.
var pesaJanelas = []struct {
	porta    dungeon.Gate
	chave    string
	primeiro int32
}{
	{dungeon.PesadeloN, "pesadelo_normal", 0},
	{dungeon.PesadeloM, "pesadelo_mistico", 5},
	{dungeon.PesadeloA, "pesadelo_arcano", 10},
}

// linhaAgenda is one scheduled thing.
//
// Numbers, not a sentence: the site writes the sentence, and a number survives a
// change of wording.
//
// TodaHora says whether this repeats inside every hour. It is a field of its own
// rather than a magic value in Hora: a reader that misses a sentinel renders
// "às -1h", and nobody ever finds out where the -1 came from.
type linhaAgenda struct {
	Chave  string `json:"chave"`
	Rotulo string `json:"rotulo"`
	Ligado bool   `json:"ligado"`
	// TodaHora true: this happens in every hour, and Hora means nothing.
	TodaHora bool    `json:"toda_hora"`
	Hora     int32   `json:"hora"`
	Minutos  []int32 `json:"minutos"`
	Duracao  int32   `json:"duracao"`
	Detalhe  string  `json:"detalhe"`
}

type respostaAgenda struct {
	Agenda []linhaAgenda `json:"agenda"`
}

// portaAberta answers whether a door lets people in. A door ABSENT from the
// table is open: that is how the server behaved before the table existed, and
// the domain type says so.
func portaAberta(cfg domain.DungeonGateConfig, g dungeon.Gate) bool {
	for _, p := range cfg.Gates {
		if p.Gate == int32(g) {
			return p.Open
		}
	}
	return true
}

// janelasDoPesadelo turns a tier's first minute into its three windows.
func janelasDoPesadelo(primeiro int32) []int32 {
	return []int32{primeiro, primeiro + pesaPasso, primeiro + 2*pesaPasso}
}

func agendaDaConfig(evt domain.WorldEventConfig, portas domain.DungeonGateConfig) []linhaAgenda {
	out := []linhaAgenda{{
		Chave:   "guerra_de_torres",
		Rotulo:  "Guerra de Torres",
		Ligado:  evt.TowerWarEnabled,
		Hora:    evt.TowerWarHour,
		Minutos: []int32{torreAbre},
		Duracao: torreFim - torreAbre,
		Detalhe: "o aviso sai no minuto :00 da hora marcada, a torre aparece às :06 e a guerra termina às :30",
	}}
	// The panel's own order for the three tiers.
	for _, j := range pesaJanelas {
		out = append(out, linhaAgenda{
			Chave:    j.chave,
			Rotulo:   j.porta.Name(),
			Ligado:   portaAberta(portas, j.porta),
			TodaHora: true,
			Minutos:  janelasDoPesadelo(j.primeiro),
			Duracao:  pesaJanelaMin,
			Detalhe:  fmt.Sprintf("três janelas por hora, de %d minutos cada", pesaJanelaMin),
		})
	}
	return out
}

// agenda answers what is scheduled. No account in the path: the same answer for
// everyone, and nothing here a player could not learn by watching the clock.
func (a *API) agenda(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Eventos == nil || a.cfg.Masmorras == nil {
		responde(w, http.StatusServiceUnavailable, falha{Erro: "agenda_desligada"})
		return
	}
	evt, err := a.cfg.Eventos.WorldEventConfig(r.Context())
	if err != nil {
		a.interno(w, "read world event config", 0, err)
		return
	}
	portas, err := a.cfg.Masmorras.DungeonGates(r.Context())
	if err != nil {
		a.interno(w, "read dungeon gates", 0, err)
		return
	}
	responde(w, http.StatusOK, respostaAgenda{Agenda: agendaDaConfig(evt, portas)})
}
