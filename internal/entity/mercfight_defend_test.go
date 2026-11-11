package entity

import (
	"testing"
	"time"
)

// Two players in a group, each with a merc (the owner, 2026-11-11: "when 2
// players in a group have a merc, if one merc gets aggro, sometimes the other
// does not act to defend"): a creature on the other player's merc, the first
// merc (assist, not aggressive) and its pet take it on; so for one on the
// other player. The control: a passive merc stays out.
func TestMercsDefendTheirGroup(t *testing.T) {
	for _, stance := range []MercStance{MercAssist, MercPassive} {
		r := newMercRig(t)
		ms := mercWithPet(t, r, stance)
		r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
		bs, b := r.player("Bea", 0, 0)
		g := r.group(b)
		k := r.kit(0, 0, 0)
		k.Name = "Kel"
		if _, why := r.w.HireMerc(bs, k, MercAssist, r.at(10*time.Second)); why != "" {
			t.Fatalf("Bea's hire: %s", why)
		}
		r.w.GroupAccept(bs, bs.PlayerObjectID())
		other := r.w.mercs[b.ServerID]
		if other == nil || !groupHas(g, other.npc.ServerID) || !groupHas(g, ms.npc.ServerID) {
			t.Fatalf("the group %v: both mercs in it", g.Members)
		}
		for i, victim := range []EntityID{other.npc.ServerID, b.ServerID} {
			crab := r.creature("crab", 28, float32(6+4*i))
			crab.HP, crab.MaxHP = 1_000_000, 1_000_000
			r.w.mu.Lock()
			ms.target, ms.pet.target = 0, 0
			crab.CurrentEnemy = victim
			crab.Threats = map[EntityID]int64{victim: 10}
			r.w.mercChooseTargetLocked(ms)
			r.w.mercPetChooseTargetLocked(ms, ms.pet)
			got, pet := ms.target, ms.pet.target
			crab.CurrentEnemy, crab.Threats = 0, nil
			r.w.mu.Unlock()
			if stance == MercPassive {
				if got != 0 || pet != 0 {
					t.Errorf("a passive merc took on a crab on %d: merc %d pet %d", victim, got, pet)
				}
				continue
			}
			if got != crab.ServerID || pet != crab.ServerID {
				t.Errorf("a crab on %d: the merc's target %d, its pet's %d, want %d", victim, got, pet, crab.ServerID)
			}
		}
	}
}
