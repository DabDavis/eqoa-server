package entity

import "testing"

// The zombie training dummies (tools/petpass_both_1124ao.py, Heirloom petition
// 2877: "flag training dummys as undead please") are Training Dummies by their
// name, so they never die and heal after each hit, and undead by it too, with
// or without their npcundead rows.
func TestZombieTrainingDummyIsDummyAndUndead(t *testing.T) {
	for _, name := range []string{"Training Dummy Zombie Lv20", "Training Dummy Zombie Lv50", "Training Dummy Zombie Lv60"} {
		if k := DummyKindOf(name); k != TrainingDummy {
			t.Errorf("%q: kind %d, want TrainingDummy", name, k)
		}
		if !undeadName(name) {
			t.Errorf("%q: name not undead", name)
		}
		w := &World{undead: map[string]bool{}}
		if !w.undeadLocked(&NPC{Name: name}) {
			t.Errorf("%q: undeadLocked false with no row", name)
		}
	}
	for _, name := range []string{"Training Dummy Lv20", "Training Dummy Jib Lv60", "Regular Dummy Lv55"} {
		if undeadName(name) {
			t.Errorf("%q: an existing dummy reads undead", name)
		}
	}
}
