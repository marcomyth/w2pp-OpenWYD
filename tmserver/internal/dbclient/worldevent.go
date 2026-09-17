package dbclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/worldcfg"
)

// WorldEventConfig adapts dbServer's WorldEventConfigService to the tmServer
// worldcfg.Source port.
type WorldEventConfig struct {
	api dbv1.WorldEventConfigServiceClient
}

// NewWorldEventConfig wraps a gRPC connection as a world-event config source.
func NewWorldEventConfig(conn grpc.ClientConnInterface) *WorldEventConfig {
	return &WorldEventConfig{api: dbv1.NewWorldEventConfigServiceClient(conn)}
}

var _ worldcfg.Source = (*WorldEventConfig)(nil)

// Version returns the current portal-managed world-event config version.
func (c *WorldEventConfig) Version(ctx context.Context) (int64, error) {
	resp, err := c.api.WorldEventConfigVersion(ctx, &dbv1.WorldEventConfigVersionRequest{})
	if err != nil {
		return 0, fmt.Errorf("dbclient: world event config version: %w", err)
	}
	return resp.GetVersion(), nil
}

// Snapshot fetches the full event config snapshot.
func (c *WorldEventConfig) Snapshot(ctx context.Context) (worldcfg.Snapshot, error) {
	resp, err := c.api.GetWorldEventConfig(ctx, &dbv1.GetWorldEventConfigRequest{})
	if err != nil {
		return worldcfg.Snapshot{}, fmt.Errorf("dbclient: get world event config: %w", err)
	}
	return worldcfg.Snapshot{Version: resp.GetVersion(), Event: dbWorldEventToConfig(resp.GetConfig())}, nil
}

// UpdateProgress persists the current event counter without bumping config version.
func (c *WorldEventConfig) UpdateProgress(ctx context.Context, expectedVersion int64, currentIndex int32) (bool, error) {
	resp, err := c.api.UpdateWorldEventProgress(ctx, &dbv1.UpdateWorldEventProgressRequest{
		ExpectedVersion: expectedVersion,
		CurrentIndex:    currentIndex,
	})
	if err != nil {
		return false, fmt.Errorf("dbclient: update world event progress: %w", err)
	}
	return resp.GetApplied(), nil
}

// SetKefraState records the Kefra state through dbServer and returns the new
// config version.
func (c *WorldEventConfig) SetKefraState(ctx context.Context, live bool, guildID int32) (int64, error) {
	resp, err := c.api.SetKefraState(ctx, &dbv1.SetKefraStateRequest{Live: live, GuildId: guildID})
	if err != nil {
		return 0, fmt.Errorf("dbclient: set kefra state: %w", err)
	}
	return resp.GetVersion(), nil
}

func dbWorldEventToConfig(cfg *dbv1.WorldEventConfig) worldcfg.EventConfig {
	if cfg == nil {
		return worldcfg.EventConfig{
			NoticeEnabled:    true,
			TowerWarEnabled:  domain.DefaultTowerWarEnabled,
			TowerWarHour:     domain.DefaultTowerWarHour,
			BossRespawnHours: domain.DefaultBossRespawnHours,
			RoundXPCap:       domain.DefaultRoundXPCap,
			RoundXPCapDouble: domain.DefaultRoundXPCapDouble,
		}
	}
	// The Tower War pair is `optional` on the wire. Absent means a dbServer that
	// predates migration 0051, and then the decided default (on, 20h) runs — the
	// same value the migration gives the row — rather than the zero values,
	// which would read as "off, at midnight" and cancel the daily war during a
	// rolling deploy.
	ligada := domain.DefaultTowerWarEnabled
	if cfg.TowerWarEnabled != nil {
		ligada = *cfg.TowerWarEnabled
	}
	hora := int32(domain.DefaultTowerWarHour)
	if cfg.TowerWarHour != nil {
		hora = *cfg.TowerWarHour
	}
	// The lone-boss respawn the same way (migration 0056): absent is a dbServer
	// that predates it, and the decided 24 h runs instead of a zero.
	chefes := int32(domain.DefaultBossRespawnHours)
	if cfg.BossRespawnHours != nil {
		chefes = *cfg.BossRespawnHours
	}
	// The Mortal round XP cap (migration 0073): five values or none. None is a
	// dbServer that predates it, and the decided caps run instead of zeros, which
	// would read as "no cap" in every band.
	teto, tetoDobro := domain.DefaultRoundXPCap, domain.DefaultRoundXPCapDouble
	if v := cfg.GetRoundXpCap(); len(v) == len(teto) {
		copy(teto[:], v)
	}
	if v := cfg.GetRoundXpCapDouble(); len(v) == len(tetoDobro) {
		copy(tetoDobro[:], v)
	}
	return worldcfg.EventConfig{
		Enabled: cfg.GetEnabled(), ItemIndex: cfg.GetItemIndex(), Rate: cfg.GetRate(),
		StartIndex: cfg.GetStartIndex(), CurrentIndex: cfg.GetCurrentIndex(), EndIndex: cfg.GetEndIndex(),
		Indexed: cfg.GetIndexed(), NoticeEnabled: cfg.GetNoticeEnabled(),
		DoubleExpEnabled: cfg.GetDoubleExpEnabled(), NewbieEventEnabled: cfg.GetNewbieEventEnabled(),
		KefraLiveEnabled: cfg.GetKefraLiveEnabled(),
		// Absent (a dbServer before 0067) reads as 0: no guild, the same as a Kefra
		// killed by someone without one.
		KefraGuildID:    cfg.GetKefraGuildId(),
		TowerWarEnabled: ligada, TowerWarHour: hora,
		BossRespawnHours: chefes,
		RoundXPCap:       teto,
		RoundXPCapDouble: tetoDobro,
	}
}
