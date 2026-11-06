package entity

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// The workbook's Renew as the live record has it: a target by its record's scope and
// its line, a group's at an endless radius by the workbook.
const renewJSON = `{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"stack":0,"level":20,"min_level":1,"radius":100000,"req_equipment":"Any","sheet_type":"Ressurect Player","effects":[{"id":"021D","type":"Ressurect Player","kind":"resurrect","rez_pct":-20}]}`

func wbRenew() CastSpell {
	s := wbSpell(453, 20, 1, "Renew", renewJSON)
	s.Range = 0x7F7F
	return s
}

// Petition 1128: "rez does not work i think it my be self casting?". Cast at a
// friend, in the caster's group or not, the rez takes the friend's share off its debt
// and says so, and nothing else: until now it landed on its caster, "with nothing to
// recover", and the friend's debt stayed as it was.
func TestAWorkbookRezLandsOnWhomItIsCastAt(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		r := newRezRig(t)
		if grouped {
			r.w.groups = map[uint32]*Group{9: {ID: 9, Members: []EntityID{r.a.ServerID, r.b.ServerID}}}
			r.a.GroupID, r.b.GroupID = 9, 9
		}
		r.die(r.b, r.sb, 0)
		owed := r.b.Debt
		r.respawn(15 * time.Second)
		drain(r.sa)
		drain(r.sb)
		if !r.cast(r.sa, wbRenew(), rezB, 20*time.Second) {
			t.Fatalf("grouped %v: not begun", grouped)
		}
		want := owed - (owed*20+50)/100
		if r.b.Debt != want {
			t.Errorf("grouped %v: Bran owes %d, want %d (of %d)", grouped, r.b.Debt, want, owed)
		}
		for _, l := range linesOf(drain(r.sa)) {
			if strings.Contains(l, "no experience to recover") {
				t.Errorf("grouped %v: the caster told %q", grouped, l)
			}
		}
		if !hasLine(linesOf(drain(r.sb)), "Gnolie's Renew took "+strconv.Itoa(owed-want)+" off your experience debt ("+strconv.Itoa(want)+" left).") {
			t.Errorf("grouped %v: Bran not told", grouped)
		}
	}
}

// Cast at its caster, a group's rez reaches the group: every member with something to
// recover, dead or standing, and its caster's own; none of them told of the others'.
// A member with nothing to recover is no one's business, and with nobody to recover
// it fizzles as ever. The control: no group, its caster's own alone.
func TestAGroupsRezCastAtItsCasterReachesTheGroup(t *testing.T) {
	r := newRezRig(t)
	r.w.groups = map[uint32]*Group{9: {ID: 9, Members: []EntityID{r.a.ServerID, r.b.ServerID, r.c.ServerID}}}
	r.a.GroupID, r.b.GroupID, r.c.GroupID = 9, 9, 9
	r.die(r.b, r.sb, 0) // dead still at the landing
	owed := r.b.Debt
	drain(r.sa)
	if !r.cast(r.sa, wbRenew(), rezA, 2*time.Second) {
		t.Fatal("not begun, with Bran to recover")
	}
	if want := owed - (owed*20+50)/100; r.b.Debt != want {
		t.Errorf("Bran, dead, owes %d, want %d", r.b.Debt, want)
	}
	for _, l := range linesOf(drain(r.sa)) {
		if strings.Contains(l, "no experience to recover") {
			t.Errorf("the caster told %q", l)
		}
	}

	f := newRezRig(t)
	f.w.groups = map[uint32]*Group{9: {ID: 9, Members: []EntityID{f.a.ServerID, f.c.ServerID}}}
	f.a.GroupID, f.c.GroupID = 9, 9
	lockouts := len(f.a.Lockouts)
	began := f.w.StartCast(f.sa, wbRenew(), rezA, f.at(0))
	refusedNoRez(t, began, drain(f.sa), "Gnolie has no experience to recover.", f.a, f.a.Power, lockouts)

	g := newRezRig(t)
	g.b.Debt, g.c.Debt = 500, 500
	g.die(g.a, g.sa, 0)
	own := g.a.Debt
	g.respawn(15 * time.Second)
	if !g.cast(g.sa, wbRenew(), rezA, 20*time.Second) {
		t.Fatal("no group: not begun")
	}
	if want := own - (own*20+50)/100; g.a.Debt != want || g.b.Debt != 500 || g.c.Debt != 500 {
		t.Errorf("no group: owes %d (want %d), the others %d and %d", g.a.Debt, want, g.b.Debt, g.c.Debt)
	}
}
