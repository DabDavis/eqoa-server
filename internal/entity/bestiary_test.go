package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// bestiaryRig is the rez rig (Gnolie, Bran, Liss, Liss's pet, the crab) with the
// bestiary's hook taking its notes and a zone namer.
func bestiaryRig(t *testing.T) (*rezRig, *[]database.BestiaryNote) {
	t.Helper()
	r := newRezRig(t)
	var got []database.BestiaryNote
	r.w.OnBestiary(func(ns []database.BestiaryNote) { got = append(got, ns...) })
	r.w.SetBestiaryZoner(func(world uint8, x, z float32) string {
		if world == 0 && x < 2000 && z < 2000 {
			return "The Shore"
		}
		return ""
	})
	return r, &got
}

// A creature's hate on a player, or on a player's pet, is the player's meeting with
// it: written once a login for its kind and zone, however many hits follow; the kill
// counts for the killer. The controls: a player's pet, a mercenary, a training dummy
// and a creature no one may attack are no kinds, and nothing is queued without a hook.
func TestBestiaryNotesMeetingsAndKills(t *testing.T) {
	r, got := bestiaryRig(t)
	crab := r.w.NPCs[3002]
	crab.SpawnX, crab.SpawnZ = 98, 98
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.b.ServerID, 10)
	r.w.addThreatLocked(crab, r.b.ServerID, 10)
	r.w.addThreatLocked(crab, 3001, 10) // Liss's pet: Liss met it
	r.w.addThreatLocked(r.w.NPCs[3001], r.a.ServerID, 10)
	merc := &NPC{ServerID: 3009, Name: "Hired Blade", merc: &mercState{}}
	dummy := &NPC{ServerID: 3010, Name: "training dummy", Dummy: DummyKind(1)}
	quiet := &NPC{ServerID: 3011, Name: "a banker", NPCType: 0x80}
	for _, n := range []*NPC{merc, dummy, quiet} {
		r.w.NPCs[n.ServerID] = n
		r.w.addThreatLocked(n, r.a.ServerID, 10)
	}
	crab.Dead = true
	r.w.onKill = nil
	r.w.creditKillLocked(nil, r.b, crab)
	r.w.mu.Unlock()
	r.w.runBestiary()
	want := []database.BestiaryNote{
		{ServerID: 72, Kind: "Desert Crab", Zone: "The Shore"},
		{ServerID: 73, Kind: "Desert Crab", Zone: "The Shore"},
		{ServerID: 72, Kind: "Desert Crab", Zone: "The Shore", Kill: true},
	}
	if len(*got) != len(want) {
		t.Fatalf("notes: %+v", *got)
	}
	for i, n := range *got {
		n.At = time.Time{}
		if n != want[i] {
			t.Errorf("note %d: %+v, want %+v", i, n, want[i])
		}
	}

	// another zone is another meeting; the namer's silence names the world
	*got = nil
	crab.Dead, crab.HP, crab.Threats = false, crab.MaxHP, nil
	forgetHate(crab)
	crab.SpawnX = 5000
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.b.ServerID, 10)
	r.w.mu.Unlock()
	r.w.FlushBestiary()
	if len(*got) != 1 || (*got)[0].Zone != "World 0" {
		t.Errorf("another zone: %+v", *got)
	}

	// no hook: nothing kept
	*got = nil
	r.w.OnBestiary(nil)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, r.c.ServerID, 10)
	r.w.mu.Unlock()
	r.w.runBestiary()
	if len(*got) != 0 || len(r.w.bestiary.queue) != 0 {
		t.Errorf("no hook: %+v, queue %d", *got, len(r.w.bestiary.queue))
	}
}

// A kill counts for every player it is credited to (a group's members within reach),
// and for the killer once.
func TestBestiaryKillCountsForTheGroup(t *testing.T) {
	r, got := bestiaryRig(t)
	r.w.mu.Lock()
	g := r.w.formGroupLocked(r.b)
	g.Members = append(g.Members, r.a.ServerID)
	r.a.GroupID = g.ID
	r.w.mu.Unlock()
	hcKill(t, r, r.b, r.b.ServerID, r.a.ServerID)
	r.w.runBestiary()
	kills := map[uint32]int{}
	for _, n := range *got {
		if n.Kill {
			kills[n.ServerID]++
		}
	}
	if kills[72] != 1 || kills[71] != 1 || len(kills) != 2 {
		t.Errorf("kills: %v (%+v)", kills, *got)
	}
}
