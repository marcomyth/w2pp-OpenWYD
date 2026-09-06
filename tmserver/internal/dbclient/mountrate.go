package dbclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mountrate"
)

// MountRateSource fetches the mount growth curves (0030_mount_growth_rate) from
// the dbServer's NpcConfigService. Sibling of ItemStatSource, fetched the same
// way: once at boot, no version poll, because there is no hot-reload here.
type MountRateSource struct {
	api dbv1.NpcConfigServiceClient
}

// NewMountRateSource wraps a gRPC connection as a MountRateSource.
func NewMountRateSource(conn grpc.ClientConnInterface) *MountRateSource {
	return &MountRateSource{api: dbv1.NewNpcConfigServiceClient(conn)}
}

// Fetch reads every configured curve point and folds it into the table the game
// reads. A lineage with no rows is simply absent from the result, and the
// caller's default applies to it.
func (s *MountRateSource) Fetch(ctx context.Context) (mountrate.Table, error) {
	resp, err := s.api.ListMountGrowthRates(ctx, &dbv1.ListMountGrowthRatesRequest{})
	if err != nil {
		return nil, fmt.Errorf("dbclient: list mount growth rates: %w", err)
	}
	table := make(mountrate.Table, len(resp.GetRates())/domain.MountGrowthBands+1)
	for _, r := range resp.GetRates() {
		// The wire is int32 (proto3 has no 16-bit scalar) and the game is int16;
		// the narrowing lives here, as it does for the item stats. Out-of-range
		// rows are dropped rather than clamped: a band outside 0..5 or a rate
		// outside 0..100 means the writer and this reader disagree about the
		// model, and silently folding it in would hide that.
		if r.GetBand() < 0 || r.GetBand() >= domain.MountGrowthBands {
			continue
		}
		if r.GetRate() < 0 || r.GetRate() > 100 {
			continue
		}
		idx := int16(r.GetMountIndex())
		curve, ok := table[idx]
		if !ok {
			curve = mountrate.UnsetCurve()
		}
		curve[r.GetBand()] = int8(r.GetRate())
		table[idx] = curve
	}
	return table, nil
}
