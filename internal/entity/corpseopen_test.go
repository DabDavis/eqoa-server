package entity

import (
	"testing"
	"time"
)

// The owner, 2026-11-14: "corpse if not looted for 5 mins should be open to all if
// not a raid". A group of three's kill (no raid) carries OpenAt five minutes after
// the death and its corpse lies at least a minute past it, though the world's decay
// is three; a raid's kill (six) carries none and lies its decay alone; a pet's corpse
// (Owner) none either; and the rule off (corpse_open_after below 0) gives none.
func TestANonRaidCorpseOpensToAllAfterFiveMinutes(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	kill := func(id EntityID, owner EntityID) (*NPC, KillCredit) {
		npc := r.fight(id, 0)
		r.w.mu.Lock()
		npc.Owner = owner
		npc.Threats, npc.CurrentEnemy = nil, 0
		r.w.killNPC(npc)
		r.w.creditKillLocked(r.sess[0], r.p[0], npc)
		r.w.mu.Unlock()
		return npc, heard[len(heard)-1]
	}
	decay, _ := r.w.CorpseTimes()

	npc, k := kill(9700, 0) // solo
	if !k.OpenAt.Equal(npc.DeathTime.Add(5 * time.Minute)) {
		t.Fatalf("a solo kill opens to all at %v after its death, want 5m", k.OpenAt.Sub(npc.DeathTime))
	}
	if got := npc.corpseEnds(decay).Sub(npc.DeathTime); got != 6*time.Minute {
		t.Fatalf("a solo kill's corpse lies %v, want 6m", got)
	}

	r.form(3)
	if g := r.raidOf(0); g == nil || raidingLocked(g) {
		t.Fatal("not a plain group of three")
	}
	npc, k = kill(9701, 0)
	if !k.OpenAt.Equal(npc.DeathTime.Add(5*time.Minute)) || npc.corpseEnds(decay).Sub(npc.DeathTime) != 6*time.Minute {
		t.Fatalf("a group's kill: open at %v, lies %v", k.OpenAt.Sub(npc.DeathTime), npc.corpseEnds(decay).Sub(npc.DeathTime))
	}
	_, k = kill(9702, 1) // a pet's corpse: no one's to loot
	if !k.OpenAt.IsZero() {
		t.Fatal("a pet's corpse opens to all")
	}

	for i := 3; i < 6; i++ {
		r.join(0, i)
	}
	if g := r.raidOf(0); g == nil || !raidingLocked(g) {
		t.Fatal("not a raid of six")
	}
	npc, k = kill(9703, 0)
	if !k.OpenAt.IsZero() {
		t.Fatal("a raid's kill opens to all")
	}
	if got := npc.corpseEnds(decay).Sub(npc.DeathTime); got != decay {
		t.Fatalf("a raid's kill's corpse lies %v, want its decay %v", got, decay)
	}

	r.w.SetCorpseOpenAfter(-1)
	r.w.mu.Lock()
	r.w.leaveGroupLocked(r.p[0])
	r.w.mu.Unlock()
	if r.p[0].GroupID != 0 {
		t.Fatal("still in the raid")
	}
	npc, k = kill(9704, 0)
	if !k.OpenAt.IsZero() || npc.corpseEnds(decay).Sub(npc.DeathTime) != decay {
		t.Fatal("the rule off, a solo kill still opens to all")
	}
}
