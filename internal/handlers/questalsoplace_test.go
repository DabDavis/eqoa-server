package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Heirloom petition 2718: the River Saren tower's quests take an undead sigil
// off its tomb rats, and the rat families standing there (Also); "large rat"
// stands all over the world, and Halas's dropped the sigils. A step with a
// place of its own takes its others only within huntPlaceReach of it; its own
// target anywhere. The controls: a step with no place takes its others
// anywhere, and an area's any-creature step its others anywhere too.
func TestAPlacedStepTakesItsOthersNearIt(t *testing.T) {
	st := &database.QuestStep{Kind: database.StepLoot, Target: "tomb rat", Also: []string{"large rat"}, World: 0, X: 23943, Z: 8650, Radius: 15}
	kill := func(name string, x, z float32) entity.KillCredit {
		return entity.KillCredit{NPCName: name, World: 0, X: x, Z: z}
	}
	for _, tc := range []struct {
		what string
		st   *database.QuestStep
		k    entity.KillCredit
		want bool
	}{
		{"a large rat by the tower", st, kill("large rat", 23500, 9000), true},
		{"a large rat in Halas", st, kill("large rat", 12926, 4396), false},
		{"a large rat in another world by the tower's place", st, entity.KillCredit{NPCName: "large rat", World: 1, X: 23943, Z: 8650}, false},
		{"a tomb rat far off: its own target", st, kill("tomb rat", 7700, 17470), true},
		{"a crab by the tower", st, kill("Desert Crab", 23943, 8650), false},
		{"the control: no place", &database.QuestStep{Kind: database.StepLoot, Target: "tomb rat", Also: []string{"large rat"}}, kill("large rat", 12926, 4396), true},
		{"the control: an area's step", &database.QuestStep{Kind: database.StepLoot, Target: database.AnyCreature, Also: []string{"Teir'Dal knight"},
			X: 23601, Z: 23711, Radius: 45}, kill("Teir'Dal knight", 18138, 27329), true},
	} {
		if got := stepKill(tc.st, tc.k); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.what, got, tc.want)
		}
	}
}
