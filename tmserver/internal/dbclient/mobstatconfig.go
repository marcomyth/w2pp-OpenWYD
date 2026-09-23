package dbclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/mobstat"
)

// MobStatSource fetches the moderator-edited mob/NPC template stat overrides
// (mob-template-editing-plan.md) from the dbServer's NpcConfigService. It is
// deliberately independent of NpcConfig/npccfg: a stat override applies to ANY
// npc/<template_name> file, not just the DB-managed merchant subset NpcConfig
// materializes.
//
// It used to be a boot-only read ("restart to apply", matching EDITAPPMOB). That
// cost a real night: a moderator raised a monster's EXP at 21:18, the game kept
// paying the old value, and it only took effect at 22:19 when an unrelated deploy
// happened to restart production. Now Version() lets the tmServer poll and
// reload, the same shape the Mesa de XP uses.
//
// THE VERSION IS SHARED, on purpose. Every mob-stat mutation already bumps
// npc_config_meta — internal/store/mobtemplate.go routes both UpsertMobTemplateStat
// and DeleteMobTemplateStat through auditAndBump, reusing npc_audit/npc_config_meta
// instead of a parallel trail. So the version and the RPC to read it already
// existed and were already exercised; adding a dedicated mob_template_stat_meta
// would have meant a migration plus a proto change for no behaviour difference.
// The cost of sharing is that NPC and shop edits bump it too, so a reload can
// find nothing to rebuild. That is the NORMAL case of sharing, not a defect —
// see the fichas=0 line in handler.pollMobStats.
type MobStatSource struct {
	api dbv1.NpcConfigServiceClient
}

// NewMobStatSource wraps a gRPC connection as a MobStatSource.
func NewMobStatSource(conn grpc.ClientConnInterface) *MobStatSource {
	return &MobStatSource{api: dbv1.NewNpcConfigServiceClient(conn)}
}

// Version returns the config version the overrides live under (cheap poll). It
// is npc_config_meta's, shared with the NPC/shop config — see the type doc.
func (c *MobStatSource) Version(ctx context.Context) (int64, error) {
	resp, err := c.api.NpcConfigVersion(ctx, &dbv1.NpcConfigVersionRequest{})
	if err != nil {
		return 0, fmt.Errorf("dbclient: mob stat version: %w", err)
	}
	return resp.GetVersion(), nil
}

// Fetch returns every template stat override, keyed by template_name, ready
// to apply over raw template bytes via mobstat.Apply.
func (c *MobStatSource) Fetch(ctx context.Context) (map[string]mobstat.Override, error) {
	resp, err := c.api.ListMobTemplateStats(ctx, &dbv1.ListMobTemplateStatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("dbclient: list mob template stats: %w", err)
	}
	out := make(map[string]mobstat.Override, len(resp.GetOverrides()))
	for _, st := range resp.GetOverrides() {
		ov := mobstat.Override{
			DisplayName: st.GetDisplayName(),
			Clan:        uint8(st.GetClan()), Merchant: uint8(st.GetMerchant()), Class: uint8(st.GetClass()),
			Coin: st.GetCoin(), Exp: st.GetExp(), SPX: int16(st.GetSpx()), SPY: int16(st.GetSpy()),
			Level: st.GetLevel(), AC: st.GetAc(), Damage: st.GetDamage(), ChaosRate: uint8(st.GetChaosRate()),
			AttackRun: uint8(st.GetAttackRun()), Direction: uint8(st.GetDirection()),
			Str: int16(st.GetStr()), Int: int16(st.GetIntel()), Dex: int16(st.GetDex()), Con: int16(st.GetCon()),
			Special: [4]int16{int16(st.GetSpecial1()), int16(st.GetSpecial2()), int16(st.GetSpecial3()), int16(st.GetSpecial4())},
			MaxHp:   st.GetMaxHp(), Hp: st.GetHp(), MaxMp: st.GetMaxMp(), Mp: st.GetMp(),
			LearnedSkill: st.GetLearnedSkill(), ScoreBonus: uint16(st.GetScoreBonus()),
			SkillBar: [4]uint8{uint8(st.GetSkillBar1()), uint8(st.GetSkillBar2()), uint8(st.GetSkillBar3()), uint8(st.GetSkillBar4())},
			RegenHP:  uint16(st.GetRegenHp()), RegenMP: uint16(st.GetRegenMp()),
			Resist: [4]int8{int8(st.GetResist1()), int8(st.GetResist2()), int8(st.GetResist3()), int8(st.GetResist4())},
		}
		for _, it := range st.GetEquip() {
			ov.Equip = append(ov.Equip, mobstat.EquipItem{
				Slot: int(it.GetSlot()), Index: uint16(it.GetItemIndex()),
				Eff: [3][2]uint8{
					{uint8(it.GetEff1()), uint8(it.GetEffv1())},
					{uint8(it.GetEff2()), uint8(it.GetEffv2())},
					{uint8(it.GetEff3()), uint8(it.GetEffv3())},
				},
			})
		}
		out[st.GetTemplateName()] = ov
	}
	return out, nil
}
