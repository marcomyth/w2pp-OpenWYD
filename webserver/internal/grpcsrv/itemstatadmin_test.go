package grpcsrv

import (
	"context"
	"reflect"
	"testing"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemstatadmin"
)

type fakeItemStatAdmin struct {
	item     itemstatadmin.Item
	res      itemstatadmin.Result
	upserted []domain.ItemStat
	deleted  []int32
}

func (f *fakeItemStatAdmin) Get(context.Context, int64, int32) (itemstatadmin.Result, itemstatadmin.Item, error) {
	return f.res, f.item, nil
}

func (f *fakeItemStatAdmin) Upsert(_ context.Context, _ int64, st domain.ItemStat) (itemstatadmin.Result, error) {
	f.upserted = append(f.upserted, st)
	return f.res, nil
}

func (f *fakeItemStatAdmin) Delete(_ context.Context, _ int64, idx int32) (itemstatadmin.Result, error) {
	f.deleted = append(f.deleted, idx)
	return f.res, nil
}

// distinta fills every int16 field with a different value, so a field dropped
// from either conversion shows up as a mismatch instead of coinciding with a
// zero that was already there.
func distinta() domain.ItemStat {
	st := domain.ItemStat{ItemIndex: 2000}
	v := reflect.ValueOf(&st).Elem()
	for i, n := 0, v.NumField(); i < n; i++ {
		if f := v.Field(i); f.Kind() == reflect.Int16 {
			f.SetInt(int64(i + 1))
		}
	}
	return st
}

func TestItemStatSurvivesTheRoundTripThroughTheWire(t *testing.T) {
	// Three tables name these columns: the canonical one in domain, the dbv1
	// accessors in the tmServer client, and the webv1 accessors here. A column
	// missing from this one would compile and would simply stop being sent.
	quero := distinta()
	pb := itemStatToProto(itemstatadmin.Item{Stat: quero, DisplayName: "Espada Longa", Overridden: true})

	if pb.GetItemIndex() != 2000 || !pb.GetOverridden() || pb.GetDisplayName() != "Espada Longa" {
		t.Errorf("header lost: %+v", pb)
	}
	if got := itemStatFromProto(pb); !reflect.DeepEqual(got, quero) {
		t.Fatalf("round trip lost a column:\n got %+v\nwant %+v", got, quero)
	}
}

func TestEveryColumnHasAWireAccessor(t *testing.T) {
	for _, f := range domain.ItemStatFields {
		if _, ok := adminItemStatFields[f.Col]; !ok {
			t.Errorf("column %q has no accessor on the wire message", f.Col)
		}
	}
	if len(adminItemStatFields) != len(domain.ItemStatFields) {
		t.Errorf("wire accessors = %d, canonical columns = %d — one table has an entry the other does not",
			len(adminItemStatFields), len(domain.ItemStatFields))
	}
}

func TestGetItemStatOmitsTheStatWhenTheAnswerIsNotOK(t *testing.T) {
	// A refusal that still carried a stat would let a caller render numbers it
	// was told it may not see.
	srv := NewItemStatAdmin(&fakeItemStatAdmin{
		res:  itemstatadmin.Forbidden,
		item: itemstatadmin.Item{Stat: distinta()},
	})
	resp, err := srv.GetItemStat(context.Background(), &webv1.GetItemStatRequest{ModeratorId: 1, ItemIndex: 2000})
	if err != nil {
		t.Fatalf("GetItemStat: %v", err)
	}
	if resp.GetResult() != webv1.AdminResult_ADMIN_RESULT_FORBIDDEN {
		t.Errorf("result = %v, want FORBIDDEN", resp.GetResult())
	}
	if resp.GetStat() != nil {
		t.Error("a refused read still returned the numbers")
	}
}

func TestUpsertItemStatRefusesAnEmptyBody(t *testing.T) {
	admin := &fakeItemStatAdmin{res: itemstatadmin.OK}
	srv := NewItemStatAdmin(admin)
	resp, err := srv.UpsertItemStat(context.Background(), &webv1.UpsertItemStatRequest{ModeratorId: 1})
	if err != nil {
		t.Fatalf("UpsertItemStat: %v", err)
	}
	if resp.GetResult() != webv1.AdminResult_ADMIN_RESULT_INVALID {
		t.Errorf("result = %v, want INVALID", resp.GetResult())
	}
	if len(admin.upserted) != 0 {
		t.Error("a request with no stat still reached the store")
	}
}

func TestDeleteItemStatPassesTheIndexThrough(t *testing.T) {
	admin := &fakeItemStatAdmin{res: itemstatadmin.OK}
	srv := NewItemStatAdmin(admin)
	if _, err := srv.DeleteItemStat(context.Background(),
		&webv1.DeleteItemStatRequest{ModeratorId: 1, ItemIndex: 2000}); err != nil {
		t.Fatalf("DeleteItemStat: %v", err)
	}
	if len(admin.deleted) != 1 || admin.deleted[0] != 2000 {
		t.Errorf("deleted %v, want [2000]", admin.deleted)
	}
}

func TestItemStatResultsMapToTheirOwnCode(t *testing.T) {
	// NotFound folding into INVALID would tell a moderator their edit was
	// malformed when the item simply had no override.
	for _, c := range []struct {
		in   itemstatadmin.Result
		want webv1.AdminResult
	}{
		{itemstatadmin.OK, webv1.AdminResult_ADMIN_RESULT_OK},
		{itemstatadmin.Forbidden, webv1.AdminResult_ADMIN_RESULT_FORBIDDEN},
		{itemstatadmin.NotFound, webv1.AdminResult_ADMIN_RESULT_NOT_FOUND},
		{itemstatadmin.Invalid, webv1.AdminResult_ADMIN_RESULT_INVALID},
	} {
		if got := itemStatResultToProto(c.in); got != c.want {
			t.Errorf("result %v mapped to %v, want %v", c.in, got, c.want)
		}
	}
}
