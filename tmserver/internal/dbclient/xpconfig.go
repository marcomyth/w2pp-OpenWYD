package dbclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/level"
)

// XPConfigSource fetches the Mesa de XP — the panel-managed reward tables — from
// dbServer's XPConfigService.
//
// It is fetched once at boot and not polled, for the same reason the mob stat
// overlay is not: the reward tables shape a grind, and swapping them under a
// running server would pay two players different experience for the same mob
// depending on when each one's kill landed. Restart-to-apply is the honest
// behaviour, and the panel says so.
type XPConfigSource struct {
	api dbv1.XPConfigServiceClient
}

// NewXPConfigSource wraps a gRPC connection as an XPConfigSource.
func NewXPConfigSource(conn grpc.ClientConnInterface) *XPConfigSource {
	return &XPConfigSource{api: dbv1.NewXPConfigServiceClient(conn)}
}

// Fetch returns the configuration ready for level.ExpReward. A reply with no
// rules yields a zero Config, which is the pure legacy behaviour.
func (c *XPConfigSource) Fetch(ctx context.Context) (level.Config, error) {
	resp, err := c.api.GetXPConfig(ctx, &dbv1.GetXPConfigRequest{})
	if err != nil {
		return level.Config{}, fmt.Errorf("dbclient: get xp config: %w", err)
	}
	cfg := level.Config{Version: resp.GetVersion()}
	for _, r := range resp.GetRules() {
		ov := level.Override{RatePercent: r.GetRatePercent()}
		if r.GetHasCuts() {
			// Non-nil even when empty: an edited branch with no cuts divides by
			// nothing, and a nil slice would silently mean "use the legacy's".
			ov.Cuts = make([]level.Cut, 0, len(r.GetCuts()))
			for _, c := range r.GetCuts() {
				ov.Cuts = append(ov.Cuts, level.Cut{UpTo: c.GetUpTo(), Divisor: c.GetDivisor()})
			}
		}
		if cfg.Overrides == nil {
			cfg.Overrides = make(map[level.ConfigKey]level.Override, len(resp.GetRules()))
		}
		cfg.Overrides[level.ConfigKey{Zone: level.Zone(r.GetZone()), Tier: uint8(r.GetTier())}] = ov
	}
	return cfg, nil
}
