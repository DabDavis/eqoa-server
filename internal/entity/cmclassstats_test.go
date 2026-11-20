package entity

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A rank's minstats may name the stats a group of classes needs in place of
// everyone's (petition 2480: the Focus line opens at 100 WIS for the tanks and
// healers, 100 INT for the melee and casters). A warrior with 200 WIS and 50
// INT may buy Focus 1 and is shown WIS 100 needed; a wizard of the same stats
// may not, and is shown INT 100; a cleric with 100 INT and 50 WIS may not.
// The controls: a row with one group reads as before; a group without a mask
// adds to everyone's.
func TestCMClassMinStats(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 231, Classes: 32767, Races: 2047, Mastery: "Focus", Rank: 1, Name: "Focus 1", Cost: 5, Level: 10,
			MinStats: "INT:100;WIS:100@909", Enabled: true},
	})
	port := 7400
	in := func(class, wis, intel int) *session.Session {
		port++
		id := uint32(port)
		p := &Player{ServerID: EntityID(id), CharServerID: id, Name: "P", Class: class, Level: 60, BaseWis: wis,
			BaseInt: intel, Mastery: Mastery{Points: 100}, CMs: map[int]int{}}
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: port}, id, id, 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		return s
	}
	k := CMKnows{Quests: map[int]bool{}}
	for _, c := range []struct {
		name             string
		class, wis, intl int
		known            bool
		wantWis, wantInt int
	}{
		{"warrior", 0, 200, 50, true, 100, 0},
		{"wizard", 13, 200, 50, false, 0, 100},
		{"cleric", 9, 50, 100, false, 100, 0},
		{"rogue", 6, 50, 100, true, 0, 100},
	} {
		n, ok := w.CMNodeOf(in(c.class, c.wis, c.intl), 231, k)
		if !ok {
			t.Fatalf("%s: Focus 1 not offered", c.name)
		}
		if n.Known != c.known || n.MinStats[4] != c.wantWis || n.MinStats[5] != c.wantInt {
			t.Errorf("%s: known %v, WIS %d INT %d needed; want %v, %d, %d (%s)", c.name, n.Known, n.MinStats[4],
				n.MinStats[5], c.known, c.wantWis, c.wantInt, n.Text)
		}
	}
	for _, c := range []struct {
		in   string
		all  [7]int
		nGrp int
	}{
		{"STR:100,STA:150", [7]int{100, 150}, 0},
		{"STR:100;DEX:20", [7]int{100, 0, 0, 20}, 0},
		{"INT:100;WIS:100@909", [7]int{5: 100}, 1},
		{"", [7]int{}, 0},
	} {
		all, g := parseMinStats(c.in)
		if all != c.all || len(g) != c.nGrp {
			t.Errorf("%q: %v and %d groups, want %v and %d", c.in, all, len(g), c.all, c.nGrp)
		}
	}
}
