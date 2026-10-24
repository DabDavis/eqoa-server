package session

import "testing"

// Every entity slot has a state channel of its own, the first 24 are CLIENT's
// 0x00-0x17 as they always were, and none is a channel CLIENT reads as
// something else (0x40 the group, 0x42 stats, 0x43 effects, 0x44-0x6B
// Channel44's records) or a DRDP transport (0xF8 and up).
func TestEntityChannelMap(t *testing.T) {
	if MaxEntitySlots != 204 {
		t.Fatalf("MaxEntitySlots = %d, want 204: 0x00-0x3F and 0x6C-0xF7", MaxEntitySlots)
	}
	seen := map[byte]int{}
	for slot := 0; slot < MaxEntitySlots; slot++ {
		ch := EntityChannel(slot)
		if slot < RetailEntitySlots && int(ch) != slot {
			t.Fatalf("slot %d on channel %#x: CLIENT's slots are channels 0x00-0x17", slot, ch)
		}
		if ch >= 0x40 && ch < 0x6C || ch >= 0xF8 {
			t.Fatalf("slot %d on channel %#x, which CLIENT reads as something else", slot, ch)
		}
		if other, ok := seen[ch]; ok {
			t.Fatalf("slots %d and %d share channel %#x", other, slot, ch)
		}
		seen[ch] = slot
		if back := EntitySlot(ch); back != slot {
			t.Fatalf("channel %#x reads back as slot %d, want %d", ch, back, slot)
		}
	}
	for ch := 0; ch < 256; ch++ {
		if _, ok := seen[byte(ch)]; !ok && EntitySlot(byte(ch)) != -1 {
			t.Errorf("channel %#x carries no slot, yet EntitySlot says %d", ch, EntitySlot(byte(ch)))
		}
	}
	for _, ch := range []byte{0x40, 0x42, 0x43, 0x44, 0x6B, 0xF8, 0xFB, 0xFF} {
		if EntitySlot(ch) != -1 {
			t.Errorf("EntitySlot(%#x) = %d, want -1", ch, EntitySlot(ch))
		}
	}
}

func TestClampEntitySlots(t *testing.T) {
	for _, c := range []struct{ n, limit, want int }{
		{0, MaxEntitySlots, RetailEntitySlots},
		{23, MaxEntitySlots, RetailEntitySlots},
		{24, MaxEntitySlots, 24},
		{128, MaxEntitySlots, 128},
		{1 << 20, MaxEntitySlots, MaxEntitySlots},
		{128, 64, 64},
		{128, 1000, 128},
		{1000, 1000, MaxEntitySlots},
		{128, 10, RetailEntitySlots},
	} {
		if got := ClampEntitySlots(c.n, c.limit); got != c.want {
			t.Errorf("ClampEntitySlots(%d, %d) = %d, want %d", c.n, c.limit, got, c.want)
		}
	}
}

// A connection keeps CLIENT's 24 until asked; more are allocated as asked and
// kept when the number falls again, and an ack's channel finds its slot.
func TestSetEntitySlots(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	if cd.EntitySlots() != RetailEntitySlots || len(cd.GetServerObjects()) != RetailEntitySlots {
		t.Fatalf("a new connection has %d slots in use, %d allocated; want 24 and 24",
			cd.EntitySlots(), len(cd.GetServerObjects()))
	}
	if cd.GetServerObject(RetailEntitySlots) != nil {
		t.Fatal("slot 24 allocated before anyone asked")
	}

	cd.SetEntitySlots(100)
	if cd.EntitySlots() != 100 || cd.GetServerObject(99) == nil || cd.GetServerObject(100) != nil {
		t.Fatalf("asked 100: %d in use, slot 99 %v, slot 100 %v",
			cd.EntitySlots(), cd.GetServerObject(99) != nil, cd.GetServerObject(100) != nil)
	}
	if got := len(cd.GetServerObject(99).GetData()); got != entityStateSize {
		t.Fatalf("slot 99 holds %d bytes, an object channel is %d", got, entityStateSize)
	}
	if cd.ServerObjectForChannel(0x6C) != cd.GetServerObject(64) || cd.ServerObjectForChannel(0x3F) != cd.GetServerObject(63) {
		t.Fatal("channels 0x3F and 0x6C are not slots 63 and 64")
	}
	if cd.ServerObjectForChannel(0x42) != nil || cd.ServerObjectForChannel(0xA0) != nil {
		t.Fatal("the stat channel, or slot 116's channel with 100 in use, found an object")
	}

	kept := cd.GetServerObject(80)
	cd.SetEntitySlots(30)
	if cd.EntitySlots() != 30 || len(cd.GetServerObjects()) != 30 || cd.GetServerObject(80) != kept {
		t.Fatalf("asked 30: %d in use, %d listed, slot 80 kept %v", cd.EntitySlots(), len(cd.GetServerObjects()), cd.GetServerObject(80) == kept)
	}
	cd.SetEntitySlots(1 << 20)
	if cd.EntitySlots() != MaxEntitySlots || cd.ServerObjectForChannel(0xF7) == nil {
		t.Fatalf("asked too many: %d in use, want %d with channel 0xF7", cd.EntitySlots(), MaxEntitySlots)
	}
	cd.SetEntitySlots(3)
	if cd.EntitySlots() != RetailEntitySlots {
		t.Fatalf("asked 3: %d in use, want 24", cd.EntitySlots())
	}
}

// A connection looks for its entities within 300 until asked for farther, no
// nearer than that and no farther than 1000.
func TestSetViewDistance(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	for _, c := range []struct{ ask, want int }{{0, 300}, {600, 600}, {100, 300}, {1000, 1000}, {5000, 1000}} {
		cd.SetViewDistance(c.ask)
		if got := cd.ViewDistance(); got != c.want {
			t.Errorf("asked %d: %d, want %d", c.ask, got, c.want)
		}
	}
	if got := ClampViewDistance(800, 500); got != 500 {
		t.Errorf("ClampViewDistance(800, 500) = %d", got)
	}
	if got := ClampViewDistance(800, 100); got != ViewDistance {
		t.Errorf("ClampViewDistance(800, 100) = %d, want %d", got, ViewDistance)
	}
}
