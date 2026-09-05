package handler

import (
	"encoding/binary"
	"testing"

	"github.com/jeanluca/w2pp-openwyd/tmserver/internal/protocol"
)

// TestWriteAttackerStatusPerMessageLayout pins the one thing that is NOT the same
// across the three attack messages. CurrentExp@12 and ReqMp@46 sit still, but
// MSG_Attack carries CurrentHp@4/CurrentMp@40 while MSG_AttackOne and
// MSG_AttackTwo carry them the other way round (Basedef.h:2400, 2452, 2488).
// Writing one fixed layout hands the client its MP as HP on two of the three.
func TestWriteAttackerStatusPerMessageLayout(t *testing.T) {
	const (
		hp    = int32(1234)
		mp    = int32(5678)
		exp   = int64(229572)
		reqMp = int16(42)
	)
	tests := []struct {
		name              string
		msgType           protocol.Type
		wantAt4, wantAt40 int32
	}{
		{"MSG_Attack keeps HP first", protocol.MsgAttack, hp, mp},
		{"MSG_AttackOne swaps them", protocol.MsgAttackOne, mp, hp},
		{"MSG_AttackTwo swaps them", protocol.MsgAttackTwo, mp, hp},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := make([]byte, protocol.MsgAttackDamOffset)
			writeAttackerStatus(payload, tc.msgType, hp, mp, exp, reqMp)

			if got := int32(binary.LittleEndian.Uint32(payload[4:8])); got != tc.wantAt4 {
				t.Errorf("body@4 = %d, want %d", got, tc.wantAt4)
			}
			if got := int32(binary.LittleEndian.Uint32(payload[40:44])); got != tc.wantAt40 {
				t.Errorf("body@40 = %d, want %d", got, tc.wantAt40)
			}
			// Exp is the field the exp bar reads, and it never moves between layouts.
			if got := int64(binary.LittleEndian.Uint64(payload[12:20])); got != exp {
				t.Errorf("CurrentExp@12 = %d, want %d", got, exp)
			}
			if got := int16(binary.LittleEndian.Uint16(payload[46:48])); got != reqMp {
				t.Errorf("ReqMp@46 = %d, want %d", got, reqMp)
			}
		})
	}
}

// TestWriteAttackerStatusIgnoresShortBody: a frame too small to hold the fixed
// fields is left untouched rather than panicking.
func TestWriteAttackerStatusIgnoresShortBody(t *testing.T) {
	payload := make([]byte, protocol.MsgAttackDamOffset-1)
	writeAttackerStatus(payload, protocol.MsgAttackOne, 1, 2, 3, 4)
	for i, b := range payload {
		if b != 0 {
			t.Fatalf("byte %d = %d, want the short body left alone", i, b)
		}
	}
}
