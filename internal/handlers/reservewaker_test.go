package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A woken boss kept for the party that woke it (entity/reserve.go, the owner 2026-11-26):
// a kill of the necrologist fires a trigger whose spawn says reserve "waker".

const reserveTyrantObj = 0x01400077

// reserveRig is a trigger world with a necrologist (row 900) whose death fires "Wake
// the Tyrant", spawning a copy of row 901 kept for the killer's party for 15 minutes.
func reserveRig(t *testing.T) (*trigWorld, *GroupHandler) {
	t.Helper()
	w := newTrigWorld(t)
	w.gw.NPCs[900] = &entity.NPC{ServerID: 900, Name: "necrologist", HP: 100, MaxHP: 100, Level: 50,
		X: 25005, Y: 50, Z: 15005, World: 0}
	w.gw.NPCs[901] = &entity.NPC{ServerID: 901, Name: "Tyrant", HP: 1000, MaxHP: 1000, Level: 60,
		X: 25005, Y: 50, Z: 15005, World: 0, SpawnOnly: true}
	w.set(5, "Wake the Tyrant", "near", 0, `[{"do":"lock","lock":"tyrant"},`+
		`{"do":"spawn","npc":"#901","hostile":true,"spread":1,"minutes":60,"reserve":"waker","reserveminutes":15},{"do":"awaitdead"}]`)
	if _, err := w.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES ('necrologist', 'Wake the Tyrant', 100)`); err != nil {
		t.Fatal(err)
	}
	if err := w.e.Reload(); err != nil {
		t.Fatal(err)
	}
	return w, NewGroupHandler(w.gw)
}

// tyrant is the trigger's living copy.
func (w *trigWorld) tyrant() *entity.NPC {
	w.t.Helper()
	for _, n := range w.spawned() {
		if n.Name == "Tyrant" {
			return n
		}
	}
	w.t.Fatal("no Tyrant spawned")
	return nil
}

// engage is s's press on the Tyrant, it on s's slot 1.
func (w *trigWorld) engageTyrant(s *session.Session, n *entity.NPC, now time.Time) bool {
	vs := entity.GetOrCreateVisibility(s)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = n.ServerID, reserveTyrantObj
	return w.gw.EngageAttack(s, reserveTyrantObj, entity.Swing{Delay: time.Second, Reach: 4.5}, now)
}

// told reports s was sent a chat line holding text.
func told(t *testing.T, s *session.Session, text string) bool {
	t.Helper()
	for _, l := range heard(t, s) {
		if strings.Contains(l.text, text) {
			return true
		}
	}
	return false
}

// The necrologist's killer's party keeps the Tyrant: a bystander's press is refused with the
// line, a member's engages, and the GM who joins the killer's group afterwards may engage it
// too. A GM's own firing reserves nothing.
func TestReserveWakerBystanderRefusedPartyAllowed(t *testing.T) {
	w, gh := reserveRig(t)
	w.e.Killed(entity.KillCredit{NPCName: "necrologist", NPCID: 900,
		Players: []entity.KillPlayer{{ServerID: 2, Name: "Player"}}})
	ty := w.tyrant()
	if name, chars := w.gw.ReservedFor(ty.ServerID); name != "Player" || len(chars) != 1 || chars[0] != 2 {
		t.Fatalf("kept for %q %v, want Player [2]", name, chars)
	}
	now := w.now
	w.clear()
	if w.engageTyrant(w.fs, ty, now) {
		t.Fatal("a bystander engaged the reserved Tyrant")
	}
	if !told(t, w.fs, "Tyrant is bound to another party's challenge.") {
		t.Fatalf("the bystander was not told: %+v", heard(t, w.fs))
	}
	if w.engageTyrant(w.gs, ty, now) {
		t.Fatal("the GM, not yet in the party, engaged it")
	}
	if !w.engageTyrant(w.ps, ty, now) {
		t.Fatal("the waker could not engage the Tyrant")
	}

	// the GM joins the waker's group: the party grows at the next look
	if err := gh.handleInvite(w.ps, groupMsg(protocol.OpcodeGroupInvite, u32b(w.gw.IDSeenBy(w.ps, w.gp)), str8b(w.gp.Name))); err != nil {
		t.Fatal(err)
	}
	if err := gh.handleAccept(w.gs, u32Msg(protocol.OpcodeAcceptGroupInvite, w.gw.IDSeenBy(w.gs, w.gw.SessionPlayer(w.ps)))); err != nil {
		t.Fatal(err)
	}
	w.gw.Tick(now.Add(2 * time.Second))
	if _, chars := w.gw.ReservedFor(ty.ServerID); len(chars) != 2 {
		t.Fatalf("the group member did not join the party: %v", chars)
	}
	if !w.engageTyrant(w.gs, ty, now.Add(3*time.Second)) {
		t.Fatal("a member of the waker's group could not engage the Tyrant")
	}

	// a GM's firing (no kill) spawns one free
	w.e.StopRuns(0)
	if _, err := w.fire(5); err != nil {
		t.Fatal(err)
	}
	for _, n := range w.spawned() {
		if name, _ := w.gw.ReservedFor(n.ServerID); n.Name == "Tyrant" && name != "" {
			t.Fatalf("a GM's firing kept the Tyrant for %q", name)
		}
	}
}

// The reservation lifts at its end (unless the party fights it then), and when its party is
// all gone from the world or dead for the grace.
func TestReserveWakerLifts(t *testing.T) {
	w, _ := reserveRig(t)
	kill := func() *entity.NPC {
		w.e.StopRuns(0)
		w.e.Killed(entity.KillCredit{NPCName: "necrologist", NPCID: 900,
			Players: []entity.KillPlayer{{ServerID: 2, Name: "Player"}}})
		return w.tyrant()
	}
	start := w.now

	// its end: 15 minutes, the party standing by
	ty := kill()
	w.gw.Tick(start.Add(14 * time.Minute))
	if name, _ := w.gw.ReservedFor(ty.ServerID); name == "" {
		t.Fatal("lifted before its end")
	}
	w.gw.Tick(start.Add(16 * time.Minute))
	if name, _ := w.gw.ReservedFor(ty.ServerID); name != "" {
		t.Fatal("still kept past its end")
	}
	if !w.engageTyrant(w.fs, ty, start.Add(16*time.Minute)) {
		t.Fatal("the bystander could not engage it once it lifted")
	}

	// the party gone from the world: kept through the grace, lifted after it
	t0 := start.Add(20 * time.Minute)
	w.now = t0
	ty = kill()
	pp := w.gw.SessionPlayer(w.ps)
	pp.World = 2
	w.gw.Tick(t0.Add(time.Second))
	w.gw.Tick(t0.Add(time.Second + entity.ReserveGrace/2))
	if name, _ := w.gw.ReservedFor(ty.ServerID); name == "" {
		t.Fatal("lifted inside the grace")
	}
	w.gw.Tick(t0.Add(2*time.Second + entity.ReserveGrace))
	if name, _ := w.gw.ReservedFor(ty.ServerID); name != "" {
		t.Fatal("still kept with its party gone from the world")
	}
	pp.World = 0

	// the party dead
	t1 := start.Add(40 * time.Minute)
	w.now = t1
	ty = kill()
	w.gw.Tick(t1.Add(time.Second))
	if name, _ := w.gw.ReservedFor(ty.ServerID); name == "" {
		t.Fatal("lifted with its party alive and here")
	}
	pp.HP = 0
	w.gw.Tick(t1.Add(10 * time.Second))
	w.gw.Tick(t1.Add(11*time.Second + entity.ReserveGrace))
	if name, _ := w.gw.ReservedFor(ty.ServerID); name != "" {
		t.Fatal("still kept with its party dead")
	}
}

// A reserved spawn step checks out, and a reserve that is not "waker" or a reserveminutes
// with no reserve is refused.
func TestReserveWakerStepProblems(t *testing.T) {
	ok := database.GMTrigger{Slot: 1, Scope: "near", World: -1, Steps: `[{"do":"spawn","npc":"x","reserve":"waker","reserveminutes":15}]`}
	if p := database.TriggerProblems(ok); len(p) != 0 {
		t.Fatalf("a waker reserve refused: %v", p)
	}
	for _, steps := range []string{`[{"do":"spawn","npc":"x","reserve":"group"}]`, `[{"do":"spawn","npc":"x","reserveminutes":15}]`,
		`[{"do":"spawn","npc":"x","reserve":"waker","reserveminutes":-1}]`} {
		if p := database.TriggerProblems(database.GMTrigger{Slot: 1, Scope: "near", World: -1, Steps: steps}); len(p) == 0 {
			t.Fatalf("%s: no problem found", steps)
		}
	}
}
