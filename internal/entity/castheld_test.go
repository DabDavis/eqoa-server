package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A casting creature stands still and keeps its melee going (the GM
// Jibrish's petition 1593: "Mobs should stop moving while casting (but the
// spell always resolves unless otherwise specified); however they should
// also keep quading IF in melee range of the target"). The rig's wizard
// template chain-casts Fire Storm at 2 s (2 s cast, held to 4.4 s) and
// Burning Gaze at 4.4 s (held to 6.8 s) at a player in its reach: its melee
// hits land all through both casts, and no swing of a round is sent while a
// cast holds it (the cast's 0x00B1 is what CLIENT draws), some before and
// after them.
func TestACastingCreatureKeepsItsMeleeInReach(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	mage := r.npc("Gnoll Mage")
	p, sess := r.player("Hero", 1001.5, 100)
	r.fight(mage, p.ServerID)
	var spells, swingsHeld, swingsFree int
	var at []time.Duration // its melee hits
	for d := time.Duration(0); d <= 9*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		msgs := drain(sess)
		held := mage.castHeld(r.at(d))
		for _, c := range castsIn(msgs) {
			if c.caster != r.objectID(mage.ServerID) || c.fx&0xFFFFFF00 == 0x0A0A0A00 || c.fx == templateSpecialFX {
				continue // a spell's, or a special's: not a round's swing
			}
			if held {
				swingsHeld++
			} else {
				swingsFree++
			}
		}
		lines := 0
		for _, l := range linesOf(msgs) {
			if strings.HasPrefix(l, "Gnoll Mage's ") {
				lines++
			}
		}
		spells += lines
		hits := 0
		for _, f := range damageIndications(msgs) {
			if f[0] == int64(r.objectID(mage.ServerID)) && f[2] == int64(sess.PlayerObjectID()) && f[1] < 0 {
				hits++
			}
		}
		hits -= lines // a spell's landing is told in a line beside its figure
		for range hits {
			at = append(at, d)
		}
	}
	if spells != 2 {
		t.Errorf("%d spells landed, want Fire Storm and Burning Gaze", spells)
	}
	// its rounds of 3.2 s, a hit 0.5 s in, on their cadence through the casts
	// (before, its round at 3.2 s was lost and the next began at 6.8 s: hits
	// at 0.5 and 7.3 s alone)
	if want := []time.Duration{500 * time.Millisecond, 3700 * time.Millisecond, 6900 * time.Millisecond}; fmt.Sprint(at) != fmt.Sprint(want) {
		t.Errorf("melee hits at %v, want %v: its rounds on through the casts", at, want)
	}
	if swingsHeld != 0 || swingsFree == 0 {
		t.Errorf("round swings sent: %d while a cast held it, %d otherwise; want none and some", swingsHeld, swingsFree)
	}
}

// Its cast holds it where it stands, its enemy walking out of its reach
// meanwhile: no step until the cast is over, the spell lands on the enemy
// out of reach all the same, and no melee hit lands from out of reach; then
// it runs after its enemy again.
func TestACastingCreatureStandsStill(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	mage := r.npc("Gnoll Mage")
	p, sess := r.player("Hero", 1001.5, 100)
	r.fight(mage, p.ServerID)
	var landed []string
	var x0, z0 float32
	for d := time.Duration(0); d <= 7*time.Second; d += 100 * time.Millisecond {
		if d == 2100*time.Millisecond {
			// the storm's cast on its way: the hero walks off out of its reach
			r.w.mu.Lock()
			p.X = 1009
			r.w.SpatialIndex.Update(p.ServerID, 1001.5, 100, p.X, p.Z)
			r.w.mu.Unlock()
			x0, z0 = mage.X, mage.Z
			drain(sess)
		}
		wasHeld, bx, bz := mage.castHeld(r.at(d)), mage.X, mage.Z // held as this tick begins
		r.tick(d)
		msgs := drain(sess)
		if d < 2100*time.Millisecond {
			continue
		}
		landed = append(landed, linesOf(msgs)...)
		if wasHeld && (mage.X != bx || mage.Z != bz) {
			t.Fatalf("at %v, its cast on its way, it moved from (%.2f, %.2f) to (%.2f, %.2f)", d, bx, bz, mage.X, mage.Z)
		}
		if d < 4400*time.Millisecond && (mage.X != x0 || mage.Z != z0) {
			t.Fatalf("at %v, the storm's cast on its way, it moved from (%.2f, %.2f)", d, x0, z0)
		}
		if d < 4400*time.Millisecond {
			for _, f := range damageIndications(msgs) {
				if f[0] == int64(r.objectID(mage.ServerID)) && f[1] < 0 && len(linesOf(msgs)) == 0 {
					t.Errorf("at %v a melee hit from out of its reach", d)
				}
			}
		}
	}
	if len(landed) == 0 || !strings.HasPrefix(landed[0], "Gnoll Mage's Fire Storm hits you") {
		t.Errorf("lines %q: want the storm landed on the hero out of its reach", landed)
	}
	if dist := distXZ(mage.X, mage.Z, x0, z0); dist < 1 {
		t.Errorf("after its cast it stood %.2f from where it cast: want it after its enemy again", dist)
	}
}
