package session

import (
	"testing"
	"time"
)

// at pins the clock so expiry is tested without sleeping.
func at(s *Store, t time.Time) { s.now = func() time.Time { return t } }

func TestCreateAndGet(t *testing.T) {
	s := New(time.Hour)
	token, sess, err := s.Create(7, "teste")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if sess.AccountID != 7 || sess.AccountName != "teste" {
		t.Fatalf("session = %+v, want account 7 teste", sess)
	}
	got, ok := s.Get(token)
	if !ok {
		t.Fatal("Get: session not found")
	}
	if got.AccountID != 7 {
		t.Fatalf("Get account = %d, want 7", got.AccountID)
	}
}

func TestTokensAreDistinct(t *testing.T) {
	s := New(time.Hour)
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, _, err := s.Create(1, "teste")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if seen[token] {
			t.Fatalf("token repeated: %q", token)
		}
		seen[token] = true
	}
}

func TestUnknownToken(t *testing.T) {
	s := New(time.Hour)
	if _, ok := s.Get("nao-existe"); ok {
		t.Fatal("Get accepted an unknown token")
	}
}

func TestExpiredSessionIsRejectedAndDropped(t *testing.T) {
	start := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	s := New(30 * time.Minute)
	at(s, start)
	token, _, err := s.Create(7, "teste")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// One second before expiry it still works.
	at(s, start.Add(30*time.Minute-time.Second))
	if _, ok := s.Get(token); !ok {
		t.Fatal("session expired early")
	}

	// Exactly at expiry it does not: the boundary is closed, so a session is
	// never valid at the instant it was set to end.
	at(s, start.Add(30*time.Minute))
	if _, ok := s.Get(token); ok {
		t.Fatal("expired session accepted")
	}
	if n := s.Len(); n != 0 {
		t.Fatalf("expired session left behind: Len = %d, want 0", n)
	}
}

func TestDelete(t *testing.T) {
	s := New(time.Hour)
	token, _, err := s.Create(7, "teste")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	s.Delete(token)
	if _, ok := s.Get(token); ok {
		t.Fatal("deleted session still valid")
	}
}

func TestDeleteByAccountLeavesOthers(t *testing.T) {
	s := New(time.Hour)
	// Two sessions for the account being revoked (two browsers), one for another.
	a1, _, _ := s.Create(7, "alvo")
	a2, _, _ := s.Create(7, "alvo")
	other, _, _ := s.Create(9, "outro")

	if n := s.DeleteByAccount(7); n != 2 {
		t.Fatalf("DeleteByAccount = %d, want 2", n)
	}
	for _, token := range []string{a1, a2} {
		if _, ok := s.Get(token); ok {
			t.Fatal("revoked session still valid")
		}
	}
	if _, ok := s.Get(other); !ok {
		t.Fatal("revoking one account ended another account's session")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New(time.Hour)
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				token, _, err := s.Create(1, "teste")
				if err != nil {
					return
				}
				s.Get(token)
				s.Delete(token)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if n := s.Len(); n != 0 {
		t.Fatalf("Len = %d, want 0", n)
	}
}
