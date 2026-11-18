package entity

import (
	"strings"
	"testing"
)

// The Positronian Mace's words (tools/valkanith_loot_20261118.py, t299's "500dmg undead proc") read as a 500
// hit on undead alone: an undead creature by undead.go's rule (npcundead or the name), not one whose name
// says "undead"; a goblin proc's word stays a name's.
func TestUndeadOnlyWeaponProc(t *testing.T) {
	wp, ok := WeaponProcFromText("This mace had been imbued long ago with vast amounts of positive energy by the tal'thex " +
		"elves. It causes great harm to undead. Procs a 500 point DD, works only on undead.")
	if !ok || wp.Kind != ProcHit || wp.Damage != 500 || wp.Only != procOnUndead {
		t.Fatalf("the mace's proc: %+v %v", wp, ok)
	}
	w := NewWorld()
	w.undead = map[string]bool{"lord valkanith": true}
	if !w.procOnLocked(&wp, &NPC{Name: "Lord Valkanith"}) || !w.procOnLocked(&wp, &NPC{Name: "a decaying skeleton"}) {
		t.Error("no proc on the undead")
	}
	if w.procOnLocked(&wp, &NPC{Name: "a gnoll pup"}) {
		t.Error("a proc on the living")
	}
	gob := WeaponProc{Only: "goblin"}
	if !w.procOnLocked(&gob, &NPC{Name: "a goblin warrior"}) || w.procOnLocked(&gob, &NPC{Name: "a gnoll"}) {
		t.Error("a goblin's proc")
	}
	if s := wp.String(); s == "" || !strings.Contains(s, "on undead alone") {
		t.Errorf("its words: %q", s)
	}
}
