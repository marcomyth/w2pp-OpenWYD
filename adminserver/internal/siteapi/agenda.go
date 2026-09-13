package siteapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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

// pesaPrimeiroMinuto is each tier's first window. The other two follow at +20
// and +40 (handler/pesadelo.go: openMinute).
var pesaPrimeiroMinuto = map[dungeon.Gate]int32{
	dungeon.PesadeloN: 0,
	dungeon.PesadeloM: 5,
	dungeon.PesadeloA: 10,
}

// chaveDaPorta is the stable key the site matches on: the door's name, lowercased
// and without the accent, so "Pesadelo Místico" travels as pesadelo_mistico.
func chaveDaPorta(g dungeon.Gate) string {
	semAcento := strings.NewReplacer("í", "i", "á", "a", "â", "a", "ã", "a", "ç", "c", "é", "e", "ê", "e", "ó", "o", "ú", "u")
	return strings.ReplaceAll(strings.ToLower(semAcento.Replace(g.Name())), " ", "_")
}

// linhaAgenda is one scheduled thing.
//
// Numbers, not a sentence: the site writes the sentence, and a number survives a
// change of wording. Hora is the hour of the day for something daily, and -1 for
// something that repeats inside EVERY hour.
type linhaAgenda struct {
	Chave   string  `json:"chave"`
	Rotulo  string  `json:"rotulo"`
	Ligado  bool    `json:"ligado"`
	Hora    int32   `json:"hora"`
	Minutos []int32 `json:"minutos"`
	Duracao int32   `json:"duracao"`
	Detalhe string  `json:"detalhe"`
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
	for _, g := range []dungeon.Gate{dungeon.PesadeloN, dungeon.PesadeloM, dungeon.PesadeloA} {
		out = append(out, linhaAgenda{
			Chave:   chaveDaPorta(g),
			Rotulo:  g.Name(),
			Ligado:  portaAberta(portas, g),
			Hora:    -1,
			Minutos: janelasDoPesadelo(pesaPrimeiroMinuto[g]),
			Duracao: pesaJanelaMin,
			Detalhe: fmt.Sprintf("três janelas por hora, de %d minutos cada", pesaJanelaMin),
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
