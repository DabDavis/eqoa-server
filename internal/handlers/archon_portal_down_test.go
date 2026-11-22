package handlers

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Archons' portals down (Heirloom petition 2670): each, talked to on its
// Archon's floor, sets the player down on its own tower's ground floor between
// the portals up; the portals up still go up.
func TestTheArchonTowersPortalsDown(t *testing.T) {
	for _, c := range []struct {
		name                string
		x, y, z, toX, toZ   float32
		wantY               float32
		notToX, notToZ, top float32
	}{
		{"Portal down from Aereth", 4695, 110.375, 2493, 4700, 2512, 66, 4700, 2330, 110.375},
		{"Portal down from Vetash", 4697, 106, 2353, 4700, 2330, 66, 4700, 2512, 106},
	} {
		if !IsWaysender(c.name) {
			t.Errorf("%s is no travel NPC", c.name)
		}
		if len(c.name) > 23 {
			t.Errorf("%s: %d letters, past the state channel's 24-byte name", c.name, len(c.name))
		}
		tw := newTalkWorld(t, c.x, c.z)
		tw.p.World, tw.p.Y = 1, c.top
		portal := tw.add(3, &entity.NPC{ServerID: 703, Name: c.name, World: 1, NPCType: 384, SpawnX: c.x, SpawnY: c.y, SpawnZ: c.z})
		tw.talk(portal)
		if got := optionBox(t, tw.s); !slices.Equal(got, []string{"Yes", "No"}) {
			t.Fatalf("%s asks %q", c.name, got)
		}
		tw.pick(portal, 0)
		x, z, world, moved := teleported(tw.s)
		y, _ := teleportedY(tw.s)
		if !moved || world != 1 || dist2(x, z, c.toX, c.toZ) > 1 || y != c.wantY {
			t.Errorf("%s sent the player to (%.1f, %.1f, %.1f) world %d (%v), want (%.1f, %.1f, %.1f)", c.name, x, y, z, world, moved, c.toX, c.wantY, c.toZ)
		}
		if dist2(x, z, c.notToX, c.notToZ) < 100 {
			t.Errorf("%s sent the player to the other tower", c.name)
		}
	}
	// the control: an Archon herself is no portal
	if IsWaysender("Archon Aereth") || IsWaysender("Archon Vetash") {
		t.Error("an Archon offers trips")
	}
}

// archonDownSteps are the triggers' steps as tools/archon_portal_down_20261122.py
// writes them (its test checks they are these, letter for letter).
var archonDownSteps = map[string]string{
	"Aereth Portal Down": `[{"do":"lock","lock":"aereth portal down"},{"do":"spawn","npc":"Portal down from Aereth","spread":0.01,"minutes":10,"world":1,"x":4695,"y":110.375,"z":2493,"face":"south"},{"do":"broadcast","text":"A portal shimmers open where Archon Aereth fell. It leads down to the tower's floor and closes in ten minutes."}]`,
	"Vetash Portal Down": `[{"do":"lock","lock":"vetash portal down"},{"do":"spawn","npc":"Portal down from Vetash","spread":0.01,"minutes":10,"world":1,"x":4697,"y":106,"z":2353,"face":"north"},{"do":"broadcast","text":"A portal shimmers open where Archon Vetash fell. It leads down to the tower's floor and closes in ten minutes."}]`,
}

// An Archon's death (npcdeath) stands its portal down on its floor for ten
// minutes, one at a time though the Archon rises again inside them; another
// creature's death stands none; Sarek's quest run (tools/sarek_709_20261122.py:
// its lock, Aereth hidden by name) neither keeps the portal from rising nor hides
// it.
func TestAnArchonsDeathStandsItsPortalDown(t *testing.T) {
	w := newTrigWorld(t)
	slot := 900
	for _, name := range []string{"Aereth Portal Down", "Vetash Portal Down"} {
		g := database.GMTrigger{Slot: slot, Name: name, Scope: "near", World: 1, Steps: archonDownSteps[name]}
		if probs := database.TriggerProblems(g); len(probs) > 0 {
			t.Fatalf("%s: %v", name, probs)
		}
		w.setAt(slot, name, "near", 25, 1, 4700, 108, 2420, archonDownSteps[name], true)
		slot++
	}
	for _, d := range [][2]string{{"Archon Aereth", "Aereth Portal Down"}, {"Archon Vetash", "Vetash Portal Down"}} {
		if _, err := w.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES (?, ?, 100)`, database.NPCClassKey(d[0]), d[1]); err != nil {
			t.Fatal(err)
		}
	}
	// Sarek's quest run as tools/sarek_709_20261122.py writes its lock and hide
	w.setAt(950, "Sarek the Betrayer", "near", 40, 1, 4700, 110.375, 2500,
		`[{"do":"lock","lock":"sarek the betrayer"},{"do":"hide","npc":"Archon Aereth","radius":30,"minutes":65},{"do":"wait","seconds":3600}]`, true)
	if err := w.e.Reload(); err != nil {
		t.Fatal(err)
	}
	id := entity.EntityID(9100)
	for _, n := range []string{"Portal down from Aereth", "Portal down from Vetash"} {
		w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: n, HP: 100, MaxHP: 100, Level: 55, NPCType: 384,
			X: 4700, Y: 66, Z: 2420, World: 1, SpawnOnly: true}
		id++
	}
	aereth := &entity.NPC{ServerID: id, Name: "Archon Aereth", HP: 1000, MaxHP: 1000, Level: 67,
		X: 4699.23, Y: 110.312, Z: 2512.66, SpawnX: 4699.23, SpawnY: 110.312, SpawnZ: 2512.66, World: 1}
	w.gw.NPCs[id] = aereth
	id++
	vetash := &entity.NPC{ServerID: id, Name: "Archon Vetash", HP: 1000, MaxHP: 1000, Level: 67,
		X: 4699.55, Y: 106, Z: 2342.21, SpawnX: 4699.55, SpawnY: 106, SpawnZ: 2342.21, World: 1}
	w.gw.NPCs[id] = vetash
	id++
	guardian := &entity.NPC{ServerID: id, Name: "granite guardian", HP: 100, MaxHP: 100, Level: 60,
		X: 4704.76, Y: 66, Z: 2514.62, World: 1}
	w.gw.NPCs[id] = guardian

	// the control: a guardian's death stands nothing
	w.e.Killed(entity.KillCredit{NPCName: guardian.Name, NPCID: guardian.ServerID})
	w.at(time.Second)
	if w.alive("Portal down from Aereth")+w.alive("Portal down from Vetash") != 0 {
		t.Fatal("a granite guardian's death stood a portal")
	}

	// Sarek's run holds its lock and hides Aereth: her death still stands the portal
	if _, err := w.fire(950); err != nil {
		t.Fatal(err)
	}
	w.e.Killed(entity.KillCredit{NPCName: aereth.Name, NPCID: aereth.ServerID})
	w.at(2 * time.Second)
	p := w.first("Portal down from Aereth")
	if p == nil || w.alive("Portal down from Aereth") != 1 {
		t.Fatalf("Aereth's death stood %d portals down", w.alive("Portal down from Aereth"))
	}
	if p.World != 1 || dist2(p.X, p.Z, 4695, 2493) > 0.25 || p.Y < 110 || p.Y > 111 {
		t.Errorf("her portal stands at (%.2f, %.2f, %.2f) world %d, want her floor at (4695, 110.4, 2493)", p.X, p.Y, p.Z, p.World)
	}
	if !p.HiddenUntil.IsZero() && p.HiddenUntil.After(w.now) {
		t.Error("Sarek's run hid the portal down with Aereth")
	}
	if w.alive("Portal down from Vetash") != 0 {
		t.Error("Aereth's death stood Vetash's portal")
	}
	// she rises in 90 s and falls again: still one portal
	w.e.Killed(entity.KillCredit{NPCName: aereth.Name, NPCID: aereth.ServerID})
	w.at(3 * time.Minute)
	if n := w.alive("Portal down from Aereth"); n != 1 {
		t.Fatalf("a second death inside the ten minutes: %d portals", n)
	}
	w.at(9 * time.Minute)
	if w.alive("Portal down from Aereth") != 1 {
		t.Fatal("the portal went inside its ten minutes")
	}
	w.at(11 * time.Minute)
	if w.alive("Portal down from Aereth") != 0 {
		t.Fatal("the portal outlived its ten minutes")
	}
	w.at(11*time.Minute + time.Second)
	if w.e.BusyNamed("Aereth Portal Down") {
		t.Fatal("the lock held past the portal")
	}
	w.e.Killed(entity.KillCredit{NPCName: aereth.Name, NPCID: aereth.ServerID})
	w.at(12 * time.Minute)
	if w.alive("Portal down from Aereth") != 1 {
		t.Fatal("no portal at her next death")
	}

	// Vetash: his on his floor
	w.e.Killed(entity.KillCredit{NPCName: vetash.Name, NPCID: vetash.ServerID})
	w.at(13 * time.Minute)
	v := w.first("Portal down from Vetash")
	if v == nil || dist2(v.X, v.Z, 4697, 2353) > 0.25 || v.Y < 105.5 || v.Y > 106.5 {
		t.Fatalf("Vetash's portal down: %+v", v)
	}
}
