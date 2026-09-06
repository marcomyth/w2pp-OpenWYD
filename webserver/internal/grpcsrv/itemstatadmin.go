package grpcsrv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemstatadmin"
)

// ItemStatAdmin is the moderator item-stat-editing surface the server depends
// on (satisfied by *itemstatadmin.Service). Kept as an interface so the server
// is unit-testable.
type ItemStatAdmin interface {
	Get(ctx context.Context, moderatorID int64, itemIndex int32) (itemstatadmin.Result, itemstatadmin.Item, error)
	Upsert(ctx context.Context, moderatorID int64, st domain.ItemStat) (itemstatadmin.Result, error)
	Delete(ctx context.Context, moderatorID int64, itemIndex int32) (itemstatadmin.Result, error)
}

// ItemStatAdminServer implements webv1.ItemStatAdminServiceServer.
type ItemStatAdminServer struct {
	webv1.UnimplementedItemStatAdminServiceServer
	admin ItemStatAdmin
}

// NewItemStatAdmin builds the ItemStatAdminService over the given admin logic.
func NewItemStatAdmin(a ItemStatAdmin) *ItemStatAdminServer {
	return &ItemStatAdminServer{admin: a}
}

// adminItemStatFields reaches the wire message by column name, so both
// conversions below can walk domain.ItemStatFields — the one table that says
// which column carries which effect — instead of listing 43 fields twice more.
//
// The wire type is int32 because proto3 has no 16-bit scalar; nothing can
// overflow on the way out, and the way in narrows what the database column
// would narrow anyway.
var adminItemStatFields = map[string]struct {
	get func(*webv1.AdminItemStat) int32
	set func(*webv1.AdminItemStat, int32)
}{
	"req_level": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetReqLevel() },
		set: func(p *webv1.AdminItemStat, v int32) { p.ReqLevel = v },
	},
	"req_str": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetReqStr() },
		set: func(p *webv1.AdminItemStat, v int32) { p.ReqStr = v },
	},
	"req_int": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetReqInt() },
		set: func(p *webv1.AdminItemStat, v int32) { p.ReqInt = v },
	},
	"req_dex": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetReqDex() },
		set: func(p *webv1.AdminItemStat, v int32) { p.ReqDex = v },
	},
	"req_con": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetReqCon() },
		set: func(p *webv1.AdminItemStat, v int32) { p.ReqCon = v },
	},
	"damage": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetDamage() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Damage = v },
	},
	"damageadd": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetDamageadd() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Damageadd = v },
	},
	"ac": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetAc() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Ac = v },
	},
	"acadd": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetAcadd() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Acadd = v },
	},
	"magic": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetMagic() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Magic = v },
	},
	"magicadd": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetMagicadd() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Magicadd = v },
	},
	"critical": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetCritical() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Critical = v },
	},
	"critical2": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetCritical2() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Critical2 = v },
	},
	"runspeed": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetRunspeed() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Runspeed = v },
	},
	"str": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetStr() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Str = v },
	},
	"intel": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetIntel() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Intel = v },
	},
	"dex": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetDex() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Dex = v },
	},
	"con": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetCon() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Con = v },
	},
	"hp": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetHp() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Hp = v },
	},
	"hpadd": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetHpadd() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Hpadd = v },
	},
	"hpadd2": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetHpadd2() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Hpadd2 = v },
	},
	"mp": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetMp() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Mp = v },
	},
	"mpadd": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetMpadd() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Mpadd = v },
	},
	"mpadd2": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetMpadd2() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Mpadd2 = v },
	},
	"resist1": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetResist1() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Resist1 = v },
	},
	"resist2": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetResist2() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Resist2 = v },
	},
	"resist3": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetResist3() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Resist3 = v },
	},
	"resist4": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetResist4() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Resist4 = v },
	},
	"resistall": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetResistall() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Resistall = v },
	},
	"special1": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetSpecial1() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Special1 = v },
	},
	"special2": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetSpecial2() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Special2 = v },
	},
	"special3": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetSpecial3() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Special3 = v },
	},
	"special4": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetSpecial4() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Special4 = v },
	},
	"specialall": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetSpecialall() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Specialall = v },
	},
	"itemlevel": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetItemlevel() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Itemlevel = v },
	},
	"itemtype": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetItemtype() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Itemtype = v },
	},
	"mobtype": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetMobtype() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Mobtype = v },
	},
	"wtype": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetWtype() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Wtype = v },
	},
	"pos": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetPos() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Pos = v },
	},
	"sanc": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetSanc() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Sanc = v },
	},
	"nosanc": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetNosanc() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Nosanc = v },
	},
	"incubate": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetIncubate() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Incubate = v },
	},
	"incudelay": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetIncudelay() },
		set: func(p *webv1.AdminItemStat, v int32) { p.Incudelay = v },
	},
	"ef_range": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetEfRange() },
		set: func(p *webv1.AdminItemStat, v int32) { p.EfRange = v },
	},
	"ef_volatile": {
		get: func(p *webv1.AdminItemStat) int32 { return p.GetEfVolatile() },
		set: func(p *webv1.AdminItemStat, v int32) { p.EfVolatile = v },
	},
}

func itemStatToProto(it itemstatadmin.Item) *webv1.AdminItemStat {
	pb := &webv1.AdminItemStat{
		ItemIndex:   it.Stat.ItemIndex,
		Overridden:  it.Overridden,
		DisplayName: it.DisplayName,
	}
	st := it.Stat
	for _, f := range domain.ItemStatFields {
		adminItemStatFields[f.Col].set(pb, int32(*f.Ptr(&st)))
	}
	return pb
}

func itemStatFromProto(pb *webv1.AdminItemStat) domain.ItemStat {
	st := domain.ItemStat{ItemIndex: pb.GetItemIndex()}
	for _, f := range domain.ItemStatFields {
		*f.Ptr(&st) = int16(adminItemStatFields[f.Col].get(pb))
	}
	return st
}

// GetItemStat returns an item's override, or a read-through of the catalog's
// own numbers when it has none.
func (s *ItemStatAdminServer) GetItemStat(ctx context.Context, req *webv1.GetItemStatRequest) (*webv1.GetItemStatResponse, error) {
	res, it, err := s.admin.Get(ctx, req.GetModeratorId(), req.GetItemIndex())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get item stat: %v", err)
	}
	out := &webv1.GetItemStatResponse{Result: itemStatResultToProto(res)}
	if res == itemstatadmin.OK {
		out.Stat = itemStatToProto(it)
	}
	return out, nil
}

// UpsertItemStat writes an item's override whole.
func (s *ItemStatAdminServer) UpsertItemStat(ctx context.Context, req *webv1.UpsertItemStatRequest) (*webv1.AdminAck, error) {
	if req.GetStat() == nil {
		return &webv1.AdminAck{Result: webv1.AdminResult_ADMIN_RESULT_INVALID}, nil
	}
	res, err := s.admin.Upsert(ctx, req.GetModeratorId(), itemStatFromProto(req.GetStat()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "upsert item stat: %v", err)
	}
	return &webv1.AdminAck{Result: itemStatResultToProto(res)}, nil
}

// DeleteItemStat drops the override so ItemList.csv applies again.
func (s *ItemStatAdminServer) DeleteItemStat(ctx context.Context, req *webv1.DeleteItemStatRequest) (*webv1.AdminAck, error) {
	res, err := s.admin.Delete(ctx, req.GetModeratorId(), req.GetItemIndex())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "delete item stat: %v", err)
	}
	return &webv1.AdminAck{Result: itemStatResultToProto(res)}, nil
}

func itemStatResultToProto(r itemstatadmin.Result) webv1.AdminResult {
	switch r {
	case itemstatadmin.OK:
		return webv1.AdminResult_ADMIN_RESULT_OK
	case itemstatadmin.Forbidden:
		return webv1.AdminResult_ADMIN_RESULT_FORBIDDEN
	case itemstatadmin.NotFound:
		return webv1.AdminResult_ADMIN_RESULT_NOT_FOUND
	default:
		return webv1.AdminResult_ADMIN_RESULT_INVALID
	}
}
