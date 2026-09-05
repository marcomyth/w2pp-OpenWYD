package npcpanel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npcadmin"
)

// fakeAdmin stands in for npcadmin.Service so the HTTP layer can be exercised
// without Postgres — the panel is built and run long before a database exists.
type fakeAdmin struct {
	defs []domain.NPCDefinition
	res  npcadmin.Result
	err  error

	gotVisibility *struct {
		ID      int64
		Enabled bool
	}
	gotUpsert      *domain.NPCDefinition
	gotShop        []domain.NPCShopItem
	gotShopNpcID   int64
	gotModeratorID int64
}

func (f *fakeAdmin) List(_ context.Context, mod int64) (npcadmin.Result, []domain.NPCDefinition, error) {
	f.gotModeratorID = mod
	return f.res, f.defs, f.err
}

func (f *fakeAdmin) Get(_ context.Context, mod, id int64) (npcadmin.Result, domain.NPCDefinition, error) {
	f.gotModeratorID = mod
	for _, d := range f.defs {
		if d.ID == id {
			return f.res, d, f.err
		}
	}
	return npcadmin.NotFound, domain.NPCDefinition{}, f.err
}

func (f *fakeAdmin) Upsert(_ context.Context, mod int64, d domain.NPCDefinition) (npcadmin.Result, int64, error) {
	f.gotModeratorID, f.gotUpsert = mod, &d
	return f.res, d.ID, f.err
}

func (f *fakeAdmin) SetVisibility(_ context.Context, mod, id int64, enabled bool) (npcadmin.Result, error) {
	f.gotModeratorID = mod
	f.gotVisibility = &struct {
		ID      int64
		Enabled bool
	}{id, enabled}
	return f.res, f.err
}

func (f *fakeAdmin) SetShop(_ context.Context, mod, id int64, items []domain.NPCShopItem) (npcadmin.Result, error) {
	f.gotModeratorID, f.gotShopNpcID, f.gotShop = mod, id, items
	return f.res, f.err
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

// Without an admin service every write must be refused rather than silently
// doing nothing — a moderator has to know the panel is read-only.
func TestWritesRefusedWithoutDatabase(t *testing.T) {
	h := Handler(Config{Data: Data{}})
	for _, path := range []string{"/api/npc/visibility", "/api/npc/position", "/api/npc/shop"} {
		rec := post(t, h, path, `{"npcId":1}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("POST %s = %d, want 503 in read-only mode", path, rec.Code)
		}
	}
}

func TestInventoryReportsEditability(t *testing.T) {
	h := Handler(Config{Data: Data{}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/npcs", nil))

	var got Data
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Editable {
		t.Error("Editable = true with no admin service")
	}
}

// A database failure must not blank the page: the content map is still true, so
// the panel degrades to read-only.
func TestInventoryDegradesOnDatabaseError(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("connection refused")}
	h := Handler(Config{
		Data:  Data{NPCs: []NPC{{Slug: "Aki-1", Name: "Aki", Enabled: true}}},
		Admin: admin, ModeratorID: 7,
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/npcs", nil))

	var got Data
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got.Editable {
		t.Error("Editable = true after a database error")
	}
	if len(got.NPCs) != 1 {
		t.Errorf("got %d NPCs, want the content map preserved", len(got.NPCs))
	}
}

func TestSetVisibility(t *testing.T) {
	admin := &fakeAdmin{}
	h := Handler(Config{Data: Data{}, Admin: admin, ModeratorID: 42})

	rec := post(t, h, "/api/npc/visibility", `{"npcId":9,"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if admin.gotVisibility == nil || admin.gotVisibility.ID != 9 || admin.gotVisibility.Enabled {
		t.Errorf("SetVisibility got %+v, want id 9 disabled", admin.gotVisibility)
	}
	// The moderator id must come from the process flag, never from the body.
	if admin.gotModeratorID != 42 {
		t.Errorf("moderatorID = %d, want 42 from Config", admin.gotModeratorID)
	}
}

// Moving an NPC must preserve the rest of the row: Upsert replaces it whole, so
// sending only coordinates would blank the name, merchant type and route.
func TestSetPositionPreservesDefinition(t *testing.T) {
	admin := &fakeAdmin{defs: []domain.NPCDefinition{{
		ID: 3, Slug: "Aki-1834", TemplateName: "Aki", DisplayName: "Aki",
		Merchant: 1, RouteType: 2, Enabled: true, PosX: 1309, PosY: 312,
	}}}
	h := Handler(Config{Data: Data{}, Admin: admin, ModeratorID: 1})

	rec := post(t, h, "/api/npc/position", `{"npcId":3,"x":2100,"y":2100,"mapId":-1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	got := admin.gotUpsert
	if got == nil {
		t.Fatal("Upsert was not called")
	}
	if got.PosX != 2100 || got.PosY != 2100 {
		t.Errorf("position = (%d,%d), want (2100,2100)", got.PosX, got.PosY)
	}
	// mapId -1 asks the server to re-derive the city; (2100,2100) is Armia = 0.
	if got.MapID != 0 {
		t.Errorf("MapID = %d, want 0 (Armia derived from the new position)", got.MapID)
	}
	if got.DisplayName != "Aki" || got.Merchant != 1 || got.RouteType != 2 || !got.Enabled {
		t.Errorf("Upsert dropped fields: %+v", *got)
	}
}

func TestSetPositionHonoursExplicitZone(t *testing.T) {
	admin := &fakeAdmin{defs: []domain.NPCDefinition{{ID: 3, Slug: "s"}}}
	h := Handler(Config{Data: Data{}, Admin: admin, ModeratorID: 1})

	// A moderator overriding the derived city must win over the derivation.
	post(t, h, "/api/npc/position", `{"npcId":3,"x":2100,"y":2100,"mapId":4}`)
	if admin.gotUpsert == nil || admin.gotUpsert.MapID != 4 {
		t.Errorf("MapID = %+v, want the explicit 4 to be kept", admin.gotUpsert)
	}
}

func TestSetShop(t *testing.T) {
	admin := &fakeAdmin{}
	h := Handler(Config{Data: Data{}, Admin: admin, ModeratorID: 1})

	rec := post(t, h, "/api/npc/shop",
		`{"npcId":5,"items":[{"slot":0,"itemIndex":1234,"quantity":3}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if admin.gotShopNpcID != 5 || len(admin.gotShop) != 1 {
		t.Fatalf("SetShop got npc %d with %d items", admin.gotShopNpcID, len(admin.gotShop))
	}
	if it := admin.gotShop[0]; it.Slot != 0 || it.ItemIndex != 1234 || it.Quantity != 3 {
		t.Errorf("shop item = %+v, want slot 0 / index 1234 / qty 3", it)
	}
}

// Business results ride in the body, not as transport errors, so each one has to
// reach the browser as the right status with a message a moderator can act on.
func TestResultMapping(t *testing.T) {
	tests := []struct {
		res    npcadmin.Result
		status int
	}{
		{npcadmin.OK, http.StatusOK},
		{npcadmin.Forbidden, http.StatusForbidden},
		{npcadmin.NotFound, http.StatusNotFound},
		{npcadmin.Invalid, http.StatusBadRequest},
		{npcadmin.ContentOwned, http.StatusConflict},
	}
	for _, tt := range tests {
		admin := &fakeAdmin{res: tt.res}
		h := Handler(Config{Data: Data{}, Admin: admin, ModeratorID: 1})
		rec := post(t, h, "/api/npc/visibility", `{"npcId":1,"enabled":true}`)
		if rec.Code != tt.status {
			t.Errorf("result %v = %d, want %d", tt.res, rec.Code, tt.status)
		}
	}
}

// A database error must never reach the browser verbatim.
func TestInternalErrorIsNotLeaked(t *testing.T) {
	admin := &fakeAdmin{err: errors.New(`pq: relation "npc_definition" does not exist`)}
	h := Handler(Config{Data: Data{}, Admin: admin, ModeratorID: 1})

	rec := post(t, h, "/api/npc/visibility", `{"npcId":1,"enabled":true}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "npc_definition") {
		t.Errorf("database detail leaked to the client: %s", rec.Body)
	}
}

func TestOverlayAppliesDatabaseState(t *testing.T) {
	base := []NPC{{
		Slug: "Aki-1834", Name: "Aki", Template: "Aki", Origin: OriginContent,
		Enabled: true, X: 1309, Y: 312, ZoneID: 7, Zone: "Vila do Norte (não identificada)",
	}}
	defs := []domain.NPCDefinition{
		// The same block, moved to Armia and disabled by a moderator.
		{ID: 11, Slug: "Aki-1834", Enabled: false, PosX: 2100, PosY: 2100, Origin: OriginContent},
		// A definition with no content block: an NPC created in the panel.
		{ID: 12, Slug: "meu-npc", TemplateName: "Aki", DisplayName: "Meu NPC",
			Enabled: true, PosX: 2100, PosY: 2100, Origin: OriginCustom},
	}

	got := overlay(base, defs)
	if len(got) != 2 {
		t.Fatalf("overlay returned %d rows, want 2", len(got))
	}
	if got[0].DBID != 11 || got[0].Enabled {
		t.Errorf("row 0 = %+v, want dbId 11 and disabled", got[0])
	}
	// Moving the NPC must re-derive its city rather than keep the shipped one.
	if got[0].Zone != "Armia" {
		t.Errorf("row 0 zone = %q, want Armia after the move", got[0].Zone)
	}
	if got[1].Slug != "meu-npc" || got[1].Origin != OriginCustom || got[1].Zone != "Armia" {
		t.Errorf("row 1 = %+v, want the custom NPC placed in Armia", got[1])
	}
}
