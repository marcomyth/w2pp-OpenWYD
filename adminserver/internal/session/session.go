// Package session holds staff panel sessions in memory.
//
// In memory, not in Postgres, and that is a decision rather than a shortcut.
// Sessions are worthless after a redeploy anyway if they are meant to be short,
// the panel runs as a single instance, and a table would be the one piece of
// this feature that is NOT additive — it would have to be created, migrated and
// later dropped to undo the panel. Losing sessions on restart costs staff one
// login; keeping them in the database costs the property that the whole feature
// can be deleted by removing a service.
//
// Sessions carry the account id and nothing else. The role is deliberately absent:
// it is re-read from the database on every request, so demoting someone takes
// effect immediately instead of when their token happens to expire — and demotion
// is precisely the action that must not wait.
package session

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// tokenBytes is the entropy behind a session id. 256 bits makes guessing
// irrelevant as an attack, which is why lookup can be a plain map read rather
// than a constant-time compare.
const tokenBytes = 32

// Session is one signed-in staff member.
type Session struct {
	AccountID   int64
	AccountName string // for audit lines and the panel header; never for authorization
	Created     time.Time
	Expires     time.Time
}

// Store keeps live sessions. Unlike the game's world state, this is touched from
// many goroutines (one per request), so it is mutex-guarded — the single-owner
// rule belongs to the game loop and has no bearing here.
type Store struct {
	mu   sync.Mutex
	ttl  time.Duration
	now  func() time.Time // injectable so tests do not sleep
	live map[string]Session
}

// New builds a store whose sessions live for ttl.
func New(ttl time.Duration) *Store {
	return &Store{ttl: ttl, now: time.Now, live: make(map[string]Session)}
}

// Create issues a session and returns its token.
func (s *Store) Create(accountID int64, accountName string) (string, Session, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	now := s.now()
	sess := Session{
		AccountID:   accountID,
		AccountName: accountName,
		Created:     now,
		Expires:     now.Add(s.ttl),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[token] = sess
	return token, sess, nil
}

// Get returns the session for a token, or ok=false if it is unknown or expired.
// An expired entry is dropped on the way out, which keeps the common path doing
// the cleanup and avoids needing a sweeper goroutine for a map this small.
func (s *Store) Get(token string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.live[token]
	if !ok {
		return Session{}, false
	}
	if !s.now().Before(sess.Expires) {
		delete(s.live, token)
		return Session{}, false
	}
	return sess, true
}

// Delete ends one session (logout).
func (s *Store) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, token)
}

// DeleteByAccount ends every session belonging to one account. Not wired to a
// route yet; it is what a future "revoke access" has to call, and it exists here
// so that path does not get invented as a second, weaker mechanism.
func (s *Store) DeleteByAccount(accountID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for token, sess := range s.live {
		if sess.AccountID == accountID {
			delete(s.live, token)
			n++
		}
	}
	return n
}

// Len reports how many sessions are held, expired ones included. For tests and
// for a future status line; not an authorization input.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.live)
}
