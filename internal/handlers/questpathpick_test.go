package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A path quest's pick of another path than the one the Spell Archivist wrote is refused (Heirloom
// petition 3200: a necromancer took Blood from the archivist, then Power at the level 20 quest, and
// held both); its pick of the same path is taken; and a path held by a quest's pick alone refuses
// nothing (petition 1325's "unless they do the quests manually").
func TestQuestPathPickAgainstTheArchivistsPath(t *testing.T) {
	f := newPathRig(t, 30)
	jet := &database.StepOption{Answer: "Power Jet", Reward: database.ClassReward{Spells: []int{spPowerJet}}}
	expansive := &database.StepOption{Answer: "Expansive Mind", Reward: database.ClassReward{Spells: []int{spExpansive}}}
	other := &database.StepOption{Answer: "A sword"}
	refused := func(o *database.StepOption) (string, bool) {
		f.qe.mu.Lock()
		defer f.qe.mu.Unlock()
		return f.qe.pathPickRefusedLocked(f.p, o)
	}
	if _, r := refused(jet); r {
		t.Fatal("refused with no path in the book")
	}
	f.know(spExpansive, 2, -1) // the archivist's Expansive Mind: no quest gave it
	if line, r := refused(jet); !r || line != "Expansive Mind" {
		t.Fatalf("Power Jet over the archivist's Expansive Mind: refused %v (%q)", r, line)
	}
	if _, r := refused(expansive); r {
		t.Fatal("the held path's own pick refused")
	}
	if _, r := refused(other); r {
		t.Fatal("an option of no path refused")
	}
	f.done(qEnc30, "1:0") // a quest's pick gave Expansive Mind: it is the quests', not the archivist's
	if _, r := refused(jet); r {
		t.Fatal("refused over a path a quest gave")
	}
}
