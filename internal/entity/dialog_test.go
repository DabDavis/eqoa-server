package entity

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The roles by the speaker's name, the first role a word names; a citizen
// when none does.
func TestNPCRole(t *testing.T) {
	for name, want := range map[string]string{
		"Guard Dahvel": RoleGuard, "Deputy Rorgan": RoleGuard, "Oggok Basher": RoleGuard, "Captain Rohand": RoleGuard,
		"Merchant Ulla": RoleTrader, "Smithy Grenn": RoleTrader, "Banker Rom": RoleTrader,
		"Acolyte Sindra": RolePriest, "Historian Lorn": RoleScholar, "Senator Hastel": RoleNoble,
		"Farmer Mill": RoleWorker, "Miner Tok": RoleWorker, "Sabeth Riverhand": RoleCitizen, "": RoleCitizen,
	} {
		if got := NPCRole(name); got != want {
			t.Errorf("%q: %s, want %s", name, got, want)
		}
	}
}

func TestDialogTown(t *testing.T) {
	for f, want := range map[string]string{
		"Citizens of Freeport": "Freeport", "Citizens of Klick`Anon": "Klick`Anon", "The Qeynos Guard": "Qeynos",
		"House U`Dedne": "Neriak", "Lord Nagafen": "", "": "",
	} {
		if got := DialogTown(f); got != want {
			t.Errorf("%q: %q, want %q", f, got, want)
		}
	}
}

func TestDialogMoodFits(t *testing.T) {
	for _, c := range []struct {
		line, mood string
		fits       bool
	}{
		{"", MoodNeutral, true}, {"", MoodWarm, true}, {"", MoodFriendly, true}, {"", MoodWary, false},
		{"", MoodHostile, false}, {"hostile", MoodHostile, true}, {"wary, hostile", MoodHostile, true},
		{"warm", MoodNeutral, false}, {"Friendly,Warm", MoodWarm, true},
	} {
		if got := DialogMoodFits(c.line, c.mood); got != c.fits {
			t.Errorf("%q in %s: %v", c.line, c.mood, got)
		}
	}
}

// A town's own line counts three times a generic one, its role's twice, a
// line for the speaker's name six times; a line of another town, role,
// race or mood is not said.
func TestDialogCandidates(t *testing.T) {
	lines := []database.NPCDialogLine{
		{ID: 1, Line: "Any.", Race: -1, Weight: 1},
		{ID: 2, Town: "Freeport", Line: "Freeport.", Race: -1, Weight: 1},
		{ID: 3, Town: "Freeport", Role: RoleGuard, Line: "A Freeport guard.", Race: -1, Weight: 1},
		{ID: 4, Name: "guard dahvel", Line: "Dahvel.", Race: -1, Weight: 1},
		{ID: 5, Town: "Qeynos", Line: "Qeynos.", Race: -1, Weight: 1},
		{ID: 6, Role: RoleTrader, Line: "A trader.", Race: -1, Weight: 1},
		{ID: 7, Mood: MoodHostile, Line: "Begone.", Race: -1, Weight: 1},
		{ID: 8, Race: 2, Line: "A dark elf.", Weight: 1},
		{ID: 9, Line: "Never.", Race: -1, Weight: 0},
	}
	got := map[int]int{}
	for _, c := range DialogCandidates(lines, "Freeport", RoleGuard, "Guard Dahvel", MoodNeutral, 0) {
		got[c.Line.ID] = c.Weight
	}
	want := map[int]int{1: 1, 2: 3, 3: 6, 4: 6}
	if len(got) != len(want) {
		t.Fatalf("lines %v, want %v", got, want)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("line %d: weight %d, want %d", id, got[id], w)
		}
	}
	hostile := DialogCandidates(lines, "Freeport", RoleGuard, "Guard Dahvel", MoodHostile, 2)
	if len(hostile) != 1 || hostile[0].Line.ID != 7 {
		t.Errorf("hostile to a dark elf: %+v, want only line 7 (the race's line is not hostile)", hostile)
	}
}

// Who speaks: a town's citizen and a guard on a road; not a creature's body
// (a gnoll guard), wildlife, the undead, a pet, a dummy, the dead, or a
// person of no town who is no guard. The mood is the standing's: a warm line
// to an Allied player, the hostile one to one a guard attacks on sight. A
// speaker does not say its last line again while another fits, and the
// line's words are filled in.
func TestTownLine(t *testing.T) {
	f := newAggroFix(t, 20)
	f.p.Name, f.p.Race, f.p.Class = "Walker", 0, 0
	lines := []database.NPCDialogLine{
		{ID: 1, Line: "Welcome to %t, %p.", Race: -1, Weight: 1},
		{ID: 2, Line: "Fine day, %r %c.", Race: -1, Weight: 1},
		{ID: 3, Mood: MoodWarm, Line: "Always good to see you.", Race: -1, Weight: 1000},
		{ID: 4, Mood: MoodHostile, Line: "You'll get nothing from me.", Race: -1, Weight: 1},
	}
	citizen := f.npc(2001, "Sabeth Riverhand", 10, humanBody, 1010, 1010)
	guard := f.npc(2002, "Guard Hollin", 15, humanBody, 1020, 1010)
	gnoll := f.npc(2003, "Gnoll Guard", 15, 0x12345678, 1030, 1010)
	rat := f.npc(2004, "Giant Rat", 2, humanBody, 1040, 1010)
	bones := f.npc(2005, "Skeleton Guard", 15, humanBody, 1050, 1010)
	pet := f.npc(2006, "Guard Pet", 15, humanBody, 1060, 1010)
	pet.Owner = 9
	dummy := f.npc(2007, "Training Dummy Guard", 15, humanBody, 1070, 1010)
	dummy.Dummy = TrainingDummy
	nobody := f.npc(2008, "Ulric Fenn", 10, humanBody, 1080, 1010)
	camp := f.npc(2009, "Siloth knight", 15, humanBody, 1090, 1010) // a camp's spawn, named for its kind
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "sabeth riverhand", World: -1, Faction: "Citizens of Freeport"},
		{Name: "giant rat", World: -1, Faction: "Citizens of Freeport"},
	})
	f.p.Factions = map[string]int{"Citizens of Freeport": 0}

	for _, n := range []*NPC{gnoll, rat, bones, pet, dummy, nobody, camp} {
		if _, line, _, ok := f.w.TownLine(f.p, n, lines, 0); ok {
			t.Errorf("%s spoke: %q", n.Name, line)
		}
	}
	for i := 0; i < 20; i++ { // a line naming the town is not said by a speaker of none
		speaker, line, id, ok := f.w.TownLine(f.p, guard, lines, 0)
		if !ok || speaker != "Guard Hollin" || id != 2 || line != "Fine day, human warrior." {
			t.Fatalf("the road's guard: %q %q %d %v", speaker, line, id, ok)
		}
	}
	_, line, id, ok := f.w.TownLine(f.p, citizen, lines, 1)
	if !ok || id != 2 {
		t.Fatalf("the citizen said line %d %q again or nothing (%v)", id, line, ok)
	}
	_, line, _, _ = f.w.TownLine(f.p, citizen, lines, 2)
	if line != "Welcome to Freeport, Walker." {
		t.Errorf("the town's words: %q", line)
	}

	f.p.Factions = map[string]int{"Citizens of Freeport": 1000}
	if _, _, id, _ := f.w.TownLine(f.p, citizen, lines, 0); id != 3 && id != 1 && id != 2 {
		t.Errorf("Allied: line %d", id)
	}
	warm := 0
	for i := 0; i < 50; i++ {
		if _, _, id, _ := f.w.TownLine(f.p, citizen, lines, 0); id == 3 {
			warm++
		}
	}
	if warm < 40 {
		t.Errorf("Allied: the warm line %d times in 50, want nearly all", warm)
	}

	f.p.Factions = map[string]int{"Citizens of Freeport": -2000}
	if _, line, id, ok := f.w.TownLine(f.p, citizen, lines, 0); !ok || id != 4 {
		t.Errorf("KOS: %q %d %v, want the hostile line", line, id, ok)
	}
	citizen.Dead = true
	if _, _, _, ok := f.w.TownLine(f.p, citizen, lines, 0); ok {
		t.Error("the dead spoke")
	}
	if !strings.Contains(DialogSay("%t", f.p, "x", "Klick`Anon"), "Klick'Anon") {
		t.Error("Klick`Anon is said Klick'Anon")
	}
}
