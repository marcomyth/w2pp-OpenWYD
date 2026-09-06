// Package mobspawns answers "where does this monster come from" by joining the
// content tree's spawn table (NPCGener.txt) to its named-rectangle table
// (Regions.txt).
//
// The moderator problem it solves is concrete: Release/TMsrv/run/npc/ holds
// 1991 templates and NPCGener.txt only ever names 620 of them, so two thirds of
// what the mob editor will happily let somebody rebalance never spawns from a
// generator at all. @@Gargula is one of those, and it sits in the picker
// directly beside Gargula_, which is the Água Místico monster somebody
// rebalancing "the gargoyle" actually means. Without this the panel cannot tell
// them apart, and the edit lands on a template no player ever meets.
//
// A template with no origins is NOT proof that nothing spawns it — instances,
// quests and summons create mobs by name from code, and the roster in
// content.baseSummonFiles is one such path. So the absence is reported as "not
// from a generator", never as "unused".
package mobspawns

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/jeanluca/w2pp-openwyd/internal/mapzones"
	"github.com/jeanluca/w2pp-openwyd/internal/npcgener"
	"github.com/jeanluca/w2pp-openwyd/internal/regions"
)

// Origin is one place a template spawns, aggregated across every generator
// block that names it. Aggregated rather than raw because the raw list is
// unreadable: the busier templates hold dozens of blocks a few tiles apart,
// which is one place to a person and forty rows on a screen.
type Origin struct {
	// Place is the name to show, already translated.
	Place string
	// Points is how many generator blocks spawn this template here, counting
	// blocks where it is the follower as well as the leader.
	Points int32
	// Amount sums MaxNumMob over the blocks where this template LEADS. A
	// follower's count belongs to its leader's block, so adding it here would
	// double-count the same monsters.
	Amount int32
	// RespawnMin is the shortest respawn period in minutes among those blocks.
	// Zero means none of them regenerate — the legacy treats MinuteGenerate<=0
	// as spawn-once.
	RespawnMin int32
	// X, Y is one representative spawn point, for a moderator who wants to go
	// and look.
	X, Y int32
}

// Index maps template_name to its origins, busiest first.
type Index map[string][]Origin

// Build reads NPCGener.txt and Regions.txt from the content tree and returns
// the index.
//
// A missing or unreadable Regions.txt is not fatal: without it every origin
// falls back to the mapzones settlement labels, which still beats printing bare
// coordinates. A missing NPCGener.txt is, since without it there is no index at
// all.
func Build(contentDir string, logger *slog.Logger) (Index, error) {
	blocks, err := npcgener.Load(filepath.Join(contentDir, "TMsrv", "run", "NPCGener.txt"))
	if err != nil {
		return nil, fmt.Errorf("mobspawns: %w", err)
	}
	tab, err := regions.Load(filepath.Join(contentDir, "TMsrv", "run", "Regions.txt"))
	if err != nil && logger != nil {
		logger.Warn("mobspawns: sem Regions.txt, os locais vão sair só com nome de cidade", "err", err)
	}

	// Accumulate per (template, place) before sorting, so the dozens of blocks
	// that make up one spawn field collapse into one row.
	type key struct{ tmpl, place string }
	acc := map[key]*Origin{}
	add := func(tmpl, place string, x, y int32, amount, respawn int32) {
		if tmpl == "" {
			return
		}
		k := key{tmpl, place}
		o := acc[k]
		if o == nil {
			o = &Origin{Place: place, X: x, Y: y}
			acc[k] = o
		}
		o.Points++
		o.Amount += amount
		// Shortest non-zero period wins: it is the one a player actually feels.
		if respawn > 0 && (o.RespawnMin == 0 || respawn < o.RespawnMin) {
			o.RespawnMin = respawn
		}
	}

	for _, b := range blocks {
		x, y := int32(b.SegX[0]), int32(b.SegY[0])
		place := Place(tab, x, y)
		add(b.Leader, place, x, y, int32(b.MaxNumMob), int32(b.MinuteGenerate))
		// A follower spawns in the same place but its numbers belong to the
		// leader's block, so it contributes a point and no amount.
		if b.Follower != "" && b.Follower != b.Leader {
			add(b.Follower, place, x, y, 0, int32(b.MinuteGenerate))
		}
	}

	out := Index{}
	for k, o := range acc {
		out[k.tmpl] = append(out[k.tmpl], *o)
	}
	for tmpl := range out {
		list := out[tmpl]
		sort.Slice(list, func(i, j int) bool {
			if list[i].Amount != list[j].Amount {
				return list[i].Amount > list[j].Amount
			}
			if list[i].Points != list[j].Points {
				return list[i].Points > list[j].Points
			}
			return list[i].Place < list[j].Place
		})
		out[tmpl] = list
	}
	return out, nil
}

// Place names a position the way somebody would say it out loud.
//
// Regions.txt is consulted first because it is the only table that names the
// instanced dungeons — Água, Cubo, Kefra, Lan, Submundo, the Deserto belt — and
// those are exactly the places whose monsters get rebalanced. It is also the
// narrower table: it says nothing about most of the map, and its "Armia"
// rectangles start well east of the town center mapzones classifies by. So
// mapzones answers second, and a landmark answers last.
func Place(tab regions.Table, x, y int32) string {
	if n := tab.At(x, y); n != "" {
		return regions.Label(n)
	}
	if id := mapzones.Classify(x, y); id != mapzones.Field {
		return mapzones.Name(id)
	}
	if z, _ := mapzones.Nearest(x, y); z.Name != "" {
		return "Campo — perto de " + z.Name
	}
	return "Campo"
}
