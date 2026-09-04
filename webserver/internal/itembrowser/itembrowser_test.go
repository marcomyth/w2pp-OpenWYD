package itembrowser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeCSV lays out a minimal content tree holding just Common/ItemList.csv.
func writeCSV(t *testing.T, dir string, lines ...string) {
	t.Helper()
	commonDir := filepath.Join(dir, "Common")
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(commonDir, "ItemList.csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergesColumnsAndEffects(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir,
		// The real Baú de Experiência row (4140), byte-for-byte apart from the
		// Latin-1 name, which Scan decodes.
		"4140,Bau_de_Experiencia,2864.0,0.0.0.0.0,0,500000,0,0,0,EF_GRID,0,EF_VOLATILE,198",
		// An equippable with a requirement and several effects.
		"1100,Espada_Curta,10.2,50.30.0.20.0,0,1200,0,0,3,EF_DAMAGE,25,EF_RANGE,1",
		"garbage,row,without,index",
	)

	data, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(data.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(data.Items))
	}
	// Sorted by index: 1100 before 4140.
	sword, chest := data.Items[0], data.Items[1]

	if chest.Index != 4140 || chest.Price != 500000 || chest.Volatile != 198 {
		t.Errorf("chest = index %d price %d volatile %d; want 4140/500000/198",
			chest.Index, chest.Price, chest.Volatile)
	}
	wantEffects := []Effect{{Name: "EF_GRID", Value: 0}, {Name: "EF_VOLATILE", Value: 198}}
	if !reflect.DeepEqual(chest.Effects, wantEffects) {
		t.Errorf("chest effects = %+v, want %+v", chest.Effects, wantEffects)
	}
	if chest.Req != (Req{}) {
		t.Errorf("chest req = %+v, want zero", chest.Req)
	}

	if sword.Price != 1200 || sword.Volatile != 0 {
		t.Errorf("sword = price %d volatile %d; want 1200/0", sword.Price, sword.Volatile)
	}
	if want := (Req{Lvl: 50, Str: 30, Int: 0, Dex: 20, Con: 0}); sword.Req != want {
		t.Errorf("sword req = %+v, want %+v", sword.Req, want)
	}
	if sword.Mesh != 10 || sword.Texture != 2 {
		t.Errorf("sword mesh.texture = %d.%d, want 10.2", sword.Mesh, sword.Texture)
	}
}

// TestLoadNeverEmitsNullSlices guards the UI, whose filter calls indexOf/join on
// both fields — a JSON null would throw on the first keystroke.
func TestLoadNeverEmitsNullSlices(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "2000,Item_Sem_Nada,0.0,0.0.0.0.0,0,0")

	data, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, err := json.Marshal(data.Items[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"slots", "effects"} {
		if round[key] == nil {
			t.Errorf("%s serialized as null; want an empty array", key)
		}
	}
}

func TestRawEffectsSkipsMalformedPairs(t *testing.T) {
	got := rawEffects([]string{"1", "Nome", "0.0", "EF_DAMAGE", "12", "EF_BROKEN", "xx", "EF_AC", "5"})
	want := []Effect{{Name: "EF_DAMAGE", Value: 12}, {Name: "EF_AC", Value: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rawEffects = %+v, want %+v", got, want)
	}
}

func TestHandlerServesUIAndCatalog(t *testing.T) {
	data := Data{Items: []Item{{Index: 7, Name: "Teste", Slots: []string{}, Effects: []Effect{}}}, CatalogVersion: "abc"}
	srv := httptest.NewServer(Handler(data, ""))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/items")
	if err != nil {
		t.Fatalf("GET /api/items: %v", err)
	}
	defer resp.Body.Close()
	var got Data
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Index != 7 || got.CatalogVersion != "abc" {
		t.Errorf("payload = %+v, want the single seeded item", got)
	}

	page, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Errorf("GET / = %d, want 200", page.StatusCode)
	}

	missing, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET /nope: %v", err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", missing.StatusCode)
	}
}

// TestShippedCatalogColumns pins the column positions against the real
// Release/ tree, so a change to the shipped CSV layout fails here instead of
// silently showing wrong prices in the UI. Skipped when the tree is absent.
func TestShippedCatalogColumns(t *testing.T) {
	const contentDir = "../../../Release"
	if _, err := os.Stat(filepath.Join(contentDir, "Common", "ItemList.csv")); err != nil {
		t.Skip("Release content tree not present")
	}
	data, err := Load(contentDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	byIndex := make(map[int32]Item, len(data.Items))
	for _, it := range data.Items {
		byIndex[it.Index] = it
	}
	// The two Baú de Experiência rows: identical except for Price.
	for _, tc := range []struct {
		index int32
		price int
	}{{4140, 500000}, {4144, 0}} {
		it, ok := byIndex[tc.index]
		if !ok {
			t.Fatalf("item %d missing from the shipped catalog", tc.index)
		}
		if it.Price != tc.price {
			t.Errorf("item %d price = %d, want %d", tc.index, it.Price, tc.price)
		}
		if it.Volatile != 198 {
			t.Errorf("item %d volatile = %d, want 198", tc.index, it.Volatile)
		}
	}
}
