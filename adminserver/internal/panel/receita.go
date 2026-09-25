package panel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/adminserver/internal/audit"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
)

// Receitas is the block recipes kept in the database (0165_receita_de_bloco),
// satisfied by *store.Store. A row replaces what NPCGener.txt says for one block;
// the game re-reads the table every ~15 s, so a zone changes without a restart.
type Receitas interface {
	GeneratorRecipes(ctx context.Context) (domain.GeneratorRecipeConfig, error)
	SetGeneratorRecipe(ctx context.Context, rec domain.GeneratorRecipe, renovar bool, moderatorID int64) (domain.GeneratorRecipe, bool, error)
	DeleteGeneratorRecipe(ctx context.Context, index int32, moderatorID int64) (domain.GeneratorRecipe, bool, error)
}

// MoldeExiste says whether a template name resolves to a file in the content
// tree, and to which one. The game refuses a recipe whose leader does not load
// and keeps the block as it was; refusing it here says so to the person who
// typed it instead of to the log.
type MoldeExiste func(name string) (file string, ok bool)

// Limits of the form. The database CHECKs are looser, so a block of the file
// can always be saved back as it is; these are what a person may type.
const (
	receitaMinutoMax = 1000
	receitaGrupoMax  = 100
	receitaVivosMax  = 1000
	receitaRotaMax   = 6 // the file's own highest RouteType
	receitaFormMax   = 4 // g_pFormation has five rows
	receitaRaioMax   = 50
	receitaEsperaMax = 600
	receitaCoordMax  = 4095
	receitaNotaMax   = 200
)

// nomesDosPontos are the five legacy waypoints, in the order the file keeps them.
var nomesDosPontos = [5]string{"Início", "Ponto 1", "Ponto 2", "Ponto 3", "Destino"}

// pontoForm is one waypoint as the form shows it.
type pontoForm struct {
	I                int
	Nome             string
	X, Y, Raio, Espe int32
}

// receitaForm is the recipe page: what the block runs, where it came from, and
// the file's version beside it when there is one.
type receitaForm struct {
	Bloco     int32
	Novo      bool // a block only the database has (NewGeneratorIndexBase and up)
	NoBanco   bool // there is a row: the page shows the database's recipe
	DoArquivo bool // the file has this block
	Leader    string
	Follower  string
	Minuto    int32
	MinGrupo  int32
	MaxGrupo  int32
	Vivos     int32
	Rota      int32
	Formacao  int32
	Pontos    []pontoForm
	Nota      string
	// Arquivo is the file's line for this block, shown beside the form so a
	// change reads as a change. Nil for a new block.
	Arquivo *domain.GeneratorRecipe
}

// receitaLinha is one row of the list.
type receitaLinha struct {
	Bloco            int32
	Novo             bool
	Leader, Follower string
	Vivos            int32
	X, Y             int32
	Nota             string
	Arquivo          string // the file's leader for this block, when it differs
}

// receitaDoArquivo is the file's block idx as a recipe, false when the file has
// no such block.
func (h *Handler) receitaDoArquivo(idx int32) (domain.GeneratorRecipe, bool) {
	if idx < 0 || int(idx) >= len(h.cfg.NPCGener) {
		return domain.GeneratorRecipe{}, false
	}
	return receitaDeGerador(idx, h.cfg.NPCGener[idx]), true
}

func receitaDeGerador(idx int32, g npcgener.Generator) domain.GeneratorRecipe {
	r := domain.GeneratorRecipe{
		Index: idx, Leader: g.Leader, Follower: g.Follower,
		MinuteGenerate: int32(g.MinuteGenerate), MinGroup: int32(g.MinGroup), MaxGroup: int32(g.MaxGroup),
		MaxNumMob: int32(g.MaxNumMob), RouteType: int32(g.RouteType), Formation: int32(g.Formation),
	}
	for i := 0; i < 5; i++ {
		r.SegX[i], r.SegY[i] = int32(g.SegX[i]), int32(g.SegY[i])
		r.SegRange[i], r.SegWait[i] = int32(g.SegRange[i]), int32(g.SegWait[i])
	}
	return r
}

// indiceDeReceitaValido is the panel's half of the index rule: a block of the
// file, or a new one from NewGeneratorIndexBase up. Between the two is where
// the file grows, and a row there would be taken by the next appended block.
func (h *Handler) indiceDeReceitaValido(idx int32) bool {
	return (idx >= 0 && int(idx) < len(h.cfg.NPCGener)) ||
		(idx >= domain.NewGeneratorIndexBase && idx <= domain.MaxGeneratorIndex)
}

// receitas lists every block the database changes or created.
func (h *Handler) receitas(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.cfg.Receitas.GeneratorRecipes(r.Context())
	if err != nil {
		h.cfg.Logger.Error("block recipes read failed", "err", err)
		http.Error(w, "Erro ao ler as receitas de bloco.", http.StatusInternalServerError)
		return
	}
	var linhas []receitaLinha
	for _, rec := range cfg.Recipes {
		l := receitaLinha{
			Bloco: rec.Index, Novo: rec.Index >= domain.NewGeneratorIndexBase,
			Leader: rec.Leader, Follower: rec.Follower, Vivos: rec.MaxNumMob,
			X: rec.SegX[0], Y: rec.SegY[0], Nota: rec.Nota,
		}
		if arq, ok := h.receitaDoArquivo(rec.Index); ok && arq.Leader != rec.Leader {
			l.Arquivo = arq.Leader
		}
		linhas = append(linhas, l)
	}
	h.render(w, "receitas.html", struct {
		page
		Linhas    []receitaLinha
		Arquivo   int
		Base      int
		Aviso     string
		Historico []audit.Entry
	}{
		page: h.pageFor(r, "receitas"), Linhas: linhas, Arquivo: len(h.cfg.NPCGener),
		Base: domain.NewGeneratorIndexBase, Aviso: r.URL.Query().Get("aviso"),
		Historico: h.receitasHistorico(r.Context()),
	})
}

// receita shows one block's recipe as a form: ?bloco=N for a block that exists,
// ?novo=1 for a new one, and &de=N to start the new one as a copy of block N.
func (h *Handler) receita(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cfg, err := h.cfg.Receitas.GeneratorRecipes(r.Context())
	if err != nil {
		h.cfg.Logger.Error("block recipes read failed", "err", err)
		http.Error(w, "Erro ao ler as receitas de bloco.", http.StatusInternalServerError)
		return
	}
	noBanco := make(map[int32]domain.GeneratorRecipe, len(cfg.Recipes))
	for _, rec := range cfg.Recipes {
		noBanco[rec.Index] = rec
	}

	var idx int32
	if q.Get("novo") != "" {
		idx = proximoBlocoNovo(cfg)
	} else {
		n, err := strconv.Atoi(strings.TrimSpace(q.Get("bloco")))
		if err != nil || !h.indiceDeReceitaValido(int32(n)) {
			http.Error(w, fmt.Sprintf("Bloco %q não existe. Os do arquivo vão de 0 a %d; os novos, de %d para cima.",
				q.Get("bloco"), len(h.cfg.NPCGener)-1, domain.NewGeneratorIndexBase), http.StatusNotFound)
			return
		}
		idx = int32(n)
	}

	arquivo, doArquivo := h.receitaDoArquivo(idx)
	rec, tem := noBanco[idx]
	switch {
	case tem:
	case doArquivo:
		rec = arquivo
	case idx >= domain.NewGeneratorIndexBase:
		// A new block: a copy of another one when asked, or a small group that
		// respawns one by one — the shape most hunting blocks have.
		rec = domain.GeneratorRecipe{Index: idx, MaxNumMob: 5, MinGroup: 0, MaxGroup: 0}
		if de, err := strconv.Atoi(q.Get("de")); err == nil {
			if origem, ok := noBanco[int32(de)]; ok {
				rec = origem
			} else if origem, ok := h.receitaDoArquivo(int32(de)); ok {
				rec = origem
			}
			rec.Index, rec.Renovar, rec.Nota = idx, 0, ""
		}
	}

	form := receitaForm{
		Bloco: idx, Novo: idx >= domain.NewGeneratorIndexBase, NoBanco: tem, DoArquivo: doArquivo,
		Leader: rec.Leader, Follower: rec.Follower, Minuto: rec.MinuteGenerate,
		MinGrupo: rec.MinGroup, MaxGrupo: rec.MaxGroup, Vivos: rec.MaxNumMob,
		Rota: rec.RouteType, Formacao: rec.Formation, Nota: rec.Nota,
	}
	for i := 0; i < 5; i++ {
		form.Pontos = append(form.Pontos, pontoForm{I: i, Nome: nomesDosPontos[i],
			X: rec.SegX[i], Y: rec.SegY[i], Raio: rec.SegRange[i], Espe: rec.SegWait[i]})
	}
	if doArquivo {
		form.Arquivo = &arquivo
	}
	h.render(w, "receita.html", struct {
		page
		Form     receitaForm
		Aviso    string
		TemJogo  bool
		MinutoMx int
	}{
		page: h.pageFor(r, "receitas"), Form: form, Aviso: q.Get("aviso"),
		TemJogo: h.cfg.Blocos != nil, MinutoMx: receitaMinutoMax,
	})
}

// proximoBlocoNovo is the first free index from NewGeneratorIndexBase up.
func proximoBlocoNovo(cfg domain.GeneratorRecipeConfig) int32 {
	usados := make([]int32, 0, len(cfg.Recipes))
	for _, r := range cfg.Recipes {
		usados = append(usados, r.Index)
	}
	sort.Slice(usados, func(i, j int) bool { return usados[i] < usados[j] })
	next := int32(domain.NewGeneratorIndexBase)
	for _, u := range usados {
		if u == next {
			next++
		}
	}
	return next
}

// setReceita saves one block's recipe.
func (h *Handler) setReceita(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	rec, problema := h.receitaDoForm(r)
	if problema != "" {
		http.Error(w, problema, http.StatusBadRequest)
		return
	}
	renovar := r.PostFormValue("renovar") == "1"
	antes, tinha, err := h.cfg.Receitas.SetGeneratorRecipe(r.Context(), rec, renovar, sess.AccountID)
	if err != nil {
		h.cfg.Logger.Error("block recipe save failed", "bloco", rec.Index, "err", err)
		http.Error(w, "Erro ao gravar a receita do bloco.", http.StatusInternalServerError)
		return
	}
	var old map[string]any
	if tinha {
		old = receitaParaAudit(antes)
	} else if arq, ok := h.receitaDoArquivo(rec.Index); ok {
		old = receitaParaAudit(arq)
		old["origem"] = "NPCGener.txt"
	}
	novo := receitaParaAudit(rec)
	if renovar {
		novo["renovar"] = "trocar os vivos agora"
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionSetBlockRecipe, Old: old, New: novo,
	}); err != nil {
		h.auditoriaFalhou(w, err)
		return
	}
	aviso := fmt.Sprintf("Bloco #%d gravado. Em até 15 s o jogo usa a receita nova para quem nascer; "+
		"os que já estão no mapa ficam até morrer.", rec.Index)
	if renovar {
		aviso = fmt.Sprintf("Bloco #%d gravado. Em até 15 s o jogo tira os vivos do bloco e gera de novo pela receita nova.", rec.Index)
	}
	voltarParaReceita(w, r, rec.Index, aviso)
}

// limparReceita deletes one block's row: a block of the file goes back to it, a
// new block leaves the world.
func (h *Handler) limparReceita(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !h.checkCSRF(w, r) {
		if err != nil {
			http.Error(w, "Formulário ilegível.", http.StatusBadRequest)
		}
		return
	}
	sess, _ := staffFrom(r.Context())
	n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("bloco")))
	if err != nil || !h.indiceDeReceitaValido(int32(n)) {
		http.Error(w, "Bloco desconhecido.", http.StatusBadRequest)
		return
	}
	idx := int32(n)
	antes, tinha, err := h.cfg.Receitas.DeleteGeneratorRecipe(r.Context(), idx, sess.AccountID)
	if err != nil {
		h.cfg.Logger.Error("block recipe clear failed", "bloco", idx, "err", err)
		http.Error(w, "Erro ao apagar a receita do bloco.", http.StatusInternalServerError)
		return
	}
	var old map[string]any
	if tinha {
		old = receitaParaAudit(antes)
	}
	if err := h.cfg.Audit.Write(r.Context(), audit.Record{
		ActorID: sess.AccountID, AtorPainelID: sess.PainelUsuarioID, ActorRole: roleFrom(r.Context()),
		Action: audit.ActionClearBlockRecipe, Old: old,
		New: map[string]any{"bloco": idx},
	}); err != nil {
		h.auditoriaFalhou(w, err)
		return
	}
	if idx >= domain.NewGeneratorIndexBase {
		http.Redirect(w, r, "/blocos/receitas?aviso="+url.QueryEscape(fmt.Sprintf(
			"Bloco novo #%d apagado. Em até 15 s os mobs dele saem do mapa.", idx)), http.StatusSeeOther)
		return
	}
	voltarParaReceita(w, r, idx, fmt.Sprintf(
		"Bloco #%d voltou ao NPCGener. Em até 15 s vale para quem nascer; os vivos ficam até morrer.", idx))
}

func voltarParaReceita(w http.ResponseWriter, r *http.Request, idx int32, aviso string) {
	q := url.Values{"bloco": {strconv.Itoa(int(idx))}, "aviso": {aviso}}
	http.Redirect(w, r, "/blocos/receita?"+q.Encode(), http.StatusSeeOther)
}

// receitaDoForm reads and checks the form. The message is for the person who
// filled it in, and names the field.
func (h *Handler) receitaDoForm(r *http.Request) (domain.GeneratorRecipe, string) {
	var rec domain.GeneratorRecipe
	num := func(campo, nome string, lo, hi int) (int32, string) {
		v := strings.TrimSpace(r.PostFormValue(campo))
		if v == "" {
			v = "0"
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < lo || n > hi {
			return 0, fmt.Sprintf("%s precisa ser um número entre %d e %d.", nome, lo, hi)
		}
		return int32(n), ""
	}

	n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("bloco")))
	if err != nil || !h.indiceDeReceitaValido(int32(n)) {
		return rec, fmt.Sprintf("Bloco desconhecido. Os do arquivo vão de 0 a %d; os novos, de %d a %d.",
			len(h.cfg.NPCGener)-1, domain.NewGeneratorIndexBase, domain.MaxGeneratorIndex)
	}
	rec.Index = int32(n)

	rec.Leader = strings.TrimSpace(r.PostFormValue("leader"))
	rec.Follower = strings.TrimSpace(r.PostFormValue("follower"))
	if rec.Follower == "0" {
		rec.Follower = "" // the file's own way of saying "no follower"
	}
	if rec.Leader == "" {
		return rec, "Falta o monstro líder (o nome do arquivo em npc/)."
	}
	for _, nome := range []string{rec.Leader, rec.Follower} {
		if nome == "" {
			continue
		}
		if strings.ContainsAny(nome, " \t/\\") {
			return rec, fmt.Sprintf("%q não é um nome de molde: sem espaços nem barras.", nome)
		}
		if h.cfg.MoldeExiste != nil {
			if _, ok := h.cfg.MoldeExiste(nome); !ok {
				return rec, fmt.Sprintf("Não existe molde %q em npc/. Confira o nome na tela de Monstros.", nome)
			}
		}
	}

	var problema string
	campos := []struct {
		dst      *int32
		campo    string
		nome     string
		min, max int
	}{
		{&rec.MinuteGenerate, "minuto", "O ritmo (MinuteGenerate)", -1, receitaMinutoMax},
		{&rec.MinGroup, "min_grupo", "O mínimo de seguidores", 0, receitaGrupoMax},
		{&rec.MaxGroup, "max_grupo", "O máximo de seguidores", 0, receitaGrupoMax},
		{&rec.MaxNumMob, "vivos", "O teto de vivos (MaxNumMob)", -1, receitaVivosMax},
		{&rec.RouteType, "rota", "O tipo de rota", 0, receitaRotaMax},
		{&rec.Formation, "formacao", "A formação", 0, receitaFormMax},
	}
	for _, c := range campos {
		if *c.dst, problema = num(c.campo, c.nome, c.min, c.max); problema != "" {
			return rec, problema
		}
	}
	for i := 0; i < 5; i++ {
		p := nomesDosPontos[i]
		s := strconv.Itoa(i)
		if rec.SegX[i], problema = num("x"+s, "O X de "+p, 0, receitaCoordMax); problema != "" {
			return rec, problema
		}
		if rec.SegY[i], problema = num("y"+s, "O Y de "+p, 0, receitaCoordMax); problema != "" {
			return rec, problema
		}
		if rec.SegRange[i], problema = num("raio"+s, "O raio de "+p, 0, receitaRaioMax); problema != "" {
			return rec, problema
		}
		if rec.SegWait[i], problema = num("espera"+s, "A espera de "+p, -1, receitaEsperaMax); problema != "" {
			return rec, problema
		}
	}
	// The generator anchors on the first waypoint that is set; with none the
	// block never raises anything, and nothing anywhere would say why.
	if rec.SegX[0] == 0 || rec.SegY[0] == 0 {
		return rec, "O Início precisa de X e Y: é onde o grupo nasce."
	}
	rec.Nota = strings.TrimSpace(r.PostFormValue("nota"))
	if len([]rune(rec.Nota)) > receitaNotaMax {
		return rec, fmt.Sprintf("A nota passa de %d letras.", receitaNotaMax)
	}
	return rec, ""
}

// receitaParaAudit is the log entry in the words of the form.
func receitaParaAudit(r domain.GeneratorRecipe) map[string]any {
	out := map[string]any{
		"bloco":  r.Index,
		"lider":  r.Leader,
		"vivos":  r.MaxNumMob,
		"grupo":  fmt.Sprintf("%d-%d", r.MinGroup, r.MaxGroup),
		"minuto": r.MinuteGenerate,
		"inicio": fmt.Sprintf("%d,%d r%d", r.SegX[0], r.SegY[0], r.SegRange[0]),
	}
	if r.Follower != "" {
		out["seguidor"] = r.Follower
	}
	if r.SegX[4] != 0 {
		out["destino"] = fmt.Sprintf("%d,%d r%d", r.SegX[4], r.SegY[4], r.SegRange[4])
	}
	if r.Nota != "" {
		out["nota"] = r.Nota
	}
	return out
}

// receitasHistorico is the recipes' own slice of the audit log.
func (h *Handler) receitasHistorico(ctx context.Context) []audit.Entry {
	lista, err := h.cfg.Audit.ListActions(ctx, []string{audit.ActionSetBlockRecipe, audit.ActionClearBlockRecipe})
	if err != nil {
		h.cfg.Logger.Error("block recipe history failed", "err", err)
		return nil
	}
	return lista
}

// temReceitas says whether the recipe pages exist: they need the table to
// write and the file's blocks to start the form from.
func (h *Handler) temReceitas() bool {
	return h.cfg.Receitas != nil && len(h.cfg.NPCGener) > 0
}
