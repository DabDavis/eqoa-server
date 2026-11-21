package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// An NPC of a faction refuses a player at or below FactionKOSAt with it
// (Refuses, board #742: the red bands) and serves one at -349; one of no
// faction serves everyone, as does everyone with world.faction_aggro off.
// The faction edges follow the players' guide: -350 red, 150 Neutral.
func TestRefusesInTheRedBands(t *testing.T) {
	f := newAggroFix(t, 20)
	coach := f.npc(2001, "Coachman Lothi", 30, humanBody, 1100, 1210)
	f.p.Factions = map[string]int{"Citizens of Neriak": -2000}
	if f.w.Refuses(f.sess, coach) {
		t.Fatal("an NPC of no faction refused")
	}
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "coachman lothi", World: -1, Faction: "Citizens of Neriak"}})
	for _, c := range []struct {
		standing int
		refuses  bool
	}{{-2000, true}, {-351, true}, {-350, true}, {-349, false}, {-100, false}, {0, false}, {500, false}} {
		f.p.Factions = map[string]int{"Citizens of Neriak": c.standing}
		if got := f.w.Refuses(f.sess, coach); got != c.refuses {
			t.Errorf("standing %d: refuses %v, want %v", c.standing, got, c.refuses)
		}
	}
	f.p.Factions = map[string]int{"Citizens of Neriak": -2000}
	f.w.SetFactionAggro(false)
	if f.w.Refuses(f.sess, coach) {
		t.Error("refused with world.faction_aggro off")
	}
	f.w.SetFactionAggro(true)
	if !f.w.Refuses(f.sess, coach) {
		t.Error("not refused with world.faction_aggro back on")
	}
	if !Kos(-350) || Kos(-349) || StandingDisposition(150) != DispNeutral || StandingDisposition(151) != DispFriendly {
		t.Error("the guide's edges: -350 red, 150 Neutral, 151 Friendly")
	}
}

// RefusalLine says an NPC's own hostile line, %p filled in, and nothing for
// an NPC with only other moods' lines of its own or only a town's hostile
// lines.
func TestRefusalLineOwnHostileOnly(t *testing.T) {
	f := newAggroFix(t, 20)
	coach := f.npc(2001, "Coachman Lothi", 30, humanBody, 1100, 1210)
	other := f.npc(2002, "Coachman Ronks", 30, humanBody, 1100, 1220)
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "coachman lothi", World: -1, Faction: "Citizens of Neriak"},
		{Name: "coachman ronks", World: -1, Faction: "Citizens of Neriak"}})
	row := func(id int, name, mood, line string) database.NPCDialogLine {
		return database.NPCDialogLine{ID: id, Name: name, Mood: mood, Line: line, Weight: 1, Race: -1, SpeakerRace: -1, Class: -1, NotRace: -1}
	}
	lines := []database.NPCDialogLine{
		row(1, "Coachman Lothi", "hostile", "I will not help you %p. Be glad I do not slay you where you stand for crimes against Neriak!"),
		row(2, "Coachman Lothi", "", "Welcome, %p."),
		row(3, "Coachman Ronks", "", "All you need to do is talk to the coachman."),
		row(4, "", "hostile", "Begone, %p!"), // a town's hostile line: not a refusal
	}
	for i := 0; i < 20; i++ {
		speaker, line, ok := f.w.RefusalLine(f.p, coach, lines)
		if !ok || speaker != "Coachman Lothi" || line != "I will not help you Walker. Be glad I do not slay you where you stand for crimes against Neriak!" {
			t.Fatalf("Lothi: %q %q %v", speaker, line, ok)
		}
	}
	if s, l, ok := f.w.RefusalLine(f.p, other, lines); ok {
		t.Errorf("Ronks has no hostile line of his own, said %q: %q", s, l)
	}
}
