package handlers

import (
	"fmt"
	"strings"
	"testing"
)

// A reach step that names items is done only with them held, and takes them at its doing: the primal stew of
// "The Scepter of the Ritualists", set down in Permafrost to raise the enraged Lady Vox
// (tools/enraged_vox_20261116.py). The player there without the stew is told the step's reminder once in two
// looks and the step stays; the stew in the pack, the next look does the step and the stew is gone. The control:
// a reach step with no items (TestAReachStepWaitsForRoom) is done on arrival, taking nothing.
func TestAReachStepWithItemsTakesThem(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP, f.tw.p.MaxHP = 100, 100 // a dead player stands nowhere
	stew := f.pattern("primal stew", 1)
	other := f.pattern("bone dust", 20)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (87, 'The Scepter', 1, 'Mistress Sanchez')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, items, world, x, z, radius, journal, reminder, events) VALUES
		(87, 1, 'reach', ?, 0, 25403, 15725, 40, 'Set the primal stew down.', 'You need the primal stew to set down here.',
		'[{"on":"done","do":"popup","text":"You set the primal stew down."}]')`, fmt.Sprint(stew))
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (87, 2, 'talk', 'Mistress Sanchez', 'Return.')`)
	f.qe.Reload()
	f.own(other, 3)
	if _, err := f.qe.SetCharacterQuest(f.sid, 87, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	// there, no stew: told once, the step stays
	f.qe.checkReach()
	f.qe.checkReach()
	if s, _ := f.progress(87); s != 1 {
		t.Fatalf("no stew: step %d, want still 1", s)
	}
	told := 0
	for _, m := range f.msgs() {
		if strings.Contains(m, "You need the primal stew to set down here.") {
			told++
		}
	}
	if told != 1 {
		t.Errorf("told %d times in two looks, want once", told)
	}

	// the stew held: done, and taken (nothing else)
	f.own(stew, 1)
	f.qe.checkReach()
	if s, _ := f.progress(87); s != 2 {
		t.Fatalf("with the stew: step %d, want 2", s)
	}
	if n := f.held(stew); n != 0 {
		t.Errorf("the stew still held (%d): a reach step's items are set down", n)
	}
	if n := f.held(other); n != 3 {
		t.Errorf("bone dust %d, want 3 kept", n)
	}
	popup := false
	for _, m := range f.msgs() {
		popup = popup || strings.HasSuffix(m, ":You set the primal stew down.|OK")
	}
	if !popup {
		t.Error("the step's done popup was not shown")
	}
}
