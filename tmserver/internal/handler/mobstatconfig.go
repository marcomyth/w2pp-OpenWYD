package handler

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mobstat"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/world"
)

const (
	// mobStatPollPeriod is how many ticks between version checks — the same
	// cadence the Mesa de XP, the doors and the spawn pacing use.
	mobStatPollPeriod = 15
	mobStatTimeout    = 5 * time.Second
)

// MobStatSource is the moderator-edited monster sheet (mob_template_stat), read
// live from dbServer.
//
// The boot read is NOT here: it happens in main.go, synchronously, because the
// sheets have to be applied to the templates before the first generator is built.
// This interface is what comes after.
//
// WHY THIS EXISTS: the sheet used to be boot-only, and that cost a night of
// work. A moderator raised a monster's EXP at 21:18, the game kept paying the old
// value, and the edit only took effect at 22:19 when an unrelated deploy happened
// to restart production. "Grave e reinicie" is not usable when reiniciar means
// disconnecting everyone who is playing.
type MobStatSource interface {
	Version(ctx context.Context) (int64, error)
	Fetch(ctx context.Context) (map[string]mobstat.Override, error)
}

// MobTemplateLoader reads a raw npc/<name> STRUCT_MOB from the content tree. The
// reload needs it to rebuild a sheet from the FILE plus the current override —
// which is also what makes deleting an override restore the file's values.
type MobTemplateLoader func(name string) ([]byte, error)

// pollMobStats reloads the monster sheets when the saved version moves.
//
// The gRPC call and the file reads run OFF the loop (GoDetached) and only the
// swap re-enters it, for the reason every other reload here does it that way: the
// loop is single-owner and is drained ahead of player input, so a disk read
// inside it is a stutter everybody feels. There are 2.014 template files in the
// content tree (8,7 MB), which is exactly why the rebuild is limited to the names
// that actually changed instead of the whole tree.
//
// THE VERSION IS SHARED with the NPC/shop config (npc_config_meta): every
// mob-stat mutation already bumps it (store.UpsertMobTemplateStat and
// DeleteMobTemplateStat both end in auditAndBump). The cost is that an NPC or
// shop edit bumps it too, so a reload can find NOTHING to rebuild. That is the
// normal case of sharing, not a defect — the fichas=0 log line below exists to
// say so out loud, because a silent no-op here is what would send the next reader
// hunting a bug that is not there.
func (d *Dispatcher) pollMobStats(w *world.World) {
	if d.mobStatSource == nil || d.mobStatPolling {
		return
	}
	d.mobStatPollTick++
	if d.mobStatPollTick%mobStatPollPeriod != 0 {
		return
	}
	known := d.mobStatVersion
	src := d.mobStatSource
	carregar := d.mobTemplateLoader
	antigos := d.mobStatOverrides
	d.mobStatPolling = true

	w.GoDetached(func() func(*world.World) {
		ctx, cancel := context.WithTimeout(context.Background(), mobStatTimeout)
		defer cancel()

		versao, err := src.Version(ctx)
		if err != nil {
			return func(*world.World) {
				d.mobStatPolling = false
				d.log.Warn("ficha de monstro: versão não lida", "err", err)
			}
		}
		if versao == known {
			return func(*world.World) { d.mobStatPolling = false }
		}
		novos, err := src.Fetch(ctx)
		if err != nil {
			return func(*world.World) {
				d.mobStatPolling = false
				// Keep the sheets that are loaded. A failed read must not drop the
				// monsters back to the file's values behind everyone's back: that
				// is a silent, server-wide pace change caused by a network blip,
				// the same reason the Mesa de XP keeps its tables on failure.
				d.log.Warn("ficha de monstro: recarga falhou; mantendo as fichas carregadas",
					"version", known, "err", err)
			}
		}

		// Candidates are the UNION of the names that had an override and the ones
		// that have one now. The old side is what makes a DELETED override take
		// effect: rebuilding from the file with no override in the map restores the
		// file's values, which is the case a first implementation always forgets.
		candidatos := make(map[string]struct{}, len(antigos)+len(novos))
		for n := range antigos {
			candidatos[n] = struct{}{}
		}
		for n := range novos {
			candidatos[n] = struct{}{}
		}

		// Rebuilt bytes per candidate, off the loop. A file that fails to load is
		// SKIPPED rather than failing the whole reload: one unreadable template
		// must not cost the other fifty-six their update.
		refeitos := make(map[string][]byte, len(candidatos))
		var ilegiveis []string
		for nome := range candidatos {
			if carregar == nil {
				break
			}
			cru, lerr := carregar(nome)
			if lerr != nil {
				ilegiveis = append(ilegiveis, nome)
				continue
			}
			refeitos[nome] = mobstat.ApplyOverride(cru, nome, novos)
		}
		sort.Strings(ilegiveis)

		return func(w *world.World) {
			d.mobStatPolling = false
			d.mobStatOverrides = novos
			d.mobStatVersion = versao

			// THE SWAP HAS TO REACH THE GENERATORS, not just a table: the spawn
			// copies g.LeaderTmpl (world.MobSpawn{Template: g.LeaderTmpl}), so a
			// sheet that changed only in some side table would never reach a
			// monster. Comparing bytes — instead of comparing Override structs —
			// also settles "did this actually change" for free, including the
			// equipment slice, and counts a template once however many blocks it
			// has.
			mexidos := map[string]struct{}{}
			for idx := 0; idx < w.GeneratorCount(); idx++ {
				g := w.GeneratorAt(idx)
				if g == nil {
					continue
				}
				if b, ok := refeitos[g.LeaderName]; ok && g.LeaderTmpl != nil && !bytes.Equal(g.LeaderTmpl, b) {
					g.LeaderTmpl = b
					mexidos[g.LeaderName] = struct{}{}
				}
				if b, ok := refeitos[g.FollowerName]; ok && g.FollowerTmpl != nil && !bytes.Equal(g.FollowerTmpl, b) {
					g.FollowerTmpl = b
					mexidos[g.FollowerName] = struct{}{}
				}
			}

			// Always logged, including fichas=0. Zero is the expected answer when
			// the bump came from an NPC or shop edit, and saying it is what lets
			// somebody reading the log a month from now tell that apart from a
			// broken reload.
			d.log.Info("fichas de monstro recarregadas",
				"version", versao, "fichas", len(mexidos), "excecoes", len(novos))
			if len(ilegiveis) > 0 {
				d.log.Warn("ficha de monstro: molde ilegível, ficou com o valor anterior",
					"moldes", ilegiveis)
			}
			// Monsters ALREADY on the map keep the old sheet: the spawn copied the
			// template bytes when they were born (MobSpawn{Template: ...}), so the
			// new numbers reach the next ones to be born. The panel says this next
			// to the fields; it is not something the server can fix without
			// rewriting live entities under the players' feet.
		}
	})
}
