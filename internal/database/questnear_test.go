package database

import (
	"strings"
	"testing"
)

// A near event (EventNear, Heirloom petition 3332's fortress wake) is a step's: one firing a trigger at its own
// place, or with none at its step's, has no problem; a quest's near, a near spawn or teleport, a near give that is
// not unless_held, and a negative radius each have one.
func TestNearEventProblems(t *testing.T) {
	for _, ok := range []string{
		`[{"on":"near","do":"trigger","trigger":"Tallon's Fortress","world":5,"x":4715,"z":7500,"radius":110,"text":"Awake already."}]`,
		`[{"on":"near","do":"trigger","trigger":"Fortress Post"}]`,
		`[{"on":"near","do":"popup","text":"The doors grind."},{"on":"near","do":"give","items":"1*1","unless_held":true}]`,
	} {
		if p := EventProblems(ok, true); len(p) != 0 {
			t.Errorf("%s: %q, want none", ok, p)
		}
	}
	for _, c := range []struct{ evs, want string }{
		{`[{"on":"near","do":"trigger","trigger":"T"}]`, `on "near" is not one of`}, // a quest's (step false below)
		{`[{"on":"near","do":"spawn","npc":"Zek sentry"}]`, "a near spawn is not done"},
		{`[{"on":"near","do":"teleport","x":1,"z":1}]`, "a near teleport is not done"},
		{`[{"on":"near","do":"give","items":"1*1"}]`, "a near give is unless_held"},
		{`[{"on":"near","do":"trigger","trigger":"T","x":1,"z":1,"radius":-1}]`, "its radius -1 is below 0"},
	} {
		p := EventProblems(c.evs, !strings.Contains(c.want, "is not one of"))
		if !strings.Contains(strings.Join(p, "\n"), c.want) {
			t.Errorf("%s: %q, want %q", c.evs, p, c.want)
		}
	}
}

// NearPlace is the event's own place (its radius, NearReach when 0; no height) when its x or z is not 0, else its
// step's (its height; its radius unless the event names one); none when the step has no place either.
func TestNearPlace(t *testing.T) {
	st := &QuestStep{World: 5, X: 4715, Y: 185, Z: 7500, Radius: 110, Height: 20}
	if pl, ok := NearPlace(st, QuestEvent{World: 5, X: 4730, Z: 7600}); !ok || pl != (QuestPlace{World: 5, X: 4730, Z: 7600, Radius: NearReach}) {
		t.Errorf("own place: %+v %v", pl, ok)
	}
	if pl, ok := NearPlace(st, QuestEvent{}); !ok || pl != (QuestPlace{World: 5, X: 4715, Y: 185, Z: 7500, Radius: 110, Height: 20}) {
		t.Errorf("the step's: %+v %v", pl, ok)
	}
	if pl, ok := NearPlace(st, QuestEvent{Radius: 50}); !ok || pl.Radius != 50 || pl.X != 4715 {
		t.Errorf("the step's, its own radius: %+v %v", pl, ok)
	}
	if _, ok := NearPlace(&QuestStep{Radius: 15}, QuestEvent{}); ok {
		t.Error("a step with no place gave one")
	}
}
