package session_test

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// ack walks a channel through one full send/acknowledge cycle so that its
// baseXOR holds `data` and its baseMessageCounter is non-zero — the state every
// channel is in once the client has been in world for a moment, and the state
// the world-change bug was invisible in.
func ack(t *testing.T, st *session.StateUpdateData, data []byte) {
	t.Helper()
	st.SetData(data)
	_, seq, _ := st.GenerateXORDelta()
	if seq == 0 {
		t.Fatal("channel produced no update for fresh data")
	}
	st.UpdateBaseXor(seq)
}

func filled(n int, b byte) []byte {
	d := make([]byte, n)
	for i := range d {
		d[i] = b
	}
	return d
}

// TestResendAllReDescribesActiveChannels is the world-change fix.
//
// After the client has acknowledged a channel, an unchanged channel is silent —
// correct while the client keeps its scene, and the whole bug the moment a world
// change makes it throw that scene away. ResendAll has to make every armed
// channel speak again.
func TestResendAllReDescribesActiveChannels(t *testing.T) {
	cd := session.NewConnectionData(nil, 0, 0)

	ch0 := cd.GetServerObject(0)
	ch1 := cd.GetServerObject(1)
	ack(t, ch0, filled(162, 0xAA))
	ack(t, ch1, filled(162, 0xBB))

	// Baseline: in sync, so nothing is sent. If this ever stops being true the
	// test below proves nothing.
	if data, _, _ := ch0.GenerateXORDelta(); data != nil {
		t.Fatal("an acknowledged, unchanged channel sent an update; " +
			"the silence this test is about is gone")
	}

	if n := cd.ResendAll(); n != 2 {
		t.Fatalf("ResendAll re-armed %d channels, want 2 (the two that are active)", n)
	}

	for _, c := range []struct {
		name string
		st   *session.StateUpdateData
	}{{"channel 0", ch0}, {"channel 1", ch1}} {
		if data, _, _ := c.st.GenerateXORDelta(); data == nil {
			t.Errorf("%s stayed silent after ResendAll — the client would still "+
				"have no entity for it", c.name)
		}
	}
}

// TestResendAllSkipsInactiveChannels: an unarmed channel has never been
// described, and its buffer is all zeros. Sending one would ask the client to
// build an entity out of nothing.
func TestResendAllSkipsInactiveChannels(t *testing.T) {
	cd := session.NewConnectionData(nil, 0, 0)
	ack(t, cd.GetServerObject(0), filled(162, 0xAA))

	if n := cd.ResendAll(); n != 1 {
		t.Fatalf("ResendAll re-armed %d channels, want 1 — only channel 0 is active", n)
	}
	if data, _, _ := cd.GetServerObject(5).GenerateXORDelta(); data != nil {
		t.Error("an inactive channel produced an update")
	}
}

// TestResendAllSkipsClearedChannels is the assert this cost us.
//
// A cleared visibility channel is 162 zero bytes and stays ACTIVE, because
// SetData arms whatever it is handed. Byte 4 of an object channel is the entity
// type, and the client's factory (VIEntity.cpp:476) accepts only 0x81 and 0x82 —
// type 0x00 leaves its pointer NULL and asserts. Re-offering the cleared
// channels at a world change put a live console into that assert loop on
// 2026-08-10; the guest's own malloc, run AT the assert, still returned 1 MB, so
// it was never memory.
func TestResendAllSkipsClearedChannels(t *testing.T) {
	cd := session.NewConnectionData(nil, 0, 0)

	ack(t, cd.GetServerObject(0), filled(162, 0xAA)) // the player: real data
	cleared := cd.GetServerObject(3)
	ack(t, cleared, filled(162, 0xBB)) // an NPC...
	cleared.SetData(make([]byte, 162)) // ...that a world change drops
	if !cleared.IsActive() {
		t.Fatal("a cleared channel went inactive by itself; this test assumes it " +
			"stays armed, which is what makes the skip necessary")
	}

	if n := cd.ResendAll(); n != 1 {
		t.Fatalf("ResendAll re-armed %d channels, want 1 — the cleared one "+
			"describes no entity and asserts the client", n)
	}
}

// TestResendAllSurvivesTheResendCap.
//
// A channel that went unacknowledged burns through MaxResendCount and then goes
// permanently mute. That is exactly the state a world change leaves the busy
// channels in, so a resend that respects the cap re-arms nothing where it is
// needed most.
func TestResendAllSurvivesTheResendCap(t *testing.T) {
	cd := session.NewConnectionData(nil, 0, 0)
	st := cd.GetServerObject(1)
	st.SetData(filled(162, 0xCC)) // never acknowledged

	// Drain the cap: first send, then ResendInterval visits per retry.
	st.GenerateXORDelta()
	for i := 0; i < session.MaxResendCount*session.ResendInterval*2; i++ {
		st.GenerateXORDelta()
	}
	if data, _, _ := st.GenerateXORDelta(); data != nil {
		t.Fatal("channel is still sending; the resend cap did not engage, " +
			"so this test is not testing what it claims")
	}

	cd.ResendAll()
	if data, _, _ := st.GenerateXORDelta(); data == nil {
		t.Error("ResendAll left a capped-out channel mute")
	}
}

// TestForceFullResendEmitsAKeyframe pins the correction to ForceFullResend.
//
// Zeroing baseXOR alone makes the server emit the whole state with a NON-ZERO
// refnum, which tells the client to XOR it against the message it still holds:
// it computes its_state XOR full_state and gets garbage. Thirty channels of
// that at a loading client is what froze the loading screen on 2026-08-06.
// A full resend must say refnum 0.
func TestForceFullResendEmitsAKeyframe(t *testing.T) {
	cd := session.NewConnectionData(nil, 0, 0)
	st := cd.GetServerObject(0)
	want := filled(162, 0xAA)
	ack(t, st, want)

	st.ForceFullResend()
	data, _, xorByte := st.GenerateXORDelta()
	if data == nil {
		t.Fatal("ForceFullResend sent nothing")
	}
	if xorByte != 0 {
		t.Errorf("refnum = %d, want 0: a non-zero refnum makes the client XOR "+
			"this full state against a base it still holds", xorByte)
	}
	for i := range want {
		if data[i] != want[i] {
			t.Fatalf("byte %d = %#x, want %#x — a keyframe carries the state, not a delta",
				i, data[i], want[i])
		}
	}
}

// TestStateChannelHold covers the load window: held on teleport, lifted by the
// client's 0x0049, and never held forever.
func TestStateChannelHold(t *testing.T) {
	s := session.NewSession(nil, 1, 1, 0, 0, true)

	if s.StateChannelsHeld() {
		t.Fatal("a fresh session is holding state channels")
	}

	s.HoldStateChannels(time.Minute)
	if !s.StateChannelsHeld() {
		t.Fatal("HoldStateChannels did not hold")
	}

	s.ReleaseStateChannels()
	if s.StateChannelsHeld() {
		t.Fatal("ReleaseStateChannels did not release — the client's 0x0049 " +
			"would not resume the world")
	}

	// The deadline is the escape hatch for a teleport the client services with
	// no reload, and so sends no 0x0049 at all.
	s.HoldStateChannels(-time.Second)
	if s.StateChannelsHeld() {
		t.Fatal("an expired hold is still holding; entities would freeze forever")
	}
}

// TestWorldLoadPendingSurvivesAnEarly0x0049.
//
// Our client sent a 0x0049 one second after a teleport — too fast to be a
// finished world load — and the real one six seconds later. Releasing the hold
// on the early one must NOT consume the pending world load, or the first
// position update from the destination will not re-describe the scene and the
// original bug is back.
func TestWorldLoadPendingSurvivesAnEarly0x0049(t *testing.T) {
	s := session.NewSession(nil, 1, 1, 0, 0, true)

	s.HoldStateChannels(time.Minute)
	s.ReleaseStateChannels() // the early, spurious 0x0049

	if !s.ConsumeWorldLoadPending() {
		t.Fatal("an early 0x0049 consumed the pending world load; the arrival " +
			"would not re-describe the scene")
	}
	if s.ConsumeWorldLoadPending() {
		t.Error("the pending world load fired twice — every later position " +
			"update would re-send all 31 channels")
	}
	if s.StateChannelsHeld() {
		t.Error("arrival did not lift the hold")
	}
}

// TestNoWorldLoadPendingWithoutATeleport: an ordinary position update from a
// player who has not teleported must re-arm nothing.
func TestNoWorldLoadPendingWithoutATeleport(t *testing.T) {
	s := session.NewSession(nil, 1, 1, 0, 0, true)
	if s.ConsumeWorldLoadPending() {
		t.Fatal("a session that never teleported reports a pending world load")
	}
}
