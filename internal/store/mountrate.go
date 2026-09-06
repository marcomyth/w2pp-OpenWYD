package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// Mount growth curve persistence (0030_mount_growth_rate), the mount-side
// sibling of itemstat.go. Postgres owns the chance an âmago raises an adult
// mount a level, per lineage and per band of twenty levels; the tmServer only
// ever reads it (via dbServer), once at boot.
//
// Mutations reuse npc_audit/npc_config_meta through auditAndBump, exactly as the
// item and mob stats do — one moderation trail beats a parallel one nobody would
// think to read.

// ListMountGrowthRates returns every configured point, ordered so a caller can
// walk a lineage's curve in band order without sorting.
func (s *Store) ListMountGrowthRates(ctx context.Context) ([]domain.MountGrowthRate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT mount_index, band, rate FROM mount_growth_rate ORDER BY mount_index, band`)
	if err != nil {
		return nil, fmt.Errorf("store: list mount growth rates: %w", err)
	}
	defer rows.Close()

	var out []domain.MountGrowthRate
	for rows.Next() {
		var r domain.MountGrowthRate
		if err := rows.Scan(&r.MountIndex, &r.Band, &r.Rate); err != nil {
			return nil, fmt.Errorf("store: scan mount growth rate: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate mount growth rates: %w", err)
	}
	return out, nil
}

// SetMountGrowthCurve replaces one lineage's whole curve in a single
// transaction.
//
// Whole-curve rather than per-band: the bands are read together and only make
// sense together — a save that landed band 3 and lost band 4 would leave a mount
// with a curve nobody chose, and the shape is the thing being edited, not the
// individual number.
func (s *Store) SetMountGrowthCurve(ctx context.Context, mountIndex int16, rates []int16, moderatorID int64, moderator string) error {
	if len(rates) != domain.MountGrowthBands {
		return fmt.Errorf("store: mount growth curve needs %d bands, got %d", domain.MountGrowthBands, len(rates))
	}
	if mountIndex < domain.MountAdultLo || mountIndex > domain.MountAdultHi {
		return fmt.Errorf("store: %d is not an adult mount", mountIndex)
	}
	for band, rate := range rates {
		if rate < 0 || rate > 100 {
			return fmt.Errorf("store: band %d rate %d is out of 0..100", band, rate)
		}
	}

	return s.inTx(ctx, func(tx pgx.Tx) error {
		before, err := fetchMountCurveJSON(ctx, tx, mountIndex)
		if err != nil {
			return err
		}
		for band, rate := range rates {
			if _, err := tx.Exec(ctx,
				`INSERT INTO mount_growth_rate (mount_index, band, rate, updated_by)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (mount_index, band)
				 DO UPDATE SET rate = EXCLUDED.rate, updated_by = EXCLUDED.updated_by, updated_at = now()`,
				mountIndex, int16(band), rate, moderator); err != nil {
				return fmt.Errorf("store: upsert mount growth rate %d band %d: %w", mountIndex, band, err)
			}
		}
		after, err := fetchMountCurveJSON(ctx, tx, mountIndex)
		if err != nil {
			return err
		}
		return auditAndBump(ctx, tx, nil, moderatorID, "set_mount_growth_curve", before, after)
	})
}

// ClearMountGrowthCurve drops a lineage's rows, which returns it to the compiled
// default — the absence of a row is what "use the default" means everywhere in
// this overlay, so restoring is a delete and not a write of the default values.
func (s *Store) ClearMountGrowthCurve(ctx context.Context, mountIndex int16, moderatorID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		before, err := fetchMountCurveJSON(ctx, tx, mountIndex)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM mount_growth_rate WHERE mount_index = $1`, mountIndex); err != nil {
			return fmt.Errorf("store: clear mount growth curve %d: %w", mountIndex, err)
		}
		after, err := fetchMountCurveJSON(ctx, tx, mountIndex)
		if err != nil {
			return err
		}
		return auditAndBump(ctx, tx, nil, moderatorID, "clear_mount_growth_curve", before, after)
	})
}

// fetchMountCurveJSON snapshots a lineage's curve for the audit trail.
func fetchMountCurveJSON(ctx context.Context, tx pgx.Tx, mountIndex int16) ([]byte, error) {
	var js []byte
	err := tx.QueryRow(ctx,
		`SELECT coalesce(jsonb_agg(jsonb_build_object('band', band, 'rate', rate) ORDER BY band), '[]'::jsonb)
		 FROM mount_growth_rate WHERE mount_index = $1`, mountIndex).Scan(&js)
	if err != nil {
		return nil, fmt.Errorf("store: snapshot mount growth curve %d: %w", mountIndex, err)
	}
	return js, nil
}
