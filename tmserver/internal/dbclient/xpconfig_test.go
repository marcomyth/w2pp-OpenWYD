package dbclient

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/level"
)

type fakeXPClient struct {
	dbv1.XPConfigServiceClient
	resp *dbv1.GetXPConfigResponse
}

func (f *fakeXPClient) GetXPConfig(context.Context, *dbv1.GetXPConfigRequest, ...grpc.CallOption) (*dbv1.GetXPConfigResponse, error) {
	return f.resp, nil
}

// TestFetchKeepsEmptyApartFromUnedited is the same distinction the wire format
// exists for, checked on the side that has to act on it: has_cuts=false must
// leave the legacy table in place, has_cuts=true with no cuts must replace it
// with nothing.
func TestFetchKeepsEmptyApartFromUnedited(t *testing.T) {
	src := &XPConfigSource{api: &fakeXPClient{resp: &dbv1.GetXPConfigResponse{
		Version: 9,
		Rules: []*dbv1.XPRule{
			{Zone: 0, Tier: 2, RatePercent: 150, HasCuts: false},
			{Zone: 3, Tier: 2, RatePercent: 100, HasCuts: true},
			{Zone: 4, Tier: 1, RatePercent: 100, HasCuts: true, Cuts: []*dbv1.XPCut{
				{UpTo: 200, Divisor: 1.5},
			}},
		},
	}}}

	cfg, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if cfg.Version != 9 {
		t.Errorf("version = %d, want 9", cfg.Version)
	}
	if got := cfg.Overrides[level.ConfigKey{Zone: 0, Tier: level.TierMortal}]; got.Cuts != nil {
		t.Error("has_cuts=false must arrive as a nil table, so the legacy one stays")
	}
	empty := cfg.Overrides[level.ConfigKey{Zone: 3, Tier: level.TierMortal}]
	if empty.Cuts == nil || len(empty.Cuts) != 0 {
		t.Errorf("an edited-but-empty table arrived as %v", empty.Cuts)
	}
	full := cfg.Overrides[level.ConfigKey{Zone: 4, Tier: level.TierArch}]
	if len(full.Cuts) != 1 || full.Cuts[0].UpTo != 200 || full.Cuts[0].Divisor != 1.5 {
		t.Errorf("cut table arrived as %+v", full.Cuts)
	}

	// And the whole point: the fetched config has to change what a kill pays.
	in := level.ExpRewardInput{
		Zone: level.ZoneField, MobExp: 20_000, KillerLevel: 300, MobLevel: 300,
		Tier: level.Tier{ClassMaster: level.TierMortal}, Events: level.ExpEvents{KefraLive: true},
	}
	base := level.ExpReward(in)
	in.Config = cfg
	if got, want := level.ExpReward(in), base*150/100; got != want {
		t.Errorf("a taxa de 150%% pagou %d, esperava %d", got, want)
	}
}

// TestFetchEmptyIsTheLegacy: a server whose panel nobody touched must end up
// with the zero Config, which is exactly the pre-Mesa behaviour.
func TestFetchEmptyIsTheLegacy(t *testing.T) {
	src := &XPConfigSource{api: &fakeXPClient{resp: &dbv1.GetXPConfigResponse{}}}
	cfg, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if cfg.Overrides != nil || cfg.Version != 0 {
		t.Fatalf("cfg = %+v, want the zero Config", cfg)
	}
}
