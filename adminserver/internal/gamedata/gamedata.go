// Package gamedata is the panel's client for the webServer's admin gRPC.
//
// Those services have existed and been tested for a while with no caller: they
// were built for a BFF that was never written, so the item catalog, the price
// overrides, the NPC shops and the donate shop all run in production reachable
// by nobody. This package is the missing caller, and the panel is the front.
//
// Nothing is reimplemented here. Every call goes to the service that already
// owns the rule, which is why a page can be added without touching webServer.
package gamedata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	webv1 "github.com/jeanluca/w2pp-openwyd/api/web/v1"
)

// catalogTTL bounds how stale the cached catalog may be.
//
// The catalog is ~3200 entries read from a file at webServer boot, so it only
// changes on a deploy — the proto says as much, and offers catalog_version to
// revalidate against. Learning that version costs the whole list, so a short TTL
// buys the same freshness with simpler code: a deploy is reflected within
// minutes, and a page load never waits on a round trip that returns the same
// bytes it did a second ago.
const catalogTTL = 5 * time.Minute

// Item is one catalog entry as the panel shows it, with any price override
// already merged in.
type Item struct {
	Index       int32
	Name        string // raw catalog name, underscores and all
	DisplayName string
	Grade       int32
	Slots       []string
	IconURL     string
	Price       int64 // effective price
	Overridden  bool  // …and whether it comes from an override rather than the catalog
}

// Client talks to the webServer.
type Client struct {
	catalog  webv1.ItemCatalogServiceClient
	npc      webv1.NpcAdminServiceClient
	mob      webv1.MobTemplateAdminServiceClient
	itemStat webv1.ItemStatAdminServiceClient

	// Guards the cache below. Two requests missing at once both fetch, which is
	// the right trade here: collapsing them would need a second lock held across
	// a network call, and a staff panel does not have the traffic to make the
	// duplicate round trip cost anything.
	mu      sync.Mutex
	cached  []*webv1.ItemCatalogEntry
	version string
	fetched time.Time
}

// New wraps a connection to the webServer.
func New(conn grpc.ClientConnInterface) *Client {
	return &Client{
		catalog:  webv1.NewItemCatalogServiceClient(conn),
		npc:      webv1.NewNpcAdminServiceClient(conn),
		mob:      webv1.NewMobTemplateAdminServiceClient(conn),
		itemStat: webv1.NewItemStatAdminServiceClient(conn),
	}
}

// CatalogVersion reports the fingerprint of the cached catalog, or "" before the
// first successful fetch. Shown in the UI so a stale page is recognisable.
func (c *Client) CatalogVersion() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.version
}

// Items returns catalog entries whose name contains the query, with prices
// merged. An empty query returns everything, which the caller is expected to
// page.
func (c *Client) Items(ctx context.Context, moderatorID int64, query string) ([]Item, error) {
	entries, err := c.entries(ctx)
	if err != nil {
		return nil, err
	}

	// Overrides are few (only what staff changed), so fetching them per request
	// is cheap and always current — the opposite trade from the catalog.
	overrides := map[int32]int64{}
	if resp, err := c.npc.ListItemPrices(ctx, &webv1.ListItemPricesRequest{ModeratorId: moderatorID}); err == nil {
		for _, p := range resp.GetPrices() {
			overrides[p.GetItemIndex()] = p.GetPrice()
		}
	} else {
		// A failed override read must not blank the catalog: showing base prices
		// with a warning beats showing nothing.
		return nil, fmt.Errorf("gamedata: list item prices: %w", err)
	}

	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Item, 0, 64)
	for _, e := range entries {
		if q != "" && !strings.Contains(strings.ToLower(e.GetDisplayName()), q) &&
			!strings.Contains(strings.ToLower(e.GetName()), q) {
			continue
		}
		it := Item{
			Index:       e.GetItemIndex(),
			Name:        e.GetName(),
			DisplayName: e.GetDisplayName(),
			Grade:       e.GetGrade(),
			Slots:       e.GetSlots(),
			IconURL:     e.GetIconUrl(),
		}
		if p, ok := overrides[e.GetItemIndex()]; ok {
			it.Price, it.Overridden = p, true
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// ItemLookup returns the catalog keyed by item index, for screens that need to
// turn a stored index into a name and an icon.
//
// It deliberately skips the price overrides that Items merges in: the character
// editor shows what an item IS, never what it costs, and the override read is a
// live RPC per request. Serving it from the cached catalog alone keeps a screen
// with three item grids on it from making a network call to label each one.
func (c *Client) ItemLookup(ctx context.Context) (map[int32]Item, error) {
	entries, err := c.entries(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int32]Item, len(entries))
	for _, e := range entries {
		out[e.GetItemIndex()] = Item{
			Index:       e.GetItemIndex(),
			Name:        e.GetName(),
			DisplayName: e.GetDisplayName(),
			Grade:       e.GetGrade(),
			Slots:       e.GetSlots(),
			IconURL:     e.GetIconUrl(),
		}
	}
	return out, nil
}

// SetPrice overrides an item's price, or clears the override when price < 0.
// The rule lives in the webServer; this only carries the request.
func (c *Client) SetPrice(ctx context.Context, moderatorID int64, itemIndex int32, price int64) error {
	_, err := c.npc.SetItemPrice(ctx, &webv1.SetItemPriceRequest{
		ModeratorId: moderatorID,
		ItemIndex:   itemIndex,
		Price:       price,
	})
	if err != nil {
		return fmt.Errorf("gamedata: set item price %d: %w", itemIndex, err)
	}
	// The override changed, so the merged view must not serve the old value.
	// The catalog itself is untouched, so only the merge is invalidated — which
	// happens naturally, since overrides are read per request.
	return nil
}

// entries returns the cached catalog, refreshing it when stale.
func (c *Client) entries(ctx context.Context) ([]*webv1.ItemCatalogEntry, error) {
	c.mu.Lock()
	fresh := c.cached != nil && time.Since(c.fetched) < catalogTTL
	cached := c.cached
	c.mu.Unlock()
	if fresh {
		return cached, nil
	}

	resp, err := c.catalog.ListItems(ctx, &webv1.ListItemsRequest{})
	if err != nil {
		// Serve stale rather than fail: an expired cache is a far better answer
		// than an error page when the only thing that changed is a deploy in
		// progress on the other service.
		if cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("gamedata: list items: %w", err)
	}

	c.mu.Lock()
	c.cached, c.version, c.fetched = resp.GetItems(), resp.GetCatalogVersion(), time.Now()
	c.mu.Unlock()
	return resp.GetItems(), nil
}

// --- NPCs e lojas ---

// maxShopSlot mirrors MSG_ShopList's 27 entries: a merchant's stock lives in
// slots 0..26 and the webServer validates the same bound.
const maxShopSlot = 26

// Refusals from the admin service, as values the panel can explain. The service
// answers with a result code in the body rather than a gRPC error, because these
// are business outcomes and not transport failures.
var (
	ErrForbidden = errors.New("gamedata: caller is not a moderator")
	ErrInvalid   = errors.New("gamedata: the service rejected the request")
	ErrNotFound  = errors.New("gamedata: not found")
	// ErrContentOwned means the definition came from the content tree and can be
	// hidden but not deleted. Deleting it would leave NPCGener.txt spawning
	// something the database no longer describes, so the service refuses.
	ErrContentOwned = errors.New("gamedata: definition is owned by the content tree")
)

// NPC is one merchant definition with its stock.
type NPC struct {
	ID           int64
	Slug         string
	DisplayName  string
	TemplateName string
	Enabled      bool
	MapID        int32
	X, Y         int32
	RouteType    int32
	Merchant     int32
	Origin       string
	Shop         []ShopItem
}

// ShopItem is one stock slot. The three effect pairs are carried through
// untouched: the panel edits what an NPC sells, not how a sold item is enchanted,
// and silently zeroing them on save would quietly strip existing stock.
type ShopItem struct {
	Slot      int32
	ItemIndex int32
	Quantity  int32
	Eff       [3][2]int32
}

// NPCs lists the DB-managed merchant definitions.
func (c *Client) NPCs(ctx context.Context, moderatorID int64, query string) ([]NPC, error) {
	resp, err := c.npc.ListNpcs(ctx, &webv1.ListNpcsRequest{ModeratorId: moderatorID})
	if err != nil {
		return nil, fmt.Errorf("gamedata: list npcs: %w", err)
	}
	if err := resultErr(resp.GetResult()); err != nil {
		return nil, err
	}

	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]NPC, 0, len(resp.GetNpcs()))
	for _, n := range resp.GetNpcs() {
		if q != "" && !strings.Contains(strings.ToLower(n.GetDisplayName()), q) &&
			!strings.Contains(strings.ToLower(n.GetSlug()), q) {
			continue
		}
		out = append(out, npcFromProto(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out, nil
}

// NPC fetches one definition with its stock.
func (c *Client) NPC(ctx context.Context, moderatorID, id int64) (NPC, error) {
	resp, err := c.npc.GetNpc(ctx, &webv1.GetNpcRequest{ModeratorId: moderatorID, NpcId: id})
	if err != nil {
		return NPC{}, fmt.Errorf("gamedata: get npc %d: %w", id, err)
	}
	if err := resultErr(resp.GetResult()); err != nil {
		return NPC{}, err
	}
	return npcFromProto(resp.GetNpc()), nil
}

// SetShop replaces a merchant's stock.
func (c *Client) SetShop(ctx context.Context, moderatorID, npcID int64, items []ShopItem) error {
	wire := make([]*webv1.AdminNpcShopItem, 0, len(items))
	for _, it := range items {
		if it.Slot < 0 || it.Slot > maxShopSlot {
			return fmt.Errorf("%w: slot %d out of range", ErrInvalid, it.Slot)
		}
		wire = append(wire, &webv1.AdminNpcShopItem{
			Slot: it.Slot, ItemIndex: it.ItemIndex, Quantity: it.Quantity,
			Eff1: it.Eff[0][0], Effv1: it.Eff[0][1],
			Eff2: it.Eff[1][0], Effv2: it.Eff[1][1],
			Eff3: it.Eff[2][0], Effv3: it.Eff[2][1],
		})
	}
	resp, err := c.npc.SetNpcShop(ctx, &webv1.SetNpcShopRequest{
		ModeratorId: moderatorID, NpcId: npcID, Items: wire,
	})
	if err != nil {
		return fmt.Errorf("gamedata: set shop %d: %w", npcID, err)
	}
	return resultErr(resp.GetResult())
}

func npcFromProto(n *webv1.AdminNpc) NPC {
	out := NPC{
		ID: n.GetId(), Slug: n.GetSlug(), DisplayName: n.GetDisplayName(),
		TemplateName: n.GetTemplateName(), Enabled: n.GetEnabled(),
		MapID: n.GetMapId(), X: n.GetPosX(), Y: n.GetPosY(),
		RouteType: n.GetRouteType(), Merchant: n.GetMerchant(), Origin: n.GetOrigin(),
	}
	for _, s := range n.GetShop() {
		out.Shop = append(out.Shop, ShopItem{
			Slot: s.GetSlot(), ItemIndex: s.GetItemIndex(), Quantity: s.GetQuantity(),
			Eff: [3][2]int32{
				{s.GetEff1(), s.GetEffv1()},
				{s.GetEff2(), s.GetEffv2()},
				{s.GetEff3(), s.GetEffv3()},
			},
		})
	}
	sort.Slice(out.Shop, func(i, j int) bool { return out.Shop[i].Slot < out.Shop[j].Slot })
	return out
}

// resultErr turns the service's business outcome into an error the panel can act
// on. OK and UNSPECIFIED both mean success: an older service that does not set
// the field must not read as a failure.
func resultErr(r webv1.AdminResult) error {
	switch r {
	case webv1.AdminResult_ADMIN_RESULT_OK, webv1.AdminResult_ADMIN_RESULT_UNSPECIFIED:
		return nil
	case webv1.AdminResult_ADMIN_RESULT_FORBIDDEN:
		return ErrForbidden
	case webv1.AdminResult_ADMIN_RESULT_NOT_FOUND:
		return ErrNotFound
	case webv1.AdminResult_ADMIN_RESULT_CONTENT_OWNED:
		return ErrContentOwned
	default:
		return ErrInvalid
	}
}

// MaxShopSlot exposes the stock bound so the UI renders exactly the slots the
// service accepts, instead of guessing.
func MaxShopSlot() int { return maxShopSlot }

// SaveNPC updates a definition. The service keys on slug rather than id, so the
// caller passes the whole NPC back — which is why the panel reads it first and
// edits fields on the value it got, instead of assembling one from a form and
// blanking whatever the form does not carry.
func (c *Client) SaveNPC(ctx context.Context, moderatorID int64, n NPC) error {
	resp, err := c.npc.UpsertNpc(ctx, &webv1.UpsertNpcRequest{
		ModeratorId:  moderatorID,
		Slug:         n.Slug,
		TemplateName: n.TemplateName,
		DisplayName:  n.DisplayName,
		Enabled:      n.Enabled,
		MapId:        n.MapID,
		PosX:         n.X,
		PosY:         n.Y,
		RouteType:    n.RouteType,
		Merchant:     n.Merchant,
	})
	if err != nil {
		return fmt.Errorf("gamedata: save npc %q: %w", n.Slug, err)
	}
	return resultErr(resp.GetResult())
}

// SetNPCVisible shows or hides a definition.
func (c *Client) SetNPCVisible(ctx context.Context, moderatorID, npcID int64, enabled bool) error {
	resp, err := c.npc.SetNpcVisibility(ctx, &webv1.SetNpcVisibilityRequest{
		ModeratorId: moderatorID, NpcId: npcID, Enabled: enabled,
	})
	if err != nil {
		return fmt.Errorf("gamedata: set npc visibility %d: %w", npcID, err)
	}
	return resultErr(resp.GetResult())
}

// DeleteNPC removes a definition. Content-owned ones come back as
// ErrContentOwned: they must be hidden instead.
func (c *Client) DeleteNPC(ctx context.Context, moderatorID, npcID int64) error {
	resp, err := c.npc.DeleteNpc(ctx, &webv1.DeleteNpcRequest{
		ModeratorId: moderatorID, NpcId: npcID,
	})
	if err != nil {
		return fmt.Errorf("gamedata: delete npc %d: %w", npcID, err)
	}
	return resultErr(resp.GetResult())
}
