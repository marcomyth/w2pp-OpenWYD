package dbclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/itemstat"
)

// ItemStatSource fetches the moderator-edited item base stat overrides
// (0023_item_stats) from the dbServer's NpcConfigService. Sibling of
// MobStatSource, and fetched the same way: once at boot, no version poll,
// because there is no hot-reload for this feature.
type ItemStatSource struct {
	api dbv1.NpcConfigServiceClient
}

// NewItemStatSource wraps a gRPC connection as an ItemStatSource.
func NewItemStatSource(conn grpc.ClientConnInterface) *ItemStatSource {
	return &ItemStatSource{api: dbv1.NewNpcConfigServiceClient(conn)}
}

// itemEffectColumns binds each override column to the EF_* token ItemList.csv
// uses for the same number.
//
// The effect ids themselves are NOT repeated here: they are resolved through
// content.EffectID, so the catalog parser and this overlay cannot drift apart.
// A name this table gets wrong fails loudly at boot rather than quietly writing
// the wrong stat, which is the failure that would be almost impossible to spot
// in game — an item would simply grant something else.
var itemEffectColumns = []struct {
	ef  string
	get func(*dbv1.ItemStat) int32
}{
	{"EF_DAMAGE", func(s *dbv1.ItemStat) int32 { return s.GetDamage() }},
	{"EF_DAMAGEADD", func(s *dbv1.ItemStat) int32 { return s.GetDamageadd() }},
	{"EF_AC", func(s *dbv1.ItemStat) int32 { return s.GetAc() }},
	{"EF_ACADD", func(s *dbv1.ItemStat) int32 { return s.GetAcadd() }},
	{"EF_MAGIC", func(s *dbv1.ItemStat) int32 { return s.GetMagic() }},
	{"EF_MAGICADD", func(s *dbv1.ItemStat) int32 { return s.GetMagicadd() }},
	{"EF_CRITICAL", func(s *dbv1.ItemStat) int32 { return s.GetCritical() }},
	{"EF_CRITICAL2", func(s *dbv1.ItemStat) int32 { return s.GetCritical2() }},
	{"EF_RUNSPEED", func(s *dbv1.ItemStat) int32 { return s.GetRunspeed() }},

	{"EF_STR", func(s *dbv1.ItemStat) int32 { return s.GetStr() }},
	{"EF_INT", func(s *dbv1.ItemStat) int32 { return s.GetIntel() }},
	{"EF_DEX", func(s *dbv1.ItemStat) int32 { return s.GetDex() }},
	{"EF_CON", func(s *dbv1.ItemStat) int32 { return s.GetCon() }},

	{"EF_HP", func(s *dbv1.ItemStat) int32 { return s.GetHp() }},
	{"EF_HPADD", func(s *dbv1.ItemStat) int32 { return s.GetHpadd() }},
	{"EF_HPADD2", func(s *dbv1.ItemStat) int32 { return s.GetHpadd2() }},
	{"EF_MP", func(s *dbv1.ItemStat) int32 { return s.GetMp() }},
	{"EF_MPADD", func(s *dbv1.ItemStat) int32 { return s.GetMpadd() }},
	{"EF_MPADD2", func(s *dbv1.ItemStat) int32 { return s.GetMpadd2() }},

	{"EF_RESIST1", func(s *dbv1.ItemStat) int32 { return s.GetResist1() }},
	{"EF_RESIST2", func(s *dbv1.ItemStat) int32 { return s.GetResist2() }},
	{"EF_RESIST3", func(s *dbv1.ItemStat) int32 { return s.GetResist3() }},
	{"EF_RESIST4", func(s *dbv1.ItemStat) int32 { return s.GetResist4() }},
	{"EF_RESISTALL", func(s *dbv1.ItemStat) int32 { return s.GetResistall() }},

	{"EF_SPECIAL1", func(s *dbv1.ItemStat) int32 { return s.GetSpecial1() }},
	{"EF_SPECIAL2", func(s *dbv1.ItemStat) int32 { return s.GetSpecial2() }},
	{"EF_SPECIAL3", func(s *dbv1.ItemStat) int32 { return s.GetSpecial3() }},
	{"EF_SPECIAL4", func(s *dbv1.ItemStat) int32 { return s.GetSpecial4() }},
	{"EF_SPECIALALL", func(s *dbv1.ItemStat) int32 { return s.GetSpecialall() }},

	{"EF_ITEMLEVEL", func(s *dbv1.ItemStat) int32 { return s.GetItemlevel() }},
	{"EF_ITEMTYPE", func(s *dbv1.ItemStat) int32 { return s.GetItemtype() }},
	{"EF_MOBTYPE", func(s *dbv1.ItemStat) int32 { return s.GetMobtype() }},
	{"EF_WTYPE", func(s *dbv1.ItemStat) int32 { return s.GetWtype() }},
	{"EF_POS", func(s *dbv1.ItemStat) int32 { return s.GetPos() }},
	{"EF_SANC", func(s *dbv1.ItemStat) int32 { return s.GetSanc() }},
	{"EF_NOSANC", func(s *dbv1.ItemStat) int32 { return s.GetNosanc() }},
	{"EF_INCUBATE", func(s *dbv1.ItemStat) int32 { return s.GetIncubate() }},
	{"EF_INCUDELAY", func(s *dbv1.ItemStat) int32 { return s.GetIncudelay() }},
}

// Fetch returns every item stat override, keyed by item index, ready for
// itemstat.Apply.
func (c *ItemStatSource) Fetch(ctx context.Context) (map[int]itemstat.Override, error) {
	resp, err := c.api.ListItemStats(ctx, &dbv1.ListItemStatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("dbclient: list item stats: %w", err)
	}
	out := make(map[int]itemstat.Override, len(resp.GetOverrides()))
	for _, st := range resp.GetOverrides() {
		ov := itemstat.Override{
			Req: content.ItemReq{
				Lvl: int16(st.GetReqLevel()), Str: int16(st.GetReqStr()),
				Int: int16(st.GetReqInt()), Dex: int16(st.GetReqDex()), Con: int16(st.GetReqCon()),
			},
		}
		for _, col := range itemEffectColumns {
			v := col.get(st)
			// Zero means the effect is absent, not present-and-zero. The catalog
			// parser produces no pair for an effect a row does not carry, and an
			// EF with value 0 adds nothing to any score — so emitting one would
			// only change which effects the item is seen to HAVE, which some
			// recipe and refine gates read.
			if v == 0 {
				continue
			}
			id, ok := content.EffectID(col.ef)
			if !ok {
				return nil, fmt.Errorf("dbclient: item stat column names %s, which the score model does not know", col.ef)
			}
			ov.Effects = append(ov.Effects, content.BaseEffect{Eff: id, Val: int16(v)})
		}
		out[int(st.GetItemIndex())] = ov
	}
	return out, nil
}
