package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// XPConfigVersion returns the monotonic Mesa de XP version. tmServer compares it
// against the one it booted with; an untouched database answers 0, which is the
// pure legacy configuration.
func (s *Store) XPConfigVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `SELECT version FROM xp_rule_meta WHERE id = TRUE`).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: xp config version: %w", err)
	}
	return v, nil
}

// XPConfig returns every edited branch plus the version they belong to. Rows
// are only the branches a moderator touched: what is absent runs on the legacy
// tables, so an empty result is the normal state of a fresh server.
func (s *Store) XPConfig(ctx context.Context) (domain.XPConfig, error) {
	var cfg domain.XPConfig
	if err := s.inTx(ctx, func(tx pgx.Tx) error {
		// One transaction so the version and the rows describe the same
		// generation: a read that straddles an edit would hand tmServer a
		// version it never actually ran.
		if err := tx.QueryRow(ctx, `SELECT version FROM xp_rule_meta WHERE id = TRUE`).Scan(&cfg.Version); err != nil &&
			!errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("store: xp config version: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT zone, tier, rate_percent, cuts
			  FROM xp_rule
			 ORDER BY zone, tier`)
		if err != nil {
			return fmt.Errorf("store: xp rules: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r domain.XPRule
			var raw []byte
			if err := rows.Scan(&r.Zone, &r.Tier, &r.RatePercent, &raw); err != nil {
				return fmt.Errorf("store: scan xp rule: %w", err)
			}
			if raw != nil {
				// A non-nil column is an edited table, including the empty one:
				// json.Unmarshal of "[]" leaves a nil slice, so the decoded
				// value is normalised back to non-nil here or the meaning
				// flips to "not edited" on the way out.
				if err := json.Unmarshal(raw, &r.Cuts); err != nil {
					return fmt.Errorf("store: decode xp cuts (zona %d, evolução %d): %w", r.Zone, r.Tier, err)
				}
				if r.Cuts == nil {
					r.Cuts = []domain.XPCut{}
				}
			}
			cfg.Rules = append(cfg.Rules, r)
		}
		return rows.Err()
	}); err != nil {
		return domain.XPConfig{}, err
	}
	return cfg, nil
}

// UpsertXPRule replaces one branch's configuration and bumps the version, both
// in one transaction. It returns the rule as it stood before, so the caller can
// write the before/after pair into the panel's audit log — which is the Mesa de
// XP's history, and the reason this returns anything at all.
//
// A nil rule.Cuts stores SQL NULL ("this table was not edited"); a non-nil empty
// slice stores '[]' ("this table has no cuts"). Those are different
// configurations and the column keeps them apart.
func (s *Store) UpsertXPRule(ctx context.Context, rule domain.XPRule, moderatorID int64) (before domain.XPRule, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockXPRuleMeta(ctx, tx); err != nil {
			return err
		}
		before, err = fetchXPRule(ctx, tx, rule.Zone, rule.Tier)
		if err != nil {
			return err
		}
		var cuts any
		if rule.Cuts != nil {
			raw, err := json.Marshal(rule.Cuts)
			if err != nil {
				return fmt.Errorf("store: encode xp cuts: %w", err)
			}
			cuts = raw
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO xp_rule (zone, tier, rate_percent, cuts, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (zone, tier) DO UPDATE SET
				rate_percent = EXCLUDED.rate_percent,
				cuts         = EXCLUDED.cuts,
				updated_by   = EXCLUDED.updated_by,
				updated_at   = now()`,
			rule.Zone, rule.Tier, rule.RatePercent, cuts, nullableID(moderatorID)); err != nil {
			return fmt.Errorf("store: upsert xp rule: %w", err)
		}
		return bumpXPRuleVersion(ctx, tx)
	})
	return before, err
}

// DeleteXPRule returns one branch to the legacy tables and bumps the version.
// It reports what was removed so the audit entry can say what the server stopped
// doing; a branch that was never edited returns a zero rule and changes nothing.
func (s *Store) DeleteXPRule(ctx context.Context, zone, tier int32) (before domain.XPRule, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockXPRuleMeta(ctx, tx); err != nil {
			return err
		}
		before, err = fetchXPRule(ctx, tx, zone, tier)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM xp_rule WHERE zone = $1 AND tier = $2`, zone, tier); err != nil {
			return fmt.Errorf("store: delete xp rule: %w", err)
		}
		return bumpXPRuleVersion(ctx, tx)
	})
	return before, err
}

func fetchXPRule(ctx context.Context, tx pgx.Tx, zone, tier int32) (domain.XPRule, error) {
	r := domain.XPRule{Zone: zone, Tier: tier, RatePercent: 100}
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT rate_percent, cuts FROM xp_rule WHERE zone = $1 AND tier = $2`,
		zone, tier).Scan(&r.RatePercent, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.XPRule{Zone: zone, Tier: tier, RatePercent: 100}, nil
	}
	if err != nil {
		return domain.XPRule{}, fmt.Errorf("store: read xp rule: %w", err)
	}
	if raw != nil {
		if err := json.Unmarshal(raw, &r.Cuts); err != nil {
			return domain.XPRule{}, fmt.Errorf("store: decode xp cuts: %w", err)
		}
		if r.Cuts == nil {
			r.Cuts = []domain.XPCut{}
		}
	}
	return r, nil
}

func lockXPRuleMeta(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT version FROM xp_rule_meta WHERE id = TRUE FOR UPDATE`); err != nil {
		return fmt.Errorf("store: lock xp rule meta: %w", err)
	}
	return nil
}

func bumpXPRuleVersion(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `UPDATE xp_rule_meta SET version = version + 1 WHERE id = TRUE`); err != nil {
		return fmt.Errorf("store: bump xp rule version: %w", err)
	}
	return nil
}
