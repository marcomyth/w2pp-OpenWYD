package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool sizing. Every service that opens a pool takes its share of the server's
// max_connections, so the ceiling is a deployment-wide budget, not a per-service
// preference: dbServer and webServer together stay well under a default
// PostgreSQL (100) with room for psql and migrations.
const (
	poolMaxConns = 10
	poolMinConns = 2
	// Recycling bounds how long one connection can pin a backend (or a pooler's
	// slot) on a server that runs for weeks without a restart.
	poolMaxConnLifetime = 30 * time.Minute
	poolMaxConnIdleTime = 5 * time.Minute
	poolHealthCheck     = time.Minute
)

// Pool opens a PostgreSQL pool with explicit sizing.
//
// Why not pgxpool.New: its default ceiling is max(4, NumCPU), which in a
// container is neither predictable — NumCPU reports the host's processors, not
// the cgroup quota — nor coordinated with the database's own connection limit.
//
// It matters here more than the traffic suggests. The game keeps world state in
// memory and touches the database on login, on the periodic save and on logout,
// so steady play barely uses the pool. The load that does arrive comes all at
// once: a shutdown persists every online character in a burst. A pool sized by
// accident is the wrong place to discover that.
func Pool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.MaxConnIdleTime = poolMaxConnIdleTime
	cfg.HealthCheckPeriod = poolHealthCheck

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	return pool, nil
}
