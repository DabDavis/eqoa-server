package entity

import (
	"testing"
	"time"
)

// The GM Jibrish's Live petition 5035 (the owner: "use jibrish rule"): every kill's corpse is
// its owners' for six minutes, then anyone's: a solo kill, a group of three's, and a raid's (six)
// all carry OpenAt six minutes after the death; a pet's corpse (Owner) none; the rule off
// (corpse_open_after below 0) none.
func TestEveryCorpseOpensToAllAfterSixMinutes(t *testing.T) {
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
	six := func(what string, npc *NPC, k KillCredit) {
		t.Helper()
		if !k.OpenAt.Equal(npc.DeathTime.Add(6 * time.Minute)) {
			t.Errorf("%s opens to all %v after its death, want 6m", what, k.OpenAt.Sub(npc.DeathTime))
		}
	}
	npc, k := kill(9700, 0)
	six("a solo kill", npc, k)
	r.form(3)
	npc, k = kill(9701, 0)
	six("a group's kill", npc, k)
	if _, k = kill(9702, 1); !k.OpenAt.IsZero() {
		t.Error("a pet's corpse opens to all")
	}
	for i := 3; i < 6; i++ {
		r.join(0, i)
	}
	if g := r.raidOf(0); g == nil || !raidingLocked(g) {
		t.Fatal("not a raid of six")
	}
	npc, k = kill(9703, 0)
	six("a raid's kill", npc, k)

	r.w.SetCorpseOpenAfter(-1)
	if _, k = kill(9704, 0); !k.OpenAt.IsZero() {
		t.Error("the rule off, a kill still opens to all")
	}
}
