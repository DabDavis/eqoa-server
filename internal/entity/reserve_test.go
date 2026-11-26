package entity

import (
	"testing"
	"time"
)

// A creature kept for its waker's party (reserve.go): an outsider is barred (told the line),
// puts no hate on it and is not noticed on sight; the party's player hurts it and is; past
// its end it stays kept while it fights the party, and lifts once that fight is over.
func TestReserveBarsOutsidersHoldsThroughItsFight(t *testing.T) {
	r := newProcRig(t)
	party, out, npc := r.p, r.friend, r.npc
	party.CharServerID, out.CharServerID = 41, 42
	npc.EventAggro = true
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	if !r.w.ReserveNPC(npc.ServerID, WakerParty{Name: "a party", Chars: []uint32{41}}, now.Add(15*time.Minute)) {
		t.Fatal("not reserved")
	}
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if !challengeBars(npc, out.ServerID) || challengeRefusal(npc, out.ServerID) != npc.Name+" is bound to another party's challenge." {
		t.Fatalf("the outsider: barred %v, told %q", challengeBars(npc, out.ServerID), challengeRefusal(npc, out.ServerID))
	}
	if challengeBars(npc, party.ServerID) || challengeRefusal(npc, party.ServerID) != "" {
		t.Fatal("the party's own player barred")
	}
	if r.w.threatensLocked(npc, out) || !r.w.threatensLocked(npc, party) {
		t.Fatalf("on sight: the outsider %v, the party %v", r.w.threatensLocked(npc, out), r.w.threatensLocked(npc, party))
	}
	r.w.addThreatLocked(npc, out.ServerID, 50)
	if npc.Threats[out.ServerID] != 0 {
		t.Fatal("the outsider is on its hate list")
	}
	r.w.addThreatLocked(npc, party.ServerID, 50)
	if npc.Threats[party.ServerID] == 0 {
		t.Fatal("the party's player is not on its hate list")
	}
	// past its end, fighting the party: kept
	r.w.reserveSystem(now.Add(16 * time.Minute))
	if npc.reserve == nil {
		t.Fatal("lifted in the middle of the party's fight")
	}
	// the fight over: lifted
	npc.Threats, npc.CurrentEnemy = nil, 0
	r.w.reserveSystem(now.Add(17 * time.Minute))
	if npc.reserve != nil || challengeBars(npc, out.ServerID) {
		t.Fatal("still kept past its end with no fight")
	}
}

// A keep restart carries the reservation: written with the creature, put back on its copy.
func TestReserveKeptOverARestart(t *testing.T) {
	r := newProcRig(t)
	r.p.CharServerID = 41
	until := time.Date(2026, 11, 26, 12, 15, 0, 0, time.UTC)
	r.w.ReserveNPC(r.npc.ServerID, WakerParty{Name: "a party", Chars: []uint32{41, 43}}, until)
	r.w.mu.Lock()
	k := keptReserveLocked(r.npc)
	r.npc.reserve = nil
	delete(r.w.reserved, r.npc.ServerID)
	r.w.restoreReserveLocked(r.npc, k)
	r.w.mu.Unlock()
	r.w.DelayReserve(r.npc.ServerID, 30*time.Second)
	name, chars := r.w.ReservedFor(r.npc.ServerID)
	if name != "a party" || len(chars) != 2 || !r.npc.reserve.until.Equal(until.Add(30*time.Second)) {
		t.Fatalf("restored %q %v until %v", name, chars, r.npc.reserve.until)
	}
}
