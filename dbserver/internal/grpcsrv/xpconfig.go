package grpcsrv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
)

// XPConfigStore is the read surface tmServer needs for the Mesa de XP
// (satisfied by *store.Store). Moderator writes go through the admin panel.
type XPConfigStore interface {
	XPConfigVersion(ctx context.Context) (int64, error)
	XPConfig(ctx context.Context) (domain.XPConfig, error)
}

// XPConfigServer implements dbv1.XPConfigServiceServer.
type XPConfigServer struct {
	dbv1.UnimplementedXPConfigServiceServer
	store XPConfigStore
}

// NewXPConfig builds the service over the given store.
func NewXPConfig(s XPConfigStore) *XPConfigServer { return &XPConfigServer{store: s} }

// XPConfigVersion returns the monotonic config version for tmServer.
func (s *XPConfigServer) XPConfigVersion(ctx context.Context, _ *dbv1.XPConfigVersionRequest) (*dbv1.XPConfigVersionResponse, error) {
	v, err := s.store.XPConfigVersion(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "xp config version: %v", err)
	}
	return &dbv1.XPConfigVersionResponse{Version: v}, nil
}

// GetXPConfig returns every edited branch and the version it belongs to.
func (s *XPConfigServer) GetXPConfig(ctx context.Context, _ *dbv1.GetXPConfigRequest) (*dbv1.GetXPConfigResponse, error) {
	cfg, err := s.store.XPConfig(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "xp config: %v", err)
	}
	resp := &dbv1.GetXPConfigResponse{Version: cfg.Version, Rules: make([]*dbv1.XPRule, 0, len(cfg.Rules))}
	for _, r := range cfg.Rules {
		resp.Rules = append(resp.Rules, xpRuleToDBProto(r))
	}
	return resp, nil
}

func xpRuleToDBProto(r domain.XPRule) *dbv1.XPRule {
	// HasCuts, not the length of the list: an edited branch with no cuts and an
	// unedited branch are opposite configurations, and proto3 cannot tell an
	// absent repeated field from an empty one.
	out := &dbv1.XPRule{
		Zone: r.Zone, Tier: r.Tier, RatePercent: r.RatePercent, HasCuts: r.Cuts != nil,
	}
	for _, c := range r.Cuts {
		out.Cuts = append(out.Cuts, &dbv1.XPCut{UpTo: c.UpTo, Divisor: c.Divisor})
	}
	return out
}
