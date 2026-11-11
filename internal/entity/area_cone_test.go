package entity

import (
	"encoding/binary"
	"testing"
)

// The frontal cone (area.go; the owner, 2026-11-11: "lets make cleave hit up
// to 3 in a frontal cone"): a GM's row, centre 3, aimed from its caster at its
// own target.

// cleave is the paladin's Cleave (700534) as its spelleffect line reads, with
// the row tools/cleave_cone_20261111.py writes: a cone, radius 6, 3 at most,
// half-angle 60.
func cleave() CastSpell {
	s := spellOf(700534, 1, "Cleave", "Target, Decrease Hitpoints by 600 ; Increase 30% based on STR")
	s.Area = s.Area.With(AreaRule{Centre: AreaRuleCone, Radius: 6, Max: 3, Cone: 60})
	return s
}

func TestAConesGeometry(t *testing.T) {
	a := SpellArea{Centre: AreaCone}
	// aimed east (+x) from (0, 0, 0) at a target at (3, 0)
	c := a.coneOf(0, 0, 0, 0, true, 3, 0)
	if c == nil || c.r != ConeRadius {
		t.Fatalf("the cone: %+v", c)
	}
	for _, p := range []struct {
		name    string
		x, z    float32
		extra   float32
		in      bool
		atLeast float32
	}{
		{"straight ahead, 4", 4, 0, 0, true, 4},
		{"30 degrees off, 5", 4.33, 2.5, 0, true, 5},
		{"59 degrees off, 3", 1.545, 2.571, 0, true, 3},
		{"61 degrees off, 3", 1.454, 2.624, 0, false, 3},
		{"at the side (90 degrees), 2", 0, 2, 0, false, 2},
		{"behind, 2", -2, 0, 0, false, 2},
		{"straight ahead, 6.5", 6.5, 0, 0, false, 6.5},
		{"straight ahead, 6.5, a large creature (+3)", 6.5, 0, 3, true, 6.5},
		{"straight ahead, 12, a huge creature (+5)", 12, 0, 5, false, 12},
		{"on the apex", 0, 0, 0, true, 0},
	} {
		in, d := c.holds(p.x, 0, p.z, p.extra)
		if in != p.in || d < p.atLeast-0.01 || d > p.atLeast+0.01 {
			t.Errorf("%s: in %v at %.2f, want %v at %.2f", p.name, in, d, p.in, p.atLeast)
		}
	}
	// no target: along the caster's facing (CLIENT's heading: x along sin, z along cos)
	c = a.coneOf(0, 0, 0, 0, false, 0, 0)
	if in, _ := c.holds(0, 0, 3, 0); !in {
		t.Error("facing 0 (+z): a creature at +z 3 is not in it")
	}
	if in, _ := c.holds(3, 0, 0, 0); in {
		t.Error("facing 0 (+z): a creature at +x 3 is in it")
	}
	// a GM's half-angle, and its radius
	w := SpellArea{Centre: AreaCone}.With(AreaRule{Centre: AreaRuleCone, Radius: 10, Cone: 20})
	c = w.coneOf(0, 0, 0, 0, true, 1, 0)
	if in, _ := c.holds(4.33, 0, 2.5, 0); in {
		t.Error("a 20-degree cone holds one 30 degrees off")
	}
	if in, _ := c.holds(9, 0, 1, 0); !in {
		t.Error("a 20-degree cone of 10 does not hold one 6 degrees off at 9")
	}
	// not a cone: none
	if (&SpellArea{Centre: AreaOnCaster}).coneOf(0, 0, 0, 0, true, 1, 0) != nil {
		t.Error("an area round its caster has a cone")
	}
}

// Cleave through the cast: its target and the two nearest in front of the
// caster take it; a third in front (the cap), one behind, one at the side and
// one in front past its reach do not; the 0x00B1 names the two.
func TestCleaveHitsThreeInAFrontalCone(t *testing.T) {
	r := newAreaRig(t)                                       // the caster at (1000, 10, 1000)
	target := r.creature(2001, "Deathfist Pawn", 1003, 1000) // east, 3: the aim
	nearIn := r.creature(2002, "Deathfist Pawn", 1002, 1001) // 2.2, 27 degrees off
	farIn := r.creature(2003, "Deathfist Pawn", 1004, 1002)  // 4.5, 27 degrees off
	capped := r.creature(2004, "Deathfist Pawn", 1005, 1000.5)
	behind := r.creature(2005, "Deathfist Pawn", 997, 1000)
	side := r.creature(2006, "Deathfist Pawn", 1000, 1003)
	past := r.creature(2007, "Deathfist Pawn", 1009, 1000)
	b1 := r.cast(t, cleave(), target.ServerID)
	got := hits(target, nearIn, farIn, capped, behind, side, past)
	if got[0] <= 0 || got[1] != got[0] || got[2] != got[0] {
		t.Errorf("the target and the two nearest in front lost %v, want each the target's", got[:3])
	}
	for k, name := range []string{"the third in front (the cap)", "behind", "at the side", "past its reach"} {
		if got[3+k] != 0 {
			t.Errorf("%s lost %d", name, got[3+k])
		}
	}
	if len(b1) < 11 || b1[10] != 2 {
		t.Fatalf("the 0x00B1 % X: want two others", b1)
	}
	for k, n := range []*NPC{nearIn, farIn} {
		if got, want := binary.LittleEndian.Uint32(b1[11+4*k:]), r.objectID(n.ServerID); got != want {
			t.Errorf("the 0x00B1's other %d is %#x, want %#x", k, got, want)
		}
	}
}

// Its own target is hit wherever it stands (at the side here: the cone is
// aimed at it), and the cone turns with it: what stood in front of the caster
// is now behind the aim.
func TestCleavesConeIsAimedAtItsTarget(t *testing.T) {
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1000, 1004)   // +z, 4
	besideIt := r.creature(2002, "Deathfist Pawn", 1001, 1003) // 18 degrees off the aim
	east := r.creature(2003, "Deathfist Pawn", 1003, 1000)     // 90 degrees off it
	r.p.Facing = 1.5708                                        // facing east: the aim wins
	r.cast(t, cleave(), target.ServerID)
	if got := hits(target, besideIt, east); got[0] <= 0 || got[1] != got[0] || got[2] != 0 {
		t.Errorf("aimed at +z: lost %v, want the target, the one beside it, not the one east", got)
	}
}

// The control: Cleave without its row is a strike of one.
func TestCleaveWithoutItsRowIsOneTarget(t *testing.T) {
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1003, 1000)
	other := r.creature(2002, "Deathfist Pawn", 1002, 1001)
	s := spellOf(700534, 1, "Cleave", "Target, Decrease Hitpoints by 600 ; Increase 30% based on STR")
	if s.Area.Centre != AreaNone {
		t.Fatalf("Cleave's line read as an area: %+v", s.Area)
	}
	r.cast(t, s, target.ServerID)
	if got := hits(target, other); got[0] <= 0 || got[1] != 0 {
		t.Errorf("lost %v", got)
	}
}
