package panel

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/level"
)

// MesaXP is the Mesa de XP's configuration store, satisfied by *store.Store.
//
// It lives in internal/store rather than in a panel-owned package — unlike the
// account writes — because the game reads these same rows through dbServer. A
// second decoder here could only disagree with the one tmServer boots on, and a
// disagreement about experience is the kind nobody notices until a week of
// levelling is wrong.
type MesaXP interface {
	XPConfig(ctx context.Context) (domain.XPConfig, error)
	UpsertXPRule(ctx context.Context, rule domain.XPRule, moderatorID int64) (domain.XPRule, error)
	DeleteXPRule(ctx context.Context, zone, tier int32) (domain.XPRule, error)
}

// linhasEmBranco is how many empty cut rows the editor always shows below the
// table. With no JavaScript there is no "add row" button that works without a
// round trip, so the form simply carries spare rows: filling one adds a cut,
// clearing a level removes it, and nothing needs a second request.
const linhasEmBranco = 3

// bandaPlano is the level step the time-to-cap table is folded into.
const bandaPlano = 25

// --- the view model -------------------------------------------------------

type mesaAba struct {
	Rotulo string
	URL    string
	Ativa  bool
	Tocada bool // this branch has a saved rule, so the tab can say so
}

type mesaCorte struct {
	Ate     string // empty for a spare row; "acima" for the open-ended one
	Divisor string
	Aberto  bool
}

type mesaBanda struct {
	De, Ate     int32
	ExpPorMorte int64
	Mortes      int64
	Tempo       string
}

type mesaSimulacao struct {
	Feita       bool
	ExpPorMorte int64
	ExpDoNivel  int64
	MortesNivel int64
	TotalMortes int64
	Tempo       string
	Muro        int32
	AteNivel    int32
	Bandas      []mesaBanda
	Aviso       string
}

type mesaForm struct {
	Zona      int
	Evolucao  int
	Mob       string
	MobExp    int64
	MobNivel  int32
	Nivel     int32
	Bau       int32
	Fada      int16
	Grau7     int32
	Gemas     int32
	Segundos  int32
	XPDobro   bool
	Novato    bool
	KefraViva bool
	Quests    bool
}

// --- the page -------------------------------------------------------------

// mesaXP renders the Mesa de XP: what the game pays for a kill, where that
// number comes from, and what changing it would do.
func (h *Handler) mesaXP(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.mesaConfig(r.Context())
	if err != nil {
		h.cfg.Logger.Error("mesa de XP read failed", "err", err)
		http.Error(w, "Erro ao ler a Mesa de XP.", http.StatusInternalServerError)
		return
	}

	form := lerMesaForm(r.URL.Query())
	zona := level.Zone(form.Zona)
	evo := uint8(form.Evolucao)

	// A mob name fills the reward and level boxes from the real template, so
	// the simulation is about a monster that exists rather than two numbers
	// somebody guessed.
	var mobAviso string
	if form.Mob != "" {
		exp, nivel, err := h.mobExpNivel(r, form.Mob)
		switch {
		case errors.Is(err, errSemGameData):
			mobAviso = "O editor de monstros não está ligado neste painel; digite a XP e o nível à mão."
		case err != nil:
			mobAviso = fmt.Sprintf("Não achei o monstro %q. Confira o nome em /monstros.", form.Mob)
		default:
			form.MobExp, form.MobNivel = exp, nivel
		}
	}

	sim := simularMesa(form, cfg)
	if mobAviso != "" {
		sim.Aviso = mobAviso
	}

	var historico []audit.Entry
	if lista, err := h.mesaHistorico(r.Context()); err != nil {
		h.cfg.Logger.Error("mesa de XP history failed", "err", err)
	} else {
		historico = lista
	}

	h.render(w, "mesaxp.html", struct {
		page
		Versao    int64
		Abas      []mesaAba
		Zonas     []mesaAba
		Zona      string
		Evolucao  string
		Taxa      int32
		Cortes    []mesaCorte
		Editada   bool
		Legado    []mesaCorte
		Form      mesaForm
		Sim       mesaSimulacao
		Historico []audit.Entry
		Fadas     []fadaOpcao
		Monstros  []string
		Aviso     string
		VoltarURL string
	}{
		page:      h.pageFor(r, "mesaxp"),
		Versao:    cfg.Version,
		Abas:      abasEvolucao(form, cfg),
		Zonas:     abasZona(form, cfg),
		Zona:      zona.Name(),
		Evolucao:  level.TierName(evo),
		Taxa:      cfg.RatePercent(zona, evo),
		Cortes:    cortesParaTela(cfg.Cuts(zona, evo), true),
		Editada:   temRegra(cfg, zona, evo),
		Legado:    cortesParaTela(level.LegacyCuts(zona, evo), false),
		Form:      form,
		Sim:       sim,
		Historico: historico,
		Fadas:     fadas,
		Monstros:  h.nomesDeMonstro(r),
		Aviso:     r.URL.Query().Get("aviso"),
		VoltarURL: form.query(),
	})
}

// setMesaXP saves one branch's rate and cut table.
func (h *Handler) setMesaXP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())

	zona, evo, ok := zonaEvolucaoDoForm(w, r)
	if !ok {
		return
	}

	taxa, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("taxa")))
	if err != nil || taxa < 0 || taxa > 100000 {
		http.Error(w, "A taxa precisa ser um número entre 0 e 100000.", http.StatusBadRequest)
		return
	}

	cortes, err := cortesDoForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	regra := domain.XPRule{Zone: zona, Tier: evo, RatePercent: int32(taxa), Cuts: cortes}
	antes, err := h.cfg.MesaXP.UpsertXPRule(r.Context(), regra, sess.AccountID)
	if err != nil {
		h.cfg.Logger.Error("mesa de XP save failed", "zona", zona, "evolucao", evo, "err", err)
		http.Error(w, "Erro ao gravar a Mesa de XP.", http.StatusInternalServerError)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetXPRule,
		Old:    regraParaAudit(antes), New: regraParaAudit(regra),
	}); err != nil {
		h.auditoriaFalhou(w, err)
		return
	}
	h.voltarParaMesa(w, r, "Gravado. O jogo só passa a usar isto no próximo reinício.")
}

// limparMesaXP returns one branch to the legacy tables.
func (h *Handler) limparMesaXP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())

	zona, evo, ok := zonaEvolucaoDoForm(w, r)
	if !ok {
		return
	}
	antes, err := h.cfg.MesaXP.DeleteXPRule(r.Context(), zona, evo)
	if err != nil {
		h.cfg.Logger.Error("mesa de XP clear failed", "zona", zona, "evolucao", evo, "err", err)
		http.Error(w, "Erro ao limpar a regra.", http.StatusInternalServerError)
		return
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionClearXPRule, Old: regraParaAudit(antes),
	}); err != nil {
		h.auditoriaFalhou(w, err)
		return
	}
	h.voltarParaMesa(w, r, "Voltou ao legado. Vale no próximo reinício do jogo.")
}

func (h *Handler) voltarParaMesa(w http.ResponseWriter, r *http.Request, aviso string) {
	destino := "/auditoria/xp"
	if q := r.PostFormValue("voltar"); strings.HasPrefix(q, "?") && !strings.ContainsAny(q, "/\\") {
		destino += q + "&aviso=" + url.QueryEscape(aviso)
	} else {
		destino += "?aviso=" + url.QueryEscape(aviso)
	}
	http.Redirect(w, r, destino, http.StatusSeeOther)
}

// --- reading the forms ----------------------------------------------------

func zonaEvolucaoDoForm(w http.ResponseWriter, r *http.Request) (zona, evo int32, ok bool) {
	z, errZ := strconv.Atoi(r.PostFormValue("zona"))
	e, errE := strconv.Atoi(r.PostFormValue("evolucao"))
	if errZ != nil || errE != nil || z < 0 || z >= len(level.Zones()) || !evolucaoValida(uint8(e)) {
		http.Error(w, "Zona ou evolução inválida.", http.StatusBadRequest)
		return 0, 0, false
	}
	return int32(z), int32(e), true
}

func evolucaoValida(t uint8) bool {
	for _, v := range level.Tiers() {
		if v == t {
			return true
		}
	}
	return false
}

// cortesDoForm reads the cut rows. A row with an empty level is skipped, which
// is how a cut is removed and how the spare rows stay harmless. The result is
// always non-nil: saving the form means "this table is what I see", and an
// empty table is a real answer, not "use the legacy's".
func cortesDoForm(r *http.Request) ([]domain.XPCut, error) {
	niveis := r.PostForm["corte_nivel"]
	divisores := r.PostForm["corte_divisor"]
	cortes := make([]domain.XPCut, 0, len(niveis))
	for i, raw := range niveis {
		nivel := strings.TrimSpace(raw)
		if nivel == "" {
			continue
		}
		var ate int32
		if strings.EqualFold(nivel, "acima") || nivel == "*" {
			ate = level.CutOpenEnded
		} else {
			n, err := strconv.Atoi(nivel)
			if err != nil || n < 0 || n > int(level.CutOpenEnded) {
				return nil, fmt.Errorf("a linha %d tem um nível inválido: %q", i+1, nivel)
			}
			ate = int32(n)
		}
		var divisor float64
		if i < len(divisores) {
			d, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(divisores[i]), ",", ".", 1), 64)
			if err != nil || d <= 0 {
				return nil, fmt.Errorf("a linha %d precisa de um divisor maior que zero", i+1)
			}
			divisor = d
		}
		if divisor == 0 {
			return nil, fmt.Errorf("a linha %d precisa de um divisor", i+1)
		}
		cortes = append(cortes, domain.XPCut{UpTo: ate, Divisor: divisor})
	}
	return cortes, nil
}

func lerMesaForm(q url.Values) mesaForm {
	f := mesaForm{
		Zona:     intDe(q, "zona", 0, 0, len(level.Zones())-1),
		Nivel:    int32(intDe(q, "nivel", 1, 0, int(level.MaxLevel))),
		MobExp:   int64(intDe(q, "mob_exp", 0, 0, math.MaxInt32)),
		MobNivel: int32(intDe(q, "mob_nivel", 0, 0, int(level.MaxLevel)+1)),
		Bau:      int32(intDe(q, "bau", 0, 0, 400)),
		Grau7:    int32(intDe(q, "grau7", 0, 0, 16)),
		Gemas:    int32(intDe(q, "gemas", 0, 0, 16)),
		Segundos: int32(intDe(q, "segundos", 6, 1, 3600)),
		Fada:     int16(intDe(q, "fada", 0, 0, 4000)),
		Mob:      strings.TrimSpace(q.Get("mob")),
	}
	f.Evolucao = intDe(q, "evolucao", int(level.TierMortal), 1, 3)
	if !evolucaoValida(uint8(f.Evolucao)) {
		f.Evolucao = int(level.TierMortal)
	}
	// The three event switches and the quest gates default to the state a live
	// server is normally in: no events running, Kefra alive, walls already
	// opened — so the first number the page shows is the ordinary one.
	primeira := q.Get("simular") == ""
	f.XPDobro = q.Get("xp_dobro") != ""
	f.Novato = q.Get("novato") != ""
	f.KefraViva = primeira || q.Get("kefra") != ""
	f.Quests = primeira || q.Get("quests") != ""
	return f
}

func intDe(q url.Values, chave string, padrao, piso, teto int) int {
	v, err := strconv.Atoi(strings.TrimSpace(q.Get(chave)))
	if err != nil {
		return padrao
	}
	if v < piso {
		return piso
	}
	if v > teto {
		return teto
	}
	return v
}

// query rebuilds the page's own address, so a save can come back to exactly the
// tab and simulation the moderator was looking at.
func (f mesaForm) query() string {
	q := url.Values{}
	q.Set("zona", strconv.Itoa(f.Zona))
	q.Set("evolucao", strconv.Itoa(f.Evolucao))
	return "?" + q.Encode()
}

// --- the simulation -------------------------------------------------------

type fadaOpcao struct {
	Index int16
	Nome  string
	Bonus int32
}

// fadas are the exp fairies as CMob.cpp:711-732 grants them, plus the Fada
// Suprema's own +30 fairy content (CMob.cpp:1269) which only the Água and field
// branches add — the page shows the total it is worth in the chosen zone.
var fadas = []fadaOpcao{
	{0, "sem fada", 0},
	{3900, "Fada Verde 3D (3900)", 16},
	{3903, "Fada Verde (3903)", 16},
	{3906, "Fada Verde (3906)", 16},
	{3911, "Fada Verde (3911)", 16},
	{3912, "Fada Verde (3912)", 16},
	{3913, "Fada Suprema (3913)", 16},
	{3902, "Fada Vermelha (3902)", 32},
	{3905, "Fada Vermelha (3905)", 32},
	{3908, "Fada Vermelha (3908)", 32},
	{3904, "Fada Verde-Azul (3904)", 32},
	{3907, "Fada Verde-Azul (3907)", 32},
}

func bonusDaFada(idx int16) int32 {
	for _, f := range fadas {
		if f.Index == idx {
			return f.Bonus
		}
	}
	return 0
}

// entrada turns the form into the very call the game makes on a kill. This is
// the whole point of moving internal/level to the repo root: the panel does not
// model the reward, it runs it.
func (f mesaForm) entrada(cfg level.Config) level.ExpRewardInput {
	evo := uint8(f.Evolucao)
	tier := level.Tier{ClassMaster: evo}
	if f.Quests {
		tier.ArchLv355, tier.ArchLv370 = true, true
		tier.CelLv40, tier.CelLv90 = true, true
	}
	// The item bonus is the sum the game keeps in ExpBonus: the chest affect,
	// the fairy, +2 per grade-7 piece and +2 per gem-2 piece
	// (handler/exp_bonus.go, citing CMob.cpp:838 and :870).
	bonus := f.Bau + bonusDaFada(f.Fada) + 2*f.Grau7 + 2*f.Gemas
	var fairyContent int32
	if f.Fada == 3913 {
		fairyContent = 30
	}
	return level.ExpRewardInput{
		Zone:         level.Zone(f.Zona),
		MobExp:       f.MobExp,
		KillerLevel:  f.Nivel,
		MobLevel:     f.MobNivel,
		Tier:         tier,
		ExpBonus:     bonus,
		FairyContent: fairyContent,
		Events: level.ExpEvents{
			DoubleMode: f.XPDobro, NewbieEvent: f.Novato, KefraLive: f.KefraViva,
		},
		Config: cfg,
	}
}

func simularMesa(f mesaForm, cfg level.Config) mesaSimulacao {
	if f.MobExp <= 0 {
		return mesaSimulacao{Aviso: "Escolha um monstro ou digite quanta XP ele dá."}
	}
	in := f.entrada(cfg)
	evo := uint8(f.Evolucao)

	sim := mesaSimulacao{Feita: true, ExpPorMorte: level.ExpReward(in)}
	sim.ExpDoNivel = level.NextLevelExpTier(f.Nivel, evo) - level.ExpTier(f.Nivel, evo)
	if sim.ExpPorMorte > 0 && sim.ExpDoNivel > 0 {
		sim.MortesNivel = (sim.ExpDoNivel + sim.ExpPorMorte - 1) / sim.ExpPorMorte
	}

	plano := level.PlanKills(in, f.Nivel)
	sim.TotalMortes = plano.TotalKills
	sim.Muro = plano.Wall
	sim.AteNivel = plano.Capped
	sim.Tempo = duracao(plano.TotalKills, f.Segundos)
	for _, b := range plano.Bands(bandaPlano) {
		sim.Bandas = append(sim.Bandas, mesaBanda{
			De: b.From, Ate: b.To, ExpPorMorte: b.ExpPerKill, Mortes: b.Kills,
			Tempo: duracao(b.Kills, f.Segundos),
		})
	}
	if sim.ExpPorMorte == 0 {
		sim.Aviso = "Este monstro não paga nada para um personagem deste nível."
	}
	return sim
}

// duracao turns a kill count into something a person can judge. Hours are the
// unit that matters for a grind; days are shown alongside once the number stops
// fitting in an evening.
func duracao(mortes int64, segundosPorMorte int32) string {
	if mortes <= 0 || segundosPorMorte <= 0 {
		return "—"
	}
	total := mortes * int64(segundosPorMorte)
	horas := float64(total) / 3600
	switch {
	case horas < 1:
		return fmt.Sprintf("%d min", int64(horas*60+0.5))
	case horas < 48:
		return fmt.Sprintf("%.1f h", horas)
	default:
		return fmt.Sprintf("%.0f h (%.1f dias)", horas, horas/24)
	}
}

// --- helpers --------------------------------------------------------------

var errSemGameData = errors.New("panel: sem editor de monstros configurado")

// mobExpNivel reads a template's reward and level through the same editor that
// changes them, so the Mesa always simulates the numbers /monstros would show.
func (h *Handler) mobExpNivel(r *http.Request, nome string) (exp int64, nivel int32, err error) {
	if h.cfg.GameData == nil {
		return 0, 0, errSemGameData
	}
	sess, _ := staffFrom(r.Context())
	stat, err := h.cfg.GameData.MobStat(r.Context(), sess.AccountID, nome)
	if err != nil {
		return 0, 0, err
	}
	for _, f := range stat.Fields() {
		switch f.Nome {
		case "exp":
			exp = f.Valor
		case "level":
			nivel = int32(f.Valor)
		}
	}
	return exp, nivel, nil
}

func (h *Handler) mesaConfig(ctx context.Context) (level.Config, error) {
	raw, err := h.cfg.MesaXP.XPConfig(ctx)
	if err != nil {
		return level.Config{}, err
	}
	cfg := level.Config{Version: raw.Version}
	for _, r := range raw.Rules {
		ov := level.Override{RatePercent: r.RatePercent}
		if r.Cuts != nil {
			ov.Cuts = make([]level.Cut, 0, len(r.Cuts))
			for _, c := range r.Cuts {
				ov.Cuts = append(ov.Cuts, level.Cut{UpTo: c.UpTo, Divisor: c.Divisor})
			}
		}
		if cfg.Overrides == nil {
			cfg.Overrides = make(map[level.ConfigKey]level.Override, len(raw.Rules))
		}
		cfg.Overrides[level.ConfigKey{Zone: level.Zone(r.Zone), Tier: uint8(r.Tier)}] = ov
	}
	return cfg, nil
}

func (h *Handler) mesaHistorico(ctx context.Context) ([]audit.Entry, error) {
	return h.cfg.Audit.ListActions(ctx, []string{audit.ActionSetXPRule, audit.ActionClearXPRule})
}

func temRegra(cfg level.Config, zona level.Zone, evo uint8) bool {
	_, ok := cfg.Overrides[level.ConfigKey{Zone: zona, Tier: evo}]
	return ok
}

func cortesParaTela(cortes []level.Cut, comBrancos bool) []mesaCorte {
	out := make([]mesaCorte, 0, len(cortes)+linhasEmBranco)
	for _, c := range cortes {
		linha := mesaCorte{Divisor: strconv.FormatFloat(c.Divisor, 'f', -1, 64)}
		if c.UpTo >= level.CutOpenEnded {
			linha.Ate, linha.Aberto = "acima", true
		} else {
			linha.Ate = strconv.Itoa(int(c.UpTo))
		}
		out = append(out, linha)
	}
	if comBrancos {
		for range linhasEmBranco {
			out = append(out, mesaCorte{})
		}
	}
	return out
}

func abasEvolucao(f mesaForm, cfg level.Config) []mesaAba {
	out := make([]mesaAba, 0, len(level.Tiers()))
	for _, t := range level.Tiers() {
		q := url.Values{"zona": {strconv.Itoa(f.Zona)}, "evolucao": {strconv.Itoa(int(t))}}
		out = append(out, mesaAba{
			Rotulo: level.TierName(t), URL: "/auditoria/xp?" + q.Encode(),
			Ativa:  int(t) == f.Evolucao,
			Tocada: temRegra(cfg, level.Zone(f.Zona), t),
		})
	}
	return out
}

func abasZona(f mesaForm, cfg level.Config) []mesaAba {
	zonas := level.Zones()
	out := make([]mesaAba, 0, len(zonas))
	for _, z := range zonas {
		q := url.Values{"zona": {strconv.Itoa(int(z))}, "evolucao": {strconv.Itoa(f.Evolucao)}}
		out = append(out, mesaAba{
			Rotulo: z.Name(), URL: "/auditoria/xp?" + q.Encode(),
			Ativa:  int(z) == f.Zona,
			Tocada: temRegra(cfg, z, uint8(f.Evolucao)),
		})
	}
	return out
}

// regraParaAudit is what the history shows. It is a plain map rather than the
// domain struct so the log reads as words — "Campo / Mortal" instead of two
// numbers nobody can decode a year later.
func regraParaAudit(r domain.XPRule) map[string]any {
	out := map[string]any{
		"zona":     level.Zone(r.Zone).Name(),
		"evolucao": level.TierName(uint8(r.Tier)),
		"taxa":     fmt.Sprintf("%d%%", r.RatePercent),
	}
	if r.Cuts == nil {
		out["cortes"] = "tabela do legado"
		return out
	}
	if len(r.Cuts) == 0 {
		out["cortes"] = "nenhum corte"
		return out
	}
	partes := make([]string, 0, len(r.Cuts))
	for _, c := range r.Cuts {
		ate := strconv.Itoa(int(c.UpTo))
		if c.UpTo >= level.CutOpenEnded {
			ate = "acima"
		}
		partes = append(partes, fmt.Sprintf("%s÷%s", ate, strconv.FormatFloat(c.Divisor, 'f', -1, 64)))
	}
	out["cortes"] = strings.Join(partes, ", ")
	return out
}

// nomesDeMonstro lists the mob template names for the simulator's picker.
//
// The field used to be free text, and a typo answered "não achei o monstro" —
// which is a fine error and a bad experience, since the names live one screen
// away in /monstros and the operator has no reason to memorise them.
//
// A datalist rather than a select, for two reasons: the roster is in the
// hundreds, so typing three letters and picking beats scrolling; and it degrades
// to exactly the free-text field it replaces, which matters because this panel
// serves no JavaScript and a widget that needed it would simply not work.
//
// A failure here returns nothing rather than an error: the simulator still works
// by typing, and refusing to draw the whole page because the suggestion list is
// unavailable would be the worse trade.
func (h *Handler) nomesDeMonstro(r *http.Request) []string {
	if h.cfg.GameData == nil {
		return nil
	}
	sess, _ := staffFrom(r.Context())
	achados, err := h.cfg.GameData.MobTemplates(r.Context(), sess.AccountID, "")
	if err != nil {
		return nil
	}
	nomes := make([]string, 0, len(achados))
	for _, m := range achados {
		if m.Name != "" {
			nomes = append(nomes, m.Name)
		}
	}
	return nomes
}
