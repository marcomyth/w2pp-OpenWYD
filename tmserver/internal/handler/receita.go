package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mobstat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

// The block recipe kept in the database (0165_receita_de_bloco): who a block
// raises, how many, where and how often. NPCGener.txt ships inside the image, so
// editing it is a deploy and a deploy disconnects everyone; the team still has
// many hunting zones to rework, so a block's recipe can live in the database
// instead, and this file applies it while the server runs.
//
// The rule is the Mesa de Drops one: a row REPLACES what the file says for that
// block, and deleting the row gives the block back to the file. A block from
// domain.NewGeneratorIndexBase up has no file line at all — the panel created it.
//
// WHAT A CHANGE REACHES. The recipe is swapped in place and the block's Rev goes
// up; nothing already standing is touched. The next group the block raises uses
// the new recipe, and a monster of the old recipe that dies comes back from the
// new one (world.SpawnDueRespawns compares its GenRev). So a zone turns over as it
// is hunted, the way a drop change lands on the next kill. `renovar` is the
// exception the panel asks for explicitly: the block's live mobs are taken out and
// raised again now.

const (
	recipePollPeriod = 15
	recipeTimeout    = 5 * time.Second
	// recipeBootTimeout is longer: the boot read also resolves every template a
	// row names, from disk, before the first player can connect.
	recipeBootTimeout = 15 * time.Second
)

// GeneratorRecipeSource is the block recipes, read live from dbServer.
type GeneratorRecipeSource interface {
	Version(ctx context.Context) (int64, error)
	Snapshot(ctx context.Context) (domain.GeneratorRecipeConfig, error)
}

// RecipeTemplateLoader reads the raw npc/<name> STRUCT_MOB a recipe names and
// says which file the name resolved to — the name the Mesa de Drops and the
// sheet reload key on (npctemplate.Resolve).
type RecipeTemplateLoader func(name string) (raw []byte, file string, err error)

// receitaAplicada is what a database row put in force on one block: the recipe,
// and the renovar counter it was applied at.
type receitaAplicada struct {
	rec     npcgener.Generator
	renovar int64
}

// moldeLido is one template read off the loop for the apply.
type moldeLido struct {
	raw  []byte
	file string
	err  error
}

// SetRecipeBase hands the dispatcher the file's blocks, in file order — what a
// block goes back to when its row is deleted. Wiring-time, after spawnNPCs.
func (d *Dispatcher) SetRecipeBase(base []npcgener.Generator) { d.recipeBase = base }

// ApplyGeneratorRecipesBoot applies the database recipes before the loop takes
// players. It runs right after the boot populate and replaces what the populate
// raised from the file for every block the database changes: with nobody
// connected that is invisible, and it keeps a restart from filling an edited zone
// with the old monsters until they die.
//
// A failed read is not fatal: the file is how the server ran before the table
// existed, and the poll retries.
func (d *Dispatcher) ApplyGeneratorRecipesBoot(w *world.World) {
	if d.recipeSource == nil || d.recipeTemplate == nil || d.recipeBase == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), recipeBootTimeout)
	defer cancel()
	cfg, err := d.recipeSource.Snapshot(ctx)
	if err != nil {
		d.log.Warn("receitas de bloco não lidas no boot (o poll tenta de novo)", "err", err)
		return
	}
	moldes := lerMoldes(d.recipeTemplate, nomesAResolver(cfg, d.recipeBase, nil))
	n := d.applyGeneratorRecipes(w, cfg, moldes, true)
	d.log.Info("receitas de bloco aplicadas no boot", "version", cfg.Version,
		"linhas", len(cfg.Recipes), "trocados", n)
}

// pollGeneratorRecipes reloads the recipes when the version moves. The gRPC call
// and the template reads run OFF the loop; only the swap re-enters it.
func (d *Dispatcher) pollGeneratorRecipes(w *world.World) {
	if d.recipeSource == nil || d.recipeTemplate == nil || d.recipeBase == nil || d.recipePolling {
		return
	}
	d.recipePollTick++
	if d.recipePollTick%recipePollPeriod != 0 {
		return
	}
	known, src, carregar, base := d.recipeVersion, d.recipeSource, d.recipeTemplate, d.recipeBase
	// A copy, read off the loop to decide which templates to read. The loop keeps
	// writing the real map; this one is never touched again in here.
	aplicadas := make(map[int]receitaAplicada, len(d.recipeApplied))
	for idx, a := range d.recipeApplied {
		aplicadas[idx] = a
	}
	d.recipePolling = true
	w.GoDetached(func() func(*world.World) {
		ctx, cancel := context.WithTimeout(context.Background(), recipeTimeout)
		defer cancel()
		version, err := src.Version(ctx)
		if err != nil {
			return func(*world.World) {
				d.recipePolling = false
				d.log.Warn("receitas de bloco: versão não lida", "err", err)
			}
		}
		if version == known {
			return func(*world.World) { d.recipePolling = false }
		}
		cfg, err := src.Snapshot(ctx)
		if err != nil {
			return func(*world.World) {
				d.recipePolling = false
				// The recipes in force stay. Dropping every edited zone back to the
				// file because of a network blip is a server-wide change nobody made.
				d.log.Warn("receitas de bloco: recarga falhou; mantendo as que estão valendo", "err", err)
			}
		}
		moldes := lerMoldes(carregar, nomesAResolver(cfg, base, aplicadas))
		return func(w *world.World) {
			d.recipePolling = false
			n := d.applyGeneratorRecipes(w, cfg, moldes, false)
			d.log.Info("receitas de bloco recarregadas", "version", cfg.Version,
				"linhas", len(cfg.Recipes), "trocados", n)
		}
	})
}

// forceRecipeReload makes the next tick re-read the recipes.
func (d *Dispatcher) forceRecipeReload() {
	if d.recipeSource == nil {
		return
	}
	d.recipeVersion = -1
	d.recipePollTick = recipePollPeriod - 1
}

// receitaDoBanco turns a row into the file's own shape. The fight and death
// lines are not in the table: a block of the file keeps its own.
func receitaDoBanco(r domain.GeneratorRecipe, base []npcgener.Generator) npcgener.Generator {
	g := npcgener.Generator{
		Leader: r.Leader, Follower: r.Follower,
		MinuteGenerate: int(r.MinuteGenerate), MinGroup: int(r.MinGroup), MaxGroup: int(r.MaxGroup),
		MaxNumMob: int(r.MaxNumMob), RouteType: int(r.RouteType), Formation: int(r.Formation),
	}
	for i := 0; i < 5; i++ {
		g.SegX[i], g.SegY[i] = int16(r.SegX[i]), int16(r.SegY[i])
		g.SegRange[i], g.SegWait[i] = int(r.SegRange[i]), int(r.SegWait[i])
	}
	if idx := int(r.Index); idx >= 0 && idx < len(base) {
		g.FightAction, g.DieAction = base[idx].FightAction, base[idx].DieAction
	}
	return g
}

// nomesAResolver lists the templates an apply may need: every name a row gives,
// and — for a row that went away — the file's own names, which is what the block
// goes back to. aplicadas nil (the boot) means nothing was applied before.
func nomesAResolver(cfg domain.GeneratorRecipeConfig, base []npcgener.Generator, aplicadas map[int]receitaAplicada) []string {
	set := map[string]struct{}{}
	add := func(n string) {
		if n != "" {
			set[n] = struct{}{}
		}
	}
	naLinha := make(map[int]bool, len(cfg.Recipes))
	for _, r := range cfg.Recipes {
		naLinha[int(r.Index)] = true
		if a, ok := aplicadas[int(r.Index)]; ok && a.rec == receitaDoBanco(r, base) {
			continue // unchanged: its bytes are already on the block
		}
		add(r.Leader)
		add(r.Follower)
	}
	for idx := range aplicadas {
		if !naLinha[idx] && idx < len(base) {
			add(base[idx].Leader)
			add(base[idx].Follower)
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// lerMoldes reads each template once. Off the loop: it is disk.
func lerMoldes(carregar RecipeTemplateLoader, nomes []string) map[string]moldeLido {
	out := make(map[string]moldeLido, len(nomes))
	for _, n := range nomes {
		raw, file, err := carregar(n)
		out[n] = moldeLido{raw: raw, file: file, err: err}
	}
	return out
}

// applyGeneratorRecipes brings every block to what the snapshot says and returns
// how many it changed. boot=true replaces what the populate raised for each block
// it changes; otherwise only a block the panel asked to renew is replaced now.
// Loop-only.
func (d *Dispatcher) applyGeneratorRecipes(w *world.World, cfg domain.GeneratorRecipeConfig, moldes map[string]moldeLido, boot bool) int {
	if d.recipeApplied == nil {
		d.recipeApplied = make(map[int]receitaAplicada)
	}
	linhas := make(map[int]domain.GeneratorRecipe, len(cfg.Recipes))
	for _, r := range cfg.Recipes {
		linhas[int(r.Index)] = r
	}
	indices := make([]int, 0, len(linhas)+len(d.recipeApplied))
	for idx := range linhas {
		indices = append(indices, idx)
	}
	for idx := range d.recipeApplied {
		if _, ok := linhas[idx]; !ok {
			indices = append(indices, idx)
		}
	}
	// In index order: a run that raises groups spends the parity RNG, and map
	// order would make two restarts over the same table spend it differently.
	sort.Ints(indices)

	n := 0
	for _, idx := range indices {
		r, naLinha := linhas[idx]
		antes, tinha := d.recipeApplied[idx]
		switch {
		case naLinha:
			rec := receitaDoBanco(r, d.recipeBase)
			// What the block runs now: the last row applied, or else the file.
			// A row equal to the file changes nothing, and must not move Rev —
			// that would send every queued death of the block through a
			// regeneration for no reason.
			vigente, conhecida := antes.rec, tinha
			if !tinha && idx < len(d.recipeBase) {
				vigente, conhecida = d.recipeBase[idx], true
			}
			mudou := !conhecida || vigente != rec
			pediuRenovar := r.Renovar != antes.renovar
			if !mudou && !pediuRenovar && tinha {
				continue
			}
			if !d.validaIndiceDeReceita(idx) {
				continue
			}
			novo := w.GeneratorAt(idx) == nil
			if mudou {
				if !d.aplicarReceita(w, idx, rec, moldes) {
					continue
				}
				n++
			}
			d.recipeApplied[idx] = receitaAplicada{rec: rec, renovar: r.Renovar}
			// At boot every changed block is raised again from its recipe (the
			// populate raised it from the file); later, only a block that did not
			// exist, or one the panel asked to renew.
			if (boot && mudou) || (!boot && (novo || pediuRenovar)) {
				d.renovarBloco(w, idx, !boot)
			}
		case idx < len(d.recipeBase):
			// The row went away: the block goes back to the file, lazily, like
			// any other change.
			if antes.rec != d.recipeBase[idx] && d.aplicarReceita(w, idx, d.recipeBase[idx], moldes) {
				n++
			}
			delete(d.recipeApplied, idx)
		default:
			// A block only the database had: it leaves the world.
			d.retirarBloco(w, idx)
			delete(d.recipeApplied, idx)
			n++
		}
	}
	d.recipeVersion = cfg.Version
	return n
}

// validaIndiceDeReceita refuses a row for an index the server cannot honor. The
// panel refuses the same; this is what keeps a hand-written row from landing.
func (d *Dispatcher) validaIndiceDeReceita(idx int) bool {
	if idx < len(d.recipeBase) || (idx >= domain.NewGeneratorIndexBase && idx <= domain.MaxGeneratorIndex) {
		return true
	}
	d.log.Warn("receita de bloco num índice fora das duas faixas, ignorada",
		"bloco", idx, "blocos_do_arquivo", len(d.recipeBase), "novos_a_partir_de", domain.NewGeneratorIndexBase)
	return false
}

// aplicarReceita swaps block idx to rec and reports whether it did. A leader
// template that does not resolve keeps the block as it was: a zone that goes
// empty because of a typo in a name is worse than one that did not change.
func (d *Dispatcher) aplicarReceita(w *world.World, idx int, rec npcgener.Generator, moldes map[string]moldeLido) bool {
	g := w.GeneratorAt(idx)
	if g != nil && g.DBManaged {
		// The NPC panel owns this block (npcconfig.go) and would put its own
		// recipe back on the next reload.
		d.log.Warn("receita de bloco num mercador do painel de NPCs, ignorada", "bloco", idx, "leader", rec.Leader)
		return false
	}
	lider, ok := moldes[rec.Leader]
	if !ok || lider.err != nil || lider.raw == nil {
		d.log.Warn("receita de bloco: molde do líder não carregou, o bloco ficou como estava",
			"bloco", idx, "leader", rec.Leader, "err", lider.err)
		return false
	}
	var seguidor moldeLido
	if rec.Follower != "" {
		seguidor = moldes[rec.Follower]
		if seguidor.err != nil || seguidor.raw == nil {
			// The boot's own rule: a missing follower degrades the block to
			// leader-only groups instead of losing it.
			d.log.Warn("receita de bloco: molde do seguidor não carregou, o bloco gera só o líder",
				"bloco", idx, "follower", rec.Follower, "err", seguidor.err)
			seguidor = moldeLido{}
		}
	}
	if g == nil {
		g = &world.Generator{}
		w.SetGenerator(idx, g)
	}
	g.Name = rec.Leader
	g.MinuteGenerate, g.MinGroup, g.MaxGroup, g.MaxNumMob = rec.MinuteGenerate, rec.MinGroup, rec.MaxGroup, rec.MaxNumMob
	g.RouteType, g.Formation = uint8(rec.RouteType), rec.Formation
	g.SegX, g.SegY = rec.SegX, rec.SegY
	for s := 0; s < 5; s++ {
		g.SegRange[s], g.SegWait[s] = int16(rec.SegRange[s]), int16(rec.SegWait[s])
	}
	g.FightAction, g.DieAction = rec.FightAction, rec.DieAction
	g.LeaderTmpl, g.LeaderName = d.comFicha(lider), lider.file
	g.FollowerTmpl, g.FollowerName = d.comFicha(seguidor), seguidor.file
	g.Rev++
	// Both tables are keyed by what a block is and where it is born, and are
	// rebuilt on their next use.
	d.genAreas, d.genChefe = nil, nil
	return true
}

// comFicha puts the /monstros sheet in force over a template's file bytes, by
// the file name first — the key the sheet reload swaps blocks by.
func (d *Dispatcher) comFicha(m moldeLido) []byte {
	if m.raw == nil {
		return nil
	}
	return mobstat.ApplyOverride(m.raw, m.file, d.mobStatOverrides)
}

// retirarBloco takes a block only the database had out of the world: its mobs
// go, and it can raise nothing more. The slot stays, empty, because a mob still
// on its way through the respawn queue names it.
func (d *Dispatcher) retirarBloco(w *world.World, idx int) {
	g := w.GeneratorAt(idx)
	if g == nil {
		return
	}
	w.ClearGenerator(idx)
	g.LeaderTmpl, g.FollowerTmpl = nil, nil
	g.Rev++
	d.genAreas, d.genChefe = nil, nil
}

// renovarBloco takes a block's live mobs out and raises it again from its
// recipe. It reports what it did in the same words /gm renovar answers with.
// Blocks an event or a dungeon run owns are left to their owner: clearing one
// would empty a room under a party.
func (d *Dispatcher) renovarBloco(w *world.World, idx int, reveal bool) (tirados, gerados int, motivo string) {
	g := w.GeneratorAt(idx)
	switch {
	case g == nil:
		return 0, 0, "não existe"
	case g.Off:
		return 0, 0, "está desligado"
	case g.DBManaged:
		return 0, 0, "é NPC do painel"
	case world.IsWaterDungeonGenerator(idx), world.IsEventOwnedGenerator(idx), world.IsKefraGenerator(idx):
		return 0, 0, "é de evento ou masmorra; quem gera é o evento"
	}
	w.ForEachMob(func(_ int, m *world.Entity) {
		if int(m.GenIndex) == idx {
			tirados++
		}
	})
	w.ClearGenerator(idx)
	ids := w.GenerateMob(idx)
	if reveal {
		d.revealSpawned(w, ids)
	}
	return tirados, len(ids), ""
}

// renovarCmd is /gm renovar <bloco>: the block's live mobs out, a new group in,
// from the recipe it has now.
func (d *Dispatcher) renovarCmd(w *world.World, o blocoOrigem, args []string) []string {
	if len(args) == 0 {
		return []string{"Uso: renovar <bloco>"}
	}
	idx, err := strconv.Atoi(args[0])
	if err != nil || w.GeneratorAt(idx) == nil {
		return []string{fmt.Sprintf("Bloco %q não existe.", args[0])}
	}
	tirados, gerados, motivo := d.renovarBloco(w, idx, true)
	if motivo != "" {
		return []string{fmt.Sprintf("Bloco #%d %s.", idx, motivo)}
	}
	d.log.Info("renew block", "by", o.by, "index", idx, "removed", tirados, "raised", gerados)
	return []string{fmt.Sprintf("Bloco #%d %s renovado: %d tirados, %d gerados.",
		idx, w.GeneratorAt(idx).Name, tirados, gerados)}
}
