package content

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/internal/itemeffect"
)

// ItemEntry is one row of ItemList.csv (data-formats.md §3.1).
//
// UNVERIFIED: only Index and Name are reliably mapped; the full column→
// STRUCT_ITEMLIST mapping (Price/Grade/Extra/nPos and the EF_* effect pairs) is
// not confirmed from the CSV and is kept raw for later (the .bin is the compiled
// form). Fields holds the raw comma-separated columns.
type ItemEntry struct {
	Index  int
	Name   string
	Fields []string
}

// ItemList is the item catalog indexed by item index (g_pItemList).
type ItemList struct {
	items map[int]ItemEntry
}

// Get returns the entry for an item index.
func (l *ItemList) Get(index int) (ItemEntry, bool) { e, ok := l.items[index]; return e, ok }

// Len returns the number of loaded items.
func (l *ItemList) Len() int { return len(l.items) }

// Prices returns a map of item index → base Price (STRUCT_ITEMLIST.Price). In the
// CSV that is the 6th column (0-based index 5):
// "831,Garra,837.0,4.10.0.0.11,43,530,..." → 530 (BASE_ReadItemListFile, Basedef.cpp).
func (l *ItemList) Prices() map[int]int32 {
	out := make(map[int]int32, len(l.items))
	for idx, e := range l.items {
		if len(e.Fields) > 5 {
			if p, err := strconv.Atoi(strings.TrimSpace(e.Fields[5])); err == nil {
				out[idx] = int32(p)
			}
		}
	}
	return out
}

// BaseEffect and ItemReq moved to internal/itemeffect so the webServer can read
// the same table: it shows a moderator what an item grants today, and Go's
// internal rule keeps it out of tmserver/internal. These are aliases, not new
// types, so every caller that already speaks in content.BaseEffect is unchanged.
type (
	// BaseEffect is one static item effect (STRUCT_ITEMLIST.stEffect): an EF_*
	// effect id and its value — the item's inherent stats (weapon damage, armor
	// AC, attribute bonuses). These come from the catalog by item index,
	// distinct from the per-item instance refines stored on STRUCT_ITEM.
	BaseEffect = itemeffect.BaseEffect

	// ItemReq is what a character must have to equip an item.
	ItemReq = itemeffect.Req
)

// EffectID returns the STRUCT_EFFECT id for an EF_<name> token, and whether the
// score model understands it.
func EffectID(name string) (uint8, bool) { return itemeffect.ID(name) }

// BaseEffects returns item index → its score-relevant static effects, parsed from
// the trailing "EF_<name>,value" pairs of each ItemList row (the row's stEffect
// array). It scans for EF_ tokens anywhere in the row (robust to the exact column
// count) and reads the following column as the value.
func (l *ItemList) BaseEffects() map[int][]BaseEffect {
	out := make(map[int][]BaseEffect, len(l.items))
	for idx, e := range l.items {
		if effs := itemeffect.ParsePairs(e.Fields); len(effs) > 0 {
			out[idx] = effs
		}
	}
	return out
}

// Volatiles returns item index → its EF_VOLATILE value (the cValue of the EF_VOLATILE
// pair). On _MSG_UseItem the server classifies the action by this value: 0 = equippable,
// 64/65/66 = Divine 7/15/30d, 58 = Vigor, 1 = HP/MP potion, etc.
// (captura-wyd-affect-divina.md §B; note EF_VOLATILE the *id* is 38).
func (l *ItemList) Volatiles() map[int]int {
	out := make(map[int]int)
	for idx, e := range l.items {
		for i := 0; i+1 < len(e.Fields); i++ {
			if strings.TrimSpace(e.Fields[i]) == "EF_VOLATILE" {
				if v, err := strconv.Atoi(strings.TrimSpace(e.Fields[i+1])); err == nil {
					out[idx] = v
				}
				break
			}
		}
	}
	return out
}

// Ranges returns item index → its EF_RANGE value (the attack reach an equipped
// item grants). A mob's reach is the max EF_RANGE over its template's 16 equips
// (BASE_GetMobAbility → BASE_GetMaxAbility, Basedef.cpp:2415/2523); EF_RANGE is
// deliberately NOT in efName/BaseEffects — it isn't a score stat and must not
// fold into CurrentScore. Note EF_RANGE is exempt from the refine multiplier
// (Basedef.cpp:1854).
func (l *ItemList) Ranges() map[int]int16 {
	out := make(map[int]int16)
	for idx, e := range l.items {
		for i := 0; i+1 < len(e.Fields); i++ {
			if strings.TrimSpace(e.Fields[i]) == "EF_RANGE" {
				if v, err := strconv.Atoi(strings.TrimSpace(e.Fields[i+1])); err == nil {
					out[idx] = int16(v)
				}
				break
			}
		}
	}
	return out
}

// Grades returns item index → Grade (STRUCT_ITEMLIST.Grade — CSV column 8 per
// BASE_ReadItemListFile sscanf in Basedef.cpp:5718). UNVERIFIED against every row
// until content_test pins samples; used for grade-7 ExpBonus (+2 per piece).
func (l *ItemList) Grades() map[int]int {
	out := make(map[int]int)
	for idx, e := range l.items {
		if len(e.Fields) > 8 {
			if v, err := strconv.Atoi(strings.TrimSpace(e.Fields[8])); err == nil {
				out[idx] = v
			}
		}
	}
	return out
}

// Positions returns item index → nPos (STRUCT_ITEMLIST.nPos, the equip-slot class —
// CSV column 6). nPos drives the refine (+9) threshold bonuses: weapons 64/192 add
// +40 weapon damage, defense pieces 4/8/128 add +25 AC (captura §E). Confirmed by
// Garra (weapon) nPos=64 and potions nPos=0.
func (l *ItemList) Positions() map[int]int {
	out := make(map[int]int)
	for idx, e := range l.items {
		if len(e.Fields) > 6 {
			if v, err := strconv.Atoi(strings.TrimSpace(e.Fields[6])); err == nil {
				out[idx] = v
			}
		}
	}
	return out
}

// Uniques returns item index → nUnique (STRUCT_ITEMLIST.nUnique — CSV column 4,
// sscanf field `unique` in BASE_ReadItemListFile). nUnique in [41,50] marks the
// damage-jewel items whose EF_DAMAGEADD counts in the score (captura §B/E) and
// gates the base Anct combine recipe (GetMatchCombine, GetFunc.cpp:94).
func (l *ItemList) Uniques() map[int]int {
	out := make(map[int]int)
	for idx, e := range l.items {
		if len(e.Fields) > 4 {
			if v, err := strconv.Atoi(strings.TrimSpace(e.Fields[4])); err == nil {
				out[idx] = v
			}
		}
	}
	return out
}

// Extras returns item index → Extra (STRUCT_ITEMLIST.Extra — CSV column 7).
// Extra is the result-item base index used by the Anct combine (joia + Extra).
func (l *ItemList) Extras() map[int]int {
	out := make(map[int]int)
	for idx, e := range l.items {
		if len(e.Fields) > 7 {
			if v, err := strconv.Atoi(strings.TrimSpace(e.Fields[7])); err == nil {
				out[idx] = v
			}
		}
	}
	return out
}

// ItemReq is an item's equip requirement (STRUCT_ITEMLIST ReqLvl/Str/Int/Dex/Con).
// A zero value means no requirement.

// Requirements returns item index → its equip requirement, parsed from the
// dot-separated 4th CSV column "ReqLvl.ReqStr.ReqInt.ReqDex.ReqCon" (the column
// order matches STRUCT_ITEMLIST, confirmed against warrior weapons: axes/swords
// put their STR requirement in the 2nd value). Items with no requirement are
// omitted.
func (l *ItemList) Requirements() map[int]ItemReq {
	out := make(map[int]ItemReq, len(l.items))
	for idx, e := range l.items {
		if len(e.Fields) < 4 {
			continue
		}
		if req, ok := itemeffect.ParseReq(e.Fields[3]); ok {
			out[idx] = req
		}
	}
	return out
}

// LoadItemList reads ItemList.csv (index,Name,...).
func LoadItemList(path string) (*ItemList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("content: open ItemList: %w", err)
	}
	defer f.Close()
	return parseItemList(f)
}

func parseItemList(r io.Reader) (*ItemList, error) {
	l := &ItemList{items: make(map[int]ItemEntry)}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // rows can be long
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		l.items[idx] = ItemEntry{Index: idx, Name: strings.TrimSpace(fields[1]), Fields: fields}
	}
	return l, sc.Err()
}

// durationInName matches the "(30dias)" suffix the catalog puts on every
// temporary item — "Conjunto_Yin-Yang(30dias)", "Fada_Verde(5dias)",
// "Panqueca_(7dias)". The count is the item's lifetime in days.
var durationInName = regexp.MustCompile(`\((\d+)\s*dias?\)`)

// Durations returns item index → lifetime in days for every item whose NAME
// declares one. The catalog has no field for it: the fairies spell their life
// out in EF_WDAY, but the costumes and mounts carry it only in the name, so the
// name is the one source that covers all of them — and it keeps covering them
// when 7- and 14-day variants are added, with no table to maintain.
//
// A timed item's clock is not started here; it starts when the item is first
// equipped (see startTimedItem). This map only says how long it will then run.
func (l *ItemList) Durations() map[int]int {
	out := make(map[int]int)
	for idx, e := range l.items {
		m := durationInName.FindStringSubmatch(e.Name)
		if m == nil {
			continue
		}
		days, err := strconv.Atoi(m[1])
		if err != nil || days <= 0 {
			continue
		}
		out[idx] = days
	}
	return out
}

// Names maps item index to its catalog name. The legacy interpolates these into
// NPC dialogue (_SN_BRINGITEM, "Voce deve trazer o item %s"), which is the only
// place a player is told WHICH item a quest wants.
func (l *ItemList) Names() map[int]string {
	out := make(map[int]string, len(l.items))
	for idx, e := range l.items {
		if e.Name != "" {
			out[idx] = e.Name
		}
	}
	return out
}
