package entity

import (
	"strings"
	"testing"
	"time"
)

// weaponhold_test.go: petition 1821 (the GM Jibrish: "Enable code for weapons to
// have working root or snare procs"): any record whose words name a root or a
// snare proc holds or slows, alone or beside its damage.

// A record's words make the hold: a root, a snare, their own time and slow, a hold
// beside a damage figure; the controls: words of roots with no proc, a proc with no
// hold.
func TestWeaponHoldFromText(t *testing.T) {
	for _, c := range []struct {
		desc            string
		ok              bool
		kind            ProcKind
		damage          int
		root, snare     bool
		pct             int
		hold            time.Duration
		words, explains string
	}{
		{"A masterwork spear. It reeks of the Plane of Disease.\nProcs a root.", true, ProcRoot, 0, false, false, 0, 0,
			"a root for 12 s", "roots the creature for 12 s"},
		{"Procs: Snare", true, ProcSnare, 0, false, false, 0, 0, "a 50% snare for 18 s", "snares the creature, 50% slower for 18 s"},
		{"A thorny whip. Procs a 6 second root.", true, ProcRoot, 0, false, false, 0, 6 * time.Second, "a root for 6 s", "for 6 s"},
		{"Procs a 40% snare for 10 seconds.", true, ProcSnare, 0, false, false, 40, 10 * time.Second,
			"a 40% snare for 10 s", "40% slower for 10 s"},
		{"A bow. Snare proc, slows by 30%.", true, ProcSnare, 0, false, false, 30, 0, "a 30% snare for 18 s", "30% slower"},
		{"Procs a 100 point DD and roots the target.", true, ProcHit, 100, true, false, 0, 0, "100 and a root for 12 s",
			"and roots it for 12 s"},
		{"Procs a 50 point cold DoT that ensnares.", true, ProcDoT, 50, false, true, 0, 0, "and a 50% snare for 18 s", "and snares it"},
		{"Procs 150 Ice Damage.", true, ProcHit, 150, false, false, 0, 0, "150", "hits for 150"},
		{"This tunic is woven from the roots of an ancient skypine.", false, 0, 0, false, false, 0, 0, "", ""},
		{"The base of this root is wrapped carefully in oak leaves.", false, 0, 0, false, false, 0, 0, "", ""},
	} {
		wp, ok := WeaponProcOf(9002, "a test weapon", c.desc)
		if ok != c.ok {
			t.Errorf("%q: proc %v, want %v", c.desc, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if wp.Kind != c.kind || wp.Damage != c.damage || wp.Root != c.root || wp.Snare != c.snare || wp.SnarePct != c.pct ||
			wp.HoldFor != c.hold {
			t.Errorf("%q: %+v", c.desc, wp)
		}
		if !strings.Contains(wp.String(), c.words) || !strings.Contains(wp.Explain(), c.explains) {
			t.Errorf("%q: says %q / %q", c.desc, wp.String(), wp.Explain())
		}
		if (c.kind == ProcRoot || c.kind == ProcSnare) && wp.FX != fxBuff {
			t.Errorf("%q: its look %#x", c.desc, wp.FX)
		}
	}
}

// A snare proc slows the creature by its share for its time; a root by its words
// holds for its own time; a hit that roots does both. The controls: over the
// chance, resisted by the proc's roll, resisted by its con (root.go), nothing.
func TestWeaponHoldLands(t *testing.T) {
	defer func(f, g func(int) int) { procIntn, rootIntn = f, g }(procIntn, rootIntn)
	rootIntn = func(int) int { return 9999 } // the con's resist never, unless a case says so
	land := func(desc string, rolls []int) (*procRig, time.Time) {
		r := weaponRig(t, "a test weapon", desc)
		procIntn = dice(rolls...)
		now := time.Now()
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.weaponProcLocked(r.fsess, r.friend, r.npc, now)
		return r, now
	}
	// a snare, 40% for 10 s
	r, now := land("Procs a 40% snare for 10 seconds.", []int{0, 99})
	if f := r.npc.snareFactor(now.Add(time.Second)); f < 0.59 || f > 0.61 {
		t.Errorf("a 40%% snare leaves %v of its pace", f)
	}
	if f := r.npc.snareFactor(now.Add(11 * time.Second)); f != 1 {
		t.Errorf("after its 10 s: %v of its pace", f)
	}
	if r.npc.rooted(now.Add(time.Second)) {
		t.Error("a snare rooted")
	}
	// a 6 s root
	r, now = land("Procs a 6 second root.", []int{0, 99})
	if !r.npc.rooted(now.Add(time.Second)) || r.npc.rooted(now.Add(7*time.Second)) {
		t.Errorf("a 6 s root: at 1 s %v, at 7 s %v", r.npc.rooted(now.Add(time.Second)), r.npc.rooted(now.Add(7*time.Second)))
	}
	// a hit that roots: the figure and the hold
	r, now = land("Procs a 100 point DD and roots the target.", []int{0, 99})
	if f := figures(r.fsess); len(f) != 1 || f[0] != -100 || !r.npc.rooted(now.Add(time.Second)) {
		t.Errorf("a hit that roots: figures %v, rooted %v", f, r.npc.rooted(now.Add(time.Second)))
	}
	// over the chance, resisted by the proc's roll: nothing
	for _, rolls := range [][]int{{99}, {0, 0}} {
		r, now = land("Procs: Snare", rolls)
		if f := r.npc.snareFactor(now.Add(time.Second)); f != 1 {
			t.Errorf("rolls %v: snared to %v", rolls, f)
		}
	}
	r, now = land("Procs a 100 point DD and roots the target.", []int{0, 0}) // resisted: the least hit, no root
	if r.npc.rooted(now.Add(time.Second)) {
		t.Error("a resisted hit rooted")
	}
	// resisted by its con: a con table that resists every root and snare (root.go)
	rootIntn = func(int) int { return 0 }
	r = weaponRig(t, "a test weapon", "Procs: Snare")
	r.w.balance = rootBalance(100, 0)
	procIntn = dice(0, 99)
	now = time.Now()
	r.w.mu.Lock()
	r.w.weaponProcLocked(r.fsess, r.friend, r.npc, now)
	f := r.npc.snareFactor(now.Add(time.Second))
	r.w.mu.Unlock()
	if f != 1 {
		t.Errorf("a con resist of 100%%: snared to %v", f)
	}
}

// The data's own: the Noroxus Spine Spear's record ("Procs a root.") roots by its words
// too, were its name not in weaponProcsByName.
func TestNoroxusRecordWordsRoot(t *testing.T) {
	wp, ok := WeaponProcFromText("A masterwork spear. It reeks of the Plane of Disease.\nProcs a root.")
	if !ok || wp.Kind != ProcRoot || wp.rootFor() != procRootFor {
		t.Fatalf("%+v %v", wp, ok)
	}
}
