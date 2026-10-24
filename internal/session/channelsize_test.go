package session_test

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// TestChannelBufferSizesMatchSerializers pins the two halves of the state
// channel size together.
//
// This is the bug this test exists for, and it shipped twice:
//
//	SerializeStatUpdate was corrected from 236 to 233 bytes, with a comment
//	explaining that 236 "was writing three bytes past the end of the client's
//	stat buffer" — but NewConnectionData still allocated the channel at 0xEC
//	(236). SetData only copy()s into the allocated buffer and cannot shrink it,
//	so the corrected serializer left three zero bytes in place and the wire
//	output never changed. The same thing happened to the object channels: the
//	serializer's tail was zeroed while the channel stayed 201 bytes.
//
// Fixing a serializer therefore does nothing on its own. Both numbers have to
// move, and nothing in either package's compiler output relates them, so this
// test is the only thing that does.
func TestChannelBufferSizesMatchSerializers(t *testing.T) {
	cd := session.NewConnectionData(nil, 0, 0)

	t.Run("object channels", func(t *testing.T) {
		check := func(want int) {
			objs := cd.GetServerObjects()
			if len(objs) != want {
				t.Fatalf("%d object channels in use, want %d", len(objs), want)
			}
			for i, so := range objs {
				if got := len(so.GetData()); got != entity.EntityStateSize {
					t.Fatalf("slot %d allocated %d bytes, serializer produces %d — "+
						"the larger of the two is what reaches the client",
						i, got, entity.EntityStateSize)
				}
			}
		}
		check(session.RetailEntitySlots)
		// and every slot a client may ask for (session/entityslots.go)
		cd.SetEntitySlots(session.MaxEntitySlots)
		check(session.MaxEntitySlots)
	})

	t.Run("stat channel 0x42", func(t *testing.T) {
		if got := len(cd.GetClientStatUpdate().GetData()); got != entity.StatUpdateSize {
			t.Fatalf("stat channel allocated %d bytes, serializer produces %d",
				got, entity.StatUpdateSize)
		}
	})

	t.Run("effects channel 0x43", func(t *testing.T) {
		// as long as its count (SetSizedData): it starts as the serializer's
		// value for a player with no effect
		want := len(entity.SerializeBuffUpdate(&entity.Player{}))
		if got := len(cd.GetClientBuffUpdate().GetData()); got != want {
			t.Fatalf("effects channel starts at %d bytes, the serializer's value for none is %d",
				got, want)
		}
	})
}

// TestRetailMeasuredSizes states the wire sizes as measured off
// pcaps/ps2_sandstorm.pcap, so that a future change to the serializers has to
// disagree with the capture explicitly rather than by accident.
//
// Object channels: all 24 occupied channels, every update in the capture, decode
// to exactly 162. Stat channel: 233, ending with the 0xFF at offset 232. Effects
// channel: every value in the captures is 4 + 132 bytes an effect.
func TestRetailMeasuredSizes(t *testing.T) {
	if entity.EffectRecordSize != 132 {
		t.Errorf("EffectRecordSize = %d, retail's records are 132", entity.EffectRecordSize)
	}
	if entity.EntityStateSize != 162 {
		t.Errorf("EntityStateSize = %d, retail sends 162", entity.EntityStateSize)
	}
	if entity.StatUpdateSize != 233 {
		t.Errorf("StatUpdateSize = %d, retail sends 233", entity.StatUpdateSize)
	}
}
