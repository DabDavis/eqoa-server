package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// dynmathSteps are the trigger's steps as tools/dynmath_raid_20261122.py writes
// them (its test checks they are these, letter for letter).
const dynmathSteps = `[{"do":"lock","lock":"dynmath the tyrant"},{"do":"clock","seconds":1800,"empty":600},{"do":"broadcast","text":"The Etheneric Portal tears open. Dynmath the Tyrant strides through, his dead behind him."},{"do":"spawn","npc":"Dynmath the Tyrant","hostile":true,"spread":0.5,"minutes":35,"world":0,"x":4765.39,"y":58.95,"z":14028.43},{"do":"spawn","npc":"minion of Dynmath","hostile":true,"count":8,"spread":12,"minutes":35,"noloot":true,"world":0,"x":4765.39,"y":58.95,"z":14028.43},{"do":"resummon","npc":"minion of Dynmath","by":"Dynmath the Tyrant","seconds":15,"hostile":true,"minutes":35,"text":"Dynmath the Tyrant calls another of his dead out of the portal."},{"do":"awaitdead","npc":"Dynmath the Tyrant"},{"do":"broadcast","text":"Dynmath the Tyrant falls, and his dead crumble to dust."},{"do":"despawn","npc":"minion of Dynmath"},{"do":"end"},{"do":"despawn"}]`

// dynmathAbilities are his template's abilities as the tool writes them (its
// test checks they are these, letter for letter).
const dynmathAbilities = `[{"kind":"rampage","chance":15},{"kind":"lifetap","every":18,"first":6},{"kind":"dot","every":24,"first":10},{"kind":"portnear","every":45,"first":30,"radius":150,"top":true,"foes":["minion of Dynmath"],"name":"Grasp of the Dead","say":"Your place is among my dead!"}]`

// His template's abilities pass the template check, and Grasp of the Dead is a
// portnear with top, among his dead.
func TestDynmathsAbilitiesHold(t *testing.T) {
	list, err := database.ParseTemplateAbilities(dynmathAbilities)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	var port *database.TemplateAbility
	for i := range list {
		if list[i].Kind == database.AbilityPortNear {
			port = &list[i]
		}
	}
	if port == nil || !port.Top || len(port.Foes) != 1 || port.Foes[0] != "minion of Dynmath" {
		t.Fatalf("his port: %+v", port)
	}
}

// Dynmath the Tyrant as a raid boss (Live petition 5500, the owner 2026-11-22):
// the totem's hand-in raises him with eight of his dead; with all of them down,
// one more comes 15 s after the last fell while he lives; at his death his dead
// are taken away and his corpse is left; one fight at a time (its lock).
func TestDynmathRisesWithHisDead(t *testing.T) {
	w := newTrigWorld(t)
	g := database.GMTrigger{Slot: 900, Name: "Dynmath the Tyrant", Scope: "near", World: 0, Steps: dynmathSteps}
	if probs := database.TriggerProblems(g); len(probs) > 0 {
		t.Fatalf("the steps: %v", probs)
	}
	w.setAt(900, "Dynmath the Tyrant", "near", 80, 0, 4765.39, 58.95, 14028.43, dynmathSteps, true)
	if err := w.e.Reload(); err != nil {
		t.Fatal(err)
	}
	for i, n := range []string{"Dynmath the Tyrant", "minion of Dynmath"} {
		id := entity.EntityID(9100 + i)
		w.gw.NPCs[id] = &entity.NPC{ServerID: id, Name: n, HP: 1000, MaxHP: 1000, Level: 60,
			X: 4765, Y: 59, Z: 14028, World: 0, SpawnOnly: true}
	}
	if _, err := w.fire(900); err != nil {
		t.Fatal(err)
	}
	w.at(time.Second)
	if w.alive("Dynmath the Tyrant") != 1 || w.alive("minion of Dynmath") != 8 {
		t.Fatalf("risen: %d Dynmath, %d of his dead; want 1 and 8", w.alive("Dynmath the Tyrant"), w.alive("minion of Dynmath"))
	}
	// a second hand-in while he stands raises nothing more
	if _, err := w.fire(900); err == nil {
		w.at(2 * time.Second)
		if n := w.alive("Dynmath the Tyrant"); n != 1 {
			t.Fatalf("a second firing: %d Dynmaths", n)
		}
	}
	// all eight of his dead down: one more 15 s after the last fell
	for w.first("minion of Dynmath") != nil {
		w.gw.KillNPCNow(w.first("minion of Dynmath").ServerID)
	}
	w.at(5 * time.Second)
	if n := w.alive("minion of Dynmath"); n != 0 {
		t.Fatalf("%d of his dead back before 15 s", n)
	}
	w.at(25 * time.Second)
	if n := w.alive("minion of Dynmath"); n != 1 {
		t.Fatalf("%d of his dead back after 15 s, want one", n)
	}
	// his death: his dead go, his corpse stays
	boss := w.first("Dynmath the Tyrant")
	w.gw.KillNPCNow(boss.ServerID)
	w.e.Killed(entity.KillCredit{NPCName: boss.Name, NPCID: boss.ServerID})
	w.at(27 * time.Second)
	w.at(28 * time.Second)
	if n := w.alive("minion of Dynmath"); n != 0 {
		t.Errorf("%d of his dead stand after his death", n)
	}
	if !boss.Dead {
		t.Error("his corpse was taken away")
	}
	w.at(5 * time.Minute)
	if n := w.alive("minion of Dynmath"); n != 0 {
		t.Errorf("%d of his dead came back after his death", n)
	}
	if w.e.BusyNamed("Dynmath the Tyrant") {
		t.Error("the fight's lock held past his death")
	}
}
