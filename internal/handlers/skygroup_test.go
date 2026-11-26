package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Each player does its own Sky chain steps (the owner, 2026-11-26, Heirloom
// petition 3146: "Talker only"): a step one player does at an NPC is its own;
// a groupmate by it on the same step stays on it until it does it itself.
func TestASkyChainStepIsTheTalkersAlone(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.essence("singe", "Singe Burnfeathers", 4611.2, 113.0, 4939.0)
	r.on(r.pa, 101303, 4)
	r.on(r.pc, 101303, 4)
	r.by("singe")
	r.talk(r.anna, r.npcs["singe"])
	if s, _ := r.step(r.pa, 101303); s != 5 {
		t.Fatalf("Anna's step: %d, want 5 (%q)", s, r.msgs(r.anna))
	}
	if s, _ := r.step(r.pc, 101303); s != 4 {
		t.Errorf("Cora's step, by Anna in her group: %d, want 4 (her own talk to do)", s)
	}
	r.talk(r.cora, r.npcs["singe"])
	if s, _ := r.step(r.pc, 101303); s != 5 {
		t.Errorf("Cora's step after her own talk: %d, want 5 (%q)", s, r.msgs(r.cora))
	}
}

// A Sky chain quest one player takes is the taker's alone.
func TestASkyChainQuestIsTakenByTheTakerAlone(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.essence("singe", "Singe Burnfeathers", 4611.2, 113.0, 4939.0)
	r.by("singe")
	r.talk(r.anna, r.npcs["singe"])
	got := r.msgs(r.anna)
	if s, _ := r.step(r.pa, 101303); s != 4 {
		t.Fatalf("Anna did not take the quest: %q", got)
	}
	if s, _ := r.step(r.pc, 101303); s != -1 {
		t.Errorf("Cora, in Anna's group, has the quest at %d", s)
	}
}

// At Granavik's portal a player one step behind the step its way back waits
// for goes back, a groupmate past that step or not.
func TestGranaviksPortalSendsTheOneBehindBack(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.on(r.pa, 101303, 4)
	r.on(r.pc, 101303, 5)
	r.own(r.pa, "Rain Key 3", 20)
	r.by("portal")
	r.talk(r.anna, r.npcs["portal"])
	if s, _ := r.step(r.pa, 101303); s != 4 {
		t.Errorf("Anna's step: %d, want 4 (behind, sent back)", s)
	}
	if n := r.held(r.pa, "Rain Key 3"); n != 20 {
		t.Errorf("Anna holds %d keys, want 20 (none taken on the way back)", n)
	}
	if r.pa.X == 4242.0 && r.pa.Z == 4305.3 {
		t.Errorf("Anna went on through the portal")
	}
}

// A Sky chain's kill step is shared as every quest's is: a kill credited to
// the killer and a groupmate (entity/questhooks.go: within KillShareRange)
// counts for each; a kill credited to the killer alone counts for it alone.
func TestASkyChainKillCountsForThoseItIsCreditedTo(t *testing.T) {
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
	if c, _ := r.step(r.pc, 101303); c != 7 {
		t.Errorf("Cora's step, the kill not credited to her: %d, want 7", c)
	}
	Quests().killed(entity.KillCredit{NPCName: "Necrologist", World: 4, Players: []entity.KillPlayer{kp(r.anna, r.pa), kp(r.cora, r.pc)}})
	a, _ := r.step(r.pa, 101303)
	c, _ := r.step(r.pc, 101303)
	if a != 9 || c != 8 {
		t.Errorf("a kill credited to both: Anna at %d, Cora at %d, want 9 and 8", a, c)
	}
}
