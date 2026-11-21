package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A coachman of a faction serves a player from -99 up with it and refuses one
// below (CoachFactionMin, the players' faction guide's "-99 or better"); one of
// no faction serves everyone. The faction edges follow the same guide: -350 is
// on the red side (Kos) and 150 still Neutral.
func TestACoachmanServesFromMinus99(t *testing.T) {
	f := newAggroFix(t, 20)
	coach := f.npc(2001, "Coachman Lothi", 30, humanBody, 1100, 1210)
	if refuses, _ := f.w.CoachRefuses(f.sess, coach); refuses {
		t.Fatal("a coachman of no faction refused")
	}
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "coachman lothi", World: -1, Faction: "Citizens of Neriak"}})
	for _, c := range []struct {
		standing int
		refuses  bool
	}{{-2000, true}, {-351, true}, {-100, true}, {-99, false}, {0, false}, {500, false}} {
		f.p.Factions = map[string]int{"Citizens of Neriak": c.standing}
		refuses, faction := f.w.CoachRefuses(f.sess, coach)
		if refuses != c.refuses || faction != "Citizens of Neriak" {
			t.Errorf("standing %d: refuses %v (%q), want %v", c.standing, refuses, faction, c.refuses)
		}
	}
	if !Kos(-350) || Kos(-349) || StandingDisposition(150) != DispNeutral || StandingDisposition(151) != DispFriendly {
		t.Error("the guide's edges: -350 red, 150 Neutral, 151 Friendly")
	}
}
