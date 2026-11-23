package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Live petition 5510: a Sky chain's step one player does at an NPC is done
// for each groupmate by it on the quest at that step, with nothing handed in
// by the mate; a mate far off, and a player of no group, keep theirs.
func TestASkyChainStepIsTheGroups(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.essence("singe", "Singe Burnfeathers", 4611.2, 113.0, 4939.0)
	for _, p := range []*entity.Player{r.pa, r.pc, r.pb} {
		r.on(p, 101303, 4)
	}
	r.by("singe")
	r.talk(r.anna, r.npcs["singe"])
	if s, _ := r.step(r.pa, 101303); s != 5 {
		t.Fatalf("Anna's step: %d, want 5 (%q)", s, r.msgs(r.anna))
	}
	if s, _ := r.step(r.pc, 101303); s != 5 {
		t.Errorf("Cora's step, by Anna in her group: %d, want 5", s)
	}
	if s, _ := r.step(r.pb, 101303); s != 4 {
		t.Errorf("Bert's step, of no group: %d, want 4", s)
	}

	// Cora out of reach: Anna's next step is hers alone
	r.on(r.pa, 101303, 4)
	r.on(r.pc, 101303, 4)
	forgetQuestChar(r.pa)
	forgetQuestChar(r.pc)
	r.by("singe")
	r.gw.UpdatePosition(r.pc.ServerID, r.pa.X+teleReach+5, r.pa.Y, r.pa.Z, 0)
	r.talk(r.anna, r.npcs["singe"])
	if s, _ := r.step(r.pa, 101303); s != 5 {
		t.Fatalf("Anna's step: %d, want 5", s)
	}
	if s, _ := r.step(r.pc, 101303); s != 4 {
		t.Errorf("Cora's step, %d off: %d, want 4", teleReach+5, s)
	}
}

// Live petition 5510: a Sky chain quest one player takes, its groupmates by
// it take too; another quest is the taker's alone.
func TestASkyChainQuestIsTakenByTheGroup(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.essence("singe", "Singe Burnfeathers", 4611.2, 113.0, 4939.0)
	r.by("singe")
	r.talk(r.anna, r.npcs["singe"])
	got := r.msgs(r.anna)
	if s, _ := r.step(r.pa, 101303); s != 4 {
		t.Fatalf("Anna did not take the quest: %q", got)
	}
	if s, _ := r.step(r.pc, 101303); s != 4 {
		t.Errorf("Cora's step: %d, want 4 (taken with Anna)", s)
	}
	if s, _ := r.step(r.pb, 101303); s != -1 {
		t.Errorf("Bert, of no group, has the quest at %d", s)
	}
}

// Live petitions 5509 and 5510: at Granavik's portal a player one step behind
// the step its way back waits for, with a groupmate by it past that step,
// goes on with the group (its key taken, the step done) instead of back
// alone; a groupmate one step behind the passage's step, carried by it, has
// that step done with the passage's.
func TestGranaviksPortalTakesTheGroupOnTogether(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.on(r.pa, 101303, 4)
	r.on(r.pc, 101303, 5)
	r.own(r.pa, "Rain Key 3", 20)
	r.by("portal")
	r.talk(r.anna, r.npcs["portal"])
	got := r.msgs(r.anna)
	if !hasMsg(got, "teleport") || r.pa.X != 4242.0 || r.pa.Z != 4305.3 {
		t.Fatalf("Anna, behind Cora: %q at (%.1f, %.1f)", got, r.pa.X, r.pa.Z)
	}
	if s, _ := r.step(r.pa, 101303); s != 6 {
		t.Errorf("Anna's step: %d, want 6", s)
	}
	if n := r.held(r.pa, "Rain Key 3"); n != 19 {
		t.Errorf("Anna holds %d keys, want 19", n)
	}
	if r.pc.X != r.pa.X || r.pc.Z != r.pa.Z {
		t.Errorf("Cora was not taken along: (%.1f, %.1f)", r.pc.X, r.pc.Z)
	}
	if s, _ := r.step(r.pc, 101303); s != 6 {
		t.Errorf("Cora's step: %d, want 6", s)
	}

	// the other way round: Anna at the passage's step, Cora one behind it
	r.on(r.pa, 101303, 5)
	r.on(r.pc, 101303, 4)
	forgetQuestChar(r.pa)
	forgetQuestChar(r.pc)
	r.by("portal")
	r.talk(r.anna, r.npcs["portal"])
	if got := r.msgs(r.anna); !hasMsg(got, "teleport") || r.pa.X != 4242.0 {
		t.Fatalf("Anna at the step: %q", got)
	}
	if s, _ := r.step(r.pc, 101303); s != 6 || r.pc.X != r.pa.X {
		t.Errorf("Cora, one behind: step %d at (%.1f, %.1f), want 6 with Anna", s, r.pc.X, r.pc.Z)
	}
}

// A Sky chain's kill step: a kill credited to one player is done for a
// groupmate by it on the step; one credited to both counts once for each (the
// next step, of the same creature, untouched).
func TestASkyChainKillStepIsTheGroups(t *testing.T) {
	r := newSkyRig(t)
	if _, err := r.db.Exec(`INSERT OR REPLACE INTO queststeps (questid, step, kind, target, journal) VALUES
		(101303, 7, 'kill', 'Necrologist', 'Kill the necrologist.'), (101303, 8, 'kill', 'Necrologist', 'Kill him again.'),
		(101303, 9, 'talk', 'Nobody', 'Then nothing.')`); err != nil {
		t.Fatal(err)
	}
	r.group()
	r.on(r.pa, 101303, 7)
	r.on(r.pc, 101303, 7)
	r.by("portal")
	kp := func(s *session.Session, p *entity.Player) entity.KillPlayer {
		return entity.KillPlayer{Session: s, ServerID: p.CharServerID, Name: p.Name}
	}
	Quests().killed(entity.KillCredit{NPCName: "Necrologist", World: 4, Players: []entity.KillPlayer{kp(r.anna, r.pa)}})
	if a, _ := r.step(r.pa, 101303); a != 8 {
		t.Fatalf("Anna's step: %d, want 8", a)
	}
	if c, _ := r.step(r.pc, 101303); c != 8 {
		t.Errorf("Cora's step, by Anna: %d, want 8", c)
	}
	Quests().killed(entity.KillCredit{NPCName: "Necrologist", World: 4, Players: []entity.KillPlayer{kp(r.anna, r.pa), kp(r.cora, r.pc)}})
	a, _ := r.step(r.pa, 101303)
	c, _ := r.step(r.pc, 101303)
	if a != 9 || c != 9 {
		t.Errorf("a kill credited to both: Anna at %d, Cora at %d, want 9 each", a, c)
	}
}
