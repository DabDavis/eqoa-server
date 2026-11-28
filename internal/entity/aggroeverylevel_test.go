package entity

import "testing"

// The owner, 2026-11-28: "yes show red face to every level" (Garrak, 60, saw the level 18-20 skeletons and zombies of
// the GM's undead rules white, a level 33 Horror red). A creature a GM's aggressive rule names shows the red face at
// any level, but still leaves alone on sight a player it is green to. The controls: a creature of no rule keeps the
// level window (white below a third of the player's level), and a player near the ruled creature's level is attacked.
func TestAGMsAggressiveRuleShowsTheRedFaceToEveryLevel(t *testing.T) {
	f := newAggroFix(t, 60)
	f.w.SetAggroRules(map[string]AggroRule{"skeleton": {Aggressive: true}})
	skel := f.npc(3201, "skeleton", 18, bearBody, 1005, 1000)
	bear := f.npc(3202, "Brown Bear", 18, bearBody, 1006, 1001)
	if got := f.face(skel); got != FaceHostile {
		t.Errorf("a ruled level 18 skeleton's face to a 60: %d, want the red %d", got, FaceHostile)
	}
	if got := f.face(bear); got == FaceHostile {
		t.Errorf("a level 18 bear of no rule shows a 60 the red face: want the level window's white")
	}
	f.look()
	if f.attacks(skel) {
		t.Error("the ruled skeleton attacked a player it is green to on sight")
	}
	f.p.Level = 20
	f.look()
	if !f.attacks(skel) {
		t.Error("the ruled skeleton left a level 20 player alone")
	}
}

// The owner, 2026-11-28: "make the bandits attack green players too". A GM rule with greens attacks on sight a player
// it is green to; the control: the same rule without greens leaves that player alone.
func TestAGreensRuleAttacksGreenPlayers(t *testing.T) {
	for _, greens := range []bool{false, true} {
		f := newAggroFix(t, 60)
		f.w.SetAggroRules(map[string]AggroRule{"bandit": {Aggressive: true, Greens: greens}})
		b := f.npc(3301, "bandit", 18, bearBody, 1005, 1000)
		f.look()
		if f.attacks(b) != greens {
			t.Errorf("greens %v: a level 18 bandit attacked a 60 on sight: %v, want %v", greens, f.attacks(b), greens)
		}
	}
}
