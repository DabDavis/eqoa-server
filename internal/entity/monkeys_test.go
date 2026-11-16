package entity

import (
	"strings"
	"testing"
	"time"
)

// The Barrel of Monkeys' monkeys (monkeys.go), held to the owner's words of
// 2026-11-16: "it spawns a monkey that follows him around, up to 5. they have
// no life bar and cannot be targeted. they do not take up a pet slot, but are
// still owned by that player only", "the gorillas make small like monkeys",
// "give them random monkey names".

// monkeyTick runs the monkeys' system at d.
func (r *mercRig) monkeyTick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount++
	r.w.monkeySystem(r.at(d))
}

// Five uses let out five monkeys, a sixth sends them all home; each is a
// small gorilla under a monkey's name, none twice, an NPC with the NPC bit
// that is no one's pet (no pet slot taken: a pet out stays its owner's), on no
// hate list, and its channel is never a target (byte 26 0, 142 a pet's 3, 143
// 0) with its name. The control: a creature's channel is selectable.
func TestMonkeysUpToFiveNeverTargets(t *testing.T) {
	r := newMercRig(t)
	names := map[string]bool{}
	for i := 1; i <= MonkeysMax; i++ {
		res := r.w.UseMonkeyBarrel(r.sess, r.at(time.Duration(i)*time.Second))
		if res.Use != MonkeyCame || res.Out != i || res.Name == "" {
			t.Fatalf("use %d: %+v", i, res)
		}
		if names[res.Name] || len(res.Name) > 24 {
			t.Errorf("use %d: the name %q again, or past 24 letters", i, res.Name)
		}
		names[res.Name] = true
	}
	out := r.w.monkeys[r.p.ServerID]
	if len(out) != MonkeysMax {
		t.Fatalf("%d monkeys out", len(out))
	}
	crab := r.creature("Desert Crab", 28, 3)
	for _, ms := range out {
		n := ms.npc
		if uint32(n.ModelID) != MonkeyBody || n.Size != MonkeySize || n.NPCType != 0x80 || n.Owner != 0 || !n.Temporary ||
			n.World != r.p.World {
			t.Errorf("%q: body %#x size %v type %#x owner %d temporary %v", n.Name, uint32(n.ModelID), n.Size, n.NPCType, n.Owner, n.Temporary)
		}
		buf := SerializeNPCUpdate(0x1234, n)
		if buf[26] != 0 || buf[142] != 3 || buf[143] != 0 || !strings.HasPrefix(string(buf[116:140]), n.Name) {
			t.Errorf("%q's channel: 26=%d 142=%d 143=%d name %q", n.Name, buf[26], buf[142], buf[143], buf[116:140])
		}
		r.w.mu.Lock()
		r.w.addThreatLocked(crab, n.ServerID, 5000)
		if r.w.npcOwnerLocked(n) != r.p {
			t.Errorf("%q is not its owner's", n.Name)
		}
		r.w.mu.Unlock()
		if _, hated := crab.Threats[n.ServerID]; hated {
			t.Errorf("the crab hates %q", n.Name)
		}
	}
	if buf := SerializeNPCUpdate(0x1235, crab); buf[26] != 1 {
		t.Errorf("the control: a creature's byte 26 is %d", buf[26])
	}
	if r.w.pets[r.p.ServerID] != nil || len(r.w.familiars) != 0 {
		t.Error("a monkey took a pet's or a familiar's place")
	}
	res := r.w.UseMonkeyBarrel(r.sess, r.at(10*time.Second))
	if res.Use != MonkeysBack || len(r.w.monkeys) != 0 {
		t.Fatalf("a use with five out: %+v, %d owners with monkeys", res, len(r.w.monkeys))
	}
	for _, ms := range out {
		if r.w.NPCs[ms.npc.ServerID] != nil {
			t.Errorf("%q stayed in the world", ms.npc.Name)
		}
	}
	if res := r.w.UseMonkeyBarrel(r.sess, r.at(11*time.Second)); res.Use != MonkeyCame || res.Out != 1 {
		t.Errorf("after they went home: %+v", res)
	}
}

// A name is never one another of its owner's wears: with every name but one
// worn that one is picked; with all worn a numbered one.
func TestMonkeyNamesNeverTwice(t *testing.T) {
	old := monkeyPick
	t.Cleanup(func() { monkeyPick = old })
	monkeyPick = func(n int) int { return 0 }
	worn := map[string]bool{}
	for _, s := range monkeyNames[:len(monkeyNames)-1] {
		worn[s] = true
	}
	if got := monkeyName(worn); got != monkeyNames[len(monkeyNames)-1] {
		t.Errorf("one free: %q", got)
	}
	worn[monkeyNames[len(monkeyNames)-1]] = true
	if got := monkeyName(worn); got != "Monkey 2" {
		t.Errorf("none free: %q", got)
	}
	seen := map[string]bool{}
	for _, s := range monkeyNames {
		if seen[s] || len(s) > 24 || s == "" {
			t.Errorf("the list's %q: twice, empty or past 24 letters", s)
		}
		seen[s] = true
	}
	if len(monkeyNames) < 30 {
		t.Errorf("%d names, want 30 or more", len(monkeyNames))
	}
}

// They follow their owner as he walks: after a walk of 40 east and a stand,
// every one is near him (within its place's reach) and apart from the others
// (no two stacked); while he walks none falls far behind.
func TestMonkeysFollowSpreadOut(t *testing.T) {
	r := newMercRig(t)
	for i := 0; i < MonkeysMax; i++ {
		r.w.UseMonkeyBarrel(r.sess, r.at(0))
	}
	out := r.w.monkeys[r.p.ServerID]
	d := time.Duration(0)
	for step := 0; step < 80; step++ { // 40 east at 5 a second
		d += 100 * time.Millisecond
		r.w.mu.Lock()
		oldX := r.p.X
		r.p.X += 0.5
		r.p.Facing = headingTo(1, 0)
		r.w.SpatialIndex.Update(r.p.ServerID, oldX, r.p.Z, r.p.X, r.p.Z)
		r.w.mu.Unlock()
		r.monkeyTick(d)
		for _, ms := range out {
			if dd := distXZ(ms.npc.X, ms.npc.Z, r.p.X, r.p.Z); dd > 12 {
				t.Fatalf("step %d: %q is %.1f behind", step, ms.npc.Name, dd)
			}
		}
	}
	for i := 0; i < 50; i++ { // he stands 5 s
		d += 100 * time.Millisecond
		r.monkeyTick(d)
	}
	for i, a := range out {
		if dd := distXZ(a.npc.X, a.npc.Z, r.p.X, r.p.Z); dd > monkeyRadius+followSlack+0.5 || dd < 1 {
			t.Errorf("%q stands %.2f off its owner", a.npc.Name, dd)
		}
		for _, b := range out[i+1:] {
			if dd := distXZ(a.npc.X, a.npc.Z, b.npc.X, b.npc.Z); dd < 1 {
				t.Errorf("%q and %q stand %.2f apart", a.npc.Name, b.npc.Name, dd)
			}
		}
	}
	if len(r.w.monkeys[r.p.ServerID]) != MonkeysMax {
		t.Errorf("%d out after the walk", len(r.w.monkeys[r.p.ServerID]))
	}
}

// They all go back into the barrel at his death, a change of world, a move
// far at once (a teleport) and a logout, and only then: a walk keeps them.
func TestMonkeysGoHome(t *testing.T) {
	cases := []struct {
		name string
		do   func(r *mercRig)
	}{
		{"death", func(r *mercRig) { r.p.HP = 0 }},
		{"another world", func(r *mercRig) { r.p.World = 3 }},
		{"a teleport", func(r *mercRig) { r.p.X += 500 }},
		{"a logout", func(r *mercRig) { r.w.RemovePlayer(r.p.ServerID) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newMercRig(t)
			for i := 0; i < 3; i++ {
				r.w.UseMonkeyBarrel(r.sess, r.at(0))
			}
			r.monkeyTick(100 * time.Millisecond)
			if len(r.w.monkeys[r.p.ServerID]) != 3 {
				t.Fatal("the control: they went with nothing done")
			}
			ids := []EntityID{}
			for _, ms := range r.w.monkeys[r.p.ServerID] {
				ids = append(ids, ms.npc.ServerID)
			}
			c.do(r)
			r.monkeyTick(200 * time.Millisecond)
			if len(r.w.monkeys) != 0 {
				t.Errorf("%d owners keep monkeys", len(r.w.monkeys))
			}
			for _, id := range ids {
				if r.w.NPCs[id] != nil {
					t.Errorf("monkey %d stayed in the world", id)
				}
			}
		})
	}
	r := newMercRig(t)
	r.p.HP = 0
	if res := r.w.UseMonkeyBarrel(r.sess, r.at(0)); res.Use != MonkeyDead || len(r.w.monkeys) != 0 {
		t.Errorf("a dead owner's use: %+v", res)
	}
}
