package panel

import (
	"sync"
	"time"
)

// Login throttling. Same token-bucket shape the game edge uses for per-connection
// message rate (tmserver/internal/world/ratelimit.go), kept in-process on purpose:
// the panel runs as one instance, so a shared store would add a paid dependency
// and buy nothing.
const (
	loginRefillPerSec = 5.0 / 60.0 // five attempts a minute, sustained
	loginBurst        = 5          // …and five available at once after a quiet spell
	limiterIdleTTL    = 15 * time.Minute
)

// bucket is a token bucket over wall time.
type bucket struct {
	tokens float64
	max    float64
	refill float64
	last   time.Time
}

func (b *bucket) allow(now time.Time) bool {
	b.tokens += now.Sub(b.last).Seconds() * b.refill
	if b.tokens > b.max {
		b.tokens = b.max
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// limiter throttles login attempts by key.
//
// Two keys are checked per attempt, and both matter. Throttling by source alone
// fails when attempts come from many addresses against one account; throttling
// by account alone fails when one source sprays many account names. Neither is
// hypothetical for a public panel with staff accounts whose names are visible
// in-game.
type limiter struct {
	mu   sync.Mutex
	now  func() time.Time
	seen map[string]*bucket
}

func newLimiter() *limiter {
	return &limiter{now: time.Now, seen: make(map[string]*bucket)}
}

// allow consumes one token for every key and reports whether ALL of them had one.
//
// Every key is charged even when an earlier one already refused, so a caller
// cannot keep one bucket full by always tripping another first.
func (l *limiter) allow(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	ok := true
	for _, k := range keys {
		b := l.seen[k]
		if b == nil {
			b = &bucket{tokens: loginBurst, max: loginBurst, refill: loginRefillPerSec, last: now}
			l.seen[k] = b
		}
		if !b.allow(now) {
			ok = false
		}
	}
	return ok
}

// sweepLocked drops buckets that have been full and untouched long enough to be
// indistinguishable from a fresh one. Without it the map grows once per distinct
// account name tried, which is exactly what a spraying attacker produces.
func (l *limiter) sweepLocked(now time.Time) {
	for k, b := range l.seen {
		if now.Sub(b.last) > limiterIdleTTL {
			delete(l.seen, k)
		}
	}
}
