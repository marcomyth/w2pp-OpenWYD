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
	// ip is unexported ON PURPOSE. The page needs to say that accounts share a
	// connection; it must never print the address itself, and an unexported
	// field is unreachable from a template — so the rule is enforced by the
	// language rather than by remembering.
	ip string
}

// Conexao is a set of DIFFERENT accounts reaching the server from one address.
//
// It is the strongest bot-farm signal the panel has, and also the one most
// easily read as an accusation, so two things are deliberate. The address is
// never carried out of this package: the moderator needs the fact, not the
// number. And the count is of accounts, not sessions — one person reconnecting
// twice is not two people.
type Conexao struct {
	Contas []string
	// EmJogo is how many of them are actually in the world; the rest are sitting
	// on the character screen. A farm logging in shows here before it shows on
	// the map, which is the reason this list reads every session and not just
	// the placed ones.
	EmJogo int
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
	// MesmaConexao reports that every account in the group reaches the server
	// from one address. Position alone is circumstantial and connection alone is
	// explainable; together they are as close to proof as this panel gets.
	MesmaConexao bool
}

// CidadeMapa is one settlement drawn on the picture.
type CidadeMapa struct {
	Nome           string
	CX, CY, Radius int32
	// Rotular decides whether the name is drawn.
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
			X: p.X, Y: p.Y, Regiao: regiao(p.X, p.Y), ip: p.IP,
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
		a.MesmaConexao = umaConexaoSo(ps, grupo)
		a.RaioDesenho = a.Raio + raioMinimoDesenho
		sort.Strings(a.Contas)
		out = append(out, a)
	}

	// Biggest first: the page is read from the top and the largest group is the
	// one worth walking over to.
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Contas) > len(out[j].Contas) })
	return out
}

// umaConexaoSo reports whether every member of the group came from the same
// address. An unknown address answers no: the game does not always have one, and
// several blanks are not a match.
func umaConexaoSo(ps []PontoMapa, grupo []int) bool {
	primeiro := ps[grupo[0]].ip
	if primeiro == "" {
		return false
	}
	for _, i := range grupo[1:] {
		if ps[i].ip != primeiro {
			return false
		}
	}
	return true
}

// conexoes groups the sessions by address and keeps the ones holding more than
// one account.
//
// It reads EVERY session, not just the placed ones: a farm signing in is visible
// here while it is still on the character screen and has no position yet.
//
// Two accounts on one address is not misconduct on its own — a household, a
// LAN house and a phone behind carrier NAT all look exactly like this. The page
// says so; this function only counts.
func conexoes(ps []jogo.Player) []Conexao {
	type grupo struct {
		contas map[string]bool
		emJogo map[string]bool
	}
	porEndereco := map[string]*grupo{}
	for _, p := range ps {
		if p.IP == "" || p.Conta == "" {
			continue
		}
		g := porEndereco[p.IP]
		if g == nil {
			g = &grupo{contas: map[string]bool{}, emJogo: map[string]bool{}}
			porEndereco[p.IP] = g
		}
		g.contas[p.Conta] = true
		if p.Jogando {
			g.emJogo[p.Conta] = true
		}
	}

	var out []Conexao
	for _, g := range porEndereco {
		if len(g.contas) < 2 {
			continue
		}
		c := Conexao{EmJogo: len(g.emJogo)}
		for nome := range g.contas {
			c.Contas = append(c.Contas, nome)
		}
		sort.Strings(c.Contas)
		out = append(out, c)
	}
	// Biggest first, then by name so two groups of the same size keep their
	// order between reloads. The map iteration above has none.
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Contas) != len(out[j].Contas) {
			return len(out[i].Contas) > len(out[j].Contas)
		}
		return out[i].Contas[0] < out[j].Contas[0]
	})
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
// Only the five canonical cities are always on the picture — they are how a
// person finds themselves on it. The other four zones (the three Pesadelo
// interiors and the unidentified east city) are drawn ONLY while somebody is
// standing in one.
//
// That is not tidying. Drawn always, the three Pesadelo circles sit within 200
// units of each other in an otherwise empty corner: unlabelled they read as a
// glitch, and labelled their names overlap into a smear. A circle that appears
// exactly when it holds a player is a circle that means something.
//
// The label drops the parenthetical: "Pesadelo Místico (masmorra)" is right in a
// table and far too long to sit on a circle 60 units wide.
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
		cidade := z.LimitX2 != 0
		if !cidade && !comGente[z.Name] {
			continue
		}
		out = append(out, CidadeMapa{
			Nome: curto(z.Name), CX: z.CX, CY: z.CY, Radius: z.Radius, Rotular: true,
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

	cids := cidades(ps)
	h.render(w, "mapa.html", struct {
		page
		Pontos     []PontoMapa
		Grupos     []Aglomeracao
		Conexoes   []Conexao
		Regioes    []RegiaoContagem
		Cidades    []CidadeMapa
		Vista      Vista
		Grade      int32
		Conectados int32
		Erro       string
	}{
		h.pageFor(r, "mapa"), ps, grupos, conexoes(estado.Players), porRegiao(ps),
		cids, vistaDe(cids, ps), gradeMundo, estado.Conectados, erro,
	})
}

// Vista is the piece of the world the picture actually draws.
//
// The map used to frame the whole 4096 x 4096 grid, and the grid is mostly
// empty: the five cities span 64% of its width and 35% of its height, so the
// usual view — an empty server — was five circles in a band with a quarter of
// the width blank on the left and a quarter of the height blank at the bottom.
// It read as circles floating in a box rather than as a map.
//
// So the frame is computed from what is drawn. It always contains the five
// cities, so it never jumps somewhere unfamiliar between loads, and it grows
// to include every player, so nobody is ever cropped out.
//
// It is kept SQUARE on purpose. Letting the frame follow the content would fill
// the box better and lie: this map exists to show who is standing near whom, and
// a picture whose east-west scale differs from its north-south one makes the
// same distance look like two different distances depending on the direction.
type Vista struct {
	X, Y, Lado int32
	// Fonte and Ponto scale with the frame. They were tuned for a 4096-unit box
	// at 760 pixels; a tighter crop magnifies everything inside it, so a fixed
	// size would grow into the drawing.
	Fonte int32
	Ponto int32
	// Meio, Base and FonteVazio place the empty-state line. Computed here rather
	// than in the template: arithmetic helpers in a template are how logic ends
	// up somewhere nobody tests it.
	Meio       int32
	Base       int32
	FonteVazio int32
}

// margemVista is how much air to leave around the content, as a fraction of the
// frame. Without it a city on the edge has half its circle and all of its label
// outside the picture.
const margemVista = 12

// vistaDe computes the frame from the cities and the players on it.
func vistaDe(cidades []CidadeMapa, pontos []PontoMapa) Vista {
	// Seeded with the cities rather than starting empty: they are the anchors a
	// person reads the map by, and a frame that tightened around two players
	// would be a picture of nowhere.
	x1, y1 := int32(gradeMundo), int32(gradeMundo)
	x2, y2 := int32(0), int32(0)
	inclui := func(x, y, raio int32) {
		if x-raio < x1 {
			x1 = x - raio
		}
		if y-raio < y1 {
			y1 = y - raio
		}
		if x+raio > x2 {
			x2 = x + raio
		}
		if y+raio > y2 {
			y2 = y + raio
		}
	}
	for _, c := range cidades {
		inclui(c.CX, c.CY, c.Radius)
	}
	for _, p := range pontos {
		inclui(p.X, p.Y, 0)
	}
	if x2 <= x1 || y2 <= y1 {
		// Nothing to frame at all: fall back to the whole world rather than to a
		// zero-sized box, which would render as an invisible map.
		return vistaCompleta()
	}

	// Square, around the middle of the content.
	lado := x2 - x1
	if a := y2 - y1; a > lado {
		lado = a
	}
	lado += lado / margemVista
	cx, cy := (x1+x2)/2, (y1+y2)/2
	v := Vista{X: cx - lado/2, Y: cy - lado/2, Lado: lado}

	// Clamped to the world. A frame that started at -300 would draw a band of
	// nothing that does not exist, which is a different lie from the one this
	// replaced.
	if v.X < 0 {
		v.X = 0
	}
	if v.Y < 0 {
		v.Y = 0
	}
	if v.X+v.Lado > gradeMundo {
		v.Lado = gradeMundo - v.X
	}
	if v.Y+v.Lado > gradeMundo {
		v.Lado = gradeMundo - v.Y
	}
	return v.comEscala()
}

func vistaCompleta() Vista {
	return Vista{X: 0, Y: 0, Lado: gradeMundo}.comEscala()
}

// comEscala fills in the sizes that depend on the frame.
//
// The divisors come from the sizes that looked right on the full world at the
// width this page renders — a label at 62 units and a dot at 16 in a 4096 box —
// so a tighter crop keeps the same apparent size on screen.
func (v Vista) comEscala() Vista {
	v.Fonte = v.Lado / 66
	v.Ponto = v.Lado / 256
	if v.Ponto < 4 {
		v.Ponto = 4
	}
	v.Meio = v.X + v.Lado/2
	v.Base = v.Y + v.Lado - v.Lado/12
	v.FonteVazio = v.Fonte * 3 / 2
	return v
}
