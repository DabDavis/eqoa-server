package handlers

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Primevals' portals down (Heirloom petitions 2674 and 2676): each, talked
// to on its Primeval's middle floor, sets the player down on its own tower's
// ground floor where the Archons' portals down land.
func TestThePrimevalsPortalsDown(t *testing.T) {
	for _, c := range []struct {
		name                string
		x, y, z, toX, toZ   float32
		wantY               float32
		notToX, notToZ, top float32
	}{
		{"Portal down from Earth", 4695, 83, 2493, 4700, 2512, 66, 4700, 2330, 83},
		{"Portal down from Fire", 4697, 84, 2357, 4700, 2330, 66, 4700, 2512, 84},
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
	// the control: a Primeval itself is no portal
	if IsWaysender("Earth Primeval") || IsWaysender("Fire Primeval") {
		t.Error("a Primeval offers trips")
	}
}

// primevalDownSteps are the triggers' steps as
// tools/primeval_portal_down_20261122.py writes them (its test checks they are
// these, letter for letter).
var primevalDownSteps = map[string]string{
	"Earth Portal Down": `[{"do":"lock","lock":"earth portal down"},{"do":"spawn","npc":"Portal down from Earth","spread":0.01,"minutes":10,"world":1,"x":4695,"y":83,"z":2493,"face":"south"},{"do":"broadcast","text":"A portal shimmers open where the Earth Primeval fell. It leads down to the tower's floor and closes in ten minutes."}]`,
	"Fire Portal Down":  `[{"do":"lock","lock":"fire portal down"},{"do":"spawn","npc":"Portal down from Fire","spread":0.01,"minutes":10,"world":1,"x":4697,"y":84,"z":2357,"face":"north"},{"do":"broadcast","text":"A portal shimmers open where the Fire Primeval fell. It leads down to the tower's floor and closes in ten minutes."}]`,
}

// A Primeval's death (npcdeath) stands its portal down on its floor for ten
// minutes, one at a time; a guardian's death stands none.
func TestAPrimevalsDeathStandsItsPortalDown(t *testing.T) {
	w := newTrigWorld(t)
	slot := 900
	for _, name := range []string{"Earth Portal Down", "Fire Portal Down"} {
		g := database.GMTrigger{Slot: slot, Name: name, Scope: "near", World: 1, Steps: primevalDownSteps[name]}
		if probs := database.TriggerProblems(g); len(probs) > 0 {
			t.Fatalf("%s: %v", name, probs)
		}
		w.setAt(slot, name, "near", 25, 1, 4700, 84, 2420, primevalDownSteps[name], true)
		slot++
	}
	for _, d := range [][2]string{{"Earth Primeval", "Earth Portal Down"}, {"Fire Primeval", "Fire Portal Down"}} {
		if _, err := w.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES (?, ?, 100)`, database.NPCClassKey(d[0]), d[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.e.Reload(); err != nil {
		t.Fatal(err)
	}
	id := entity.EntityID(9100)
	for _, n := range []string{"Portal down from Earth", "Portal down from Fire"} {
		w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: n, HP: 100, MaxHP: 100, Level: 55, NPCType: 384,
			X: 4700, Y: 66, Z: 2420, World: 1, SpawnOnly: true}
		id++
	}
	earth := &entity.NPC{ServerID: id, Name: "Earth Primeval", HP: 1000, MaxHP: 1000, Level: 67,
		X: 4700, Y: 85.3, Z: 2511, SpawnX: 4700, SpawnY: 85.3, SpawnZ: 2511, World: 1}
	w.gw.NPCs[id] = earth
	id++
	fire := &entity.NPC{ServerID: id, Name: "Fire Primeval", HP: 1000, MaxHP: 1000, Level: 67,
		X: 4700, Y: 85.5, Z: 2344, SpawnX: 4700, SpawnY: 85.5, SpawnZ: 2344, World: 1}
	w.gw.NPCs[id] = fire
	id++
	guardian := &entity.NPC{ServerID: id, Name: "granite guardian", HP: 100, MaxHP: 100, Level: 60,
		X: 4704.76, Y: 66, Z: 2514.62, World: 1}
	w.gw.NPCs[id] = guardian

	w.e.Killed(entity.KillCredit{NPCName: guardian.Name, NPCID: guardian.ServerID})
	w.at(time.Second)
	if w.alive("Portal down from Earth")+w.alive("Portal down from Fire") != 0 {
		t.Fatal("a granite guardian's death stood a portal")
	}

	w.e.Killed(entity.KillCredit{NPCName: earth.Name, NPCID: earth.ServerID})
	w.at(2 * time.Second)
	p := w.first("Portal down from Earth")
	if p == nil || w.alive("Portal down from Earth") != 1 {
		t.Fatalf("the Earth Primeval's death stood %d portals down", w.alive("Portal down from Earth"))
	}
	if p.World != 1 || dist2(p.X, p.Z, 4695, 2493) > 0.25 || p.Y < 82.5 || p.Y > 83.5 {
		t.Errorf("its portal stands at (%.2f, %.2f, %.2f) world %d, want its floor at (4695, 83, 2493)", p.X, p.Y, p.Z, p.World)
	}
	if w.alive("Portal down from Fire") != 0 {
		t.Error("the Earth Primeval's death stood the Fire's portal")
	}
	w.e.Killed(entity.KillCredit{NPCName: earth.Name, NPCID: earth.ServerID})
	w.at(3 * time.Minute)
	if n := w.alive("Portal down from Earth"); n != 1 {
		t.Fatalf("a second death inside the ten minutes: %d portals", n)
	}
	w.at(11 * time.Minute)
	if w.alive("Portal down from Earth") != 0 {
		t.Fatal("the portal outlived its ten minutes")
	}

	w.e.Killed(entity.KillCredit{NPCName: fire.Name, NPCID: fire.ServerID})
	w.at(12 * time.Minute)
	v := w.first("Portal down from Fire")
	if v == nil || dist2(v.X, v.Z, 4697, 2357) > 0.25 || v.Y < 83.5 || v.Y > 84.5 {
		t.Fatalf("the Fire Primeval's portal down: %+v", v)
	}
}
