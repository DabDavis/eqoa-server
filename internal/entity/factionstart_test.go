package entity

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Starting standings by race, class and home city (factionstart), the NPCs'
// factions (npcfaction), and the face and aggro following a player's standing
// (aggro.go). Synthetic numbers throughout: the measured starts are
// tools/seed_faction_starts.py's to write.

const ghoulBody uint32 = 0x2C2C2C2C // a creature's body that no player race has

// The most specific row a character matches is its start: race and class over
// race alone over class alone, a human's city telling east from west, a
// froglok and a human of no city as the east's, a non-human's city ignored;
// its own standing over any start; a row naming no faction kept out.
func TestStartStandingResolution(t *testing.T) {
	f := newAggroFix(t, 10)
	kept, unknown := f.w.SetFactionStarts([]database.FactionStartRow{
		{Race: 2, Class: -1, HumType: -1, Faction: "Citizens of Neriak", Standing: 500},
		{Race: 2, Class: 3, HumType: -1, Faction: "Citizens of Neriak", Standing: 800},
		{Race: -1, Class: 3, HumType: -1, Faction: "Citizens of Neriak", Standing: -50},
		{Race: 0, Class: -1, HumType: 1, Faction: "Citizens of Qeynos", Standing: -1000},
		{Race: 0, Class: -1, HumType: 2, Faction: "Citizens of Qeynos", Standing: 400},
		{Race: 0, Class: 9, HumType: 2, Faction: "Citizens of Qeynos", Standing: 900},
		{Race: -1, Class: -1, HumType: -1, Faction: "Citizens of Freeport", Standing: 77},
		{Race: 2, Class: -1, HumType: -1, Faction: "No Such Faction", Standing: 5},
	})
	if kept != 7 || unknown != 1 {
		t.Fatalf("kept %d, unknown %d", kept, unknown)
	}
	cases := []struct {
		what             string
		race, class, hum int
		faction          string
		want             int
	}{
		{"a dark elf shadow knight: race and class", 2, 3, 0, "Citizens of Neriak", 800},
		{"a dark elf warrior: its race", 2, 0, 0, "Citizens of Neriak", 500},
		{"a dark elf whose client sent city 1: still a dark elf", 2, 0, 1, "Citizens of Neriak", 500},
		{"a human shadow knight: the class alone", 0, 3, 1, "Citizens of Neriak", -50},
		{"a gnome wizard: the faction's own", 3, 13, 0, "Citizens of Neriak", 1},
		{"a western human cleric", 0, 9, 2, "Citizens of Qeynos", 900},
		{"a western human warrior", 0, 0, 2, "Citizens of Qeynos", 400},
		{"an eastern human cleric", 0, 9, 1, "Citizens of Qeynos", -1000},
		{"a human of no city: the east's", 0, 9, 0, "Citizens of Qeynos", -1000},
		{"a froglok: a human of the east", raceFroglok, 3, 0, "Citizens of Qeynos", -1000},
		{"an elf: the row for everyone", 1, 1, 0, "Citizens of Freeport", 77},
	}
	for _, c := range cases {
		f.p.Race, f.p.Class, f.p.HumType = c.race, c.class, c.hum
		if got, ok := f.w.Standing(f.sess, c.faction); !ok || got != c.want {
			t.Errorf("%s: %s %d (%v), want %d", c.what, c.faction, got, ok, c.want)
		}
	}
	// a tie of two named (race and city, against race and class): the class's
	f.w.SetFactionStarts([]database.FactionStartRow{
		{Race: 0, Class: -1, HumType: 2, Faction: "Citizens of Qeynos", Standing: 10},
		{Race: 0, Class: 9, HumType: -1, Faction: "Citizens of Qeynos", Standing: 20},
		{Race: -1, Class: 9, HumType: 2, Faction: "Citizens of Qeynos", Standing: 30},
	})
	f.p.Race, f.p.Class, f.p.HumType = 0, 9, 2
	if got, _ := f.w.Standing(f.sess, "Citizens of Qeynos"); got != 20 {
		t.Errorf("race and class against race and city against class and city: %d, want 20 (race and class)", got)
	}
	// its own standing over its start; deleted, the start again
	f.p.Factions = map[string]int{"Citizens of Qeynos": -3}
	if got, _ := f.w.Standing(f.sess, "Citizens of Qeynos"); got != -3 {
		t.Errorf("its own: %d", got)
	}
	f.p.Factions = nil
	if got, _ := f.w.Standing(f.sess, "Citizens of Qeynos"); got != 20 {
		t.Errorf("no own standing: %d", got)
	}
}

// The faction window lists a character's starts in their place, in the
// window's order; a character with none lists the factions' own.
func TestTheWindowListsTheStarts(t *testing.T) {
	f := newAggroFix(t, 10)
	f.p.Race, f.p.Class = 2, 3
	f.w.SetFactionStarts([]database.FactionStartRow{
		{Race: 2, Class: -1, HumType: -1, Faction: "Citizens of Neriak", Standing: 1500},
		{Race: 2, Class: -1, HumType: -1, Faction: "Citizens of Klick`Anon", Standing: -2000},
	})
	list := f.w.PlayerFactions(f.sess)
	if len(list) != len(database.DefaultFactions) {
		t.Fatalf("%d factions listed, want %d", len(list), len(database.DefaultFactions))
	}
	if list[0].Name != "Citizens of Neriak" || list[0].Standing != 1500 {
		t.Errorf("the top of the window: %+v, want Citizens of Neriak 1500", list[0])
	}
	for _, l := range list {
		if l.Name == "Citizens of Klick`Anon" && l.Standing != -2000 {
			t.Errorf("Citizens of Klick`Anon listed at %d, want its start -2000", l.Standing)
		}
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Standing < list[i].Standing {
			t.Fatalf("the order breaks at %d: %v then %v", i, list[i-1], list[i])
		}
	}
	f.p.Race = 3 // a gnome: none of those rows
	if got := f.w.PlayerFactions(f.sess); got[0].Name != "Citizens of Klick`Anon" || got[0].Standing != 300 {
		t.Errorf("a gnome's window starts with %+v, want the capture's Klick`Anon 300", got[0])
	}
}

// A kill moves a standing from the killer's start, not the faction's own.
func TestAKillMovesFromTheStart(t *testing.T) {
	f := newGuardFight(t)
	f.p.Race, f.p.Class, f.p.HumType = 0, 0, 1
	f.w.SetFactionStarts([]database.FactionStartRow{{Race: 0, Class: -1, HumType: 1, Faction: "The Freeport Militia", Standing: 600}})
	f.w.mu.Lock()
	f.w.playerHitLocked(f.sess, f.p, f.npc, f.npcID)
	f.w.mu.Unlock()
	if got := f.p.Factions["The Freeport Militia"]; got != 550 {
		t.Errorf("The Freeport Militia %d, want 600 - 50", got)
	}
	if got := f.p.Factions["Citizens of Freeport"]; got != -24 {
		t.Errorf("Citizens of Freeport %d, want the faction's own 1 - 25", got)
	}
}

// CLIENT's faction window's edges, each side of each.
func TestStandingDispositionEdges(t *testing.T) {
	for _, c := range []struct {
		v    int
		want byte
	}{
		{-2000, DispAggressive}, {-701, DispAggressive}, {-700, DispDubious}, {-351, DispDubious},
		{-350, DispCautious}, {-151, DispCautious}, {-150, DispNeutral}, {0, DispNeutral}, {149, DispNeutral},
		{150, DispFriendly}, {349, DispFriendly}, {350, DispWarm}, {699, DispWarm}, {700, DispAllied}, {2000, DispAllied},
	} {
		if got := StandingDisposition(c.v); got != c.want {
			t.Errorf("standing %d: disposition %d, want %d", c.v, got, c.want)
		}
	}
	if DispAggressive != FaceHostile || DispCautious != FaceNeutral || DispFriendly != FaceAmiable {
		t.Error("the dispositions are not the faces' numbers")
	}
}

// moveTo puts the fixture's player at (x, z).
func (f *aggroFix) moveTo(x, z float32) {
	f.w.SpatialIndex.Update(f.p.ServerID, f.p.X, f.p.Z, x, z)
	f.p.X, f.p.Z = x, z
}

// forget is a creature's fight with the player taken back.
func forget(n *NPC) { n.Threats, n.CurrentEnemy = nil, 0 }

// Neriak's ghoul servant (2026-10-29): a creature's body 52 from the nearest
// dark elf, outside the town's 40, so hostile by nature, killed a new dark
// elf. Of Citizens of Neriak it leaves a dark elf alone, at any level, and
// shows it the face of its standing; the control is the same servant with no
// faction, and world.faction_aggro false (the old behaviour).
func TestTheGhoulServantLeavesADarkElfAlone(t *testing.T) {
	f := newAggroFix(t, 2)
	f.p.Race, f.p.Class = 2, 3
	f.town(1000, 1200) // dark elves from (1000, 1200) to (1180, 1200)
	ghoul := f.npc(2001, "ghoul servant", 55, ghoulBody, 1100, 1252)
	f.moveTo(1100, 1262)
	f.look()
	if !f.attacks(ghoul) || f.face(ghoul) != FaceHostile {
		t.Fatalf("the control, no faction: attacked %v, face %d (the bug: it attacks)", f.attacks(ghoul), f.face(ghoul))
	}
	forget(ghoul)

	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "Ghoul Servant", World: -1, Faction: "Citizens of Neriak"}})
	f.w.SetFactionStarts([]database.FactionStartRow{{Race: 2, Class: -1, HumType: -1, Faction: "Citizens of Neriak", Standing: 1000}})
	for _, lvl := range []int{1, 2, 20, 60} {
		f.p.Level = lvl
		f.look()
		if f.attacks(ghoul) {
			t.Fatalf("a level %d dark elf was attacked by Neriak's servant", lvl)
		}
		if got := f.face(ghoul); got != DispAllied {
			t.Errorf("level %d: face %d, want Allied %d (1000)", lvl, got, DispAllied)
		}
	}
	if got := f.w.NPCFaction(ghoul); got != "Citizens of Neriak" {
		t.Errorf("the servant is of %q", got)
	}

	f.w.SetFactionAggro(false)
	f.p.Level = 2
	f.look()
	if !f.attacks(ghoul) || f.face(ghoul) != FaceHostile {
		t.Errorf("world.faction_aggro false: attacked %v, face %d, want the old behaviour (attacked, red)", f.attacks(ghoul), f.face(ghoul))
	}
	if s := f.w.AggroSummary(); !strings.Contains(s, "faction_aggro false") {
		t.Errorf("the start's line: %q", s)
	}
}

// A guard of a faction attacks on sight a player whose standing with it is
// in a red band (below -350: Aggressive, then Dubious), at any level it is
// not green to (a level 60 by a level 50 guard), showing it the red face, and
// no one else; the face is the standing's from -350 up. A level 10 guard,
// green to the level 60, shows it the red face and leaves it alone on sight
// (the user, 2026-10-30). With
// world.faction_aggro false a guard with a players' race's body attacks no
// one and shows the old white.
func TestAGuardAttacksAKOSPlayerOnSight(t *testing.T) {
	f := newAggroFix(t, 60)
	f.town(1000, 1200)
	guard := f.npc(2001, "Guard Dahvel", 50, humanBody, 1100, 1210)
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "guard dahvel", World: -1, Faction: "The Freeport Militia"}})
	f.moveTo(1100, 1225)
	for _, c := range []struct {
		standing int
		attacks  bool
		face     byte
	}{
		{-2000, true, DispAggressive}, {-701, true, DispAggressive}, {-700, true, DispAggressive},
		{-500, true, DispAggressive}, {-351, true, DispAggressive}, {-350, false, DispCautious},
		{1, false, DispNeutral}, {500, false, DispWarm}, {2000, false, DispAllied},
	} {
		f.p.Factions = map[string]int{"The Freeport Militia": c.standing}
		f.look()
		if f.attacks(guard) != c.attacks || f.face(guard) != c.face {
			t.Errorf("standing %d: attacked %v face %d, want %v %d", c.standing, f.attacks(guard), f.face(guard), c.attacks, c.face)
		}
		forget(guard)
	}
	// a guard green to the player: the red face, no attack on sight
	green := f.npc(2002, "Guard Dahvel", 10, humanBody, 1100, 1215)
	f.p.Factions = map[string]int{"The Freeport Militia": -2000}
	f.look()
	if f.attacks(green) || f.face(green) != DispAggressive {
		t.Errorf("a level 10 guard, green to a level 60 KOS to it: attacked %v face %d, want the red face and no attack", f.attacks(green), f.face(green))
	}
	forget(guard)
	f.w.SpatialIndex.Remove(green.ServerID, green.X, green.Z)
	delete(f.w.NPCs, green.ServerID)
	// the start counts as the standing does: a dark elf starting KOS to it
	f.p.Factions = nil
	f.p.Race = 2
	f.w.SetFactionStarts([]database.FactionStartRow{{Race: 2, Class: -1, HumType: -1, Faction: "The Freeport Militia", Standing: -2000}})
	f.look()
	if !f.attacks(guard) {
		t.Error("a dark elf starting at -2000 was not attacked")
	}
	forget(guard)
	f.w.SetFactionAggro(false)
	f.look()
	if f.attacks(guard) || f.face(guard) != FaceNeutral {
		t.Errorf("world.faction_aggro false: attacked %v face %d, want the old white and no attack", f.attacks(guard), f.face(guard))
	}
}

// An NPC-bit NPC of a faction never attacks, and a GM's npcaggro row decides
// as before: neither shows the Aggressive face to a player it leaves alone
// (Dubious at worst); a row setting it on players makes it attack in its
// level range, whatever the standing.
func TestTheNPCBitAndAGMRuleOverFactions(t *testing.T) {
	f := newAggroFix(t, 20)
	f.town(1000, 1200)
	merchant := f.npc(2001, "Merchant Tull", 20, humanBody, 1100, 1210)
	merchant.NPCType = 0x81
	calm := f.npc(2002, "Guard Calm", 20, humanBody, 1105, 1210)
	keen := f.npc(2003, "Guard Keen", 20, humanBody, 1110, 1210)
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "merchant tull", World: -1, Faction: "Citizens of Freeport"},
		{Name: "guard calm", World: -1, Faction: "Citizens of Freeport"},
		{Name: "guard keen", World: -1, Faction: "Citizens of Freeport"},
	})
	f.w.SetAggroRules(map[string]AggroRule{"Guard Calm": {Aggressive: false}, "Guard Keen": {Aggressive: true}})
	f.moveTo(1105, 1220)
	f.p.Factions = map[string]int{"Citizens of Freeport": -2000}
	f.look()
	if f.attacks(merchant) || f.attacks(calm) {
		t.Errorf("at -2000: the merchant attacked %v, the GM's peaceful guard %v", f.attacks(merchant), f.attacks(calm))
	}
	for _, n := range []*NPC{merchant, calm} {
		if got := f.face(n); got != DispDubious {
			t.Errorf("%s at -2000: face %d, want Dubious %d (it does not attack)", n.Name, got, DispDubious)
		}
	}
	if !f.attacks(keen) || f.face(keen) != FaceHostile {
		t.Errorf("the GM's aggressive guard: attacked %v face %d", f.attacks(keen), f.face(keen))
	}
	forget(keen)
	f.p.Factions = map[string]int{"Citizens of Freeport": 2000}
	f.look()
	if !f.attacks(keen) {
		t.Error("the GM's aggressive guard left an Allied player alone: its row decides")
	}
	if got := f.face(merchant); got != DispAllied {
		t.Errorf("the merchant to an Allied player: face %d", got)
	}
}

// What the player sees is what attacks, with factions in: over standings,
// bodies, the NPC bit, a GM's rule and the player's level, a creature
// attacks a noticeable player within its radius exactly when it shows it the
// Aggressive face and is not green to it (the user, 2026-10-30), and one of a
// faction attacks exactly when the standing is below -700 and it is not green.
func TestTheFaceAndTheAttackAgreeWithFactions(t *testing.T) {
	type kind struct {
		name    string
		body    uint32
		npcType int
		rule    *AggroRule
		faction bool
	}
	peaceful := &AggroRule{Aggressive: false}
	kinds := []kind{
		{"Thing of the wild", bearBody, 0, nil, true},
		{"Thing of the town", humanBody, 0, nil, true},
		{"Thing with the bit", bearBody, 0x80, nil, true},
		{"Thing a GM calmed", bearBody, 0, peaceful, true},
		{"Thing of no faction", bearBody, 0, nil, false},
	}
	for _, pl := range []int{1, 30, 60} {
		f := newAggroFix(t, pl)
		var rows []database.NPCFactionRow
		rules := map[string]AggroRule{}
		for _, k := range kinds {
			if k.faction {
				rows = append(rows, database.NPCFactionRow{Name: k.name, World: -1, Faction: "Citizens of Neriak"})
			}
			if k.rule != nil {
				rules[k.name] = *k.rule
			}
		}
		f.w.SetNPCFactions(rows)
		f.w.SetAggroRules(rules)
		id := EntityID(3000)
		for _, standing := range []int{-2000, -701, -700, -351, -350, 0, 700, 2000} {
			f.p.Factions = map[string]int{"Citizens of Neriak": standing}
			for _, k := range kinds {
				for _, lvl := range []int{1, 10, 25} {
					id++
					n := f.npc(id, k.name, lvl, k.body, 1010, 1000)
					n.NPCType = k.npcType
					f.look()
					face := f.face(n)
					// green to the player: the red face, and no attack on
					// sight (the user, 2026-10-30)
					green := ConColor(pl, lvl) == ConGreen
					if (face == DispAggressive && !green) != f.attacks(n) {
						t.Fatalf("player %d, %s level %d, standing %d: face %d, green %v, attacked %v", pl, k.name, lvl, standing, face, green, f.attacks(n))
					}
					if k.faction && k.npcType == 0 && k.rule == nil && f.attacks(n) != (standing < FactionKOSBelow && !green) {
						t.Errorf("player %d, %s level %d, standing %d: attacked %v", pl, k.name, lvl, standing, f.attacks(n))
					}
					f.w.SpatialIndex.Remove(n.ServerID, n.X, n.Z)
					delete(f.w.NPCs, n.ServerID)
				}
			}
		}
	}
}

// An npcfaction row may hold only the NPCs of its name that spawned within a
// radius of a place, the smallest place first, then a world's, then
// anywhere's; an empty faction is none.
func TestNPCFactionPlaces(t *testing.T) {
	f := newAggroFix(t, 10)
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "decaying zombie", World: -1, Faction: "Undead of Paineel"},
		{Name: "decaying zombie", World: 0, X: 1000, Z: 1000, Radius: 50, Faction: "Citizens of Neriak"},
		{Name: "Decaying Zombie", World: 0, X: 1005, Z: 1000, Radius: 10, Faction: "House J`Narus"},
		{Name: "decaying zombie", World: 2, Faction: "Necrological Society"},
		{Name: "decaying zombie", World: 0, X: 3000, Z: 3000, Radius: 20, Faction: ""},
		{Name: "decaying zombie", World: -1, Faction: "No Such Faction"},
	})
	for _, c := range []struct {
		world uint8
		x, z  float32
		want  string
	}{
		{0, 1006, 1000, "House J`Narus"},
		{0, 1030, 1000, "Citizens of Neriak"},
		{0, 5000, 5000, "Undead of Paineel"},
		{2, 1006, 1000, "Necrological Society"},
		{0, 3010, 3000, ""},
	} {
		n := &NPC{Name: "Decaying Zombie ", World: c.world, SpawnX: c.x, SpawnZ: c.z, X: c.x + 70, Z: c.z}
		if got := f.w.NPCFaction(n); got != c.want {
			t.Errorf("spawned in world %d at (%.0f, %.0f): %q, want %q", c.world, c.x, c.z, got, c.want)
		}
	}
}

// The start's check: a creature of no faction within reach of a start spot
// attacks a new character there; of the city's faction it does not; a
// guard whose faction the new character starts KOS to does, and is named.
func TestStartThreats(t *testing.T) {
	f := newAggroFix(t, 10)
	f.town(1000, 1200)
	ghoul := f.npc(2001, "ghoul servant", 55, ghoulBody, 1100, 1252)
	_ = ghoul
	spot := database.StartSpot{Race: 2, Class: 3, World: 0, X: 1100, Y: 10, Z: 1240}
	threats := f.w.StartThreats([]database.StartSpot{spot})
	if len(threats) != 1 || threats[0].NPC != "ghoul servant" || threats[0].Faction != "" {
		t.Fatalf("threats %+v, want the ghoul servant by its nature", threats)
	}
	if lines := f.w.StartThreatLines([]database.StartSpot{spot}); len(lines) != 1 || !strings.Contains(lines[0], "Dark Elf Shadow Knight") {
		t.Errorf("lines %q", lines)
	}
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "ghoul servant", World: -1, Faction: "Citizens of Neriak"},
		{Name: "citizen 5", World: -1, Faction: "Citizens of Qeynos"},
	})
	threats = f.w.StartThreats([]database.StartSpot{spot})
	if len(threats) != 1 || threats[0].NPC != "Citizen 5" || threats[0].Faction != "Citizens of Qeynos" || threats[0].Standing != -2000 {
		t.Fatalf("threats %+v, want Citizen 5 (40 off) for the capture's Qeynos -2000", threats)
	}
	f.w.SetFactionStarts([]database.FactionStartRow{{Race: 2, Class: -1, HumType: -1, Faction: "Citizens of Qeynos", Standing: 0}})
	if lines := f.w.StartThreatLines([]database.StartSpot{spot}); len(lines) != 1 || !strings.HasPrefix(lines[0], "no NPC attacks") {
		t.Errorf("with the start written: %q", lines)
	}
}

// The start's line says what factions do.
func TestAggroSummaryNamesFactions(t *testing.T) {
	f := newAggroFix(t, 10)
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "ghoul servant", World: -1, Faction: "Citizens of Neriak"}})
	if s := f.w.AggroSummary(); !strings.Contains(s, fmt.Sprintf("below %d", FactionKOSBelow)) {
		t.Errorf("%q", s)
	}
	f.npc(2001, "ghoul servant", 55, ghoulBody, 1100, 1252)
	if s := f.w.FactionMembersSummary(); !strings.Contains(s, "1 NPCs are of 1 factions") {
		t.Errorf("%q", s)
	}
}
