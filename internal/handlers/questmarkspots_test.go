package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

func spotAt(id int, name string, world uint8, x, z float32) entity.NPCSpot {
	return entity.NPCSpot{ID: entity.EntityID(id), World: world, X: x, Z: z, Name: name}
}

// A name's spawns within huntCampLink of one another are one camp (linked, so a
// line of them is too), marked at the spawn nearest its middle; the biggest
// first; another world's spawns are never in a camp with these.
func TestCampsOfLinkSpawns(t *testing.T) {
	spots := []entity.NPCSpot{
		spotAt(1, "rat", 0, 0, 0), spotAt(2, "rat", 0, 100, 0), spotAt(3, "rat", 0, 200, 0), // a line: one camp
		spotAt(4, "rat", 0, 1000, 0), // alone
		spotAt(5, "rat", 2, 50, 0),   // another world
	}
	camps := campsOf(spots)
	if len(camps) != 3 || camps[0].n != 3 || camps[0].at.ID != 2 {
		t.Fatalf("camps %+v: want the line of three first, marked at its middle (2)", camps)
	}
	if camps[1].n != 1 || camps[2].n != 1 {
		t.Errorf("the lone one and the other world's: %+v", camps[1:])
	}
}

// A kill or loot step's mark is its creatures' nearest camp, not a stray nearer
// the player; every name the step counts makes the camps together; the world's
// biggest counts where none has huntCampFew; none in the player's world: the
// biggest elsewhere.
func TestHuntMarkIsTheCamp(t *testing.T) {
	q := &QuestEngine{}
	spots := map[string][]entity.NPCSpot{
		"orc pawn":  {spotAt(1, "orc pawn", 0, 10, 0), spotAt(2, "orc pawn", 0, 900, 0), spotAt(3, "orc pawn", 0, 960, 40)},
		"orc scout": {spotAt(4, "orc scout", 0, 930, 80), spotAt(5, "orc scout", 0, 3000, 0)},
		"gharial":   {spotAt(6, "gharial", 2, 0, 0), spotAt(7, "gharial", 2, 50, 0), spotAt(8, "gharial", 2, 5000, 0)},
	}
	st := &database.QuestStep{Kind: database.StepKill, Target: "orc pawn", Also: []string{"orc scout"}}
	kind, at := q.stepSpotsLocked(spots, st, 0, 0, 0, 0)
	if kind != markHunt || len(at) != 1 || at[0].World != 0 || at[0].X < 890 {
		t.Errorf("the pawns' and scouts' camp at 900-960, not the stray pawn at 10: %v", at)
	}
	// the control: only strays in the world: the nearest of them
	lone := &database.QuestStep{Kind: database.StepLoot, Target: "orc scout"}
	if _, at := q.stepSpotsLocked(spots, lone, 0, 0, 2900, 0); len(at) != 2 || at[0].ID != 5 {
		t.Errorf("only lone scouts: the nearest first: %v", at)
	}
	// none in the player's world: the biggest camp of another
	far := &database.QuestStep{Kind: database.StepKill, Target: "gharial"}
	if _, at := q.stepSpotsLocked(spots, far, 0, 0, 0, 0); len(at) != 1 || at[0].World != 2 || at[0].X > 100 {
		t.Errorf("gharials in world 2 only: their camp there: %v", at)
	}
	// a pinned spawn is marked
	pin := &database.QuestStep{Kind: database.StepKill, Target: "orc pawn", NPCID: 1}
	if _, at := q.stepSpotsLocked(spots, pin, 0, 0, 0, 0); len(at) != 1 || at[0].ID != 1 {
		t.Errorf("the pinned pawn: %v", at)
	}
}

// A talk, show or give step's mark is its NPC where one stands, before the
// spawn-only original of the name; a pinned spawn alone; the originals where
// none stands (the step's spawns come again by them).
func TestStepMarkPrefersStanding(t *testing.T) {
	q := &QuestEngine{}
	orig := spotAt(1, "Jarek", 0, 10, 0)
	orig.SpawnOnly = true
	spots := map[string][]entity.NPCSpot{
		"jarek": {orig, spotAt(2, "Jarek", 0, 500, 0)},
		"nox":   {orig},
		"armis": {spotAt(3, "Armis", 0, 0, 0), spotAt(4, "Armis", 0, 800, 0)},
	}
	st := &database.QuestStep{Kind: database.StepTalk, Target: "Jarek"}
	if _, at := q.stepSpotsLocked(spots, st, 0, 0, 0, 0); len(at) != 1 || at[0].ID != 2 {
		t.Errorf("the standing Jarek, not the original nearer: %v", at)
	}
	only := &database.QuestStep{Kind: database.StepTalk, Target: "nox"}
	if _, at := q.stepSpotsLocked(spots, only, 0, 0, 0, 0); len(at) != 1 || at[0].ID != 1 {
		t.Errorf("an original alone: it: %v", at)
	}
	pin := &database.QuestStep{Kind: database.StepGive, Target: "Armis", NPCID: 4}
	if _, at := q.stepSpotsLocked(spots, pin, 0, 0, 0, 0); len(at) != 1 || at[0].ID != 4 {
		t.Errorf("the pinned Armis: %v", at)
	}
	reach := &database.QuestStep{Kind: database.StepReach, World: 2, X: 5, Z: 6, Journal: "the cave"}
	if _, at := q.stepSpotsLocked(spots, reach, 0, 0, 0, 0); len(at) != 1 || at[0].World != 2 || at[0].X != 5 || at[0].Z != 6 {
		t.Errorf("a reach step's place: %v", at)
	}
}

// A step in another world: the way there is the nearest travel NPC of the
// player's world whose trips lead there in the fewest (wayRoutes: Freeport's
// Dockmaster Yarnning to Arcadin, where Dockmaster Killeron stands, in Odus);
// none where no trip leads.
func TestWayToAnotherWorld(t *testing.T) {
	q := &QuestEngine{}
	spots := map[string][]entity.NPCSpot{
		"dockmaster yarnning": {spotAt(1, "Dockmaster Yarnning", 0, 1000, 1000)},
		"dockmaster killeron": {spotAt(2, "Dockmaster Killeron", 2, 50, 50)},
		"dockmaster bibbin":   {spotAt(3, "Dockmaster Bibbin", 2, 70, 70)},
	}
	h, ok := q.wayToLocked(spots, 0, 2, 0, 0)
	if !ok || h.from.ID != 1 || h.to != 2 {
		t.Fatalf("Freeport's dockmaster to Odus: %+v %v", h, ok)
	}
	if got := wayLine(h, ""); got != "Travel with Dockmaster Yarnning to Arcadin" && got != "Travel with Dockmaster Yarnning to Stone of Morathals" {
		t.Errorf("its line %q", got)
	}
	if _, ok := q.wayToLocked(spots, 0, 9, 0, 0); ok {
		t.Errorf("no trip to world 9: no way")
	}
	if _, ok := q.wayToLocked(spots, 0, 0, 0, 0); ok {
		t.Errorf("the player's own world: no way")
	}
}
