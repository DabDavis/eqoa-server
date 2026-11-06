package entity

import (
	"testing"
	"time"
)

// A were's name says its line, whichever way round it is spelled and whatever
// its rank; no other creature's does.
func TestWereLine(t *testing.T) {
	for name, want := range map[string]string{
		"wolfwere": "wolfwere", "Elder Werewolf": "wolfwere", "bearwere": "bearwere", "werebear": "bearwere",
		"wererat": "ratwere", "Ratwere": "ratwere", "lionwere": "lionwere", "Werelion": "lionwere",
		"gatorwere": "gatorwere", "Ancient Weregator": "gatorwere",
		"grass snake": "", "wolf": "", "alligator": "", "Levandius": "", "The Bone Crusher": "", "": "",
	} {
		if got := WereLine(name); got != want {
			t.Errorf("WereLine(%q) = %q, want %q", name, got, want)
		}
	}
}

// infectionCase is a caster whose foe is a were, the bites it hears, and the
// dice: each roll is counted and answers roll.
type infectionCase struct {
	*caster
	were  *NPC
	heard []InfectionHit
	rolls int
	roll  int
}

func newInfectionCase(t *testing.T, hear bool) *infectionCase {
	c := &infectionCase{caster: newCaster(t)}
	c.p.CharServerID = 77
	c.p.AC = 100000 // every hit its floor: the druid stands through them all
	c.were = c.npc
	c.were.Name, c.were.HP, c.were.MaxHP = "wolfwere", 5000, 5000
	c.were.CurrentEnemy = c.p.ServerID
	c.were.Threats = map[EntityID]int64{c.p.ServerID: 10}
	if hear {
		c.w.OnInfection(func(h InfectionHit) { c.heard = append(c.heard, h) })
	}
	old := infectIntn
	infectIntn = func(n int) int {
		if n != 10000 {
			t.Fatalf("a bite rolled in %d, not in ten thousand", n)
		}
		c.rolls++
		return c.roll
	}
	t.Cleanup(func() { infectIntn = old })
	return c
}

// hit is one of the were's melee hits on the druid, at full hit points.
func (c *infectionCase) hit() {
	c.p.HP = c.p.MaxHPNow()
	c.w.mu.Lock()
	c.w.npcAttackPlayer(c.were, c.p.ServerID, c.at(time.Second))
	c.w.mu.Unlock()
}

// Every melee hit a were lands rolls the bite (the user's choice of
// 2026-11-06: 1% from a were that cons yellow, 0.1% from one white, dark or
// light blue or red, none from a green): a roll under the chance infects, one
// at it does not, and a green were never rolls. The bite is heard with the
// character, its line and the were.
func TestAWeresHitRollsTheBite(t *testing.T) {
	want := map[byte]int{ConYellow: 100, ConWhite: 10, ConDarkBlue: 10, ConLightBlue: 10, ConRed: 10, ConGreen: 0}
	seen := map[byte]bool{}
	for level := 1; level <= 60; level++ {
		c := newInfectionCase(t, true)
		c.were.Level = level
		con := ConColor(c.p.Level, level)
		chance, ok := want[con]
		if !ok {
			t.Fatalf("level %d cons %d to a %d: no chance in the test", level, con, c.p.Level)
		}
		seen[con] = true
		if got := infectionPer10000(con); got != chance {
			t.Errorf("con %d: %d in ten thousand, want %d", con, got, chance)
		}
		c.roll = chance // at the chance: no bite
		c.hit()
		if len(c.heard) != 0 {
			t.Errorf("level %d (con %d): a roll of %d infected", level, con, c.roll)
		}
		c.roll = chance - 1 // under it: a bite, unless there is no chance
		c.hit()
		switch {
		case chance == 0 && (c.rolls != 0 || len(c.heard) != 0):
			t.Errorf("level %d cons green: %d rolls, %d bites", level, c.rolls, len(c.heard))
		case chance > 0 && (c.rolls != 2 || len(c.heard) != 1):
			t.Errorf("level %d (con %d): %d rolls, %d bites, want 2 and 1", level, con, c.rolls, len(c.heard))
		case chance > 0:
			h := c.heard[0]
			if h.Session != c.sess || h.ServerID != 77 || h.Name != "Druid" || h.Line != "wolfwere" || h.NPCName != "wolfwere" {
				t.Errorf("level %d: heard %+v", level, h)
			}
		}
	}
	for con := range want {
		if !seen[con] {
			t.Errorf("no level 1-60 cons %d to the druid: the case is not tested", con)
		}
	}
}

// No bite from what is no were, from a were that is a pet (a charmed one: a
// player's), nor with no one to hear it; the same hits from a wild were bite.
func TestNoBiteButAWildWeres(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(c *infectionCase)
		hear  bool
		bites int
	}{
		{"a wild were (the control)", func(c *infectionCase) {}, true, 1},
		{"a wolf", func(c *infectionCase) { c.were.Name = "wolf" }, true, 0},
		{"a charmed were", func(c *infectionCase) { c.were.Owner = 4242 }, true, 0},
		{"no hearer", func(c *infectionCase) {}, false, 0},
	} {
		c := newInfectionCase(t, tc.hear)
		c.were.Level = c.p.Level + 1 // yellow
		tc.set(c)
		c.roll = 0
		c.hit()
		bites := len(c.heard)
		if !tc.hear {
			bites = c.rolls // nobody to tell: no roll either
		}
		if bites != tc.bites {
			t.Errorf("%s: %d bites (%d rolls), want %d", tc.name, bites, c.rolls, tc.bites)
		}
	}
}
