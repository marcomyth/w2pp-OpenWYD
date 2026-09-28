package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// GeneratorRecipeVersion returns the monotonic version of the block recipes
// (0165_receita_de_bloco). tmServer polls it every few seconds, so it stays one
// indexed read.
func (s *Store) GeneratorRecipeVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `SELECT version FROM npc_generator_recipe_meta WHERE id = TRUE`).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: generator recipe version: %w", err)
	}
	return v, nil
}

// recipeColumns is the SELECT list scanRecipe reads, in its order.
const recipeColumns = `generator_index, leader, follower, minute_generate, min_group, max_group,
	max_num_mob, route_type, formation, seg_x, seg_y, seg_range, seg_wait, renovar, nota`

// scanRecipe reads one row in recipeColumns order.
func scanRecipe(row pgx.Row) (domain.GeneratorRecipe, error) {
	var (
		r                      domain.GeneratorRecipe
		route, formation       int16
		segX, segY, segR, segW []int16
	)
	if err := row.Scan(&r.Index, &r.Leader, &r.Follower, &r.MinuteGenerate, &r.MinGroup, &r.MaxGroup,
		&r.MaxNumMob, &route, &formation, &segX, &segY, &segR, &segW, &r.Renovar, &r.Nota); err != nil {
		return domain.GeneratorRecipe{}, err
	}
	r.RouteType, r.Formation = int32(route), int32(formation)
	for i := 0; i < 5; i++ {
		r.SegX[i], r.SegY[i] = int32(segAt(segX, i)), int32(segAt(segY, i))
		r.SegRange[i], r.SegWait[i] = int32(segAt(segR, i)), int32(segAt(segW, i))
	}
	return r, nil
}

// segAt reads a[i], or 0 past its end. The column CHECK keeps every array at five,
// so this only guards against a row somebody wrote by hand.
func segAt(a []int16, i int) int16 {
	if i < len(a) {
		return a[i]
	}
	return 0
}

// segColumn converts one waypoint column to what the SMALLINT[] takes.
func segColumn(v [5]int32) []int16 {
	out := make([]int16, 5)
	for i, x := range v {
		out[i] = int16(x)
	}
	return out
}

// GeneratorRecipes returns every block the database gives a recipe, and the
// version they belong to. Empty is the normal state: every block spawns as the
// file says.
func (s *Store) GeneratorRecipes(ctx context.Context) (domain.GeneratorRecipeConfig, error) {
	var cfg domain.GeneratorRecipeConfig
	if err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Version and rows from one generation, as in GeneratorsOff: a read that
		// straddled a save would hand tmServer a version it never ran.
		if err := tx.QueryRow(ctx,
			`SELECT version FROM npc_generator_recipe_meta WHERE id = TRUE`).Scan(&cfg.Version); err != nil &&
			!errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("store: generator recipe version: %w", err)
		}
		rows, err := tx.Query(ctx,
			`SELECT `+recipeColumns+` FROM npc_generator_recipe ORDER BY generator_index`)
		if err != nil {
			return fmt.Errorf("store: generator recipes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRecipe(rows)
			if err != nil {
				return fmt.Errorf("store: scan generator recipe: %w", err)
			}
			cfg.Recipes = append(cfg.Recipes, r)
		}
		return rows.Err()
	}); err != nil {
		return domain.GeneratorRecipeConfig{}, err
	}
	return cfg, nil
}

// SetGeneratorRecipe writes one block's recipe and bumps the version, in one
// transaction. It returns the row as it stood before — and whether there was one
// — so the panel's audit log can say what changed and not merely what it became.
//
// renovar asks the game to replace the block's live mobs now: the stored counter
// goes up by one, whatever the caller put in rec.Renovar.
func (s *Store) SetGeneratorRecipe(ctx context.Context, rec domain.GeneratorRecipe, renovar bool, moderatorID int64) (before domain.GeneratorRecipe, had bool, err error) {
	if rec.Index < 0 || rec.Index > domain.MaxGeneratorIndex {
		return before, false, fmt.Errorf("store: generator index %d out of range", rec.Index)
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`SELECT version FROM npc_generator_recipe_meta WHERE id = TRUE FOR UPDATE`); err != nil {
			return fmt.Errorf("store: lock generator recipe meta: %w", err)
		}
		before, err = scanRecipe(tx.QueryRow(ctx,
			`SELECT `+recipeColumns+` FROM npc_generator_recipe WHERE generator_index = $1`, rec.Index))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			before, had = domain.GeneratorRecipe{Index: rec.Index}, false
		case err != nil:
			return fmt.Errorf("store: read generator recipe: %w", err)
		default:
			had = true
		}
		bump := int64(0)
		if renovar {
			bump = 1
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO npc_generator_recipe (generator_index, leader, follower, minute_generate,
				min_group, max_group, max_num_mob, route_type, formation,
				seg_x, seg_y, seg_range, seg_wait, renovar, nota, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
			ON CONFLICT (generator_index) DO UPDATE SET
				leader          = EXCLUDED.leader,
				follower        = EXCLUDED.follower,
				minute_generate = EXCLUDED.minute_generate,
				min_group       = EXCLUDED.min_group,
				max_group       = EXCLUDED.max_group,
				max_num_mob     = EXCLUDED.max_num_mob,
				route_type      = EXCLUDED.route_type,
				formation       = EXCLUDED.formation,
				seg_x           = EXCLUDED.seg_x,
				seg_y           = EXCLUDED.seg_y,
				seg_range       = EXCLUDED.seg_range,
				seg_wait        = EXCLUDED.seg_wait,
				renovar         = npc_generator_recipe.renovar + $17,
				nota            = EXCLUDED.nota,
				updated_by      = EXCLUDED.updated_by,
				updated_at      = now()`,
			rec.Index, rec.Leader, rec.Follower, rec.MinuteGenerate,
			rec.MinGroup, rec.MaxGroup, rec.MaxNumMob, int16(rec.RouteType), int16(rec.Formation),
			segColumn(rec.SegX), segColumn(rec.SegY), segColumn(rec.SegRange), segColumn(rec.SegWait),
			bump, rec.Nota, nullableID(moderatorID), bump); err != nil {
			return fmt.Errorf("store: upsert generator recipe %d: %w", rec.Index, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE npc_generator_recipe_meta SET version = version + 1 WHERE id = TRUE`); err != nil {
			return fmt.Errorf("store: bump generator recipe version: %w", err)
		}
		return nil
	})
	return before, had, err
}

// DeleteGeneratorRecipe drops one block's row: a block of the file goes back to
// what the file says, and a block that only lived here leaves the world.
func (s *Store) DeleteGeneratorRecipe(ctx context.Context, index int32, moderatorID int64) (before domain.GeneratorRecipe, had bool, err error) {
	_ = moderatorID // the audit log records who; the row is gone either way
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`SELECT version FROM npc_generator_recipe_meta WHERE id = TRUE FOR UPDATE`); err != nil {
			return fmt.Errorf("store: lock generator recipe meta: %w", err)
		}
		before, err = scanRecipe(tx.QueryRow(ctx,
			`DELETE FROM npc_generator_recipe WHERE generator_index = $1 RETURNING `+recipeColumns, index))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Nothing to clear. The version still goes up, as in DeleteSpawnRate:
			// a second click must not look like the first one did nothing.
			before = domain.GeneratorRecipe{Index: index}
		case err != nil:
			return fmt.Errorf("store: delete generator recipe %d: %w", index, err)
		default:
			had = true
		}
		if _, err := tx.Exec(ctx,
			`UPDATE npc_generator_recipe_meta SET version = version + 1 WHERE id = TRUE`); err != nil {
			return fmt.Errorf("store: bump generator recipe version: %w", err)
		}
		return nil
	})
	return before, had, err
}
