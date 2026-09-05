package protocol

import (
	"encoding/binary"
	"testing"
)

// TestEncodeCNFMobKillBodyLayout pins the NATURAL alignment of MSG_CNFMobKill.
// The struct sits outside the pack(push,1) region (which closes at Basedef.h:1850
// and reopens at 2451), so its long long Exp starts 8-aligned: with a 12-byte
// header that lands at body offset 12, behind four bytes of padding, for a body of
// 20 rather than the 16 a packed reading gives. Encoding it packed made the client
// read Exp from bytes 12..19 of a 16-byte buffer and fill the experience panel
// with whatever followed.
func TestEncodeCNFMobKillBodyLayout(t *testing.T) {
	const (
		killedMob = uint16(1234)
		killer    = uint16(7)
		exp       = int64(48150)
	)
	body := EncodeCNFMobKillBody(killedMob, killer, exp)

	if len(body) != 20 {
		t.Fatalf("body = %d bytes, want 20 (natural alignment, not the packed 16)", len(body))
	}
	if got := binary.LittleEndian.Uint32(body[0:4]); got != 0 {
		t.Errorf("Hold = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(body[4:6]); got != killedMob {
		t.Errorf("KilledMob = %d, want %d", got, killedMob)
	}
	if got := binary.LittleEndian.Uint16(body[6:8]); got != killer {
		t.Errorf("Killer = %d, want %d", got, killer)
	}
	// The four bytes the compiler inserts before an 8-aligned long long.
	for i := 8; i < 12; i++ {
		if body[i] != 0 {
			t.Errorf("padding byte %d = %d, want 0", i, body[i])
		}
	}
	if got := int64(binary.LittleEndian.Uint64(body[12:20])); got != exp {
		t.Errorf("Exp@12 = %d, want %d", got, exp)
	}
}
