package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Work board 1258's three parts (synthetic items and figures).

// A worn item's own run speed is a share of the base run added beside the
// movement buff: 7 makes SoW's 1.30 times the run 1.37; alone 1.07; a slow
// takes its share of the whole; a hold is still 0. Put on and taken off, the
// client is told the speed (SetGear: 0x00F8). The control: nothing worn.
func TestAWornItemsOwnSpeedAddsToTheRun(t *testing.T) {
	SetItemSpeeds(map[int]float64{996900: 7})
	defer SetItemSpeeds(nil)
	c := newCaster(t)
	p := c.p
	run := p.Speed
	if got := p.SpeedNow(); got != run {
		t.Fatalf("the control, nothing worn: %v, want %v", got, run)
	}
	g := p.Gear
	g.SpeedPct = ItemSpeedPct(996900) + ItemSpeedPct(1) // a record of none adds none
	c.w.SetGear(p.ServerID, g, false)
	if got, want := p.SpeedNow(), run*1.07; !speedNear(got, want) {
		t.Fatalf("worn alone: %v, want %v", got, want)
	}
	if got := speedsSent(c.sent()); len(got) != 1 || !speedNear(got[0], run*1.07) {
		t.Fatalf("speeds told at the putting on %v, want [%v]", got, run*1.07)
	}
	if r := c.use(t, NewPotionEffect(1015, "Alacrity Potion", "Self, Sow, 30min", "", 638503769, "", 0, 0), 0xFFFFFFFF, 0); r != "" {
		t.Fatalf("Alacrity refused: %q", r)
	}
	if got, want := p.SpeedNow(), run*(speedBuff+0.07); !speedNear(got, want) {
		t.Fatalf("with SoW: %v, want %v (1.37 times the run)", got, want)
	}
	p.Effects = append(p.Effects, &ActiveEffect{SpellID: 7, Root: true, Until: c.at(time.Hour)})
	if got := p.SpeedNow(); got != 0 {
		t.Fatalf("held: %v, want 0", got)
	}
	p.Effects = p.Effects[:len(p.Effects)-1]
	c.sent()
	c.w.SetGear(p.ServerID, Gear{}, false)
	if got := speedsSent(c.sent()); len(got) != 1 || !speedNear(got[0], run*speedBuff) {
		t.Fatalf("speeds told at the taking off %v, want [%v]", got, run*speedBuff)
	}
}

func speedNear(a, b float32) bool { return a-b < 1e-4 && b-a < 1e-4 }

// A race illusion whose line says "Race Faction" shows its bearer's standings
// as the race's: its own plus the race's start less its own race's; the
// faction window shows its own, a kill's hit moves its own, and at the
// illusion's end the world sees its own again. The control: the same
// illusion without the words changes no standing.
func TestARaceFactionIllusionShowsTheRacesStandings(t *testing.T) {
	const felwithe = "Test Elven Court"
	for _, withWords := range []bool{true, false} {
		c := illusionRig(t) // a gnome enchanter
		c.w.SetFactionDefaults([]database.FactionDefault{{Name: felwithe, Standing: 0}})
		c.w.SetFactionStarts([]database.FactionStartRow{
			{Race: 1, Class: -1, HumType: -1, Faction: felwithe, Standing: 800},
			{Race: 3, Class: -1, HumType: -1, Faction: felwithe, Standing: -200},
		})
		standing := func() int {
			v, _ := c.w.Standing(c.sess, felwithe)
			return v
		}
		if got := standing(); got != -200 {
			t.Fatalf("before: %d, want the gnome's -200", got)
		}
		line := "Self, Illusion: Elf, Unlimited Uses, 30min"
		if withWords {
			line = "Self, Illusion: Elf, Race Faction, Unlimited Uses, 30min"
		}
		guise := NewPotionEffect(996901, "Test Guise", line, "Illusion: Elf", 638503769, "", 0, 0)
		if r := c.use(t, guise, 0xFFFFFFFF, 0); r != "" {
			t.Fatalf("refused: %q", r)
		}
		if !withWords {
			if got := standing(); got != -200 {
				t.Errorf("the control, no Race Faction: %d, want -200", got)
			}
			continue
		}
		if got := standing(); got != 800 {
			t.Fatalf("as an elf: %d, want 800", got)
		}
		c.p.Factions = map[string]int{felwithe: -100}
		if got := standing(); got != 900 {
			t.Fatalf("own -100 as an elf: %d, want 900", got)
		}
		c.w.mu.Lock()
		c.w.takeFactionHitsLocked(nil, c.p, c.npc, []FactionHit{{Faction: felwithe, Delta: -10}})
		c.w.mu.Unlock()
		if c.p.Factions[felwithe] != -110 || standing() != 890 {
			t.Fatalf("a kill's -10: own %d (want -110), shown %d (want 890)", c.p.Factions[felwithe], standing())
		}
		for _, f := range c.w.PlayerFactions(c.sess) {
			if f.Name == felwithe && f.Standing != -110 {
				t.Errorf("the faction window: %d, want its own -110", f.Standing)
			}
		}
		c.p.Factions[felwithe] = 1900
		if got := standing(); got != 2000 {
			t.Errorf("held to 2000: %d", got)
		}
		c.w.mu.Lock()
		c.w.endIllusionsLocked(c.p, 0)
		c.w.mu.Unlock()
		if got := standing(); got != 1900 {
			t.Errorf("after the illusion: %d, want its own 1900", got)
		}
	}
}

// "Procs N power to its wielder" is a proc of power alone, its chance by the
// wielder's highest DEX, INT or WIS: 10% and 0.15% a point (100% at 600).
// Under the chance the wielder gets 100 power and the creature nothing; over
// it, nothing. The control: a hit proc's words still read as before.
func TestAPowerProcGivesItsWielderPower(t *testing.T) {
	desc := "An open, kindly eye sewn to a bag. Procs 100 power to its wielder, a 10% chance each hit, plus 0.15% for each point of its wielder's highest DEX, INT or WIS (100% at 600)."
	wp, ok := WeaponProcFromText(desc)
	if !ok || wp.Kind != ProcPower || wp.Power != 100 || wp.ChanceBase != 10 || wp.ChanceStatPer != 0.15 || wp.Chance != 0 {
		t.Fatalf("read %+v %v", wp, ok)
	}
	if hit, ok := WeaponProcFromText("Procs 140 fire damage, 10% chance."); !ok || hit.Kind != ProcHit || hit.Damage != 140 || hit.Chance != 10 {
		t.Fatalf("the control, a hit proc: %+v %v", hit, ok)
	}
	r := weaponRig(t, "Test Bag", desc)
	f := r.friend
	f.Dex, f.Int, f.Wis = 100, 300, 200
	if got := r.friend.Gear.MainProc.statChancePct(f); got != 55 {
		t.Fatalf("chance at INT 300: %v, want 55", got)
	}
	if got := (&WeaponProc{ChanceBase: 90, ChanceStatPer: 0.15}).statChancePct(f); got != 100 {
		t.Fatalf("a chance past 100: %v, want 100", got)
	}
	defer func(fn func(int) int) { procIntn = fn }(procIntn)
	f.Power, f.MaxPower = 10, 5000
	procIntn = func(n int) int { // under the chance (54,999 of 100,000), any melee roll low
		if n == 100000 {
			return 54999
		}
		return 0
	}
	hp := r.npc.HP
	r.hit()
	melee := figures(r.fsess)
	if f.Power != 110 {
		t.Errorf("under the chance: power %d, want 110", f.Power)
	}
	if len(melee) != 1 || hp-r.npc.HP != -melee[0] {
		t.Errorf("under the chance: figures %v, the creature lost %d: want the melee alone", melee, hp-r.npc.HP)
	}
	procIntn = func(n int) int {
		if n == 100000 {
			return 55000
		}
		return 0
	}
	r.hit()
	if f.Power != 110 {
		t.Errorf("over the chance: power %d, want 110", f.Power)
	}
}
