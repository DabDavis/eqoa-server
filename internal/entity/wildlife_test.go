package entity

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Only wildlife wanders by its kind (wildlife.go; F3 20261026-042712): an
// animal by its name's head noun.
func TestWildlifeWord(t *testing.T) {
	for name, want := range map[string]string{
		"Giant Rat":                   "rat",
		"rat":                         "rat",
		"Wolf Pack Leader":            "wolf",
		"Bloodfin Brood Shark Mother": "shark",
		"red ant warrior":             "ant",
		"Vulture Chick":               "vulture",
		"Lioness Stalker":             "lioness",
		"dire wolves":                 "wolves",
		"Scorchflies":                 "scorchflies",
		"jade hornfly":                "hornfly",
		"Black Widow":                 "widow",
		"Young Rattler":               "rattler",
		"Skeletal Wolf":               "wolf", // a word of wildlife's, but undead: wildlifeLocked says no
		"Fire Drake":                  "drake",
		"Fire Giant":                  "",
		"fire giant reaver":           "",
		"Lord Nagafen":                "",
		"Orc Pawn":                    "",
		"Deathfist reaver":            "",
		"gnoll scout":                 "",
		"Kobold Slayer":               "",
		"hill giant":                  "",
		"serpent guardian":            "",
		"griffon nest protector":      "",
		"mindwhipper guard":           "",
		"lizardman crusader":          "",
		"ratman warrior":              "",
		"werewolf":                    "",
		"cyclone behemoth":            "",
		"Scarecrow":                   "",
		"Hexbone":                     "",
		"aviak hatcher":               "",
		"":                            "",
	} {
		if got := WildlifeWord(name); got != want {
			t.Errorf("WildlifeWord(%q) = %q, want %q", name, got, want)
		}
	}
}

// By their kind, a fire giant, an orc and an undead wolf stand on their
// spawns for two minutes beside a player; a giant rat and a wolf pack leader
// wander (the control).
func TestOnlyWildlifeWandersByItsKind(t *testing.T) {
	w := roamWorld(t)
	still := []*NPC{
		addNPC(w, 101, "Fire Giant", 1005, 1000, creatureModel, 0),
		addNPC(w, 102, "Orc Pawn", 1006, 1000, creatureModel, 0),
		addNPC(w, 103, "Skeletal Wolf", 1007, 1000, creatureModel, 0),
		addNPC(w, 104, "serpent guardian", 1008, 1000, creatureModel, 0),
	}
	moved := []*NPC{
		addNPC(w, 105, "Giant Rat", 1009, 1000, creatureModel, 0),
		addNPC(w, 106, "Wolf Pack Leader", 1010, 1000, creatureModel, 0),
	}
	run(w, time.Now(), 2*time.Minute, nil)
	for _, npc := range still {
		if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || npc.moving() {
			t.Errorf("%q wandered to (%.1f, %.1f)", npc.Name, npc.X, npc.Z)
		}
	}
	for _, npc := range moved {
		if npc.X == npc.SpawnX && npc.Z == npc.SpawnZ {
			t.Errorf("%q never moved: the control", npc.Name)
		}
	}
}

// A GM's undead tag makes a creature of an animal's name stand; taken back it
// wanders again (the verdicts are worked out again).
func TestAnUndeadTagStandsAnAnimal(t *testing.T) {
	w := roamWorld(t)
	rat := addNPC(w, 100, "Grave Rat", 1005, 1000, creatureModel, 0)
	w.mu.Lock()
	before := w.wanders(rat)
	w.mu.Unlock()
	w.SetUndeadTags(map[string]bool{"grave rat": true})
	w.mu.Lock()
	tagged := w.wanders(rat)
	w.mu.Unlock()
	w.SetUndeadTags(nil)
	w.mu.Lock()
	after := w.wanders(rat)
	w.mu.Unlock()
	if !before || tagged || !after {
		t.Errorf("wanders: untagged %v, tagged undead %v, untagged again %v; want true, false, true", before, tagged, after)
	}
}

// Creature movement's Wanders 1 walks a camp's people on a player race's body
// (Klik'Anon's pirate patrols) and anything not wildlife; never an NPC players
// talk to (the NPC bit), whose way to move is a path.
func TestARoamerRowWalksACampButNotTheNPCBit(t *testing.T) {
	w := roamWorld(t)
	w.SetMoveRules(map[string]database.NPCMoveRule{"deckhand": {Roam: true}, "fire giant": {Roam: true}, "merchant tolly": {Roam: true}})
	deck := addNPC(w, 101, "deckhand", 1005, 1000, humanModel, 0)
	giant := addNPC(w, 102, "Fire Giant", 1006, 1000, creatureModel, 0)
	merchant := addNPC(w, 103, "Merchant Tolly", 1007, 1000, humanModel, 0x81)
	plain := addNPC(w, 104, "deckhand mate", 1008, 1000, humanModel, 0) // no row: townsfolk
	run(w, time.Now(), 2*time.Minute, nil)
	for _, npc := range []*NPC{deck, giant} {
		if npc.X == npc.SpawnX && npc.Z == npc.SpawnZ {
			t.Errorf("%q, told roam 1, never moved", npc.Name)
		}
	}
	for _, npc := range []*NPC{merchant, plain} {
		if npc.X != npc.SpawnX || npc.Z != npc.SpawnZ {
			t.Errorf("%q wandered to (%.1f, %.1f)", npc.Name, npc.X, npc.Z)
		}
	}
}

// The GM panel's NPC page says whether it wanders and why.
func TestMovementSaysWhy(t *testing.T) {
	w := roamWorld(t)
	w.SetMoveRules(map[string]database.NPCMoveRule{"hexbone": {Roam: false}})
	for _, c := range []struct {
		npc  *NPC
		want string
	}{
		{addNPC(w, 101, "Giant Rat", 1005, 1000, creatureModel, 0), `it wanders: wildlife (its name's "rat")`},
		{addNPC(w, 102, "Fire Giant", 1006, 1000, creatureModel, 0), "not wildlife: it stands"},
		{addNPC(w, 103, "Hexbone", 1007, 1000, creatureModel, 0), "Creature movement says so"},
		{addNPC(w, 104, "Merchant Tolly", 1008, 1000, humanModel, 0x81), "the NPC bit"},
		{addNPC(w, 105, "Guard Dahvel", 1009, 1000, humanModel, 0), "townsfolk"},
		{addNPC(w, 106, "Skeletal Wolf", 1010, 1000, creatureModel, 0), "undead"},
	} {
		st, _ := w.NPCStateOf(c.npc.ServerID)
		if !strings.Contains(st.Movement, c.want) {
			t.Errorf("%s: %q, want it to say %q", c.npc.Name, st.Movement, c.want)
		}
	}
}

// tools/seed_npc_move.py writes rows for the animals these words miss, so it
// must hold the same three lists.
func TestSeedToolKeepsTheWildlifeLists(t *testing.T) {
	src, err := os.ReadFile("../../tools/seed_npc_move.py")
	if err != nil {
		t.Fatal(err)
	}
	for py, set := range map[string]map[string]bool{
		"WILDLIFE_NOUNS": wildlifeNouns, "WILDLIFE_TITLES": wildlifeTitles, "WILDLIFE_GUARDS": wildlifeGuards,
	} {
		m := regexp.MustCompile(`(?s)\b` + py + ` = set\("""(.*?)"""`).FindSubmatch(src)
		if m == nil {
			t.Errorf("tools/seed_npc_move.py has no %s", py)
			continue
		}
		got := wordSet(string(m[1]))
		if missing, extra := setDiff(set, got), setDiff(got, set); len(missing)+len(extra) > 0 {
			t.Errorf("%s: the tool lacks %v and has %v that wildlife.go has not", py, missing, extra)
		}
	}
}

func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
