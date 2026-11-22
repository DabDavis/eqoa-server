package entity

import (
	"testing"
	"time"
)

// A cast at a creature just killed (its name still in the target box) says
// the live game's "You can't cast abilities on dead creatures." in red, not
// "No target." (board 759); the control: the same strike at it alive is cast.
func TestACastAtADeadCreature(t *testing.T) {
	c := newCaster(t)
	c.npc.Dead = true
	if c.w.StartCast(c.sess, strike(), c.npcO, c.at(0)) {
		t.Fatal("a strike was cast at a dead creature")
	}
	if red := redLines(c.sess); len(red) != 1 || red[0] != "You can't cast abilities on dead creatures." {
		t.Errorf("at the dead: %q", red)
	}
	c.sent()
	c.npc.Dead = false
	if !c.w.StartCast(c.sess, strike(), c.npcO, c.at(10*time.Second)) {
		t.Fatal("the control: the strike at the living creature was refused")
	}
	for _, l := range redLines(c.sess) {
		if l == castDeadTarget {
			t.Errorf("the control said %q", l)
		}
	}
}

// The live game's words for the other two refusals (board 759).
func TestTheLiveRefusalWords(t *testing.T) {
	if castNotUndead != "Invalid creature type for that ability." || castNotSeen != "No line of sight." {
		t.Errorf("castNotUndead %q, castNotSeen %q", castNotUndead, castNotSeen)
	}
}
