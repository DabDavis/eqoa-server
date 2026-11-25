package entity

import (
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A rank the player owns is listed though its race is not offered it (a GM's
// race change that kept the old race's master class: Heirloom petition 3023,
// an Erudite made another race who kept Scholar), owned and not known; a rank
// of the old race it does not own stays off the list, and buying one is still
// refused. The control: the Erudite who owns the same ranks is listed them as
// before.
func TestCMListShowsOwnedRanksOfAnotherRace(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	erudite, all := 1<<8, 32767
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 101, Category: 1, Classes: all, Races: 2047, Name: "Power", Cost: 1, Level: 1, Effects: "Pow:10", Enabled: true},
		{ID: 1411, Category: 1, Classes: all, Races: erudite, Name: "Growing Ego", Cost: 25, Level: 6, Effects: "INT:5", Enabled: true},
		{ID: 1431, Category: CMCategoryMaster, Classes: all, Races: erudite, Mastery: "Scholar", Rank: 1, Name: "Scholar", Cost: 200, Level: 55, Spent: 500, Effects: "INTMax:25", Enabled: true},
		{ID: 1432, Category: CMCategoryMaster, Classes: all, Races: erudite, Mastery: "Scholar", Rank: 2, Name: "Thirst for Knowledge", Parent: 1431, Cost: 75, Level: 60, Effects: "Pow:100", Enabled: true},
	})
	in := func(port int, p *Player) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: port}, uint32(p.ServerID), uint32(p.ServerID), 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		return s
	}
	owned := func() map[int]int { return map[int]int{1431: 200, 1432: 75} }
	changed := &Player{ServerID: 767, CharServerID: 767, Name: "Archive", Race: 2, Class: 11, Level: 60, CMs: owned(), Mastery: Mastery{Points: 1000}}
	eru := &Player{ServerID: 768, CharServerID: 768, Name: "Scroll", Race: 8, Class: 11, Level: 60, CMs: owned(), Mastery: Mastery{Points: 1000}}
	cs, es := in(7301, changed), in(7302, eru)
	list := func(s *session.Session) (ids []int, byID map[int]CMEntry) {
		byID = map[int]CMEntry{}
		for _, e := range w.CMList(s, CMKnows{}) {
			ids = append(ids, e.ID)
			byID[e.ID] = e
		}
		return
	}
	ids, by := list(cs)
	if !slices.Equal(ids, []int{101, 1431, 1432}) {
		t.Fatalf("the changed race's list: %v, want the general rank and its two owned Scholar ranks (not Growing Ego)", ids)
	}
	for _, id := range []int{1431, 1432} {
		if e := by[id]; !e.Owned || e.Known {
			t.Errorf("rank %d: owned %v known %v, want owned and not known", id, e.Owned, e.Known)
		}
	}
	if r := w.BuyCM(cs, 1411, CMKnows{}); r.Refusal == "" {
		t.Error("the changed race bought the old race's Growing Ego")
	}
	if ids, _ := list(es); !slices.Equal(ids, []int{101, 1411, 1431, 1432}) {
		t.Errorf("the Erudite's list: %v", ids)
	}
}
