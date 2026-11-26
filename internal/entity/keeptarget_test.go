package entity

import (
	"math/rand"
	"net"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// crowdFixture is slotsFixture's viewer with a crowd of 40 NPCs within 41
// units (more than a PS2's 23 slots) and one more, far, at 60 units: far
// is found by its id 20000.
func crowdFixture(t *testing.T) (*World, *session.Session, *Player, *NPC) {
	t.Helper()
	w, sess := slotsFixture(t, 40)
	far := &NPC{ServerID: 20000, Name: "Bertoxxulous", X: 1060, Z: 1000, ModelID: 0x70D898C6, Level: 66, HP: 10, MaxHP: 10}
	w.NPCs[far.ServerID] = far
	w.SpatialIndex.Insert(far.ServerID, far.X, far.Z)
	return w, sess, w.Players[1], far
}

// keptSlot is the slot and object id e has in sess's view (-1 none).
func keptSlot(sess *session.Session, id EntityID) (int, uint32) {
	vs := GetOrCreateVisibility(sess)
	for i, e := range vs.ChannelEntity {
		if e == id {
			return i, vs.ChannelObjectID[i]
		}
	}
	return -1, 0
}

// stepNPC moves n and its place in the spatial index.
func stepNPC(w *World, n *NPC, x, z float32) {
	w.SpatialIndex.Update(n.ServerID, n.X, n.Z, x, z)
	n.X, n.Z = x, z
}

// Petition 3156: the target stepped back behind a crowd and left the view,
// and the client its target and con with it. Targeted while near, it keeps
// its slot and object id as it walks off behind 40 nearer entities; an
// untargeted one in its place does not.
func TestTargetKeepsItsSlotBehindACrowd(t *testing.T) {
	for _, targeted := range []bool{true, false} {
		w, sess, p, far := crowdFixture(t)
		stepNPC(w, far, 1000, 1000.5) // beside the viewer first
		visible(w, sess)
		slot, oid := keptSlot(sess, far.ServerID)
		if slot < 0 {
			t.Fatal("the creature beside the viewer is not in view")
		}
		if targeted {
			p.TargetObjectID = oid
		}
		for step := 1; step <= 12; step++ { // walks off to 60 units
			stepNPC(w, far, 1000+float32(step*5), 1000)
			visible(w, sess)
		}
		s2, o2 := keptSlot(sess, far.ServerID)
		if targeted && (s2 != slot || o2 != oid) {
			t.Fatalf("the target at 60 units behind 40 nearer: slot %d object %d, want %d %d", s2, o2, slot, oid)
		}
		if !targeted && s2 >= 0 {
			t.Fatalf("an untargeted creature at 60 units kept a slot (%d) ahead of 40 nearer", s2)
		}
		if targeted && w.entitySeenByLocked(sess, p.TargetObjectID) != far {
			t.Fatal("the target's object id no longer names it")
		}
	}
}

// A creature hitting the player, or holding its hate, keeps its slot in a
// crowd, though nobody targets it; past the view distance it still goes.
func TestCreatureFightingThePlayerKeepsItsSlot(t *testing.T) {
	for _, how := range []string{"enemy", "hate"} {
		w, sess, p, far := crowdFixture(t)
		if how == "enemy" {
			far.CurrentEnemy = p.ServerID
		} else {
			far.Threats = map[EntityID]int64{p.ServerID: 50}
		}
		visible(w, sess)
		if s, _ := keptSlot(sess, far.ServerID); s < 0 {
			t.Fatalf("%s: the creature fighting the player at 60 units is out of view behind the crowd", how)
		}
		for i := 0; i < 23; i++ { // and the nearest of the crowd fill the rest
			if s, _ := keptSlot(sess, EntityID(10000+i)); i < 22 && s < 0 {
				t.Fatalf("%s: crowd NPC %d (at %d units) out of view", how, i, i+1)
			}
		}
		stepNPC(w, far, 1000+float32(VisibilityRadius)+50, 1000)
		visible(w, sess)
		if s, _ := keptSlot(sess, far.ServerID); s >= 0 {
			t.Fatalf("%s: kept past the view distance", how)
		}
	}
}

// The group's main assist's target is kept too: the assist stands by the
// creature, targets it in its own session, and the viewer, 60 units off
// behind the crowd, keeps it in view.
func TestMainAssistsTargetKept(t *testing.T) {
	w, sess, p, far := crowdFixture(t)
	s2 := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7010}, 10, 10, 1, 2, true)
	s2.SetCharacterInWorld(true)
	ma := &Player{ServerID: 2, Name: "Assist", X: 1059, Z: 1000, GroupID: 7}
	w.Players[ma.ServerID] = ma
	w.SpatialIndex.Insert(ma.ServerID, ma.X, ma.Z)
	s2.SetECSEntity(ma)
	visible(w, sess)
	if s, _ := keptSlot(sess, far.ServerID); s >= 0 {
		t.Fatal("kept with no fight")
	}
	_, oid := keptSlot(s2, far.ServerID)
	if oid == 0 {
		t.Fatal("the assist does not see the creature beside it")
	}
	ma.TargetObjectID = oid
	p.GroupID = 7
	if w.groups == nil {
		w.groups = map[uint32]*Group{}
	}
	w.groups[7] = &Group{ID: 7, Members: []EntityID{1, 2}, rules: raidRules{assist: 2}}
	visible(w, sess)
	if s, _ := keptSlot(sess, far.ServerID); s < 0 {
		t.Fatal("the main assist's target is out of the viewer's view")
	}
}

// Slot churn is bounded: a crowd shuffling about the viewer every pass moves
// the kept target never, and a crowd standing still moves nothing at all.
func TestKeptTargetSlotChurn(t *testing.T) {
	w, sess, p, far := crowdFixture(t)
	far.CurrentEnemy = p.ServerID
	visible(w, sess)
	slot, oid := keptSlot(sess, far.ServerID)
	if slot < 0 {
		t.Fatal("not in view")
	}
	// standing still: the same slots, pass after pass
	before := append([]EntityID(nil), GetOrCreateVisibility(sess).ChannelEntity...)
	for k := 0; k < 5; k++ {
		visible(w, sess)
	}
	for i, id := range GetOrCreateVisibility(sess).ChannelEntity {
		if id != before[i] {
			t.Fatalf("a still crowd: slot %d went from %d to %d", i, before[i], id)
		}
	}
	// shuffling: the target never moves
	rnd := rand.New(rand.NewSource(3156))
	for pass := 0; pass < 50; pass++ {
		for k := 0; k < 40; k++ {
			stepNPC(w, w.NPCs[EntityID(10000+k)], 1000+rnd.Float32()*40, 1000+rnd.Float32()*40)
		}
		visible(w, sess)
		if s, o := keptSlot(sess, far.ServerID); s != slot || o != oid {
			t.Fatalf("pass %d: the target went to slot %d object %d from %d %d", pass, s, o, slot, oid)
		}
	}
}
