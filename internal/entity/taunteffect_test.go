package entity

import "testing"

// A player's taunt's hate is 145% of its record's figure (board 1776, SOE's
// update of 2009-04-02): a taunt of 1,000 hate cast by a player already on top
// adds 1,450 for a shaman, 1,523 for a warrior (1,450 at the tank's 105%,
// rounded half up), each with the 1 any landed harmful spell angers by
// (cast.go). The control: a hate that takes away is not scaled.
func TestAPlayersTauntIsAHundredAndFortyFivePercent(t *testing.T) {
	noResist(t)
	taunt := func() CastSpell {
		s := wbSpell(709999, 20, 1, "Test Taunt", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":20,
			"effects":[{"id":"081D","type":"Modify Hate","kind":"hate","amount":1000}]}`)
		s.CastTime, s.Range = 0, 30
		return s
	}
	for _, c := range []struct {
		name  string
		class int
		want  int64
	}{{"a shaman", 8, 1450}, {"a warrior", 0, 1523}} {
		r := newAreaRig(t)
		r.p.Class, r.p.Level = c.class, 1 // level 1: a pet's margin under the hate (playerTauntFloor)
		me := r.p.ServerID
		r.player(1002, "Other", 1001, 1001)
		n := r.creature(2001, "fungal cavalier", 1005, 1000)
		n.HP, n.MaxHP = 500_000, 500_000
		r.w.mu.Lock()
		r.w.updateAggro(n, 1002, 100)
		r.w.updateAggro(n, me, 5000) // on top already: no raise
		r.w.mu.Unlock()
		before := n.Threats[me]
		r.cast(t, taunt(), n.ServerID)
		if got := n.Threats[me] - before - 1; got != c.want {
			t.Errorf("%s: the taunt added %d, want %d", c.name, got, c.want)
		}
	}
	if got := playerTauntHate(-200); got != -200 {
		t.Errorf("a hate that takes away: %d, want -200", got)
	}
}
