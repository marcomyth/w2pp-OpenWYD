package gamedata

import (
	"context"
	"fmt"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
)

// ItemStat is one catalog item base numbers: what it demands to equip and what
// it grants while equipped.
//
// The proto value is kept whole rather than copied field by field, for the same
// reason MobStat does it: the webServer replaces the entire override on save, so
// anything this package failed to carry across would be silently zeroed.
type ItemStat struct {
	raw *webv1.AdminItemStat
}

// Index is the catalog index this override belongs to.
func (m ItemStat) Index() int32 { return m.raw.GetItemIndex() }

// DisplayName is the catalog name. Read-only here: an item name lives in
// ItemList.csv and is not part of what a moderator can override.
func (m ItemStat) DisplayName() string { return m.raw.GetDisplayName() }

// Overridden reports whether these numbers are a saved override or a
// read-through of the catalog. False means nothing has been edited yet and what
// is shown is the catalog real current values.
func (m ItemStat) Overridden() bool { return m.raw.GetOverridden() }

// itemField binds a form field to its place in the proto message. A table
// rather than reflection, so the compiler checks every accessor.
type itemField struct {
	Nome, Rotulo, Grupo string
	Get                 func(*webv1.AdminItemStat) int32
	Set                 func(*webv1.AdminItemStat, int32)
}

var camposItem = func() []itemField {
	f := func(nome, rotulo, grupo string, get func(*webv1.AdminItemStat) int32, set func(*webv1.AdminItemStat, int32)) itemField {
		return itemField{Nome: nome, Rotulo: rotulo, Grupo: grupo, Get: get, Set: set}
	}
	return []itemField{
		f("req_level", "Nível mínimo", "Requisitos para equipar", func(s *webv1.AdminItemStat) int32 { return s.ReqLevel }, func(s *webv1.AdminItemStat, v int32) { s.ReqLevel = v }),
		f("req_str", "Força", "Requisitos para equipar", func(s *webv1.AdminItemStat) int32 { return s.ReqStr }, func(s *webv1.AdminItemStat, v int32) { s.ReqStr = v }),
		f("req_int", "Inteligência", "Requisitos para equipar", func(s *webv1.AdminItemStat) int32 { return s.ReqInt }, func(s *webv1.AdminItemStat, v int32) { s.ReqInt = v }),
		f("req_dex", "Destreza", "Requisitos para equipar", func(s *webv1.AdminItemStat) int32 { return s.ReqDex }, func(s *webv1.AdminItemStat, v int32) { s.ReqDex = v }),
		f("req_con", "Constituição", "Requisitos para equipar", func(s *webv1.AdminItemStat) int32 { return s.ReqCon }, func(s *webv1.AdminItemStat, v int32) { s.ReqCon = v }),
		f("damage", "Dano", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Damage }, func(s *webv1.AdminItemStat, v int32) { s.Damage = v }),
		f("damageadd", "Dano adicional", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Damageadd }, func(s *webv1.AdminItemStat, v int32) { s.Damageadd = v }),
		f("ac", "Defesa", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Ac }, func(s *webv1.AdminItemStat, v int32) { s.Ac = v }),
		f("acadd", "Defesa adicional", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Acadd }, func(s *webv1.AdminItemStat, v int32) { s.Acadd = v }),
		f("magic", "Ataque mágico", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Magic }, func(s *webv1.AdminItemStat, v int32) { s.Magic = v }),
		f("magicadd", "Ataque mágico adicional", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Magicadd }, func(s *webv1.AdminItemStat, v int32) { s.Magicadd = v }),
		f("critical", "Crítico", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Critical }, func(s *webv1.AdminItemStat, v int32) { s.Critical = v }),
		f("critical2", "Crítico (segundo)", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Critical2 }, func(s *webv1.AdminItemStat, v int32) { s.Critical2 = v }),
		f("runspeed", "Velocidade de corrida", "Combate", func(s *webv1.AdminItemStat) int32 { return s.Runspeed }, func(s *webv1.AdminItemStat, v int32) { s.Runspeed = v }),
		f("str", "Força", "Atributos que o item dá", func(s *webv1.AdminItemStat) int32 { return s.Str }, func(s *webv1.AdminItemStat, v int32) { s.Str = v }),
		f("intel", "Inteligência", "Atributos que o item dá", func(s *webv1.AdminItemStat) int32 { return s.Intel }, func(s *webv1.AdminItemStat, v int32) { s.Intel = v }),
		f("dex", "Destreza", "Atributos que o item dá", func(s *webv1.AdminItemStat) int32 { return s.Dex }, func(s *webv1.AdminItemStat, v int32) { s.Dex = v }),
		f("con", "Constituição", "Atributos que o item dá", func(s *webv1.AdminItemStat) int32 { return s.Con }, func(s *webv1.AdminItemStat, v int32) { s.Con = v }),
		f("hp", "HP", "Vida e mana", func(s *webv1.AdminItemStat) int32 { return s.Hp }, func(s *webv1.AdminItemStat, v int32) { s.Hp = v }),
		f("hpadd", "HP adicional", "Vida e mana", func(s *webv1.AdminItemStat) int32 { return s.Hpadd }, func(s *webv1.AdminItemStat, v int32) { s.Hpadd = v }),
		f("hpadd2", "HP adicional (segundo)", "Vida e mana", func(s *webv1.AdminItemStat) int32 { return s.Hpadd2 }, func(s *webv1.AdminItemStat, v int32) { s.Hpadd2 = v }),
		f("mp", "MP", "Vida e mana", func(s *webv1.AdminItemStat) int32 { return s.Mp }, func(s *webv1.AdminItemStat, v int32) { s.Mp = v }),
		f("mpadd", "MP adicional", "Vida e mana", func(s *webv1.AdminItemStat) int32 { return s.Mpadd }, func(s *webv1.AdminItemStat, v int32) { s.Mpadd = v }),
		f("mpadd2", "MP adicional (segundo)", "Vida e mana", func(s *webv1.AdminItemStat) int32 { return s.Mpadd2 }, func(s *webv1.AdminItemStat, v int32) { s.Mpadd2 = v }),
		f("resist1", "Resistência ao fogo", "Resistências", func(s *webv1.AdminItemStat) int32 { return s.Resist1 }, func(s *webv1.AdminItemStat, v int32) { s.Resist1 = v }),
		f("resist2", "Resistência ao gelo", "Resistências", func(s *webv1.AdminItemStat) int32 { return s.Resist2 }, func(s *webv1.AdminItemStat, v int32) { s.Resist2 = v }),
		f("resist3", "Resistência ao raio", "Resistências", func(s *webv1.AdminItemStat) int32 { return s.Resist3 }, func(s *webv1.AdminItemStat, v int32) { s.Resist3 = v }),
		f("resist4", "Resistência à magia", "Resistências", func(s *webv1.AdminItemStat) int32 { return s.Resist4 }, func(s *webv1.AdminItemStat, v int32) { s.Resist4 = v }),
		f("resistall", "Todas as resistências", "Resistências", func(s *webv1.AdminItemStat) int32 { return s.Resistall }, func(s *webv1.AdminItemStat, v int32) { s.Resistall = v }),
		f("special1", "Maestria 1", "Maestrias", func(s *webv1.AdminItemStat) int32 { return s.Special1 }, func(s *webv1.AdminItemStat, v int32) { s.Special1 = v }),
		f("special2", "Maestria 2", "Maestrias", func(s *webv1.AdminItemStat) int32 { return s.Special2 }, func(s *webv1.AdminItemStat, v int32) { s.Special2 = v }),
		f("special3", "Maestria 3", "Maestrias", func(s *webv1.AdminItemStat) int32 { return s.Special3 }, func(s *webv1.AdminItemStat, v int32) { s.Special3 = v }),
		f("special4", "Maestria 4", "Maestrias", func(s *webv1.AdminItemStat) int32 { return s.Special4 }, func(s *webv1.AdminItemStat, v int32) { s.Special4 = v }),
		f("specialall", "Todas as maestrias", "Maestrias", func(s *webv1.AdminItemStat) int32 { return s.Specialall }, func(s *webv1.AdminItemStat, v int32) { s.Specialall = v }),
		f("itemlevel", "Nível do item", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Itemlevel }, func(s *webv1.AdminItemStat, v int32) { s.Itemlevel = v }),
		f("itemtype", "Tipo do item", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Itemtype }, func(s *webv1.AdminItemStat, v int32) { s.Itemtype = v }),
		f("mobtype", "Tipo de alvo", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Mobtype }, func(s *webv1.AdminItemStat, v int32) { s.Mobtype = v }),
		f("wtype", "Tipo de arma", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Wtype }, func(s *webv1.AdminItemStat, v int32) { s.Wtype = v }),
		f("pos", "Onde equipa", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Pos }, func(s *webv1.AdminItemStat, v int32) { s.Pos = v }),
		f("sanc", "Refino", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Sanc }, func(s *webv1.AdminItemStat, v int32) { s.Sanc = v }),
		f("nosanc", "Não pode refinar", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Nosanc }, func(s *webv1.AdminItemStat, v int32) { s.Nosanc = v }),
		f("incubate", "Incubação", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Incubate }, func(s *webv1.AdminItemStat, v int32) { s.Incubate = v }),
		f("incudelay", "Espera de incubação", "Avançado", func(s *webv1.AdminItemStat) int32 { return s.Incudelay }, func(s *webv1.AdminItemStat, v int32) { s.Incudelay = v }),
	}
}()

// Fields returns the editable numbers, in form order. It reuses MobField
// because the two editors render identically; only the numbers differ.
func (m ItemStat) Fields() []MobField {
	out := make([]MobField, 0, len(camposItem))
	for _, c := range camposItem {
		out = append(out, MobField{Nome: c.Nome, Rotulo: c.Rotulo, Grupo: c.Grupo, Valor: int64(c.Get(m.raw))})
	}
	return out
}

// GruposItem returns the field group names, in the order they should be shown.
func GruposItem() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range camposItem {
		if !seen[c.Grupo] {
			seen[c.Grupo] = true
			out = append(out, c.Grupo)
		}
	}
	return out
}

// Set writes one field by its form name, reporting whether the name is known
// and the value storable.
//
// The database column is a 16-bit integer, and the loader narrows to int16
// besides, so a number outside that range is refused here rather than written
// and silently read back as something else.
func (m ItemStat) Set(nome string, valor int64) bool {
	if valor < -32768 || valor > 32767 {
		return false
	}
	for _, c := range camposItem {
		if c.Nome == nome {
			c.Set(m.raw, int32(valor))
			return true
		}
	}
	return false
}

// NewItemStat builds a stat value carrying only its identity. Exists for the
// same reason NewMobStat does: raw is unexported, so without it no stand-in
// could build one and the handlers could not be tested.
func NewItemStat(index int32, overridden bool) ItemStat {
	return ItemStat{raw: &webv1.AdminItemStat{ItemIndex: index, Overridden: overridden}}
}

// ItemStat reads an item override, or a read-through of the catalog own
// numbers when it has none — so the editor opens on real values, never zeros.
// Saving a zeroed form would strip the item, because an override replaces its
// whole effect list rather than merging into it.
func (c *Client) ItemStat(ctx context.Context, moderatorID int64, index int32) (ItemStat, error) {
	resp, err := c.itemStat.GetItemStat(ctx, &webv1.GetItemStatRequest{
		ModeratorId: moderatorID, ItemIndex: index,
	})
	if err != nil {
		return ItemStat{}, fmt.Errorf("gamedata: get item stat %d: %w", index, err)
	}
	if err := resultErr(resp.GetResult()); err != nil {
		return ItemStat{}, err
	}
	stat := resp.GetStat()
	if stat == nil {
		return ItemStat{}, ErrNotFound
	}
	return ItemStat{raw: stat}, nil
}

// SaveItemStat writes the override back whole.
func (c *Client) SaveItemStat(ctx context.Context, moderatorID int64, m ItemStat) error {
	resp, err := c.itemStat.UpsertItemStat(ctx, &webv1.UpsertItemStatRequest{
		ModeratorId: moderatorID, Stat: m.raw,
	})
	if err != nil {
		return fmt.Errorf("gamedata: save item stat %d: %w", m.Index(), err)
	}
	return resultErr(resp.GetResult())
}

// ClearItemStat drops the override so the catalog values apply again.
func (c *Client) ClearItemStat(ctx context.Context, moderatorID int64, index int32) error {
	resp, err := c.itemStat.DeleteItemStat(ctx, &webv1.DeleteItemStatRequest{
		ModeratorId: moderatorID, ItemIndex: index,
	})
	if err != nil {
		return fmt.Errorf("gamedata: clear item stat %d: %w", index, err)
	}
	return resultErr(resp.GetResult())
}
