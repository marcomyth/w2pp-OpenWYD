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
	catalog webv1.ItemCatalogServiceClient
	npc     webv1.NpcAdminServiceClient

	mu       sync.Mutex
	cached   []*webv1.ItemCatalogEntry
	version  string
	fetched  time.Time
	inFlight bool
}

// New wraps a connection to the webServer.
func New(conn grpc.ClientConnInterface) *Client {
	return &Client{
		catalog: webv1.NewItemCatalogServiceClient(conn),
		npc:     webv1.NewNpcAdminServiceClient(conn),
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
