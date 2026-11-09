package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// wereRatJSON is Feral Rat Form's workbook effects as the database holds
// them (spelleffect 1812): the rat's body by its sprite, and its stats.
const wereRatJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":30,"min_level":1,"req_equipment":"Any","sheet_type":"Alter Appearance","effects":[` +
	`{"id":"0931","type":"Alter Appearance","kind":"illusion","sprite":"0x81073811"},` +
	`{"id":"0932","type":"Statistic Change","kind":"stat","stat":"agi","amount":80}]}`

// wereSpell is a workbook form: name, description and effects as the
// database holds them.
func wereSpell(id int, name, desc, effects string) CastSpell {
	s := illusionSpell(id, name, "", desc)
	s.Def = SpellDefFrom(name, "Self, Gives Form, Increase AGI by 80", desc, effects)
	s.Scope, s.PowerCost = 0, 0 // the rig's power lasts six casts
	return s
}

// ownBodyRank is the rank as tools/seed_were_own_body.py writes it.
func ownBodyRank() database.CMRankRow {
	return database.CMRankRow{ID: 900179, Classes: 32767, Races: 2047, Mastery: "Were Forms Without Illusion", Rank: 1,
		Name: "Were Forms Without Illusion", Level: 30, RequireQuests: "101346|101353|101360|101367|101374",
		Effects: "NoWereIllusion:1", Enabled: true}
}

// The user, 2026-11-08: "lets add a CM 0 Cost that gives you the same Were
// Forms, but without the illusion". Owning the rank, a were form lands
// whole (its stats, its place as the one illusion on) and the channel shows
// the player's own body; removing the rank shows the beast again. The
// controls: an illusion spell, a vampire's form and a shaman's beast form
// keep their bodies under the rank, and without it the form shows as before.
func TestAWereKeepsItsOwnBodyWithTheRank(t *testing.T) {
	c := illusionRig(t)
	c.w.SetCMCatalog([]database.CMRankRow{ownBodyRank()})
	rat := spriteBody(0x81073811)
	feral := wereSpell(1812, "Feral Rat Form", "CM: Illusion", wereRatJSON)
	c.w.RestoreCMs(c.sess, nil) // the stats as a login makes them, the form's measured from there
	agi := c.p.StatNow(StatAgi)

	c.land(t, feral, 0xFFFFFFFF, 0)
	if m, _ := channelBody(c.p); m != rat {
		t.Fatalf("without the rank: model %#x, want the rat's %#x", m, rat)
	}
	c.w.RestoreCMs(c.sess, []database.OwnedCM{{ID: 900179}})
	if m, s := channelBody(c.p); m != gnomeMale || s != 1 {
		t.Fatalf("with the rank: model %#x scale %v, want the gnome's own %#x at 1", m, s, gnomeMale)
	}
	if got := c.p.StatNow(StatAgi); got != agi+80 {
		t.Errorf("the form's AGI under the rank: %d, want %d", got, agi+80)
	}

	// the next form still takes the last one's place, hidden as it is
	mouse := wereSpell(1817, "Mouse Form", "Assume the form of a mouse.", wereRatJSON)
	c.land(t, mouse, 0xFFFFFFFF, 20*time.Second)
	n := 0
	for _, e := range c.p.Effects {
		if e.Form != 0 {
			n++
		}
	}
	if n != 1 || c.p.Effects[len(c.p.Effects)-1].Name != "Mouse Form" {
		t.Errorf("%d illusions on (%+v), want Mouse Form alone", n, c.p.Effects)
	}
	if m, _ := channelBody(c.p); m != gnomeMale {
		t.Errorf("Mouse Form with the rank: model %#x, want the gnome's", m)
	}

	// the controls under the rank: every other body still shows
	c.p.Power = c.p.MaxPower
	c.land(t, troll(), 0xFFFFFFFF, 40*time.Second)
	if m, _ := channelBody(c.p); m != trollMale {
		t.Errorf("Illusion: Troll with the rank: model %#x, want the troll's", m)
	}
	vamp := wereSpell(760002, "Vampire Form", "Assume the form of a vampire.", wereRatJSON)
	c.land(t, vamp, 0xFFFFFFFF, 60*time.Second)
	if m, _ := channelBody(c.p); m != rat {
		t.Errorf("Vampire Form with the rank: model %#x, want its body %#x", m, rat)
	}
	bear := wereSpell(700296, "Bear Form", "", wereRatJSON)
	c.land(t, bear, 0xFFFFFFFF, 80*time.Second)
	if m, _ := channelBody(c.p); m != rat {
		t.Errorf("a shaman's Bear Form with the rank: model %#x, want its body %#x", m, rat)
	}

	// the rank removed: the beast again
	c.land(t, feral, 0xFFFFFFFF, 100*time.Second)
	if m, _ := channelBody(c.p); m != gnomeMale {
		t.Fatalf("Feral Rat Form again with the rank: model %#x, want the gnome's", m)
	}
	c.w.RestoreCMs(c.sess, nil)
	if m, _ := channelBody(c.p); m != rat {
		t.Errorf("the rank removed: model %#x, want the rat's", m)
	}
}

// The rank's effects key is read on its own, slot-free: the window's mods
// stay empty, and a rank without the key (or with it at 0) keeps none.
func TestNoWereIllusionIsReadOffTheEffects(t *testing.T) {
	r := ParseCMRank(ownBodyRank())
	if !r.NoWereIllusion || r.Effects != ([CMSlots]int{}) {
		t.Errorf("the rank: NoWereIllusion %v, effects %v; want true and none", r.NoWereIllusion, r.Effects)
	}
	for _, effects := range []string{"", "No Were Illusion:0", "STR:5"} {
		row := ownBodyRank()
		row.Effects = effects
		if ParseCMRank(row).NoWereIllusion {
			t.Errorf("effects %q read as NoWereIllusion", effects)
		}
	}
	for _, name := range []string{"Ratwere", "Lionesswere", "Sabretooth Lioness Form", " mouse form "} {
		if !IsWereForm(name) {
			t.Errorf("%q is not a were form", name)
		}
	}
	for _, name := range []string{"Vampire Form", "Bat Form", "Bear Form", "Illusion: Troll", "Lupine Guise"} {
		if IsWereForm(name) {
			t.Errorf("%q read as a were form", name)
		}
	}
}
