package grpcsrv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
)

// WorldEventConfigStore is the surface the tmServer needs (satisfied by
// *store.Store): the config read, the progress counter and the Kefra state.
// Moderator writes go through web-api and the panel.
type WorldEventConfigStore interface {
	WorldEventConfigVersion(ctx context.Context) (int64, error)
	WorldEventConfig(ctx context.Context) (domain.WorldEventConfig, error)
	UpdateWorldEventProgress(ctx context.Context, expectedVersion int64, currentIndex int32) (bool, error)
	SetKefraState(ctx context.Context, live bool, guildID int32, fonte string, accountID int64) (int64, error)
}

// WorldEventConfigServer implements dbv1.WorldEventConfigServiceServer.
type WorldEventConfigServer struct {
	dbv1.UnimplementedWorldEventConfigServiceServer
	store WorldEventConfigStore
}

// NewWorldEventConfig builds the service over the given store.
func NewWorldEventConfig(s WorldEventConfigStore) *WorldEventConfigServer {
	return &WorldEventConfigServer{store: s}
}

// WorldEventConfigVersion returns the monotonic config version for tmServer.
func (s *WorldEventConfigServer) WorldEventConfigVersion(ctx context.Context, _ *dbv1.WorldEventConfigVersionRequest) (*dbv1.WorldEventConfigVersionResponse, error) {
	v, err := s.store.WorldEventConfigVersion(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "world event config version: %v", err)
	}
	return &dbv1.WorldEventConfigVersionResponse{Version: v}, nil
}

// GetWorldEventConfig returns the full config snapshot and its version.
func (s *WorldEventConfigServer) GetWorldEventConfig(ctx context.Context, _ *dbv1.GetWorldEventConfigRequest) (*dbv1.GetWorldEventConfigResponse, error) {
	version, err := s.store.WorldEventConfigVersion(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "world event config version: %v", err)
	}
	cfg, err := s.store.WorldEventConfig(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "world event config: %v", err)
	}
	return &dbv1.GetWorldEventConfigResponse{Version: version, Config: worldEventConfigToDBProto(cfg)}, nil
}

// UpdateWorldEventProgress persists tmServer's live event counter.
func (s *WorldEventConfigServer) UpdateWorldEventProgress(ctx context.Context, req *dbv1.UpdateWorldEventProgressRequest) (*dbv1.UpdateWorldEventProgressResponse, error) {
	applied, err := s.store.UpdateWorldEventProgress(ctx, req.GetExpectedVersion(), req.GetCurrentIndex())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update world event progress: %v", err)
	}
	return &dbv1.UpdateWorldEventProgressResponse{Applied: applied}, nil
}

// SetKefraState records the Kefra state the tmServer asks for — the boss dying,
// the weekly return. The write is the game's, so it is audited with the source
// "jogo" and no moderator account.
func (s *WorldEventConfigServer) SetKefraState(ctx context.Context, req *dbv1.SetKefraStateRequest) (*dbv1.SetKefraStateResponse, error) {
	if req.GetGuildId() < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "set kefra state: negative guild %d", req.GetGuildId())
	}
	v, err := s.store.SetKefraState(ctx, req.GetLive(), req.GetGuildId(), store.FonteEventoJogo, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "set kefra state: %v", err)
	}
	return &dbv1.SetKefraStateResponse{Version: v}, nil
}

func worldEventConfigToDBProto(cfg domain.WorldEventConfig) *dbv1.WorldEventConfig {
	return &dbv1.WorldEventConfig{
		Enabled: cfg.Enabled, ItemIndex: cfg.ItemIndex, Rate: cfg.Rate,
		StartIndex: cfg.StartIndex, CurrentIndex: cfg.CurrentIndex, EndIndex: cfg.EndIndex,
		Indexed: cfg.Indexed, NoticeEnabled: cfg.NoticeEnabled,
		DoubleExpEnabled: cfg.DoubleExpEnabled, NewbieEventEnabled: cfg.NewbieEventEnabled,
		KefraLiveEnabled: cfg.KefraLiveEnabled,
		// Always present: presence is how tmServer tells this dbServer from one
		// that predates the fields (see the proto).
		TowerWarEnabled:  proto.Bool(cfg.TowerWarEnabled),
		TowerWarHour:     proto.Int32(cfg.TowerWarHour),
		BossRespawnHours: proto.Int32(cfg.BossRespawnHours),
		// Always five values: the length is how tmServer tells this dbServer from
		// one that predates migration 0073.
		RoundXpCap:       cfg.RoundXPCap[:],
		RoundXpCapDouble: cfg.RoundXPCapDouble[:],
		KefraGuildId:     proto.Int32(cfg.KefraGuildID),
	}
}
