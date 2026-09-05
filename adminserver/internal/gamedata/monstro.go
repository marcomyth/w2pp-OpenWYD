package gamedata

import (
	"context"
	"fmt"
	"sort"
	"strings"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
)

// MobTemplate is one STRUCT_MOB template file, for the picker.
type MobTemplate struct {
	Name        string
	DisplayName string
	Merchant    int32
}

// MobStat is a template's editable numbers.
//
// The proto value is kept whole rather than copied field by field. Upsert
// replaces the entire override row, so anything this package failed to carry
// across would be silently zeroed — including the equipment list, which has its
// own RPC and no representation in this form.
type MobStat struct {
	raw *webv1.AdminMobTemplateStat
}

// Name is the template this override belongs to.
func (m MobStat) Name() string { return m.raw.GetTemplateName() }

// DisplayName is the name shown in game; empty keeps the template file's own.
func (m MobStat) DisplayName() string { return m.raw.GetDisplayName() }

// Overridden reports whether these numbers are a saved override or a
// read-through of the template file. False means nothing has been edited yet and
// what is shown is the file's real starting values.
func (m MobStat) Overridden() bool { return m.raw.GetOverridden() }

// NewMobStat builds a stat value carrying only its identity.
//
// Nothing in the panel constructs one in production: it always edits what
// MobStat returned, so the equipment list and every other field this form does
// not show survive the round trip. This exists because raw is unexported, which
// otherwise makes MobStat impossible to build outside this package — and so
// makes the handlers that edit it impossible to test against a stand-in.
func NewMobStat(name string, overridden bool) MobStat {
	return MobStat{raw: &webv1.AdminMobTemplateStat{TemplateName: name, Overridden: overridden}}
}

// MobField is one editable number, ready for a form.
type MobField struct {
	Nome   string // form field name
	Rotulo string
	Grupo  string
	Valor  int64
}

// mobField binds a form field to its place in the proto message.
//
// A table rather than reflection: the compiler checks every accessor, so a
// renamed proto field becomes a build error instead of a number that silently
// stops being saved.
type mobField struct {
	Nome, Rotulo, Grupo string
	Get                 func(*webv1.AdminMobTemplateStat) int64
	Set                 func(*webv1.AdminMobTemplateStat, int64)
}

func i32(get func(*webv1.AdminMobTemplateStat) int32, set func(*webv1.AdminMobTemplateStat, int32)) (func(*webv1.AdminMobTemplateStat) int64, func(*webv1.AdminMobTemplateStat, int64)) {
	return func(s *webv1.AdminMobTemplateStat) int64 { return int64(get(s)) },
		func(s *webv1.AdminMobTemplateStat, v int64) { set(s, int32(v)) }
}

var camposMob = func() []mobField {
	f := func(nome, rotulo, grupo string, get func(*webv1.AdminMobTemplateStat) int32, set func(*webv1.AdminMobTemplateStat, int32)) mobField {
		g, s := i32(get, set)
		return mobField{Nome: nome, Rotulo: rotulo, Grupo: grupo, Get: g, Set: s}
	}
	return []mobField{
		f("level", "Nível", "Combate", func(s *webv1.AdminMobTemplateStat) int32 { return s.Level }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Level = v }),
		f("damage", "Dano", "Combate", func(s *webv1.AdminMobTemplateStat) int32 { return s.Damage }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Damage = v }),
		f("ac", "Defesa", "Combate", func(s *webv1.AdminMobTemplateStat) int32 { return s.Ac }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Ac = v }),
		f("attack_run", "Velocidade de ataque", "Combate", func(s *webv1.AdminMobTemplateStat) int32 { return s.AttackRun }, func(s *webv1.AdminMobTemplateStat, v int32) { s.AttackRun = v }),
		f("chaos_rate", "Taxa de caos", "Combate", func(s *webv1.AdminMobTemplateStat) int32 { return s.ChaosRate }, func(s *webv1.AdminMobTemplateStat, v int32) { s.ChaosRate = v }),

		f("max_hp", "HP máximo", "Vida", func(s *webv1.AdminMobTemplateStat) int32 { return s.MaxHp }, func(s *webv1.AdminMobTemplateStat, v int32) { s.MaxHp = v }),
		f("hp", "HP atual", "Vida", func(s *webv1.AdminMobTemplateStat) int32 { return s.Hp }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Hp = v }),
		f("max_mp", "MP máximo", "Vida", func(s *webv1.AdminMobTemplateStat) int32 { return s.MaxMp }, func(s *webv1.AdminMobTemplateStat, v int32) { s.MaxMp = v }),
		f("mp", "MP atual", "Vida", func(s *webv1.AdminMobTemplateStat) int32 { return s.Mp }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Mp = v }),
		f("regen_hp", "Regeneração de HP", "Vida", func(s *webv1.AdminMobTemplateStat) int32 { return s.RegenHp }, func(s *webv1.AdminMobTemplateStat, v int32) { s.RegenHp = v }),
		f("regen_mp", "Regeneração de MP", "Vida", func(s *webv1.AdminMobTemplateStat) int32 { return s.RegenMp }, func(s *webv1.AdminMobTemplateStat, v int32) { s.RegenMp = v }),

		f("coin", "Ouro dado", "Recompensa", func(s *webv1.AdminMobTemplateStat) int32 { return s.Coin }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Coin = v }),

		f("str", "Força", "Atributos", func(s *webv1.AdminMobTemplateStat) int32 { return s.Str }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Str = v }),
		f("intel", "Inteligência", "Atributos", func(s *webv1.AdminMobTemplateStat) int32 { return s.Intel }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Intel = v }),
		f("dex", "Destreza", "Atributos", func(s *webv1.AdminMobTemplateStat) int32 { return s.Dex }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Dex = v }),
		f("con", "Constituição", "Atributos", func(s *webv1.AdminMobTemplateStat) int32 { return s.Con }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Con = v }),

		f("resist1", "Resistência ao fogo", "Resistências", func(s *webv1.AdminMobTemplateStat) int32 { return s.Resist1 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Resist1 = v }),
		f("resist2", "Resistência ao gelo", "Resistências", func(s *webv1.AdminMobTemplateStat) int32 { return s.Resist2 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Resist2 = v }),
		f("resist3", "Resistência ao raio", "Resistências", func(s *webv1.AdminMobTemplateStat) int32 { return s.Resist3 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Resist3 = v }),
		f("resist4", "Resistência à magia", "Resistências", func(s *webv1.AdminMobTemplateStat) int32 { return s.Resist4 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Resist4 = v }),

		f("clan", "Clã", "Identidade", func(s *webv1.AdminMobTemplateStat) int32 { return s.Clan }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Clan = v }),
		f("class", "Classe", "Identidade", func(s *webv1.AdminMobTemplateStat) int32 { return s.Class }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Class = v }),
		f("merchant", "Tipo de vendedor", "Identidade", func(s *webv1.AdminMobTemplateStat) int32 { return s.Merchant }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Merchant = v }),
		f("direction", "Direção", "Identidade", func(s *webv1.AdminMobTemplateStat) int32 { return s.Direction }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Direction = v }),
		f("spx", "Origem X", "Identidade", func(s *webv1.AdminMobTemplateStat) int32 { return s.Spx }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Spx = v }),
		f("spy", "Origem Y", "Identidade", func(s *webv1.AdminMobTemplateStat) int32 { return s.Spy }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Spy = v }),

		f("learned_skill", "Habilidades aprendidas", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.LearnedSkill }, func(s *webv1.AdminMobTemplateStat, v int32) { s.LearnedSkill = v }),
		f("score_bonus", "Pontos livres", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.ScoreBonus }, func(s *webv1.AdminMobTemplateStat, v int32) { s.ScoreBonus = v }),
		f("skill_bar1", "Barra de habilidade 1", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.SkillBar1 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.SkillBar1 = v }),
		f("skill_bar2", "Barra de habilidade 2", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.SkillBar2 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.SkillBar2 = v }),
		f("skill_bar3", "Barra de habilidade 3", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.SkillBar3 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.SkillBar3 = v }),
		f("skill_bar4", "Barra de habilidade 4", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.SkillBar4 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.SkillBar4 = v }),
		f("special1", "Maestria 1", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.Special1 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Special1 = v }),
		f("special2", "Maestria 2", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.Special2 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Special2 = v }),
		f("special3", "Maestria 3", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.Special3 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Special3 = v }),
		f("special4", "Maestria 4", "Habilidades", func(s *webv1.AdminMobTemplateStat) int32 { return s.Special4 }, func(s *webv1.AdminMobTemplateStat, v int32) { s.Special4 = v }),

		// Exp is the one 64-bit field, so it does not go through the int32 helper.
		{
			Nome: "exp", Rotulo: "Experiência dada", Grupo: "Recompensa",
			Get: func(s *webv1.AdminMobTemplateStat) int64 { return s.Exp },
			Set: func(s *webv1.AdminMobTemplateStat, v int64) { s.Exp = v },
		},
	}
}()

// Fields returns the editable numbers, in form order.
func (m MobStat) Fields() []MobField {
	out := make([]MobField, 0, len(camposMob))
	for _, c := range camposMob {
		out = append(out, MobField{Nome: c.Nome, Rotulo: c.Rotulo, Grupo: c.Grupo, Valor: c.Get(m.raw)})
	}
	return out
}

// GruposMob returns the field group names, in the order they should be shown.
func GruposMob() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range camposMob {
		if !seen[c.Grupo] {
			seen[c.Grupo] = true
			out = append(out, c.Grupo)
		}
	}
	return out
}

// Set writes one field by its form name, reporting whether the name is known.
func (m MobStat) Set(nome string, valor int64) bool {
	for _, c := range camposMob {
		if c.Nome == nome {
			c.Set(m.raw, valor)
			return true
		}
	}
	return false
}

// SetDisplayName changes the in-game name; empty keeps the template file's own.
func (m MobStat) SetDisplayName(v string) { m.raw.DisplayName = v }

// MobTemplates lists every template file in the content tree.
func (c *Client) MobTemplates(ctx context.Context, moderatorID int64, query string) ([]MobTemplate, error) {
	resp, err := c.mob.ListMobTemplates(ctx, &webv1.ListMobTemplatesRequest{ModeratorId: moderatorID})
	if err != nil {
		return nil, fmt.Errorf("gamedata: list mob templates: %w", err)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]MobTemplate, 0, len(resp.GetTemplates()))
	for _, t := range resp.GetTemplates() {
		if q != "" && !strings.Contains(strings.ToLower(t.GetDisplayName()), q) &&
			!strings.Contains(strings.ToLower(t.GetTemplateName()), q) {
			continue
		}
		out = append(out, MobTemplate{
			Name: t.GetTemplateName(), DisplayName: t.GetDisplayName(), Merchant: t.GetMerchant(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// MobStat reads a template's override, or the template file's own values when
// none has been saved — so the editor opens on real numbers, not zeros.
func (c *Client) MobStat(ctx context.Context, moderatorID int64, name string) (MobStat, error) {
	resp, err := c.mob.GetMobTemplateStat(ctx, &webv1.GetMobTemplateStatRequest{
		ModeratorId: moderatorID, TemplateName: name,
	})
	if err != nil {
		return MobStat{}, fmt.Errorf("gamedata: get mob stat %q: %w", name, err)
	}
	if err := resultErr(resp.GetResult()); err != nil {
		return MobStat{}, err
	}
	stat := resp.GetStat()
	if stat == nil {
		return MobStat{}, ErrNotFound
	}
	return MobStat{raw: stat}, nil
}

// SaveMobStat writes the override back whole.
func (c *Client) SaveMobStat(ctx context.Context, moderatorID int64, m MobStat) error {
	resp, err := c.mob.UpsertMobTemplateStat(ctx, &webv1.UpsertMobTemplateStatRequest{
		ModeratorId: moderatorID, Stat: m.raw,
	})
	if err != nil {
		return fmt.Errorf("gamedata: save mob stat %q: %w", m.Name(), err)
	}
	return resultErr(resp.GetResult())
}

// MobEquipItem is one Equip[] slot of a mob template.
type MobEquipItem struct {
	Slot      int
	ItemIndex int32
	Eff1      uint8
	EffV1     uint8
	Eff2      uint8
	EffV2     uint8
	Eff3      uint8
	EffV3     uint8
}

// Vazio reports whether the slot holds nothing.
func (m MobEquipItem) Vazio() bool { return m.ItemIndex == 0 }

// Equip returns the template's sixteen equipment slots, positionally, so the
// editor can draw the same grid it draws for a character.
//
// The values ride along on the stat read, which is why there is no separate RPC
// here: GetMobTemplateStat already carries them, and asking twice would let the
// stats and the gear come from two different reads of the same row.
func (m MobStat) Equip() []MobEquipItem {
	out := make([]MobEquipItem, maxMobEquip)
	for i := range out {
		out[i].Slot = i
	}
	if m.raw == nil {
		return out
	}
	for _, e := range m.raw.GetEquip() {
		slot := int(e.GetSlot())
		if slot < 0 || slot >= maxMobEquip {
			continue // corrupt row: skip it rather than lose the whole grid
		}
		out[slot] = MobEquipItem{
			Slot: slot, ItemIndex: e.GetItemIndex(),
			Eff1: uint8(e.GetEff1()), EffV1: uint8(e.GetEffv1()),
			Eff2: uint8(e.GetEff2()), EffV2: uint8(e.GetEffv2()),
			Eff3: uint8(e.GetEff3()), EffV3: uint8(e.GetEffv3()),
		}
	}
	return out
}

// maxMobEquip is MAX_EQUIP (STRUCT_MOB.Equip[16]).
const maxMobEquip = 16

// SaveMobEquip replaces the template's Equip[] overrides.
//
// The webServer requires a stat override to exist first, so a template nobody
// has edited yet has to be saved once before it can be given gear — the caller
// does that rather than this package guessing, because an implicit stat write
// would silently freeze the file's current values as an override.
func (c *Client) SaveMobEquip(ctx context.Context, moderatorID int64, name string, itens []MobEquipItem) error {
	req := &webv1.SetMobTemplateEquipRequest{ModeratorId: moderatorID, TemplateName: name}
	for _, it := range itens {
		if it.Vazio() {
			continue // empty slots are an absence, not a row
		}
		req.Items = append(req.Items, &webv1.AdminMobTemplateEquipItem{
			Slot: int32(it.Slot), ItemIndex: it.ItemIndex,
			Eff1: int32(it.Eff1), Effv1: int32(it.EffV1),
			Eff2: int32(it.Eff2), Effv2: int32(it.EffV2),
			Eff3: int32(it.Eff3), Effv3: int32(it.EffV3),
		})
	}
	resp, err := c.mob.SetMobTemplateEquip(ctx, req)
	if err != nil {
		return fmt.Errorf("gamedata: save mob equip %q: %w", name, err)
	}
	return resultErr(resp.GetResult())
}

// ClearMobStat drops the override so the template file's values apply again.
func (c *Client) ClearMobStat(ctx context.Context, moderatorID int64, name string) error {
	resp, err := c.mob.DeleteMobTemplateStat(ctx, &webv1.DeleteMobTemplateStatRequest{
		ModeratorId: moderatorID, TemplateName: name,
	})
	if err != nil {
		return fmt.Errorf("gamedata: clear mob stat %q: %w", name, err)
	}
	return resultErr(resp.GetResult())
}
