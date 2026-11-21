package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// A talk, show or give step is done at any of its NPCs (queststeptargets, as
// a kill or loot step counts any of its creatures): the guide's red diamond,
// "Purchase a RED Diamond from either Grithmil Darkstone in Moradhim or Tsekar
// in Gree Rift" (eqoa.forumotion.com t107). The controls: an NPC the step does
// not name takes nothing, nor does a reach step's other name do anything.
func TestAGiveStepIsDoneAtEitherOfItsNPCs(t *testing.T) {
	for _, at := range []string{"Seller One", "seller two "} {
		t.Run(at, func(t *testing.T) {
			f := newQuestFixture(t)
			f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (50, 'The diamond', 1, 'Giver Test')`)
			f.exec(`INSERT INTO queststeps (questid, step, kind, target, tunar, journal, says)
				VALUES (50, 1, 'give', 'Seller One', 1450, 'Buy the diamond.', 'A diamond.')`)
			f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (50, 2, 'talk', 'Giver Test', 'Back to the giver.')`)
			f.otherCreatures(50, 1, "Seller Two")
			if _, err := f.qe.SetCharacterQuest(f.sid, 50, QuestGive, 0); err != nil {
				t.Fatal(err)
			}
			f.msgs()
			f.tw.p.Tunar = 2000
			other := f.tw.add(3, &entity.NPC{ServerID: 801, Name: "Seller Three", SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15726})
			f.tw.talk(other)
			f.msgs()
			if s, _ := f.progress(50); s != 1 || f.tw.p.Tunar != 2000 {
				t.Fatalf("at an NPC the step does not name: step %d, tunar %d", s, f.tw.p.Tunar)
			}
			seller := f.tw.add(3, &entity.NPC{ServerID: 802, Name: at, SpawnX: 25403, SpawnY: 54.1, SpawnZ: 15727})
			f.tw.talk(seller)
			m := f.msgs()
			if s, _ := f.progress(50); s != 2 || f.tw.p.Tunar != 550 {
				t.Fatalf("at %q: step %d, tunar %d (%q)", at, s, f.tw.p.Tunar, m)
			}
		})
	}
}

// The step's others are among its names for its marks and lines (Targets), a
// reach step's none.
func TestAnAtNPCStepsTargetsHoldItsOthers(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (51, 'Names', 1, 'Giver Test')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target) VALUES (51, 1, 'give', 'Seller One')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, x, z, radius) VALUES (51, 2, 'reach', '', 1, 1, 5)`)
	f.otherCreatures(51, 1, "Seller Two")
	f.otherCreatures(51, 2, "Nobody")
	qu := f.qe.cat.byID[51]
	if got := qu.Steps[0].Targets(); len(got) != 2 || got[1] != "Seller Two" {
		t.Errorf("the give step's names: %q", got)
	}
	if got := qu.Steps[1].Targets(); len(got) != 0 {
		t.Errorf("a reach step's names: %q", got)
	}
}

// The step's marks stand on both sellers; the control: none on an NPC it
// does not name.
func TestAGiveStepsMarksStandOnEitherNPC(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.tw.add(3, &entity.NPC{ServerID: 811, Name: "Seller One", SpawnX: 25450, SpawnZ: 15725})
	f.tw.add(4, &entity.NPC{ServerID: 812, Name: "Seller Two", SpawnX: 25600, SpawnZ: 15725})
	f.tw.add(5, &entity.NPC{ServerID: 813, Name: "Seller Three", SpawnX: 25420, SpawnZ: 15725})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (52, 'The diamond', 1, 'Giver Test', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, tunar, journal) VALUES (52, 1, 'give', 'Seller One', 10, 'Buy it.')`)
	f.otherCreatures(52, 1, "Seller Two")
	if _, err := f.qe.SetCharacterQuest(f.sid, 52, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(t, f.tw.s)
	got := map[string]bool{}
	for _, m := range marks {
		if m.kind == markStep {
			got[m.npc] = true
		}
	}
	if !got["Seller One"] || !got["Seller Two"] || got["Seller Three"] {
		t.Errorf("the step's marks are on %v, want both sellers and not the third", got)
	}
}
