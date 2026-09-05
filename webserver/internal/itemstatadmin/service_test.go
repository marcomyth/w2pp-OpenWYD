package itemstatadmin

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/itemeffect"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
)

type fakeStore struct {
	role      string
	roleErr   error
	stat      *domain.ItemStat
	upserted  []domain.ItemStat
	deleted   []int32
	deleteErr error
}

func (f *fakeStore) AccountRole(context.Context, int64) (string, error) {
	if f.roleErr != nil {
		return "", f.roleErr
	}
	return f.role, nil
}

func (f *fakeStore) GetItemStat(_ context.Context, idx int32) (domain.ItemStat, error) {
	if f.stat == nil || f.stat.ItemIndex != idx {
		return domain.ItemStat{}, store.ErrNotFound
	}
	return *f.stat, nil
}

func (f *fakeStore) UpsertItemStat(_ context.Context, st domain.ItemStat, _ int64) error {
	f.upserted = append(f.upserted, st)
	return nil
}

func (f *fakeStore) DeleteItemStat(_ context.Context, idx int32, _ int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, idx)
	return nil
}

// espada is a catalog entry standing in for a real ItemList.csv row.
func espada() itemcatalog.Entry {
	dano, _ := itemeffect.ID("EF_DAMAGE")
	tipo, _ := itemeffect.ID("EF_WTYPE")
	forca, _ := itemeffect.ID("EF_STR")
	return itemcatalog.Entry{
		Index: 2000, Name: "Espada_Longa", DisplayName: "Espada Longa",
		Req: itemeffect.Req{Lvl: 30, Str: 40},
		Effects: []itemeffect.BaseEffect{
			{Eff: dano, Val: 120}, {Eff: tipo, Val: 3}, {Eff: forca, Val: 5},
		},
	}
}

func comCatalogo(s *Service) *Service {
	s.SetCatalog(func(idx int32) (itemcatalog.Entry, bool) {
		if idx == 2000 {
			return espada(), true
		}
		return itemcatalog.Entry{}, false
	})
	return s
}

func TestGetSeedsFromTheCatalogWhenNothingWasEdited(t *testing.T) {
	// This is the load-bearing behaviour of the whole feature. An override
	// replaces an item's WHOLE effect list, so an editor that opened on zeros
	// would strip the item the first time somebody saved it.
	svc := comCatalogo(New(&fakeStore{role: "moderator"}))

	res, it, err := svc.Get(context.Background(), 1, 2000)
	if err != nil || res != OK {
		t.Fatalf("Get = %v, %v; want OK", res, err)
	}
	if it.Overridden {
		t.Error("an item with no saved override reported as overridden")
	}
	if it.DisplayName != "Espada Longa" {
		t.Errorf("display name = %q, want Espada Longa", it.DisplayName)
	}
	if it.Stat.Damage != 120 {
		t.Errorf("damage = %d, want 120 from the catalog", it.Stat.Damage)
	}
	if it.Stat.WType != 3 {
		t.Errorf("weapon type = %d, want 3 — carried, or a save would strip it", it.Stat.WType)
	}
	if it.Stat.Str != 5 {
		t.Errorf("str = %d, want 5", it.Stat.Str)
	}
	if it.Stat.ReqLevel != 30 || it.Stat.ReqStr != 40 {
		t.Errorf("requirement = level %d / str %d, want 30 / 40", it.Stat.ReqLevel, it.Stat.ReqStr)
	}
	// Everything the row does not carry stays zero, which is what the game
	// computes for it too.
	if it.Stat.AC != 0 || it.Stat.Resist1 != 0 {
		t.Errorf("an effect the row does not carry came back non-zero: %+v", it.Stat)
	}
}

func TestGetPrefersTheSavedOverride(t *testing.T) {
	svc := comCatalogo(New(&fakeStore{
		role: "admin",
		stat: &domain.ItemStat{ItemIndex: 2000, Damage: 999},
	}))

	res, it, err := svc.Get(context.Background(), 1, 2000)
	if err != nil || res != OK {
		t.Fatalf("Get = %v, %v; want OK", res, err)
	}
	if !it.Overridden {
		t.Error("a saved override reported as a catalog read-through")
	}
	if it.Stat.Damage != 999 {
		t.Errorf("damage = %d, want the saved 999", it.Stat.Damage)
	}
	if it.DisplayName != "Espada Longa" {
		t.Errorf("display name = %q — the name lives in the catalog, not the override", it.DisplayName)
	}
}

func TestGetIsNotFoundWithNeitherOverrideNorCatalog(t *testing.T) {
	svc := comCatalogo(New(&fakeStore{role: "moderator"}))
	if res, _, err := svc.Get(context.Background(), 1, 4242); res != NotFound || err != nil {
		t.Fatalf("Get of an unknown item = %v, %v; want NotFound", res, err)
	}

	// And with no content tree at all, an un-overridden item is NotFound rather
	// than a form full of invented zeros.
	semCatalogo := New(&fakeStore{role: "moderator"})
	if res, _, err := semCatalogo.Get(context.Background(), 1, 2000); res != NotFound || err != nil {
		t.Fatalf("Get without a catalog = %v, %v; want NotFound", res, err)
	}
}

func TestUpsertRefusesAnItemTheCatalogDoesNotKnow(t *testing.T) {
	// The overlay applies by index onto entries loaded from the CSV, so a row
	// for an unknown index would look saved and do nothing.
	st := &fakeStore{role: "moderator"}
	svc := comCatalogo(New(st))

	if res, err := svc.Upsert(context.Background(), 1, domain.ItemStat{ItemIndex: 4242}); res != Invalid || err != nil {
		t.Fatalf("Upsert of an unknown item = %v, %v; want Invalid", res, err)
	}
	if len(st.upserted) != 0 {
		t.Fatal("a refused upsert still wrote")
	}

	if res, err := svc.Upsert(context.Background(), 1, domain.ItemStat{ItemIndex: 2000, Damage: 7}); res != OK || err != nil {
		t.Fatalf("Upsert of a known item = %v, %v; want OK", res, err)
	}
	if len(st.upserted) != 1 || st.upserted[0].Damage != 7 {
		t.Errorf("stored %+v, want damage 7", st.upserted)
	}
}

func TestOnlyStaffMayReadOrWrite(t *testing.T) {
	// The panel checks the role too, but this service answers any client of the
	// web API, so the check has to live here as well.
	svc := comCatalogo(New(&fakeStore{role: "player"}))
	ctx := context.Background()

	if res, _, _ := svc.Get(ctx, 1, 2000); res != Forbidden {
		t.Errorf("Get as player = %v, want Forbidden", res)
	}
	if res, _ := svc.Upsert(ctx, 1, domain.ItemStat{ItemIndex: 2000}); res != Forbidden {
		t.Errorf("Upsert as player = %v, want Forbidden", res)
	}
	if res, _ := svc.Delete(ctx, 1, 2000); res != Forbidden {
		t.Errorf("Delete as player = %v, want Forbidden", res)
	}

	desconhecida := comCatalogo(New(&fakeStore{roleErr: store.ErrNotFound}))
	if res, _, _ := desconhecida.Get(ctx, 99, 2000); res != Forbidden {
		t.Errorf("Get as an unknown account = %v, want Forbidden", res)
	}
}

func TestDeleteReportsWhenThereWasNothingToDelete(t *testing.T) {
	st := &fakeStore{role: "admin"}
	svc := comCatalogo(New(st))
	if res, err := svc.Delete(context.Background(), 1, 2000); res != OK || err != nil {
		t.Fatalf("Delete = %v, %v; want OK", res, err)
	}
	if len(st.deleted) != 1 || st.deleted[0] != 2000 {
		t.Errorf("deleted %v, want [2000]", st.deleted)
	}

	vazio := comCatalogo(New(&fakeStore{role: "admin", deleteErr: store.ErrNotFound}))
	if res, err := vazio.Delete(context.Background(), 1, 2000); res != NotFound || err != nil {
		t.Fatalf("Delete with no override = %v, %v; want NotFound", res, err)
	}
}

func TestSeedCoversEveryEffectColumn(t *testing.T) {
	// A column whose EF_* token the effect table does not know would silently
	// seed as zero — the editor would show 0 for something the item grants, and
	// saving would strip it.
	for _, f := range domain.ItemStatFields {
		if f.EF == "" {
			continue
		}
		if _, ok := itemeffect.ID(f.EF); !ok {
			t.Errorf("column %q names %s, which the effect table does not know", f.Col, f.EF)
		}
	}
}

func TestSeedSumsARepeatedEffect(t *testing.T) {
	// The score model adds every matching pair in the list, so a row that names
	// the same effect twice grants the sum. Seeding has to agree, or the editor
	// would show a number the game does not use.
	dano, _ := itemeffect.ID("EF_DAMAGE")
	svc := New(&fakeStore{role: "admin"})
	svc.SetCatalog(func(int32) (itemcatalog.Entry, bool) {
		return itemcatalog.Entry{Index: 9, Effects: []itemeffect.BaseEffect{
			{Eff: dano, Val: 40}, {Eff: dano, Val: 60},
		}}, true
	})

	_, it, err := svc.Get(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if it.Stat.Damage != 100 {
		t.Errorf("damage = %d, want 100 (40+60, what the score model computes)", it.Stat.Damage)
	}
}

func TestErrNotFoundIsTheOnlyMissDeleteReports(t *testing.T) {
	// Any other store failure has to surface as an error, not as "there was
	// nothing there" — the two lead an operator to opposite conclusions.
	svc := comCatalogo(New(&fakeStore{role: "admin", deleteErr: errors.New("banco fora do ar")}))
	res, err := svc.Delete(context.Background(), 1, 2000)
	if err == nil {
		t.Fatal("a database failure came back as a clean result")
	}
	if res == NotFound {
		t.Error("a database failure was reported as a missing override")
	}
}

func TestItemStatFieldsAndTheStructAgree(t *testing.T) {
	campos := 0
	v := reflect.ValueOf(domain.ItemStat{})
	for i, n := 0, v.NumField(); i < n; i++ {
		if v.Field(i).Kind() == reflect.Int16 {
			campos++
		}
	}
	if campos != len(domain.ItemStatFields) {
		t.Errorf("domain.ItemStat has %d int16 fields but ItemStatFields lists %d", campos, len(domain.ItemStatFields))
	}
}
