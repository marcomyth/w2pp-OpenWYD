// Package itembrowser serves a local, searchable view of the item catalog
// (Release/Common/ItemList.csv) as a single-page UI.
//
// It is a developer/GM tool, not part of the game or the web platform: it reads
// the read-only content tree and never touches player state or the database.
//
// The presentation fields (name decoded from Latin-1, slot mask, grade, mesh,
// icon key) are reused from itemcatalog.Scan. The few remaining columns the UI
// wants — price, equip requirement and the raw EF_* pairs — are read here
// because tmserver/internal/content is not importable across the service
// boundary. Column positions follow tmserver/internal/content/catalog.go, which
// remains the authority; the package tests pin them so a drift fails loudly.
package itembrowser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemicons"
)

// Effect is one raw EF_<name>,<value> pair from the catalog row. Pairs are kept
// verbatim rather than mapped to score effects: the point of the tool is to
// show what the CSV really says.
type Effect struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// Req is an item's equip requirement, the dot-separated 4th column
// "ReqLvl.ReqStr.ReqInt.ReqDex.ReqCon".
type Req struct {
	Lvl int `json:"lvl"`
	Str int `json:"str"`
	Int int `json:"int"`
	Dex int `json:"dex"`
	Con int `json:"con"`
}

// Item is one catalog row as the UI consumes it.
type Item struct {
	Index    int32    `json:"index"`
	Name     string   `json:"name"` // DisplayName: underscores turned back into spaces
	Raw      string   `json:"raw"`  // the catalog name verbatim
	Mesh     int32    `json:"mesh"`
	Texture  int32    `json:"texture"`
	Grade    int32    `json:"grade"`
	SlotMask int32    `json:"slotMask"`
	Slots    []string `json:"slots"`
	Price    int      `json:"price"`
	Volatile int      `json:"volatile"` // EF_VOLATILE; 0 = equippable
	Effects  []Effect `json:"effects"`
	Req      Req      `json:"req"`
	IconKey  string   `json:"iconKey"` // "iNNNN"; empty when the client has no mapping
}

// Data is the payload the UI fetches once and then filters client-side.
type Data struct {
	Items          []Item `json:"items"`
	CatalogVersion string `json:"catalogVersion"`
	IconPack       string `json:"iconPack"` // pack version; empty = no icons served
	// Legend explains every EF_* token (see effects.go). It ships with the
	// catalog so the UI can render tooltips without a second round trip.
	Legend map[string]EffectInfo `json:"legend"`
	// Bus is the GM command reference rendered by the "Comandos ADM" tab.
	Bus CommandBus `json:"bus"`
}

// reqColumn and priceColumn are the CSV positions of the packed equip
// requirement and Price (content/catalog.go Requirements/Prices).
const (
	reqColumn   = 3
	priceColumn = 5
)

// emptyPayload is the degraded response used when the catalog cannot be
// marshalled — see Handler.
const emptyPayload = `{"items":[],"catalogVersion":"","iconPack":""}`

// Load reads the catalog from a content tree (the directory holding
// Common/ItemList.csv) and merges both views of it. Items come out sorted by
// index so the UI has a stable order.
func Load(contentDir string) (Data, error) {
	catalog, err := itemcatalog.Scan(contentDir)
	if err != nil {
		return Data{}, fmt.Errorf("itembrowser: scan catalog: %w", err)
	}
	extra, err := readExtras(filepath.Join(contentDir, "Common", "ItemList.csv"))
	if err != nil {
		return Data{}, err
	}

	items := make([]Item, 0, len(catalog.Items))
	for _, e := range catalog.Items {
		it := Item{
			Index: e.Index, Name: e.DisplayName, Raw: e.Name,
			Mesh: e.Mesh, Texture: e.Texture, Grade: e.Grade,
			SlotMask: e.SlotMask, Slots: e.Slots, IconKey: e.IconKey,
		}
		if x, ok := extra[e.Index]; ok {
			it.Price, it.Req, it.Effects = x.price, x.req, x.effects
			for _, ef := range x.effects {
				if ef.Name == "EF_VOLATILE" {
					it.Volatile = ef.Value
					break
				}
			}
		}
		// A JSON null would break the UI's indexOf/join on these.
		if it.Slots == nil {
			it.Slots = []string{}
		}
		if it.Effects == nil {
			it.Effects = []Effect{}
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Index < items[j].Index })
	return Data{
		Items:          items,
		CatalogVersion: catalog.Version,
		Legend:         EffectTable(),
		Bus:            CommandReference(),
	}, nil
}

// rowExtra is the part of a catalog row that itemcatalog.Entry does not carry.
type rowExtra struct {
	price   int
	req     Req
	effects []Effect
}

// readExtras pulls price, equip requirement and the EF_* pairs out of every
// row. Malformed rows are skipped rather than failing the load: the shipped CSV
// has garbage lines, and a browser that refuses to start over one is useless.
func readExtras(path string) (map[int32]rowExtra, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("itembrowser: open %s: %w", path, err)
	}
	defer f.Close()

	out := make(map[int32]rowExtra)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // rows can be long
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), ",")
		if len(fields) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || idx < 0 {
			continue
		}
		out[int32(idx)] = rowExtra{
			price:   intColumn(fields, priceColumn),
			req:     parseReq(fields),
			effects: rawEffects(fields),
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("itembrowser: read %s: %w", path, err)
	}
	return out, nil
}

// intColumn returns column i as an int, or 0 when it is missing or not numeric.
func intColumn(fields []string, i int) int {
	if i >= len(fields) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(fields[i]))
	if err != nil {
		return 0
	}
	return n
}

// parseReq splits the dotted requirement column into its five values.
func parseReq(fields []string) Req {
	if reqColumn >= len(fields) {
		return Req{}
	}
	parts := strings.Split(strings.TrimSpace(fields[reqColumn]), ".")
	get := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return 0
		}
		return n
	}
	return Req{Lvl: get(0), Str: get(1), Int: get(2), Dex: get(3), Con: get(4)}
}

// rawEffects pulls every EF_<name>,<value> pair out of a row. The CSV has no
// fixed effect count — pairs simply run to the end of the line — so the scan
// walks the whole row instead of assuming column positions.
func rawEffects(fields []string) []Effect {
	var out []Effect
	for i := 0; i+1 < len(fields); i++ {
		name := strings.TrimSpace(fields[i])
		if !strings.HasPrefix(name, "EF_") {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(fields[i+1]))
		if err != nil {
			continue
		}
		out = append(out, Effect{Name: name, Value: v})
		i++ // the value column is consumed
	}
	return out
}

// ApplyIcons stamps icon keys from a generated pack onto the data and records
// the pack version, so the UI knows icons can be served.
func ApplyIcons(d *Data, m itemicons.Manifest) {
	for i := range d.Items {
		d.Items[i].IconKey = m.IconKey(d.Items[i].Index)
	}
	d.IconPack = m.PackVersion
}

// Handler serves the UI at "/" and the catalog JSON at "/api/items". When
// iconDir is non-empty it is the generated pack directory (the one holding
// manifest.json and the <pack_version>/ PNGs), served read-only under /icons/.
func Handler(d Data, iconDir string) http.Handler {
	payload, err := json.Marshal(d)
	if err != nil {
		// Data is plain structs with no channels or funcs, so a failure here is a
		// bug rather than a runtime condition: degrade to an empty catalog instead
		// of refusing to start the tool.
		payload = []byte(emptyPayload)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(payload)
	})
	if iconDir != "" {
		mux.Handle("/icons/", http.StripPrefix("/icons/", http.FileServer(http.Dir(iconDir))))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	return mux
}
