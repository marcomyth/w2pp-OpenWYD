package npcpanel

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/npcadmin"
)

// Admin is the slice of npcadmin.Service the panel drives. Depending on the
// interface rather than the concrete service keeps the HTTP layer testable
// without a database, which matters here because the panel is developed and run
// long before a Postgres instance exists.
type Admin interface {
	List(ctx context.Context, moderatorID int64) (npcadmin.Result, []domain.NPCDefinition, error)
	Get(ctx context.Context, moderatorID, npcID int64) (npcadmin.Result, domain.NPCDefinition, error)
	Upsert(ctx context.Context, moderatorID int64, d domain.NPCDefinition) (npcadmin.Result, int64, error)
	SetVisibility(ctx context.Context, moderatorID, npcID int64, enabled bool) (npcadmin.Result, error)
	SetShop(ctx context.Context, moderatorID, npcID int64, items []domain.NPCShopItem) (npcadmin.Result, error)
}

// Config wires one panel instance.
type Config struct {
	// Data is the content-tree inventory, loaded once at boot.
	Data Data
	// Admin is optional. Without it the panel is a read-only map of the content
	// tree — which is the whole of what it can honestly offer with no database.
	Admin Admin
	// ModeratorID is the account whose role authorizes every write. The panel is
	// a loopback tool with no login of its own, so the operator passes it on the
	// command line; npcadmin still re-checks the role on every call, so a wrong
	// id fails closed rather than granting access.
	ModeratorID int64
	// IconDir is the generated item-icon pack directory; empty disables icons.
	IconDir string
	Logger  *slog.Logger
}

// Handler serves the panel UI at "/" and its JSON API under "/api/".
func Handler(cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(nopWriter{}, nil))
	}
	h := &handler{cfg: cfg}
	h.cfg.Data.Editable = cfg.Admin != nil

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/npcs", h.inventory)
	mux.HandleFunc("POST /api/npc/visibility", h.setVisibility)
	mux.HandleFunc("POST /api/npc/position", h.setPosition)
	mux.HandleFunc("POST /api/npc/shop", h.setShop)
	if cfg.IconDir != "" {
		mux.Handle("GET /icons/", http.StripPrefix("/icons/", http.FileServer(http.Dir(cfg.IconDir))))
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	return mux
}

type handler struct{ cfg Config }

// inventory returns the content-tree map overlaid with the database rows, so a
// moderator sees the position and visibility that are actually in effect.
func (h *handler) inventory(w http.ResponseWriter, r *http.Request) {
	data := h.cfg.Data
	if h.cfg.Admin != nil {
		res, defs, err := h.cfg.Admin.List(r.Context(), h.cfg.ModeratorID)
		switch {
		case err != nil:
			// The content map is still true and useful, so degrade to read-only
			// rather than failing the page: a database hiccup should not leave a
			// moderator with a blank panel.
			h.cfg.Logger.Error("npcpanel: list definitions", "err", err)
			data.Editable = false
		case res != npcadmin.OK:
			h.cfg.Logger.Warn("npcpanel: list refused", "result", res, "moderator", h.cfg.ModeratorID)
			data.Editable = false
		default:
			data.NPCs = overlay(data.NPCs, defs)
		}
	}
	writeJSON(w, http.StatusOK, data)
}

// overlay replaces content values with the stored definition wherever one
// exists, and appends the definitions that match no block — those are the NPCs
// a moderator created, which exist only in the database.
//
// Rows are matched by slug, the key dbserver import-npcs derives from the same
// block index, so the join is exact rather than positional.
func overlay(base []NPC, defs []domain.NPCDefinition) []NPC {
	bySlug := make(map[string]int, len(base))
	for i, n := range base {
		bySlug[n.Slug] = i
	}
	out := append([]NPC(nil), base...)
	for _, d := range defs {
		i, ok := bySlug[d.Slug]
		if !ok {
			out = append(out, fromDefinition(d))
			continue
		}
		out[i].DBID = d.ID
		out[i].Enabled = d.Enabled
		out[i].Origin = d.Origin
		if d.DisplayName != "" {
			out[i].Name = d.DisplayName
		}
		// A moderator may have moved the NPC, which re-derives its city.
		if d.PosX != out[i].X || d.PosY != out[i].Y {
			out[i].X, out[i].Y = d.PosX, d.PosY
			applyZone(&out[i])
		}
		if len(d.Shop) > 0 {
			out[i].Shop = shopFromDefinition(d.Shop, out[i].Shop)
		}
	}
	return out
}

// fromDefinition builds a row for a definition with no content block behind it
// — an NPC created in the panel. Its look is unknown here because it lives in
// the template file, which the UI resolves from another row using the same
// template name.
func fromDefinition(d domain.NPCDefinition) NPC {
	n := NPC{
		GeneratorIndex: int(d.GeneratorIndex),
		Slug:           d.Slug,
		Template:       d.TemplateName,
		Name:           d.DisplayName,
		Origin:         d.Origin,
		Enabled:        d.Enabled,
		Merchant:       int(d.Merchant),
		X:              d.PosX,
		Y:              d.PosY,
		RouteType:      int(d.RouteType),
		MinuteGenerate: int(d.MinuteGenerate),
		MaxNumMob:      int(d.MaxNumMob),
		DBID:           d.ID,
	}
	if n.Origin == "" {
		n.Origin = OriginCustom
	}
	if n.Name == "" {
		n.Name = d.TemplateName
	}
	n.Shop = shopFromDefinition(d.Shop, nil)
	applyZone(&n)
	return n
}

// shopFromDefinition converts stored shop slots, reusing the catalog names
// already resolved for the same item index on the content side so the panel
// does not need a second catalog lookup path.
func shopFromDefinition(items []domain.NPCShopItem, known []Item) []Item {
	names := make(map[int32]Item, len(known))
	for _, k := range known {
		names[k.Index] = k
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		item := Item{Slot: int(it.Slot), Index: it.ItemIndex}
		if k, ok := names[it.ItemIndex]; ok {
			item.Name, item.Mesh, item.Grade, item.IconKey = k.Name, k.Mesh, k.Grade, k.IconKey
		}
		item.Effects = [3][2]int{
			{int(it.Eff1), int(it.EffV1)},
			{int(it.Eff2), int(it.EffV2)},
			{int(it.Eff3), int(it.EffV3)},
		}
		out = append(out, item)
	}
	return out
}

type visibilityRequest struct {
	NpcID   int64 `json:"npcId"`
	Enabled bool  `json:"enabled"`
}

func (h *handler) setVisibility(w http.ResponseWriter, r *http.Request) {
	var req visibilityRequest
	if !h.decode(w, r, &req) {
		return
	}
	res, err := h.cfg.Admin.SetVisibility(r.Context(), h.cfg.ModeratorID, req.NpcID, req.Enabled)
	h.respond(w, res, err, "set visibility")
}

type positionRequest struct {
	NpcID int64 `json:"npcId"`
	X     int32 `json:"x"`
	Y     int32 `json:"y"`
	// MapID is the settlement the moderator confirmed. -1 means "derive it from
	// the new position", which is what the UI sends unless the operator picked a
	// zone by hand.
	MapID int32 `json:"mapId"`
}

// setPosition moves an NPC. It reads the definition first and writes it back
// whole because Upsert replaces the row: sending only the coordinates would
// blank the display name, merchant type and route.
func (h *handler) setPosition(w http.ResponseWriter, r *http.Request) {
	var req positionRequest
	if !h.decode(w, r, &req) {
		return
	}
	res, def, err := h.cfg.Admin.Get(r.Context(), h.cfg.ModeratorID, req.NpcID)
	if err != nil || res != npcadmin.OK {
		h.respond(w, res, err, "get definition")
		return
	}
	def.PosX, def.PosY = req.X, req.Y
	def.MapID = req.MapID
	if req.MapID < 0 {
		def.MapID = deriveZone(req.X, req.Y)
	}
	res, _, err = h.cfg.Admin.Upsert(r.Context(), h.cfg.ModeratorID, def)
	h.respond(w, res, err, "move npc")
}

type shopRequest struct {
	NpcID int64 `json:"npcId"`
	Items []struct {
		Slot      int16 `json:"slot"`
		ItemIndex int32 `json:"itemIndex"`
		Quantity  int16 `json:"quantity"`
	} `json:"items"`
}

// setShop replaces a merchant stock wholesale — adding and removing an item are
// the same call, which is how npcadmin.SetShop is defined (it validates slots
// and effects, so the panel does not duplicate those rules).
func (h *handler) setShop(w http.ResponseWriter, r *http.Request) {
	var req shopRequest
	if !h.decode(w, r, &req) {
		return
	}
	items := make([]domain.NPCShopItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, domain.NPCShopItem{
			Slot: it.Slot, ItemIndex: it.ItemIndex, Quantity: it.Quantity,
		})
	}
	res, err := h.cfg.Admin.SetShop(r.Context(), h.cfg.ModeratorID, req.NpcID, items)
	h.respond(w, res, err, "set shop")
}

// decode rejects a write when no database is wired up and parses the body.
func (h *handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if h.cfg.Admin == nil {
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "painel em modo leitura: nenhum banco configurado (-dsn)"})
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return false
	}
	return true
}

// respond maps a business Result to HTTP. Infrastructure errors are logged with
// their detail and reported generically, so a database message never reaches
// the browser.
func (h *handler) respond(w http.ResponseWriter, res npcadmin.Result, err error, op string) {
	if err != nil {
		h.cfg.Logger.Error("npcpanel: "+op, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "erro interno"})
		return
	}
	status, msg := http.StatusOK, ""
	switch res {
	case npcadmin.OK:
	case npcadmin.Forbidden:
		status, msg = http.StatusForbidden, "conta sem papel de moderador/admin"
	case npcadmin.NotFound:
		status, msg = http.StatusNotFound, "NPC não encontrado no banco"
	case npcadmin.Invalid:
		status, msg = http.StatusBadRequest, "dados inválidos"
	case npcadmin.ContentOwned:
		status, msg = http.StatusConflict, "NPC de conteúdo: desative em vez de remover"
	default:
		status, msg = http.StatusInternalServerError, "resultado desconhecido"
	}
	if msg != "" {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}
	writeJSON(w, status, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
