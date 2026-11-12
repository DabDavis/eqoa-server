package entity

import (
	"testing"
	"time"
)

// Two wizard mercs of one owner turned on one crab on the same tick begin
// their first hit each its own moment, mercReactLeast to mercReactMost after,
// and their lockouts are lagged apart (mercoffbeat.go). The control: off,
// the rig's casts are on the tick.
func TestMercsOffbeat(t *testing.T) {
	r := newMercRig(t)
	mercOffbeat = true
	blaze := bookSpell(700768, "Blaze", 1, 2*time.Second, 3*time.Second, 50, "Target, Decrease Hitpoints by 50")
	var mercs []*mercState
	for slot := 1; slot <= 2; slot++ {
		k := r.kit(13, 0, 0, blaze)
		k.Name = []string{"", "Alor", "Orius"}[slot]
		if _, why, _ := r.w.HireMercAt(r.sess, slot, k, MercAssist, r.at(0)); why != "" {
			t.Fatal(why)
		}
	}
	r.w.mu.Lock()
	mercs = r.w.mercsOfLocked(r.p)
	r.w.mu.Unlock()
	crab := r.creature("Crab", 28, 10)
	crab.HP, crab.MaxHP = 900000, 900000
	firsts := map[*mercState]time.Duration{}
	for d := time.Duration(0); d <= 3*time.Second; d += 50 * time.Millisecond {
		r.w.mu.Lock()
		for _, ms := range mercs {
			if _, done := firsts[ms]; done {
				continue
			}
			if r.w.mercAttackTurnLocked(ms, crab, r.at(d)) {
				firsts[ms] = d
				if lag := ms.ready[blaze.SpellID].Sub(r.at(d)) - (blaze.CastTime + blaze.Recast + castLockoutExtra); lag < 0 || lag >= mercRecastLagMost {
					t.Errorf("%s's lockout lag %v", ms.npc.Name, lag)
				}
			}
		}
		r.w.mu.Unlock()
	}
	if len(firsts) != 2 {
		t.Fatalf("first hits %v", firsts)
	}
	for ms, d := range firsts {
		if d < mercReactLeast || d > mercReactMost+50*time.Millisecond {
			t.Errorf("%s began at %v", ms.npc.Name, d)
		}
	}
	mercOffbeat = false
	r.w.mu.Lock()
	ms := mercs[0]
	ms.ready, ms.reactTo = map[int]time.Time{}, 0
	ok := r.w.mercAttackTurnLocked(ms, crab, r.at(time.Minute))
	r.w.mu.Unlock()
	if !ok {
		t.Error("off: no hit at once")
	}
}
