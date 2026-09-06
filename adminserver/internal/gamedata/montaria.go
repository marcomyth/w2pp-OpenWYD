package gamedata

import (
	"context"
	"fmt"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
)

// MountGrowthCurve is one adult lineage as the panel shows it: the chance an
// âmago raises it one level, per band of twenty levels.
//
// Rates carries one entry per band, and a band nobody configured carries -1 —
// which is NOT zero. Zero is a legitimate setting: an operator deliberately
// making that band impossible. Collapsing the two would silently turn "still on
// the default" into "blocked", which is the one mistake this screen exists to
// prevent.
type MountGrowthCurve struct {
	MountIndex  int32
	DisplayName string
	CriaIndex   int32
	AmagoIndex  int32
	Configured  bool
	Rates       []int32
}

// MountGrowthCurves lists the whole roster of adult lineages, configured or not.
func (c *Client) MountGrowthCurves(ctx context.Context) ([]MountGrowthCurve, error) {
	resp, err := c.mountGrowth.ListMountGrowthCurves(ctx, &webv1.ListMountGrowthCurvesRequest{})
	if err != nil {
		return nil, fmt.Errorf("gamedata: list mount growth curves: %w", err)
	}
	out := make([]MountGrowthCurve, 0, len(resp.GetCurves()))
	for _, cur := range resp.GetCurves() {
		out = append(out, MountGrowthCurve{
			MountIndex:  cur.GetMountIndex(),
			DisplayName: cur.GetDisplayName(),
			CriaIndex:   cur.GetCriaIndex(),
			AmagoIndex:  cur.GetAmagoIndex(),
			Configured:  cur.GetConfigured(),
			Rates:       cur.GetRates(),
		})
	}
	return out, nil
}

// SetMountGrowthCurve writes one lineage's bands, all of them at once. The
// service refuses a partial list: the bands only mean anything together, and a
// save that landed half of them would leave a curve nobody chose.
func (c *Client) SetMountGrowthCurve(ctx context.Context, moderatorID int64, moderator string, mountIndex int32, rates []int32) error {
	resp, err := c.mountGrowth.SetMountGrowthCurve(ctx, &webv1.SetMountGrowthCurveRequest{
		ModeratorId: moderatorID, Moderator: moderator,
		MountIndex: mountIndex, Rates: rates,
	})
	if err != nil {
		return fmt.Errorf("gamedata: set mount growth curve %d: %w", mountIndex, err)
	}
	return resultErr(resp.GetResult())
}

// ClearMountGrowthCurve drops the lineage's rows so the compiled default applies
// again.
func (c *Client) ClearMountGrowthCurve(ctx context.Context, moderatorID int64, mountIndex int32) error {
	resp, err := c.mountGrowth.ClearMountGrowthCurve(ctx, &webv1.ClearMountGrowthCurveRequest{
		ModeratorId: moderatorID, MountIndex: mountIndex,
	})
	if err != nil {
		return fmt.Errorf("gamedata: clear mount growth curve %d: %w", mountIndex, err)
	}
	return resultErr(resp.GetResult())
}
