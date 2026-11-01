package entity

import (
	"fmt"
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

// A town's own line counts three times a generic one, its role's twice; a
// line of another town, role, race or mood is not said. A speaker with lines
// of its own name says those alone (TestOwnLines), never the pool's.
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
	for _, c := range DialogCandidates(lines, "Freeport", RoleGuard, "Guard Hollin", MoodNeutral, "male", 0, DialogVoice{Race: -1, Resident: true}) {
		got[c.Line.ID] = c.Weight
	}
	want := map[int]int{1: 1, 2: 3, 3: 6}
	if len(got) != len(want) {
		t.Fatalf("lines %v, want %v", got, want)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("line %d: weight %d, want %d", id, got[id], w)
		}
	}
	hostile := DialogCandidates(lines, "Freeport", RoleGuard, "Guard Hollin", MoodHostile, "male", 2, DialogVoice{Race: -1, Resident: true})
	if len(hostile) != 1 || hostile[0].Line.ID != 7 {
		t.Errorf("hostile to a dark elf: %+v, want only line 7 (the race's line is not hostile)", hostile)
	}
	own := DialogCandidates(lines, "Freeport", RoleGuard, "Guard Dahvel", MoodNeutral, "male", 0, DialogVoice{Race: -1, Resident: true})
	if len(own) != 1 || own[0].Line.ID != 4 {
		t.Errorf("Guard Dahvel, a line of his own: %+v, want that one alone", own)
	}
	if mine := DialogCandidates(lines, "Freeport", RoleGuard, "Guard Dahvel", MoodHostile, "male", 0, DialogVoice{Race: -1, Resident: true}); len(mine) != 0 {
		t.Errorf("Guard Dahvel to a hostile listener: %+v, want nothing (his line is not hostile; the pool's are not his)", mine)
	}
}

// A line of a husband or a wife is said by the speaker it fits, twice as often
// as one for anyone; a speaker of no known sex says neither (the user,
// 2026-10-29: "My husband works the docks" was said by men too). The control:
// the line for anyone is said by all three.
func TestDialogBySex(t *testing.T) {
	lines := []database.NPCDialogLine{
		{ID: 1, Line: "Fine day.", Race: -1, Weight: 1},
		{ID: 2, Sex: "female", Line: "My husband works the docks.", Race: -1, Weight: 1},
		{ID: 3, Sex: "male", Line: "My wife works the docks.", Race: -1, Weight: 1},
	}
	for _, c := range []struct {
		sex  string
		want map[int]int
	}{
		{"female", map[int]int{1: 1, 2: 2}},
		{"male", map[int]int{1: 1, 3: 2}},
		{"", map[int]int{1: 1}},
	} {
		got := map[int]int{}
		for _, p := range DialogCandidates(lines, "", RoleCitizen, "Sabeth", MoodNeutral, c.sex, 0, DialogVoice{Race: -1, Resident: true}) {
			got[p.Line.ID] = p.Weight
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: lines %v, want %v", c.sex, got, c.want)
			continue
		}
		for id, w := range c.want {
			if got[id] != w {
				t.Errorf("%q: line %d weight %d, want %d", c.sex, id, got[id], w)
			}
		}
	}
}

// The speaker's sex is its body's (charactermodel): a woman's, a man's, and
// none for a body no players' race has.
func TestDialogSexOfABody(t *testing.T) {
	w := NewWorld()
	w.SetModelSexes(map[uint32]int{100: 0, 101: 1})
	for model, want := range map[int64]string{100: "male", 101: "female", 555: ""} {
		if got := w.dialogSexLocked(&NPC{ModelID: model}); got != want {
			t.Errorf("model %d: %q, want %q", model, got, want)
		}
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

// A speaker with lines of its own says those alone (its own pool: a quest's
// people), never the generic ones that fit it too; it speaks with no town's
// faction and on a creature's body; nobody speaks in a fight (a quest's attack
// event turned it on the player), and the others keep the pool.
func TestOwnLines(t *testing.T) {
	f := newAggroFix(t, 20)
	f.p.Name, f.p.Race, f.p.Class = "Walker", 0, 0
	lines := []database.NPCDialogLine{
		{ID: 1, Line: "Welcome, %p.", Race: -1, Weight: 1000},
		{ID: 2, Name: "Valrean V`Ghen", Line: "My brother keeps the tower.", Race: -1, Weight: 1},
		{ID: 3, Name: "valrean v`ghen", Line: "I keep the tent, %p.", Race: -1, Weight: 1},
		{ID: 4, Name: "Hermit Olen", Line: "Leave me be.", Race: -1, Weight: 1},
	}
	val := f.npc(2101, "Valrean V`Ghen", 13, humanBody, 1010, 1010)
	hermit := f.npc(2102, "Hermit Olen", 5, 0x12345678, 1020, 1010) // no town's faction, a creature's body (too low to attack on sight)
	citizen := f.npc(2103, "Sabeth Riverhand", 10, humanBody, 1030, 1010)
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "valrean v`ghen", World: -1, Faction: "Citizens of Freeport"},
		{Name: "sabeth riverhand", World: -1, Faction: "Citizens of Freeport"},
	})
	f.p.Factions = map[string]int{"Citizens of Freeport": 0}

	seen := map[int]int{}
	for i := 0; i < 60; i++ {
		speaker, line, id, ok := f.w.TownLine(f.p, val, lines, 0)
		if !ok || speaker != "Valrean V`Ghen" {
			t.Fatalf("Valrean: %q %q %v", speaker, line, ok)
		}
		if id == 3 && line != "I keep the tent, Walker." {
			t.Errorf("his words filled in: %q", line)
		}
		seen[id]++
	}
	if seen[1] != 0 || seen[2] == 0 || seen[3] == 0 {
		t.Errorf("Valrean's lines said %v, want his own two only", seen)
	}
	if _, line, id, ok := f.w.TownLine(f.p, hermit, lines, 0); !ok || id != 4 {
		t.Errorf("the hermit (no town, a creature's body): %q %d %v, want his own line", line, id, ok)
	}
	if _, line, id, ok := f.w.TownLine(f.p, citizen, lines, 0); !ok || id != 1 {
		t.Errorf("a citizen: %q %d %v, want the pool's", line, id, ok)
	}
	val.CurrentEnemy = f.p.ServerID
	if _, line, _, ok := f.w.TownLine(f.p, val, lines, 0); ok {
		t.Errorf("Valrean spoke in a fight: %q", line)
	}
	val.CurrentEnemy = 0
	val.Dead = true
	if _, line, _, ok := f.w.TownLine(f.p, val, lines, 0); ok {
		t.Errorf("the dead spoke his own lines: %q", line)
	}
}

// Each speaker talks in its own people's voice: a guest (an ogre in a dark
// elves' town, Bouncer Frada) never says its host town's lines and says its
// race's, the away ones too; a town's own folk say the town's lines, their
// race's lines but not the away ones, and the common lines, unless they are
// trolls or ogres; a troll on a road says its race's lines, never the common
// ones; a town's own races are a fifth of its speakers or its most common.
func TestDialogVoices(t *testing.T) {
	const human, darkElf, troll, ogre = 0, 2, 5, 9
	lines := []database.NPCDialogLine{
		{ID: 1, Line: "A common line.", Race: -1, Weight: 1},
		{ID: 2, Town: "Neriak", Line: "Neriak's own.", Race: -1, Weight: 1},
		{ID: 3, Line: "Me smash.", Race: -1, Weight: 1, Voiced: true, SpeakerRace: ogre},
		{ID: 4, Line: "Oggok far. %t smell funny.", Race: -1, Weight: 1, Voiced: true, SpeakerRace: ogre, Away: true},
		{ID: 5, Line: "Troll hungry.", Race: -1, Weight: 1, Voiced: true, SpeakerRace: troll},
		{ID: 6, Town: "Grobb", Line: "Grobb strong.", Race: -1, Weight: 1},
		{ID: 7, Line: "Far from Freeport, this.", Race: -1, Weight: 1, Voiced: true, SpeakerRace: human, Away: true},
	}
	ids := func(town string, v DialogVoice) map[int]bool {
		out := map[int]bool{}
		for _, c := range DialogCandidates(lines, town, RoleCitizen, "Someone", MoodNeutral, "male", 0, v) {
			out[c.Line.ID] = true
		}
		return out
	}
	same := func(what string, got map[int]bool, want ...int) {
		t.Helper()
		w := map[int]bool{}
		for _, id := range want {
			w[id] = true
		}
		if len(got) != len(w) {
			t.Errorf("%s: lines %v, want %v", what, got, want)
			return
		}
		for id := range w {
			if !got[id] {
				t.Errorf("%s: lines %v, want %v", what, got, want)
				return
			}
		}
	}
	same("an ogre guest in Neriak", ids("Neriak", DialogVoice{Race: ogre}), 3, 4)
	same("a dark elf of Neriak", ids("Neriak", DialogVoice{Race: darkElf, Resident: true}), 1, 2)
	same("a troll of Grobb", ids("Grobb", DialogVoice{Race: troll, Resident: true}), 5, 6)
	same("a human guest in Neriak", ids("Neriak", DialogVoice{Race: human}), 1, 7)
	same("a troll on a road", ids("", DialogVoice{Race: troll}), 5)
	same("an ogre of Oggok", ids("Oggok", DialogVoice{Race: ogre, Resident: true}), 3)
	same("a body of no players' race", ids("Neriak", DialogVoice{Race: -1, Resident: true}), 1, 2)

	// the town's own races, from the World's NPCs
	f := newAggroFix(t, 20)
	f.w.SetModelRaces(map[uint32]int{humanBody: darkElf, 0x11111111: ogre})
	for i := 0; i < 9; i++ {
		f.npc(EntityID(3000+i), fmt.Sprintf("Citizen %d", i), 10, humanBody, 1000+float32(i), 1000)
	}
	frada := f.npc(3100, "Bouncer Frada", 30, 0x11111111, 1100, 1000)
	var rows []database.NPCFactionRow
	for i := 0; i < 9; i++ {
		rows = append(rows, database.NPCFactionRow{Name: fmt.Sprintf("citizen %d", i), World: -1, Faction: "Citizens of Neriak"})
	}
	f.w.SetNPCFactions(append(rows, database.NPCFactionRow{Name: "bouncer frada", World: -1, Faction: "Citizens of Neriak"}))
	f.w.mu.Lock()
	v := f.w.dialogVoiceLocked(frada, "Neriak")
	own := f.w.townRacesLocked()["Neriak"]
	f.w.mu.Unlock()
	if v.Race != ogre || v.Resident {
		t.Errorf("Bouncer Frada's voice: %+v, want an ogre guest", v)
	}
	if !own[darkElf] || own[ogre] {
		t.Errorf("Neriak's own races %v, want the dark elves alone (one ogre of ten)", own)
	}
}
