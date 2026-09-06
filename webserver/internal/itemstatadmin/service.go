// Package itemstatadmin holds the web platform's moderator item base
// stat-editing logic (0023_item_stats) — the item-side sibling of
// webserver/internal/mobtemplateadmin. It edits what a catalog item requires to
// equip and the effects it grants, gated by account.role and recorded in the
// audit trail by the store. It never touches live game state: the tmServer only
// reads these overrides, once at boot.
//
// Item price is edited elsewhere, through npcadmin.SetItemPrice, and behaves
// differently on purpose — it hot-reloads within ~15s, because a price is only
// read at the moment of a shop transaction, while these numbers feed the equip
// score model and are recomputed per character.
package itemstatadmin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jeanluca/w2pp-openwyd/internal/domain"
	"github.com/jeanluca/w2pp-openwyd/internal/itemeffect"
	"github.com/jeanluca/w2pp-openwyd/internal/store"
	"github.com/jeanluca/w2pp-openwyd/webserver/internal/itemcatalog"
)

// Store is the persistence surface the service needs (satisfied by *store.Store).
type Store interface {
	AccountRole(ctx context.Context, id int64) (string, error)
	GetItemStat(ctx context.Context, itemIndex int32) (domain.ItemStat, error)
	UpsertItemStat(ctx context.Context, st domain.ItemStat, moderatorID int64) error
	DeleteItemStat(ctx context.Context, itemIndex int32, moderatorID int64) error
}

// CatalogReader resolves an item index to its ItemList.csv row.
//
// Used by Get's read-through fallback, so opening the editor on an un-edited
// item shows the catalog's real numbers instead of zeros. This is not a nicety:
// an override replaces an item's whole effect list, so a form that opened on
// zeros would strip the item the first time somebody saved it.
//
// nil when -content/W2PP_CONTENT was not configured, in which case Get returns
// NotFound for an un-overridden item rather than inventing values.
type CatalogReader func(itemIndex int32) (itemcatalog.Entry, bool)

// Result is the business outcome of an admin operation. Only infra failures are
// returned as errors; these ride in the response body.
type Result int

const (
	// OK means the operation succeeded.
	OK Result = iota
	// Forbidden means the caller is not a moderator/admin.
	Forbidden
	// Invalid means the request failed validation.
	Invalid
	// NotFound means the item has no override AND could not be read through
	// from the catalog either.
	NotFound
)

// Service implements the moderator item-stat-editing operations.
type Service struct {
	store   Store
	catalog CatalogReader
}

// New builds the service over the given store.
func New(s Store) *Service { return &Service{store: s} }

// SetCatalog installs the read-through catalog Get uses when no override exists.
func (s *Service) SetCatalog(r CatalogReader) { s.catalog = r }

// Item is what Get returns: the numbers, the catalog name for the UI, and
// whether the numbers are a saved override or a read-through of the catalog.
type Item struct {
	Stat        domain.ItemStat
	DisplayName string
	Overridden  bool
}

// Get returns an item's override if one exists; otherwise it reads the
// catalog's current values through CatalogReader, with Overridden false.
func (s *Service) Get(ctx context.Context, moderatorID int64, itemIndex int32) (Result, Item, error) {
	if r, err := s.authorize(ctx, moderatorID); r != OK || err != nil {
		return r, Item{}, err
	}
	if itemIndex < 0 {
		return Invalid, Item{}, nil
	}

	// The catalog is consulted either way, because the display name lives there
	// and never in the override: an item's name is ItemList.csv's to own.
	entry, temCatalogo := s.lookup(itemIndex)

	st, err := s.store.GetItemStat(ctx, itemIndex)
	if err == nil {
		return OK, Item{Stat: st, DisplayName: entry.DisplayName, Overridden: true}, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return Invalid, Item{}, fmt.Errorf("itemstatadmin: get %d: %w", itemIndex, err)
	}
	if !temCatalogo {
		return NotFound, Item{}, nil
	}
	return OK, Item{Stat: statFromCatalog(entry), DisplayName: entry.DisplayName}, nil
}

// Upsert writes an item's override whole.
func (s *Service) Upsert(ctx context.Context, moderatorID int64, st domain.ItemStat) (Result, error) {
	if r, err := s.authorize(ctx, moderatorID); r != OK || err != nil {
		return r, err
	}
	if st.ItemIndex < 0 {
		return Invalid, nil
	}
	// An index the catalog does not know would write a row the game can never
	// apply — it overlays by index onto entries loaded from the CSV. Refusing
	// beats leaving a row that looks saved and does nothing. With no catalog
	// configured there is nothing to check against, so the write is allowed.
	if s.catalog != nil {
		if _, ok := s.catalog(st.ItemIndex); !ok {
			return Invalid, nil
		}
	}
	if err := s.store.UpsertItemStat(ctx, st, moderatorID); err != nil {
		return Invalid, fmt.Errorf("itemstatadmin: upsert %d: %w", st.ItemIndex, err)
	}
	return OK, nil
}

// Delete removes the override, reverting the item to its catalog values. It
// never touches ItemList.csv — Release/ is read-only in production.
func (s *Service) Delete(ctx context.Context, moderatorID int64, itemIndex int32) (Result, error) {
	if r, err := s.authorize(ctx, moderatorID); r != OK || err != nil {
		return r, err
	}
	err := s.store.DeleteItemStat(ctx, itemIndex, moderatorID)
	switch {
	case err == nil:
		return OK, nil
	case errors.Is(err, store.ErrNotFound):
		return NotFound, nil
	default:
		return Invalid, fmt.Errorf("itemstatadmin: delete %d: %w", itemIndex, err)
	}
}

func (s *Service) lookup(itemIndex int32) (itemcatalog.Entry, bool) {
	if s.catalog == nil {
		return itemcatalog.Entry{}, false
	}
	return s.catalog(itemIndex)
}

// authorize resolves the caller's role. The panel checks the role too, but this
// service is reachable by any client of the web API, so the check belongs here
// as well rather than only at the edge.
func (s *Service) authorize(ctx context.Context, moderatorID int64) (Result, error) {
	role, err := s.store.AccountRole(ctx, moderatorID)
	if errors.Is(err, store.ErrNotFound) {
		return Forbidden, nil
	}
	if err != nil {
		return Forbidden, fmt.Errorf("itemstatadmin: role of %d: %w", moderatorID, err)
	}
	if role != "moderator" && role != "admin" {
		return Forbidden, nil
	}
	return OK, nil
}

// statFromCatalog turns an ItemList.csv row into the numbers the editor opens
// on, so a new override starts as an exact copy of what the game uses today.
//
// It walks domain.ItemStatFields rather than assigning field by field: that
// table is the one place that says which column carries which effect, and a
// second hand-written mapping here would be free to disagree with it.
func statFromCatalog(e itemcatalog.Entry) domain.ItemStat {
	st := domain.ItemStat{
		ItemIndex: e.Index,
		ReqLevel:  e.Req.Lvl, ReqStr: e.Req.Str,
		ReqInt: e.Req.Int, ReqDex: e.Req.Dex, ReqCon: e.Req.Con,
		// Outside the effect loop below because they are outside the whitelist it
		// resolves through — see domain.ItemStat.
		Range: int16(e.Range), Volatile: int16(e.Volatile),
	}
	porID := make(map[uint8]int16, len(e.Effects))
	for _, ef := range e.Effects {
		// A row that lists the same effect twice sums, which is what the score
		// model does when it reads the list.
		porID[ef.Eff] += ef.Val
	}
	for _, f := range domain.ItemStatFields {
		if f.EF == "" {
			continue // a requirement column, filled above
		}
		id, ok := itemeffect.ID(f.EF)
		if !ok {
			continue
		}
		*f.Ptr(&st) = porID[id]
	}
	return st
}
