package world

import (
	"context"
	"time"
)

// Item serials (0033_item_serial): giving an item the identity it never had.
//
// An item is an index plus three effect pairs, and two identical ones are
// indistinguishable, so "which of these two is the copy" had no answer. The
// serial gives it one. It lives in Item.Serial, which — like ExpiresAt — never
// reaches the client: the wire STRUCT_ITEM is eight bytes with no room for
// anything else, so this is the only place an identity can sit without spending
// an effect slot. A marked +11 sword is still +11.
//
// The numbers come from the database in BLOCKS. The loop owns all world state
// alone and never blocks, so it cannot ask for a number while stamping an item;
// it takes a few thousand at boot, spends them from memory, and asks for more
// before running out.

const (
	// serialBloco is how many numbers one reservation buys. Big enough that a
	// busy day never needs two refills, small enough that a crashed server
	// wastes a rounding error of the 64-bit space.
	serialBloco = 5000

	// serialReserva is the low-water mark: when fewer than this remain, the
	// world asks for the next block. It has to cover the round trip to the
	// database while the loop keeps stamping, and stamping happens on save, so
	// a few hundred is a lot of saves' worth of headroom.
	serialReserva = 500

	// serialTempo caps the reservation call. Missing the block means items go
	// unmarked for a while, which is a gap; blocking a goroutine forever on a
	// sick database is worse.
	serialTempo = 10 * time.Second
)

// NewSerial hands out the next item serial, or 0 when it cannot.
//
// ZERO IS A VALID ANSWER and means "unmarked". It happens before the first
// block lands, and again if the database is unreachable long enough to drain
// the current one. Handing out a number that might already belong to another
// item would be far worse than handing out none: the whole feature rests on
// "two items with the same serial is proof", and one reused number turns that
// proof into a false accusation.
//
// Loop-only.
func (w *World) NewSerial() int64 {
	if w.serialProximo >= w.serialFim {
		w.pedirSerials() // dry: kick a refill and mark this item unmarked
		return 0
	}
	s := w.serialProximo
	w.serialProximo++
	if w.serialFim-w.serialProximo < serialReserva {
		w.pedirSerials()
	}
	return s
}

// pedirSerials reserves the next block, off the loop. Loop-only (it reads and
// sets the in-flight flag).
func (w *World) pedirSerials() {
	if w.serialPedindo {
		return // one reservation in flight is enough; a second would waste a block
	}
	w.serialPedindo = true
	p := w.persist
	w.GoDetached(func() func(*World) {
		ctx, cancel := context.WithTimeout(context.Background(), serialTempo)
		defer cancel()
		primeiro, err := p.ReserveSerials(ctx, serialBloco)
		return func(w *World) {
			w.serialPedindo = false
			if err != nil {
				// Deliberately quiet about retrying: the next NewSerial that
				// finds the block dry asks again. Retrying in a loop here would
				// hammer a database that is already unwell.
				w.log.Warn("item serials: reservation failed; items stay unmarked", "err", err)
				return
			}
			w.instalarBloco(primeiro)
		}
	})
}

// instalarBloco adopts a freshly reserved block. Loop-only.
//
// It never rewinds: a reply that arrives late, after a newer block is already in
// use, would otherwise hand out numbers a second time.
func (w *World) instalarBloco(primeiro int64) {
	if primeiro < w.serialFim {
		w.log.Warn("item serials: ignoring a block older than the one in use",
			"recebido", primeiro, "em_uso_ate", w.serialFim)
		return
	}
	w.serialProximo, w.serialFim = primeiro, primeiro+serialBloco
	w.log.Info("item serials reserved", "de", w.serialProximo, "ate", w.serialFim-1)
}

// PrimeSerials reserves the first block before the world starts serving.
//
// Called at boot, off the loop, while nothing else is running — so it can block.
// Without it the first characters to save would be stamped 0 and stay unmarked
// until the lazy refill landed, which is a hole at exactly the moment a fresh
// server is writing every item it has for the first time.
//
// Failure is not fatal. A server that boots with no database still runs; its
// items simply carry no identity until a later reservation succeeds.
func (w *World) PrimeSerials(ctx context.Context) error {
	primeiro, err := w.persist.ReserveSerials(ctx, serialBloco)
	if err != nil {
		return err
	}
	w.serialProximo, w.serialFim = primeiro, primeiro+serialBloco
	return nil
}

// marcar stamps an item that deserves a serial and does not have one yet.
//
// It returns the item because callers hold copies; the caller writes it back so
// the live item and the row about to be saved carry the same number.
//
// Two guards, both load-bearing. An item that already has a serial keeps it —
// that is the entire point, since a re-stamp on every save would give the copy
// and the original different numbers and hide the duplicate. And an item the
// rule does not care about stays at zero rather than burning a number: the
// world mints far more potions than swords.
func (w *World) marcar(it Item) Item {
	if it.Serial != 0 || it.Empty() || w.marcavel == nil || !w.marcavel(it) {
		return it
	}
	it.Serial = w.NewSerial()
	return it
}
