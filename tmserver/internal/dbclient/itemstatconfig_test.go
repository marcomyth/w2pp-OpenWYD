package dbclient

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	dbv1 "github.com/jeanluca/w2pp-openwyd/api/db/v1"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/content"
	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/itemstat"
)

// fakeItemStatAPI serves one canned ListItemStats reply. It embeds the
// unimplemented client so the other NpcConfigService methods exist without this
// file having to grow every time that service does.
type fakeItemStatAPI struct {
	dbv1.NpcConfigServiceClient
	resp *dbv1.ListItemStatsResponse
	err  error
}

func (f *fakeItemStatAPI) ListItemStats(context.Context, *dbv1.ListItemStatsRequest, ...grpc.CallOption) (*dbv1.ListItemStatsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestItemStatFetchMapsColumnsToTheRightEffect(t *testing.T) {
	// This is the test that earns its keep. Every column carries a plain number,
	// so a column wired to the wrong EF_* would still produce a valid override —
	// the item would simply grant something else, which is close to unfindable
	// in game. Distinct values per column make a swap show up as a mismatch.
	src := &ItemStatSource{api: &fakeItemStatAPI{resp: &dbv1.ListItemStatsResponse{
		Overrides: []*dbv1.ItemStat{{
			ItemIndex: 1415,
			ReqLevel:  50, ReqStr: 51, ReqInt: 52, ReqDex: 53, ReqCon: 54,
			Damage: 10, Ac: 20, Str: 30, Resist1: 40, Special1: 60, Wtype: 70,
		}},
	}}}

	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	ov, ok := got[1415]
	if !ok {
		t.Fatalf("no override for item 1415, got %v", got)
	}

	quer := content.ItemReq{Lvl: 50, Str: 51, Int: 52, Dex: 53, Con: 54}
	if ov.Req != quer {
		t.Errorf("requirement = %+v, want %+v", ov.Req, quer)
	}

	for _, c := range []struct {
		ef  string
		val int16
	}{
		{"EF_DAMAGE", 10}, {"EF_AC", 20}, {"EF_STR", 30},
		{"EF_RESIST1", 40}, {"EF_SPECIAL1", 60}, {"EF_WTYPE", 70},
	} {
		id, ok := content.EffectID(c.ef)
		if !ok {
			t.Fatalf("%s is not a known effect", c.ef)
		}
		achou := false
		for _, e := range ov.Effects {
			if e.Eff == id {
				achou = true
				if e.Val != c.val {
					t.Errorf("%s = %d, want %d", c.ef, e.Val, c.val)
				}
			}
		}
		if !achou {
			t.Errorf("%s (id %d) is missing from the override", c.ef, id)
		}
	}

	// Six columns were set; nothing else may appear. A zero column that leaked
	// through would change which effects the item is seen to HAVE, which the
	// recipe and refine gates read even when the value adds nothing.
	if len(ov.Effects) != 6 {
		t.Errorf("effects = %d, want 6 — a zero column leaked in", len(ov.Effects))
	}
}

func TestItemStatFetchEveryColumnIsAKnownEffect(t *testing.T) {
	// A column named after an EF_* the score model does not know would fail at
	// boot, which is the right moment; this proves none of them does, so the
	// failure path stays theoretical.
	for _, col := range itemEffectColumns {
		if _, ok := content.EffectID(col.ef); !ok {
			t.Errorf("column bound to %s, which content does not know", col.ef)
		}
	}
	if len(itemEffectColumns) != 38 {
		t.Errorf("itemEffectColumns has %d entries, want 38 (the EF_* set item_stat carries)", len(itemEffectColumns))
	}
}

func TestItemStatApplyReplacesRatherThanMerges(t *testing.T) {
	// The catalog says an item grants damage and AC; the override says it grants
	// only HP. Merging would leave the first two behind, and a moderator who
	// cleared a field would watch it keep working.
	dano, _ := content.EffectID("EF_DAMAGE")
	ac, _ := content.EffectID("EF_AC")
	hp, _ := content.EffectID("EF_HP")

	effects := map[int][]content.BaseEffect{
		1415: {{Eff: dano, Val: 100}, {Eff: ac, Val: 50}},
		2000: {{Eff: dano, Val: 7}},
	}
	reqs := map[int]content.ItemReq{
		1415: {Lvl: 90},
		2000: {Lvl: 10},
	}

	itemstat.Apply(effects, reqs, map[int]itemstat.Override{
		1415: {Effects: []content.BaseEffect{{Eff: hp, Val: 300}}, Req: content.ItemReq{Lvl: 5}},
	})

	if len(effects[1415]) != 1 || effects[1415][0].Eff != hp {
		t.Errorf("item 1415 effects = %+v, want only EF_HP", effects[1415])
	}
	if reqs[1415].Lvl != 5 {
		t.Errorf("item 1415 required level = %d, want 5", reqs[1415].Lvl)
	}
	if len(effects[2000]) != 1 || effects[2000][0].Val != 7 || reqs[2000].Lvl != 10 {
		t.Error("an un-overridden item was touched")
	}
}

func TestItemStatApplyClearsRatherThanZeroes(t *testing.T) {
	// An empty override is how a moderator strips an item. The maps have to lose
	// the entry, not gain a zero one: Requirements() omits an all-zero
	// requirement rather than storing one, so a zero entry would be a state the
	// catalog loader can never produce.
	dano, _ := content.EffectID("EF_DAMAGE")
	effects := map[int][]content.BaseEffect{1415: {{Eff: dano, Val: 100}}}
	reqs := map[int]content.ItemReq{1415: {Lvl: 90}}

	itemstat.Apply(effects, reqs, map[int]itemstat.Override{1415: {}})

	if _, ok := effects[1415]; ok {
		t.Error("an emptied item kept an effects entry")
	}
	if _, ok := reqs[1415]; ok {
		t.Error("an emptied item kept a requirement entry")
	}
}
