package handler

import (
	"testing"
	"time"
)

// The presence mark is what lets the staff panel refuse to edit a character the
// loop owns. If login does not set it, the panel edits live characters and the
// writes are silently lost at the next save — the exact failure the whole
// mechanism exists to prevent.
func TestLoginMarcaPresenca(t *testing.T) {
	db := tradeDB()
	addr, stop, _ := startServerClock(t, db)
	defer stop()

	c := enterWorldAs(t, addr, "tester")
	defer c.Close()

	// The mark is written off the loop, so poll rather than assume it landed
	// before the login packets did.
	var online, ok bool
	for i := 0; i < 100; i++ {
		if online, ok = db.presenceOf("Hero"); ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ok {
		t.Fatal("o login não marcou presença")
	}
	if !online {
		t.Error("presença = false no login, want true")
	}
}

// And it has to be cleared on the way out. A character left marked in-play is
// one the panel refuses to edit forever.
func TestSaidaLimpaPresenca(t *testing.T) {
	db := tradeDB()
	addr, stop, _ := startServerClock(t, db)
	defer stop()

	c := enterWorldAs(t, addr, "tester")
	for i := 0; i < 100; i++ {
		if online, ok := db.presenceOf("Hero"); ok && online {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// A dropped socket, not a clean logout: this is the path that actually has
	// to work, because a client that crashes never sends a logout.
	_ = c.Close()

	for i := 0; i < 200; i++ {
		if online, ok := db.presenceOf("Hero"); ok && !online {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	online, ok := db.presenceOf("Hero")
	t.Fatalf("presença após a queda = %v (registrada: %v), want false", online, ok)
}
