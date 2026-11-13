package entity

import (
	"strings"
	"testing"
	"time"
)

// The GM Jibrish's petition 1894: "If mr moogie hits 1 hp, he freezes and is immune to damage, unable to take
// damage or die for 30 seconds", with his line. Over the template rig: the hit that would kill the guard leaves it
// at 1, its line in /say; for 30 s no hit lands (a spell's, an NPC's), no heal at a hit raises it, and it swings at
// no one; at 30 s its fight goes on (it swings again) and the next hit kills it. Once a fight: the next fight holds
// again. The control: a creature of no such template dies at the same hit.
func TestADeathHoldAtOneHitPoint(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	meleeIntn = func(n int) int { return n / 2 }
	r.godTemplate("stubborn king", ", class", ", 0",
		`[{"kind":"hitheal","heal":150},{"kind":"deathhold","hold":30,"say":"You thought you had me, didn't you?"}]`, "gnoll guard")
	guard, brute, priest := r.npc("Gnoll Guard"), r.npc("Gnoll Brute"), r.npc("Gnoll Priest")
	pool(guard, 100_000)
	pool(brute, 1_000_000)
	pool(priest, 100_000)
	p, sess := r.player("Tank", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 1_000_000, 1_000_000
	r.refight(guard, p.ServerID)
	hitBy := func(d time.Duration, n *NPC) {
		r.w.mu.Lock()
		r.w.npcHitNPCLocked(brute, n, r.at(d))
		if n == guard && !guard.Dead {
			delete(guard.Threats, brute.ServerID) // its fight the tank's alone (the brute is no foe it may fight)
		}
		r.w.mu.Unlock()
	}

	// the control: no hold, dead
	priest.HP = 2
	hitBy(0, priest)
	if !priest.Dead {
		t.Fatalf("the control: HP %d, want dead", priest.HP)
	}
	// the killing hit: 1 hit point, its line
	guard.HP = 2
	hitBy(0, guard)
	if guard.Dead || guard.HP != 1 {
		t.Fatalf("the killing hit: dead %v, HP %d; want held at 1", guard.Dead, guard.HP)
	}
	said := false
	for _, l := range linesOf(drain(sess)) {
		said = said || strings.Contains(l, "Gnoll Guard: You thought you had me, didn't you?")
	}
	if !said {
		t.Error("its line was not heard")
	}
	// held: nothing lands, nothing heals it, no swing
	hitBy(time.Second, guard)
	r.w.mu.Lock()
	r.w.spellHitLocked(p, sess, guard, 5000, numberSpell)
	r.w.mu.Unlock()
	if guard.Dead || guard.HP != 1 {
		t.Fatalf("held: dead %v, HP %d; want 1, nothing landed", guard.Dead, guard.HP)
	}
	r.run(2*time.Second, 29*time.Second)
	if p.HP != p.MaxHP || guard.HP != 1 {
		t.Fatalf("held 2-29 s: the tank's HP %d of %d (want untouched: no swing), guard HP %d", p.HP, p.MaxHP, guard.HP)
	}
	// its time up: it swings again, and the next hit kills it
	r.run(31*time.Second, 36*time.Second)
	if p.HP == p.MaxHP {
		t.Error("after its hold: it swings at no one")
	}
	hitBy(37*time.Second, guard)
	if !guard.Dead {
		t.Fatalf("after its hold: HP %d, want dead at the next hit", guard.HP)
	}

	// the next fight holds again
	guard.Dead, guard.HP = false, 2
	r.endFight(guard, 40*time.Second)
	r.refight(guard, p.ServerID)
	hitBy(41*time.Second, guard)
	if guard.Dead || guard.HP != 1 {
		t.Errorf("the next fight: dead %v, HP %d; want held at 1 again", guard.Dead, guard.HP)
	}
}
