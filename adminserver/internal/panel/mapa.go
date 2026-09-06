package panel

import (
	"net/http"
	"sort"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/jogo"
	"github.com/jeanluca/w2pp-openwyd/internal/mapzones"
)

// The world is one 4096x4096 grid (tmserver/internal/world.DefaultGridDim), so
// every position on this page shares a single picture — there are no per-map
// instances to switch between.
const gradeMundo = 4096

// raioAglomeracao is how close two players must be to count as together, in
// world units.
//
// It is set from what the game shows: a client's viewport is roughly 18 tiles
// across, so two characters within 24 of each other are on the same screen,
// hitting the same spawns. Wider and every busy hunting ground reads as a farm;
// tighter and a farm that spreads out to share aggro stops registering.
const raioAglomeracao = 24

// minAglomeracao is the group size worth showing. Two players together is a
// party; the number that makes a moderator look is three or more standing in
// the same spot outside a town.
const minAglomeracao = 3

// PontoMapa is one player placed on the picture.
type PontoMapa struct {
	Conta      string
	Personagem string
	Nivel      int32
	X, Y       int32
	Regiao     string
	// Junto marks a player inside an Aglomeracao, so the dot can be drawn
	// differently from someone hunting alone.
	Junto bool
}

// raioMinimoDesenho keeps a group's ring outside the dots it encloses.
//
// A tight farm has every member within a handful of units of the center, so the
// honest radius comes out smaller than the dots themselves and the ring draws
// underneath them, invisible — which is the one case the ring exists for.
const raioMinimoDesenho = 60

// Aglomeracao is a group of players standing on top of each other outside any
// town — the shape a bot farm makes.
type Aglomeracao struct {
	X, Y int32 // the group's center, for drawing
	Raio int32 // how far the farthest member sits from that center
	// RaioDesenho is Raio widened so the ring encloses the dots on screen. The
	// page shows Raio, which is the truth; the picture uses this one.
	RaioDesenho int32
	Regiao      string
	Contas      []string
}

// CidadeMapa is one settlement drawn on the picture.
type CidadeMapa struct {
	Nome           string
	CX, CY, Radius int32
	// Rotular decides whether the name is drawn. Nine circles with nine labels
	// is unreadable: the three Pesadelo instances sit within 200 units of each
	// other and their names overlap into a smear. The five canonical cities are
	// always named because they are how a person orients on this map at all; the
	// rest are named only while somebody is standing in them, which is exactly
	// when the name is worth the ink.
	Rotular bool
}

// RegiaoContagem is how many players are in one named region.
type RegiaoContagem struct {
	Nome  string
	Total int
}

// pontos places every player who is actually in the world.
//
// A session still on the character screen has no position — its coordinates are
// zero, which is a real spot on the grid (the top-left corner) and would draw a
// phantom crowd there.
func pontos(ps []jogo.Player) []PontoMapa {
	out := make([]PontoMapa, 0, len(ps))
	for _, p := range ps {
		if !p.Jogando {
			continue
		}
		out = append(out, PontoMapa{
			Conta: p.Conta, Personagem: p.Personagem, Nivel: p.Nivel,
			X: p.X, Y: p.Y, Regiao: regiao(p.X, p.Y),
		})
	}
	return out
}

// regiao names a position the way a person would say it.
//
// Outside every settlement the answer is not just "field": a moderator deciding
// whether to go look needs a landmark, and mapzones.Nearest gives one.
//
// The exact distance is deliberately NOT in this string. It is a grouping key —
// the per-region tally counts equal strings — and four bots standing on top of
// each other are 693, 695, 699 and 699 units from Erion, which would file them
// under four different regions and turn the tally into one row per player. The
// precise position is on the dot itself, where it is a fact about one player
// rather than the name of a place.
func regiao(x, y int32) string {
	if id := mapzones.Classify(x, y); id != mapzones.Field {
		return mapzones.Name(id)
	}
	z, _ := mapzones.Nearest(x, y)
	if z.Name == "" {
		return "Campo"
	}
	return "Campo — mais perto de " + z.Name
}

// aglomerar groups players standing together outside a town.
//
// Towns are excluded rather than ranked lower. Everybody stands in Armia — the
// shops, the bank and the respawn are there — so a detector that counted town
// crowds would fire constantly and be switched off within a day. What it is
// looking for is the opposite: a crowd where a crowd has no reason to be.
//
// The grouping is single-link: A and B are together if they are within
// raioAglomeracao, and a chain of those makes one group. That is deliberate — a
// farm spread along a spawn line is one operation, not four pairs.
func aglomerar(ps []PontoMapa) []Aglomeracao {
	// Only field players are candidates, but the indices must still address the
	// original slice so the Junto marks land on the right dots.
	cand := make([]int, 0, len(ps))
	for i, p := range ps {
		if !mapzones.InTown(p.X, p.Y) {
			cand = append(cand, i)
		}
	}

	visto := make(map[int]bool, len(cand))
	var out []Aglomeracao
	for _, raiz := range cand {
		if visto[raiz] {
			continue
		}
		// Breadth-first over the "within raioAglomeracao" relation.
		grupo := []int{raiz}
		visto[raiz] = true
		for k := 0; k < len(grupo); k++ {
			for _, o := range cand {
				if visto[o] || !perto(ps[grupo[k]], ps[o]) {
					continue
				}
				visto[o] = true
				grupo = append(grupo, o)
			}
		}
		if len(grupo) < minAglomeracao {
			continue
		}

		a := Aglomeracao{Regiao: ps[grupo[0]].Regiao}
		var sx, sy int64
		for _, i := range grupo {
			ps[i].Junto = true
			sx += int64(ps[i].X)
			sy += int64(ps[i].Y)
			a.Contas = append(a.Contas, ps[i].Conta)
		}
		a.X = int32(sx / int64(len(grupo)))
		a.Y = int32(sy / int64(len(grupo)))
		for _, i := range grupo {
			if d := distancia(a.X, a.Y, ps[i].X, ps[i].Y); d > a.Raio {
				a.Raio = d
			}
		}
		a.RaioDesenho = a.Raio + raioMinimoDesenho
		sort.Strings(a.Contas)
		out = append(out, a)
	}

	// Biggest first: the page is read from the top and the largest group is the
	// one worth walking over to.
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Contas) > len(out[j].Contas) })
	return out
}

func perto(a, b PontoMapa) bool {
	dx, dy := int64(a.X-b.X), int64(a.Y-b.Y)
	return dx*dx+dy*dy <= int64(raioAglomeracao)*int64(raioAglomeracao)
}

func distancia(x1, y1, x2, y2 int32) int32 {
	dx, dy := int64(x1-x2), int64(y1-y2)
	d := dx*dx + dy*dy
	// Integer square root: the number is only ever shown rounded.
	if d < 2 {
		return int32(d)
	}
	x, y := d, (d+1)/2
	for y < x {
		x, y = y, (y+d/y)/2
	}
	return int32(x)
}

// cidades prepares the settlements for drawing.
//
// The label is the name without its parenthetical: "Pesadelo Místico (masmorra)"
// is the right thing to write in a table and far too long to sit on a circle
// 60 units wide.
func cidades(ps []PontoMapa) []CidadeMapa {
	comGente := map[string]bool{}
	for _, p := range ps {
		comGente[p.Regiao] = true
	}
	var out []CidadeMapa
	for _, z := range mapzones.All {
		if z.Radius == 0 {
			continue // the Field catch-all has no place on the grid
		}
		// LimitX2 != 0 marks the five cities that carry a legacy CityLimit
		// rectangle — Armia, Azran, Erion, Nippleheim, Noatum.
		out = append(out, CidadeMapa{
			Nome: curto(z.Name), CX: z.CX, CY: z.CY, Radius: z.Radius,
			Rotular: z.LimitX2 != 0 || comGente[z.Name],
		})
	}
	return out
}

func curto(nome string) string {
	if i := strings.Index(nome, " ("); i > 0 {
		return nome[:i]
	}
	return nome
}

// porRegiao counts the players in each named region, busiest first.
func porRegiao(ps []PontoMapa) []RegiaoContagem {
	n := map[string]int{}
	for _, p := range ps {
		n[p.Regiao]++
	}
	out := make([]RegiaoContagem, 0, len(n))
	for nome, total := range n {
		out = append(out, RegiaoContagem{Nome: nome, Total: total})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Nome < out[j].Nome
	})
	return out
}

// mapa draws where everybody is.
//
// Like /servidor it does NOT refresh on a timer and must not learn to: the read
// crosses into the single-owner game loop, whose callback queue is drained ahead
// of player input, so a page polling it would front-run the people playing.
func (h *Handler) mapa(w http.ResponseWriter, r *http.Request) {
	estado, err := h.cfg.Jogo.Estado(r.Context())
	erro := ""
	if err != nil {
		h.cfg.Logger.Warn("live server state unavailable", "err", err)
		erro = explicaJogo(err)
	}

	ps := pontos(estado.Players)
	grupos := aglomerar(ps) // marks Junto on ps

	h.render(w, "mapa.html", struct {
		page
		Pontos   []PontoMapa
		Grupos   []Aglomeracao
		Regioes  []RegiaoContagem
		Cidades  []CidadeMapa
		Grade    int32
		Conexoes int32
		Erro     string
	}{
		h.pageFor(r, "mapa"), ps, grupos, porRegiao(ps),
		cidades(ps), gradeMundo, estado.Conectados, erro,
	})
}
