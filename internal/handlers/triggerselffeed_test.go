package handlers

import (
	"testing"
	"time"
)

// A trigger fired again by its own spawns' deaths (an npcdeath row naming the
// trigger that spawned the dead creature) is capped at selfRefireMax an hour,
// so a death row bringing two for one can no longer grow without end (the
// Cazic Thule phantasms, 2026-11-22). The controls: another trigger has its own
// count, and an hour on the cap is free again.
func TestASelfFeedingTriggerIsCappedAnHour(t *testing.T) {
	w := newTrigWorld(t)
	w.at(0)
	for i := 0; i < selfRefireMax; i++ {
		if !w.e.selfRefireOK("Phantasm Splits") {
			t.Fatalf("refire %d refused under the cap", i+1)
		}
	}
	if w.e.selfRefireOK("phantasm splits ") {
		t.Fatal("refire past the cap (any case, spaces) taken")
	}
	if !w.e.selfRefireOK("Khal Hound") {
		t.Fatal("the control: another trigger refused by the first's count")
	}
	w.at(61 * time.Minute)
	if !w.e.selfRefireOK("Phantasm Splits") {
		t.Fatal("an hour on, the trigger is still refused")
	}
}
