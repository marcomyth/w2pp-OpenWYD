package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// WorldEventConfigVersion returns the monotonically increasing moderator config
// version. Event progress updates do not bump this value.
func (s *Store) WorldEventConfigVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `SELECT version FROM world_event_meta WHERE id = TRUE`).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: world event config version: %w", err)
	}
	return v, nil
}

// WorldEventConfig returns the single global world-event configuration row.
func (s *Store) WorldEventConfig(ctx context.Context) (domain.WorldEventConfig, error) {
	cfg, err := scanWorldEventConfig(s.pool.QueryRow(ctx, worldEventConfigSelect))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultWorldEventConfig(), nil
	}
	if err != nil {
		return domain.WorldEventConfig{}, fmt.Errorf("store: world event config: %w", err)
	}
	return cfg, nil
}

// UpsertWorldEventConfig replaces the moderator-managed global event config,
// writing an audit row and bumping the config version in the same transaction.
//
// It does NOT write the Kefra state (kefra_live_enabled, kefra_guild_id): the
// game writes that too, when the boss dies and when it comes back on Tuesday,
// and a form opened before the kill and saved after it would otherwise write the
// old value back. The Kefra goes through SetKefraState only.
func (s *Store) UpsertWorldEventConfig(ctx context.Context, cfg domain.WorldEventConfig, moderatorID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorldEventMeta(ctx, tx); err != nil {
			return err
		}
		before, _ := fetchWorldEventConfigJSON(ctx, tx)
		if _, err := tx.Exec(ctx, `
			INSERT INTO world_event_config (
				id, enabled, item_index, rate, start_index, current_index, end_index,
				indexed, notice_enabled, double_exp_enabled, newbie_event_enabled,
				tower_war_enabled, tower_war_hour, boss_respawn_hours,
				round_xp_cap_99, round_xp_cap_199, round_xp_cap_299, round_xp_cap_349, round_xp_cap_398,
				round_xp_cap_double_99, round_xp_cap_double_199, round_xp_cap_double_299,
				round_xp_cap_double_349, round_xp_cap_double_398,
				updated_by, updated_at
			)
			VALUES (TRUE,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,
				$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,now())
			ON CONFLICT (id) DO UPDATE SET
				enabled              = EXCLUDED.enabled,
				item_index           = EXCLUDED.item_index,
				rate                 = EXCLUDED.rate,
				start_index          = EXCLUDED.start_index,
				current_index        = EXCLUDED.current_index,
				end_index            = EXCLUDED.end_index,
				indexed              = EXCLUDED.indexed,
				notice_enabled       = EXCLUDED.notice_enabled,
				double_exp_enabled   = EXCLUDED.double_exp_enabled,
				newbie_event_enabled = EXCLUDED.newbie_event_enabled,
				tower_war_enabled    = EXCLUDED.tower_war_enabled,
				tower_war_hour       = EXCLUDED.tower_war_hour,
				boss_respawn_hours   = EXCLUDED.boss_respawn_hours,
				round_xp_cap_99         = EXCLUDED.round_xp_cap_99,
				round_xp_cap_199        = EXCLUDED.round_xp_cap_199,
				round_xp_cap_299        = EXCLUDED.round_xp_cap_299,
				round_xp_cap_349        = EXCLUDED.round_xp_cap_349,
				round_xp_cap_398        = EXCLUDED.round_xp_cap_398,
				round_xp_cap_double_99  = EXCLUDED.round_xp_cap_double_99,
				round_xp_cap_double_199 = EXCLUDED.round_xp_cap_double_199,
				round_xp_cap_double_299 = EXCLUDED.round_xp_cap_double_299,
				round_xp_cap_double_349 = EXCLUDED.round_xp_cap_double_349,
				round_xp_cap_double_398 = EXCLUDED.round_xp_cap_double_398,
				updated_by           = EXCLUDED.updated_by,
				updated_at           = now()`,
			cfg.Enabled, cfg.ItemIndex, cfg.Rate, cfg.StartIndex, cfg.CurrentIndex,
			cfg.EndIndex, cfg.Indexed, cfg.NoticeEnabled, cfg.DoubleExpEnabled,
			cfg.NewbieEventEnabled, cfg.TowerWarEnabled, cfg.TowerWarHour,
			cfg.BossRespawnHours,
			cfg.RoundXPCap[0], cfg.RoundXPCap[1], cfg.RoundXPCap[2], cfg.RoundXPCap[3], cfg.RoundXPCap[4],
			cfg.RoundXPCapDouble[0], cfg.RoundXPCapDouble[1], cfg.RoundXPCapDouble[2],
			cfg.RoundXPCapDouble[3], cfg.RoundXPCapDouble[4],
			nullableID(moderatorID)); err != nil {
			return fmt.Errorf("store: upsert world event config: %w", err)
		}
		after, _ := fetchWorldEventConfigJSON(ctx, tx)
		return auditWorldEventAndBump(ctx, tx, moderatorID, FonteEventoPainel, "set_config", before, after)
	})
}

// Where a world-event write came from, as world_event_audit.fonte records it
// (migration 0067).
const (
	FonteEventoPainel = "painel"
	FonteEventoJogo   = "jogo"
)

// SetKefraState records the Kefra as defeated (live, full experience) or alive
// (half), with the guild that killed it. It is the one path both the game (the
// boss dying, the Tuesday return) and the panel's manual correction use: it locks
// the meta row, writes only the two Kefra columns, audits with the source and
// bumps the config version in one transaction, and returns the new version.
//
// An alive Kefra has no killer, so guildID is stored as 0 when live is false.
// accountID is the moderator for a panel write and 0 for the game's.
func (s *Store) SetKefraState(ctx context.Context, live bool, guildID int32, fonte string, accountID int64) (int64, error) {
	if guildID < 0 {
		return 0, fmt.Errorf("store: set kefra state: negative guild %d", guildID)
	}
	if fonte == "" {
		return 0, errors.New("store: set kefra state: empty source")
	}
	if !live {
		guildID = 0
	}
	var version int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorldEventMeta(ctx, tx); err != nil {
			return err
		}
		before, _ := fetchWorldEventConfigJSON(ctx, tx)
		if _, err := tx.Exec(ctx, `
			INSERT INTO world_event_config (id, kefra_live_enabled, kefra_guild_id, updated_by, updated_at)
			VALUES (TRUE, $1, $2, $3, now())
			ON CONFLICT (id) DO UPDATE SET
				kefra_live_enabled = EXCLUDED.kefra_live_enabled,
				kefra_guild_id     = EXCLUDED.kefra_guild_id,
				updated_by         = EXCLUDED.updated_by,
				updated_at         = now()`,
			live, guildID, nullableID(accountID)); err != nil {
			return fmt.Errorf("store: set kefra state: %w", err)
		}
		after, _ := fetchWorldEventConfigJSON(ctx, tx)
		if err := auditWorldEventAndBump(ctx, tx, accountID, fonte, "set_kefra", before, after); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT version FROM world_event_meta WHERE id = TRUE`).Scan(&version); err != nil {
			return fmt.Errorf("store: set kefra state: read version: %w", err)
		}
		return nil
	})
	return version, err
}

// UpdateWorldEventProgress persists tmServer's event counter without bumping the
// moderator config version. expectedVersion prevents a stale tmServer snapshot
// from overwriting a newer portal edit; applied is false when the version changed.
func (s *Store) UpdateWorldEventProgress(ctx context.Context, expectedVersion int64, currentIndex int32) (applied bool, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			WITH matching_meta AS MATERIALIZED (
				SELECT 1
				  FROM world_event_meta
				 WHERE id = TRUE AND version = $1
				   FOR UPDATE
			)
			UPDATE world_event_config
			   SET current_index = GREATEST(current_index, $2)
			 WHERE id = TRUE
			   AND EXISTS (SELECT 1 FROM matching_meta)`, expectedVersion, currentIndex)
		if err != nil {
			return fmt.Errorf("store: update world event progress: %w", err)
		}
		applied = ct.RowsAffected() == 1
		return nil
	})
	return applied, err
}

const worldEventConfigSelect = `
	SELECT enabled, item_index, rate, start_index, current_index, end_index,
	       indexed, notice_enabled, double_exp_enabled, newbie_event_enabled,
	       kefra_live_enabled, kefra_guild_id, tower_war_enabled, tower_war_hour, boss_respawn_hours,
	       round_xp_cap_99, round_xp_cap_199, round_xp_cap_299, round_xp_cap_349, round_xp_cap_398,
	       round_xp_cap_double_99, round_xp_cap_double_199, round_xp_cap_double_299,
	       round_xp_cap_double_349, round_xp_cap_double_398
	FROM world_event_config WHERE id = TRUE`

type worldEventScanRow interface {
	Scan(dest ...any) error
}

func scanWorldEventConfig(row worldEventScanRow) (domain.WorldEventConfig, error) {
	var cfg domain.WorldEventConfig
	err := row.Scan(&cfg.Enabled, &cfg.ItemIndex, &cfg.Rate, &cfg.StartIndex,
		&cfg.CurrentIndex, &cfg.EndIndex, &cfg.Indexed, &cfg.NoticeEnabled,
		&cfg.DoubleExpEnabled, &cfg.NewbieEventEnabled, &cfg.KefraLiveEnabled, &cfg.KefraGuildID,
		&cfg.TowerWarEnabled, &cfg.TowerWarHour, &cfg.BossRespawnHours,
		&cfg.RoundXPCap[0], &cfg.RoundXPCap[1], &cfg.RoundXPCap[2], &cfg.RoundXPCap[3], &cfg.RoundXPCap[4],
		&cfg.RoundXPCapDouble[0], &cfg.RoundXPCapDouble[1], &cfg.RoundXPCapDouble[2],
		&cfg.RoundXPCapDouble[3], &cfg.RoundXPCapDouble[4])
	return cfg, err
}

func fetchWorldEventConfigJSON(ctx context.Context, tx pgx.Tx) ([]byte, error) {
	var js []byte
	err := tx.QueryRow(ctx, `SELECT to_jsonb(c) FROM world_event_config c WHERE id = TRUE`).Scan(&js)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return js, err
}

func lockWorldEventMeta(ctx context.Context, tx pgx.Tx) error {
	// Portal edits and tmServer progress both take this lock before touching the
	// config row, so expectedVersion cannot pass while a moderator edit is pending.
	var version int64
	if err := tx.QueryRow(ctx, `SELECT version FROM world_event_meta WHERE id = TRUE FOR UPDATE`).Scan(&version); err != nil {
		return fmt.Errorf("store: lock world event meta: %w", err)
	}
	return nil
}

// auditWorldEventAndBump writes the audit row and bumps the version. accountID 0
// is stored as NULL: the game's writes have no moderator (migration 0067).
func auditWorldEventAndBump(ctx context.Context, tx pgx.Tx, accountID int64, fonte, action string, before, after []byte) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO world_event_audit (account_id, action, fonte, before, after)
		VALUES ($1,$2,$3,$4,$5)`,
		nullableID(accountID), action, fonte, nullableJSON(before), nullableJSON(after)); err != nil {
		return fmt.Errorf("store: write world event audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE world_event_meta SET version = version + 1 WHERE id = TRUE`); err != nil {
		return fmt.Errorf("store: bump world event config version: %w", err)
	}
	return nil
}
