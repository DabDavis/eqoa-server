package entity

import (
	"strings"
	"testing"
	"time"
)

// buffOf is a self or group buff of the warrior's kind: cast 1 s, landing at
// 1 s + castLand.
func buffOf(id, level, scope int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: level, Scope: scope, Name: name, Icon: uint32(id),
		CastTime: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellText(line)}
}

var (
	callToArms   = buffOf(358, 16, 2, "Call to Arms", "Group, Increase AGI/STA by 20, Increase MaxHP by 80 ; Increase 5% based on WIS, 2min")
	callToAction = buffOf(100184, 8, 2, "Call to Action", "Group, Increase AGI/STA by 15, Increase MaxHP by 30 ; Increase 5% based on WIS, 2min")
	callToCombat = buffOf(671, 24, 2, "Call to Combat", "Group, Increase AGI/STA by 25, Increase MaxHP by 160 ; Increase 5% based on WIS, 2min")
	ironSkin     = buffOf(1539, 25, 0, "Iron Skin", "Self, Increase AC by 102, 10min")
	ironWill     = buffOf(683, 20, 2, "Iron Will", "Group, Increase AC by 100, 60sec")
	recklessF    = buffOf(1536, 23, 0, "Reckless Force", "Self, Decrease AGI by 10, Increase STR by 40, 30min")
	furiousD     = buffOf(1521, 7, 0, "Furious Defense", "Self, Decrease STR by 5, Increase AGI by 20, 30min")
)

// castOn casts sp at the caster itself and lands it.
func (c *caster) castOn(t *testing.T, sp CastSpell, at time.Duration) {
	t.Helper()
	c.p.Power = c.p.MaxPower
	if !c.w.StartCast(c.sess, sp, c.me, c.at(at)) {
		t.Fatalf("%s not begun", sp.Name)
	}
	c.tick(at + sp.CastTime + castLand)
}

func (c *caster) effectNames() []string {
	var out []string
	for _, e := range c.p.Effects {
		out = append(out, e.Name)
	}
	return out
}

// The ranks of a line: a higher one takes the lower one's place, a lower one
// over a higher one does not take (its caster told), and the player's
// maximum hit points are the best rank's alone.
func TestOnlyTheBestOfABuffLineCounts(t *testing.T) {
	c := newCaster(t)
	c.castOn(t, callToArms, 0)
	if got := c.effectNames(); len(got) != 1 || c.p.Mods.MaxHP < 80 {
		t.Fatalf("Call to Arms: effects %v, max HP +%d", got, c.p.Mods.MaxHP)
	}
	arms := c.p.Mods
	c.castOn(t, callToAction, 10*time.Second) // a lower rank
	if got := c.effectNames(); len(got) != 1 || got[0] != "Call to Arms" || c.p.Mods != arms {
		t.Fatalf("a lower rank over a higher: effects %v, mods %+v (want %+v)", got, c.p.Mods, arms)
	}
	if red := redLines(c.sess); len(red) == 0 || !strings.Contains(red[len(red)-1], "Call to Arms, which is stronger") {
		t.Errorf("the caster was not told: %q", red)
	}
	c.castOn(t, callToCombat, 20*time.Second) // a higher rank
	if got := c.effectNames(); len(got) != 1 || got[0] != "Call to Combat" || c.p.Mods.MaxHP < 160 || c.p.Mods.MaxHP >= 240 {
		t.Fatalf("a higher rank: effects %v, max HP +%d (want Call to Combat's alone)", got, c.p.Mods.MaxHP)
	}
	c.castOn(t, callToCombat, 30*time.Second) // a recast of it: itself again
	if got := c.effectNames(); len(got) != 1 {
		t.Fatalf("a recast: effects %v", got)
	}
}

// The control: what is not one line stacks as before. Another stat (AC
// beside the call's), the same two stats moved the other way (Reckless
// Force's +STR -AGI and Furious Defense's +AGI -STR), and an effect kept from
// before the rule (no line: a relog of 2026-10-29's) all stay on; a self and
// a group form of one stat are one line (Iron Skin over Iron Will), as a
// player's and a pet's are not.
func TestOtherBuffLinesStillStack(t *testing.T) {
	c := newCaster(t)
	kept := &ActiveEffect{SpellID: 100184, Name: "Call to Action", Until: c.at(time.Hour), Mods: StatMods{MaxHP: 30}}
	c.p.Effects = []*ActiveEffect{kept}
	for i, sp := range []CastSpell{callToArms, ironSkin, recklessF, furiousD} {
		c.castOn(t, sp, time.Duration(i)*10*time.Second)
	}
	got := c.effectNames()
	if len(got) != 5 {
		t.Fatalf("effects %v: want all five on", got)
	}
	if red := redLines(c.sess); len(red) != 0 {
		t.Errorf("told %q", red)
	}
	for _, sp := range []CastSpell{ironSkin, ironWill, recklessF, furiousD, callToArms} {
		if BuffLine(&sp) == "" {
			t.Errorf("%s has no line", sp.Name)
		}
	}
	if BuffLine(&recklessF) == BuffLine(&furiousD) {
		t.Errorf("lines: %q %q", BuffLine(&recklessF), BuffLine(&furiousD))
	}
	if BuffLine(&ironSkin) != BuffLine(&ironWill) {
		t.Errorf("a self and a group AC buff are two lines: %q %q", BuffLine(&ironSkin), BuffLine(&ironWill))
	}
	c.castOn(t, ironWill, 50*time.Second) // a lower rank (20) than Iron Skin (25)
	if got := c.effectNames(); len(got) != 5 {
		t.Errorf("Iron Will over Iron Skin took: %v", got)
	}
	petBuff := ironWill
	petBuff.Scope = 3
	if BuffLine(&petBuff) == BuffLine(&ironWill) {
		t.Errorf("a pet's buff is a player's line: %q", BuffLine(&petBuff))
	}
	if BuffLine(&callToArms) != BuffLine(&callToAction) || BuffLine(&callToArms) != BuffLine(&callToCombat) {
		t.Errorf("the calls are not one line: %q %q %q", BuffLine(&callToAction), BuffLine(&callToArms), BuffLine(&callToCombat))
	}
	// a heal over time keeps hot.go's rule: no line
	if hot := hotSpell(901, 30, "Mending"); BuffLine(&hot) != "" {
		t.Errorf("a heal over time has a line: %q", BuffLine(&hot))
	}
}

// A kept effect keeps its line over a logout, so the rule holds after it.
func TestABuffLineIsKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	c.castOn(t, callToCombat, 0)
	rows := savedEffects(c.p, c.at(5*time.Second))
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, c.at(6*time.Second)); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if e := c.p.Effects[0]; e.Line != BuffLine(&callToCombat) || e.Rank != 24 {
		t.Fatalf("restored line %q rank %d", e.Line, e.Rank)
	}
	c.castOn(t, callToArms, 10*time.Second)
	if got := c.effectNames(); len(got) != 1 || got[0] != "Call to Combat" {
		t.Errorf("after a relog a lower rank took: %v", got)
	}
}
