package entity

import (
	"fmt"
	"net"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// slotsFixture is a player in a session with npcs NPCs in a row beside them,
// NPC k at k+1 units, all within VisibilityRadius.
func slotsFixture(t *testing.T, npcs int) (*World, *session.Session) {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7009}, 9, 9, 1, 2, true)
	sess.SetCharacterInWorld(true)
	p := &Player{ServerID: 1, Name: "Viewer", X: 1000, Z: 1000}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	for k := 0; k < npcs; k++ {
		npc := &NPC{ServerID: EntityID(10000 + k), Name: fmt.Sprintf("Npc%d", k), X: 1001 + float32(k), Z: 1000, ModelID: 0x70D898C6, Level: 1, HP: 10, MaxHP: 10}
		w.NPCs[npc.ServerID] = npc
		w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	}
	return w, sess
}

// visible runs a visibility pass and returns the entity on each occupied
// state channel.
func visible(w *World, sess *session.Session) map[byte]EntityID {
	w.TickCount = 0 // a pass runs every VisibilityTickInterval ticks
	w.visibilitySystem()
	out := map[byte]EntityID{}
	for i, id := range GetOrCreateVisibility(sess).ChannelEntity {
		if id != 0 {
			out[session.EntityChannel(i+1)] = id
		}
	}
	return out
}

// CLIENT reads an object channel only below 24 (VIClient::ProcessStateChannel:
// `type < 24` goes to ChannelEntity and its m_entities[24]). Channel 0 is the
// player's own, so the entities in view of a client that never asked for more
// slots (every PS2) use 1-23 and no more; an entity on 24-28 was invisible to a
// PS2.
func TestEntityChannelsStayBelow24(t *testing.T) {
	w, sess := slotsFixture(t, 60)
	got := visible(w, sess)
	if len(got) != 23 {
		t.Fatalf("%d entities in view, want the 23 nearest", len(got))
	}
	for ch, id := range got {
		if ch < 1 || ch > 23 {
			t.Errorf("an entity on channel %#x; a PS2 reads 0x00-0x17, 0 its own", ch)
		}
		if id >= 10000+23 {
			t.Errorf("NPC %d in view, past the 23 nearest", id-10000)
		}
	}
}

// A client that asked for more slots has them filled, nearest first, on the
// slots' own channels; asking for fewer gives the rest up: their entities are
// marked removed on channels no longer sent, and the nearest come back on the
// slots kept.
func TestEntitySlotsAskedAndGivenUp(t *testing.T) {
	w, sess := slotsFixture(t, 150)
	conn := sess.Connection()

	conn.SetEntitySlots(128)
	got := visible(w, sess)
	if len(got) != 127 {
		t.Fatalf("asked 128: %d entities in view, want 127", len(got))
	}
	for ch, id := range got {
		if session.EntitySlot(ch) < 1 || id >= 10000+127 {
			t.Fatalf("asked 128: NPC %d on channel %#x", id-10000, ch)
		}
	}
	for _, ch := range []byte{0x18, 0x3F, 0x6C, 0xAB} {
		if got[ch] == 0 {
			t.Errorf("asked 128: nothing on channel %#x", ch)
		}
	}
	if ch := session.EntityChannel(127); ch != 0xAB {
		t.Fatalf("slot 127 is channel %#x, want 0xAB", ch)
	}

	conn.SetEntitySlots(24)
	got = visible(w, sess)
	if len(got) != 23 || len(GetOrCreateVisibility(sess).ChannelEntity) != 23 {
		t.Fatalf("asked 24 again: %d entities in view, want 23", len(got))
	}
	for ch, id := range got {
		if ch > 23 || id >= 10000+23 {
			t.Errorf("asked 24 again: NPC %d on channel %#x", id-10000, ch)
		}
	}
	for slot := 24; slot < 128; slot++ {
		if !conn.GetServerObject(slot).DescribesNoEntity() {
			t.Fatalf("slot %d given up still describes its entity", slot)
		}
	}

	// and back up: the slots are filled again
	conn.SetEntitySlots(64)
	if got = visible(w, sess); len(got) != 63 {
		t.Fatalf("asked 64: %d entities in view, want 63", len(got))
	}
}

// The visibility system looks for a session's entities as far as its client
// asked (session.ViewDistance, 300, for one that never did): NPCs at 350 to
// 800 units are none of a PS2's, those within 600 when it asked for 600.
func TestViewDistanceAsked(t *testing.T) {
	w, sess := slotsFixture(t, 0)
	for k, d := range []float32{350, 450, 550, 650, 800} {
		npc := &NPC{ServerID: EntityID(20000 + k), Name: fmt.Sprintf("Far%d", k), X: 1000 + d, Z: 1000, ModelID: 0x70D898C6, Level: 1, HP: 10, MaxHP: 10}
		w.NPCs[npc.ServerID] = npc
		w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	}
	conn := sess.Connection()
	if got := visible(w, sess); len(got) != 0 {
		t.Fatalf("within the default 300: %d entities in view, want none", len(got))
	}
	conn.SetViewDistance(600)
	if got := visible(w, sess); len(got) != 3 {
		t.Fatalf("asked 600: %d entities in view, want the 3 within it", len(got))
	}
	conn.SetViewDistance(1000)
	if got := visible(w, sess); len(got) != 5 {
		t.Fatalf("asked 1000: %d entities in view, want all 5", len(got))
	}
	conn.SetViewDistance(100)
	if got := visible(w, sess); len(got) != 0 || conn.ViewDistance() != 300 {
		t.Fatalf("asked 100: looked for to %d with %d in view, want 300 and none", conn.ViewDistance(), len(got))
	}
}
