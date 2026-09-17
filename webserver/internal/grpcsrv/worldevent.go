package grpcsrv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/worldevent"
)

// WorldEventAdmin is the moderator world-event surface (satisfied by
// *worldevent.Service). Kept as an interface so the server is unit-testable.
type WorldEventAdmin interface {
	Get(ctx context.Context, moderatorID int64) (worldevent.Result, int64, domain.WorldEventConfig, error)
	Set(ctx context.Context, moderatorID int64, cfg domain.WorldEventConfig) (worldevent.Result, error)
}

// WorldEventAdminServer implements webv1.WorldEventAdminServiceServer.
type WorldEventAdminServer struct {
	webv1.UnimplementedWorldEventAdminServiceServer
	admin WorldEventAdmin
}

// NewWorldEventAdmin builds the WorldEventAdminService over the given admin logic.
func NewWorldEventAdmin(a WorldEventAdmin) *WorldEventAdminServer {
	return &WorldEventAdminServer{admin: a}
}

// GetWorldEventConfig returns the current portal-managed event settings.
func (s *WorldEventAdminServer) GetWorldEventConfig(ctx context.Context, req *webv1.GetWorldEventConfigRequest) (*webv1.GetWorldEventConfigResponse, error) {
	res, version, cfg, err := s.admin.Get(ctx, req.GetModeratorId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get world event config: %v", err)
	}
	return &webv1.GetWorldEventConfigResponse{
		Result:  worldEventResultToProto(res),
		Version: version,
		Config:  worldEventConfigToWebProto(cfg),
	}, nil
}

// SetWorldEventConfig replaces the current event settings.
//
// The optional fields are the exception to "replaces": the Tower War pair and
// the lone-boss respawn. A caller that predates them (the portal BFF until it
// learns the fields) sends none, and a plain replace would switch the daily war
// off at midnight — and bring the bosses back every few seconds, or fail the
// column's CHECK — every time somebody saved the EXP switches. An absent field
// keeps what is stored instead.
func (s *WorldEventAdminServer) SetWorldEventConfig(ctx context.Context, req *webv1.SetWorldEventConfigRequest) (*webv1.AdminAck, error) {
	in := req.GetConfig()
	cfg := webProtoToWorldEventConfig(in)
	// GetConfig is nil when the request carries no config at all; that is
	// "nothing sent" for these as well.
	temLigada := in != nil && in.TowerWarEnabled != nil
	temHora := in != nil && in.TowerWarHour != nil
	temChefes := in != nil && in.BossRespawnHours != nil
	// The round XP cap (migration 0073) is present only as five values per list;
	// anything else keeps what is stored, so an older caller cannot switch the cap
	// off by sending nothing.
	temTeto := len(in.GetRoundXpCap()) == len(cfg.RoundXPCap)
	temTetoDobro := len(in.GetRoundXpCapDouble()) == len(cfg.RoundXPCapDouble)
	if !temLigada || !temHora || !temChefes || !temTeto || !temTetoDobro {
		res, _, atual, err := s.admin.Get(ctx, req.GetModeratorId())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "set world event config: read current: %v", err)
		}
		if res != worldevent.OK {
			return &webv1.AdminAck{Result: worldEventResultToProto(res)}, nil
		}
		if !temLigada {
			cfg.TowerWarEnabled = atual.TowerWarEnabled
		}
		if !temHora {
			cfg.TowerWarHour = atual.TowerWarHour
		}
		if !temChefes {
			cfg.BossRespawnHours = atual.BossRespawnHours
		}
		if !temTeto {
			cfg.RoundXPCap = atual.RoundXPCap
		}
		if !temTetoDobro {
			cfg.RoundXPCapDouble = atual.RoundXPCapDouble
		}
	}
	res, err := s.admin.Set(ctx, req.GetModeratorId(), cfg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "set world event config: %v", err)
	}
	return &webv1.AdminAck{Result: worldEventResultToProto(res)}, nil
}

func worldEventConfigToWebProto(cfg domain.WorldEventConfig) *webv1.WorldEventConfig {
	return &webv1.WorldEventConfig{
		Enabled: cfg.Enabled, ItemIndex: cfg.ItemIndex, Rate: cfg.Rate,
		StartIndex: cfg.StartIndex, CurrentIndex: cfg.CurrentIndex, EndIndex: cfg.EndIndex,
		Indexed: cfg.Indexed, NoticeEnabled: cfg.NoticeEnabled,
		DoubleExpEnabled: cfg.DoubleExpEnabled, NewbieEventEnabled: cfg.NewbieEventEnabled,
		KefraLiveEnabled: cfg.KefraLiveEnabled,
		// Always present, so the portal can tell a war switched off at midnight
		// from a webServer too old to know about it.
		TowerWarEnabled:  proto.Bool(cfg.TowerWarEnabled),
		TowerWarHour:     proto.Int32(cfg.TowerWarHour),
		BossRespawnHours: proto.Int32(cfg.BossRespawnHours),
		RoundXpCap:       cfg.RoundXPCap[:],
		RoundXpCapDouble: cfg.RoundXPCapDouble[:],
	}
}

// webProtoToWorldEventConfig maps a request config. Absent optional fields come
// out as their zero values here; SetWorldEventConfig replaces them with the
// stored ones before anything is written.
func webProtoToWorldEventConfig(cfg *webv1.WorldEventConfig) domain.WorldEventConfig {
	return domain.WorldEventConfig{
		Enabled: cfg.GetEnabled(), ItemIndex: cfg.GetItemIndex(), Rate: cfg.GetRate(),
		StartIndex: cfg.GetStartIndex(), CurrentIndex: cfg.GetCurrentIndex(), EndIndex: cfg.GetEndIndex(),
		Indexed: cfg.GetIndexed(), NoticeEnabled: cfg.GetNoticeEnabled(),
		DoubleExpEnabled: cfg.GetDoubleExpEnabled(), NewbieEventEnabled: cfg.GetNewbieEventEnabled(),
		// Carried for the shape only: the store no longer writes the Kefra state
		// through this path (migration 0067). The game and the panel's own action
		// write it through SetKefraState, so this field is ignored on a set.
		KefraLiveEnabled: cfg.GetKefraLiveEnabled(),
		TowerWarEnabled:  cfg.GetTowerWarEnabled(), TowerWarHour: cfg.GetTowerWarHour(),
		BossRespawnHours: cfg.GetBossRespawnHours(),
		RoundXPCap:       cincoValores(cfg.GetRoundXpCap()),
		RoundXPCapDouble: cincoValores(cfg.GetRoundXpCapDouble()),
	}
}

// cincoValores copies a per-band cap list; a list of any other length comes out
// as zeros, and SetWorldEventConfig then keeps the stored values instead.
func cincoValores(v []int64) [5]int64 {
	var out [5]int64
	if len(v) == len(out) {
		copy(out[:], v)
	}
	return out
}

func worldEventResultToProto(r worldevent.Result) webv1.AdminResult {
	switch r {
	case worldevent.OK:
		return webv1.AdminResult_ADMIN_RESULT_OK
	case worldevent.Forbidden:
		return webv1.AdminResult_ADMIN_RESULT_FORBIDDEN
	default:
		return webv1.AdminResult_ADMIN_RESULT_INVALID
	}
}
