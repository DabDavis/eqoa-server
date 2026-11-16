package entity

import (
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A Master Class a character cannot buy is listed "???" and opens CLIENT's
// Requirements window, whose pages show only the level, the masteries, the
// abilities and the minimum stats (Moose, a Hill Giant shadow knight holding
// Titan, 2026-11-16: Slayer "???" with every requirement shown met). The one
// Master Class rule and a quest needed are said on its Required Masteries
// page. The rule is the same for a custom race's own Master Class as for a
// retail race's: a human holding Mercenary is told the same. The controls:
// with no Master Class held Slayer is known and the page says nothing more;
// the held class's own ability is not barred and names no second class; a
// custom race is offered its class's Master Classes (races 2047).
func TestCMMasterClassBlockIsSaidInTheRequirements(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	all, retail, sk := 32767, 2047, 1<<3
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 1741, Category: CMCategoryMaster, Classes: all, Races: 1, Mastery: "Mercenary", Rank: 1, Name: "Mercenary", Cost: 200, Level: 55, Spent: 500, Enabled: true},
		{ID: 1951, Category: CMCategoryMaster, Classes: all, Races: 1 << 11, Mastery: "Titan", Rank: 1, Name: "Titan", Cost: 200, Level: 55, Spent: 500, Enabled: true},
		{ID: 1952, Category: CMCategoryMaster, Classes: all, Races: 1 << 11, Mastery: "Titan", Rank: 2, Name: "Hurl Boulder", Parent: 1951, Cost: 75, Level: 60, Enabled: true},
		{ID: 4101, Category: CMCategoryMaster, Classes: sk, Races: retail, Mastery: "Slayer", Rank: 1, Name: "Slayer", Cost: 200, Level: 55, Spent: 500, MinStats: "STR:200", Enabled: true},
		{ID: 900854, Category: CMCategoryMaster, Classes: sk, Races: retail, Mastery: "Heroic Were-Hunter", Rank: 1, Name: "Heroic Were-Hunter", Cost: 200, Level: 55, Spent: 500, RequireQuests: "77", Enabled: true},
	})
	in := func(port int, p *Player) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: port}, p.CharServerID, p.CharServerID, 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		return s
	}
	k := CMKnows{Quests: map[int]bool{}, QuestNames: map[int]string{77: "The Were-Hunt"}}
	player := func(id uint32, race int, cms map[int]int) *Player {
		return &Player{ServerID: EntityID(id), CharServerID: id, Name: "P", Race: race, Class: 3, Level: 60, BaseStr: 250,
			Mastery: Mastery{Points: 1000, Spent: 1245}, CMs: cms}
	}
	moose := in(7301, player(974, 11, map[int]int{1951: 200}))
	human := in(7302, player(975, 0, map[int]int{1741: 200}))
	fresh := in(7303, player(976, 11, map[int]int{}))

	node := func(s *session.Session, id int) CMNode {
		t.Helper()
		n, ok := w.CMNodeOf(s, id, k)
		if !ok {
			t.Fatalf("rank %d not offered", id)
		}
		return n
	}
	known := func(s *session.Session, id int) bool {
		for _, e := range w.CMList(s, k) {
			if e.ID == id {
				return e.Known
			}
		}
		t.Fatalf("rank %d not listed", id)
		return false
	}

	if n := node(moose, 4101); n.Known || !slices.Equal(n.Masteries, []string{"Only one Master Class: you hold Titan"}) {
		t.Errorf("Moose's Slayer: known %v, masteries %q", n.Known, n.Masteries)
	}
	if n := node(human, 4101); n.Known || !slices.Equal(n.Masteries, []string{"Only one Master Class: you hold Mercenary"}) {
		t.Errorf("the human's Slayer: known %v, masteries %q", n.Known, n.Masteries)
	}
	if n := node(moose, 900854); !slices.Equal(n.Masteries, []string{"Only one Master Class: you hold Titan", "Quest: The Were-Hunt"}) {
		t.Errorf("Moose's Heroic Were-Hunter: masteries %q", n.Masteries)
	}
	if n := node(moose, 1952); !n.Known || len(n.Masteries) != 0 {
		t.Errorf("the control, Titan's own Hurl Boulder: known %v, masteries %q", n.Known, n.Masteries)
	}
	if !known(fresh, 4101) {
		t.Error("the control: a Hill Giant shadow knight with no Master Class cannot buy Slayer")
	}
	if n := node(fresh, 4101); len(n.Masteries) != 0 {
		t.Errorf("the control, Slayer with none held: masteries %q", n.Masteries)
	}
	if n := node(fresh, 900854); !slices.Equal(n.Masteries, []string{"Quest: The Were-Hunt"}) {
		t.Errorf("Heroic Were-Hunter with none held: masteries %q", n.Masteries)
	}
	if r := w.BuyCM(fresh, 4101, k); r.Refusal != "" {
		t.Fatalf("the fresh Hill Giant's Slayer refused: %s", r.Refusal)
	}
	if r := w.BuyCM(fresh, 1951, k); r.Refusal != OneMasterClass {
		t.Errorf("Titan after Slayer: %q, want the one Master Class refusal", r.Refusal)
	}
}
