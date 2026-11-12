package entity

import (
	"testing"
	"time"
)

// Three cleric mercs of one owner (the GM Jibrish's, 2026-11-12) put each
// buff on him once: the first merc's cast on its way stands for the others
// (mercBuffComingLocked), a group buff's for the whole team. Until then all
// three began Order on him within a second, and Group Resist Affliction
// landed on him three times over.
func TestThreeMercsBuffOnce(t *testing.T) {
	r := newMercRig(t)
	order := bookSpell(700805, "Order", 1, 5*time.Second, 0, 60, "Target, Increase AC by 650, Increase Maximum HP by 650, 30min")
	order.Level = mercOwnerLevel
	gra := bookSpell(700396, "Group Resist Affliction", 2, 3*time.Second, 0, 60, "Group, Increase PR by 45, Increase DR by 45, Radius 30, 30min")
	gra.Level = mercOwnerLevel
	var mercs []*mercState
	for slot := 1; slot <= 3; slot++ {
		k := r.kit(9, 3, 0, order, gra)
		k.Name = []string{"", "Jorador", "Haldor", "Arlin"}[slot]
		if _, why, _ := r.w.HireMercAt(r.sess, slot, k, MercAssist, r.at(0)); why != "" {
			t.Fatalf("slot %d: %s", slot, why)
		}
		r.w.mu.Lock()
		_, up := r.w.invites[r.p.ServerID][r.p.ServerID]
		r.w.mu.Unlock()
		if up {
			r.w.GroupAccept(r.sess, r.sess.PlayerObjectID())
		}
	}
	r.w.mu.Lock()
	mercs = r.w.mercsOfLocked(r.p)
	r.w.mu.Unlock()
	if len(mercs) != 3 {
		t.Fatalf("%d mercs out", len(mercs))
	}
	type key struct {
		ms    *mercState
		spell int
		at    EntityID
	}
	seen := map[*mercCast]bool{}
	casts := map[key]int{}
	for d := time.Duration(0); d <= 40*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		r.w.mu.Lock()
		for _, c := range r.w.mercCasts {
			if !seen[c] {
				seen[c] = true
				casts[key{c.ms, c.spell.SpellID, c.target}]++
			}
		}
		r.w.mu.Unlock()
	}
	onOwner, groups := 0, 0
	for k, n := range casts {
		if k.spell == order.SpellID && k.at == r.p.ServerID {
			onOwner += n
		}
		if k.spell == gra.SpellID {
			groups += n
		}
	}
	if onOwner != 1 || groups != 1 {
		t.Errorf("Order on the owner %d times, Group Resist Affliction %d times: want once each (%v)", onOwner, groups, casts)
	}
	n := 0
	for _, e := range r.p.Effects {
		if e.SpellID == order.SpellID || e.SpellID == gra.SpellID {
			n++
		}
	}
	if n != 2 {
		t.Errorf("the owner's effects: %d of the two", n)
	}
}
