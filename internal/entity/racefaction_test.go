package entity

import (
	"fmt"
	"math/bits"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// raceFactionRig is illusionRig's gnome enchanter with a faction whose elves
// start at 800 and gnomes at -200, and its standing as the world sees it.
func raceFactionRig(t *testing.T) (*caster, string, func() int) {
	t.Helper()
	const court = "Test Elven Court"
	c := illusionRig(t)
	c.w.SetFactionDefaults([]database.FactionDefault{{Name: court, Standing: 0}})
	c.w.SetFactionStarts([]database.FactionStartRow{
		{Race: 1, Class: -1, HumType: -1, Faction: court, Standing: 800},
		{Race: 3, Class: -1, HumType: -1, Faction: court, Standing: -200},
	})
	return c, court, func() int {
		v, _ := c.w.Standing(c.sess, court)
		return v
	}
}

// workbookIllusion is a spell as the workbook writes a race glamour (Heirloom
// petition 2792's Glamour: Halfling, record 517): DEX, a faction change and
// the race's body by its sprite, optionally without the faction change.
func workbookIllusion(id int, name string, body uint32, faction bool) CastSpell {
	fx := ""
	if faction {
		fx = `{"kind":"faction"},`
	}
	js := fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Friendly","duration_s":3600,"effects":[`+
		`{"kind":"stat","stat":"dex","amount":10},%s{"kind":"illusion","sprite":"0x%08X"}]}`, fx, bits.ReverseBytes32(body))
	return CastSpell{SpellID: id, DisplayIndex: 5, Scope: 1, Name: name, Icon: 0x12345678, CastTime: 5 * time.Second,
		Recast: time.Second, PowerCost: 120, Range: 30, Def: SpellDefFrom(name, "", "", js)}
}

// A race glamour carrying the workbook's faction change shows its bearer's
// standings as the race's (petition 2792), as an item's "Race Faction" line
// does (work board 1258): the shown standing shifted, the faction window and a
// kill's hit on the real one. The controls: the same glamour without the
// faction change shifts nothing, and a non-race form (a Clay Golem) keeps the
// disguise alone.
func TestARaceGlamourShowsTheRacesStandings(t *testing.T) {
	c, court, standing := raceFactionRig(t)
	if got := standing(); got != -200 {
		t.Fatalf("before: %d, want the gnome's -200", got)
	}
	c.land(t, workbookIllusion(517, "Glamour: Elf", elfMale, true), 0xFFFFFFFF, 0)
	if m, _ := channelBody(c.p); m != elfMale {
		t.Fatalf("the glamour's body: %d, want the elf man's", m)
	}
	if got := standing(); got != 800 {
		t.Fatalf("as an elf: %d, want 800", got)
	}
	c.w.mu.Lock()
	c.w.takeFactionHitsLocked(nil, c.p, c.npc, []FactionHit{{Faction: court, Delta: -10}})
	c.w.mu.Unlock()
	if c.p.Factions[court] != -210 || standing() != 790 {
		t.Fatalf("a kill's -10: own %d (want -210), shown %d (want 790)", c.p.Factions[court], standing())
	}
	for _, f := range c.w.PlayerFactions(c.sess) {
		if f.Name == court && f.Standing != -210 {
			t.Errorf("the faction window: %d, want its own -210", f.Standing)
		}
	}
	c.w.mu.Lock()
	c.w.endIllusionsLocked(c.p, 0)
	c.w.mu.Unlock()
	if got := standing(); got != -210 {
		t.Errorf("after the glamour: %d, want its own -210", got)
	}

	// the control: no faction change
	c2, _, standing2 := raceFactionRig(t)
	c2.land(t, workbookIllusion(518, "Glamour: Elf", elfMale, false), 0xFFFFFFFF, 0)
	if got := standing2(); got != -200 {
		t.Errorf("no faction change: %d, want -200", got)
	}

	// a body named otherwise (a sprite alone): its race's body decides
	c3, _, standing3 := raceFactionRig(t)
	c3.land(t, workbookIllusion(519, "Elven Grace", elfMale, true), 0xFFFFFFFF, 0)
	if got := standing3(); got != 800 {
		t.Errorf("an elf's body by another name: %d, want 800", got)
	}

	// a form that is no race: the disguise, no shift
	c4, _, standing4 := raceFactionRig(t)
	c4.land(t, workbookIllusion(520, "Glamour: Clay Golem", clayGolem, true), 0xFFFFFFFF, 0)
	if !c4.p.disguised(c4.at(10 * time.Second)) {
		t.Error("the golem glamour gives no disguise")
	}
	if got := standing4(); got != -200 {
		t.Errorf("a golem's glamour: %d, want -200", got)
	}
}

// The bearer's first harmful act on a creature sees through the disguise
// (ours): the regard and the disguise end, the body and stats stay to their
// time, and the bearer is told once. Engaging a creature does it; so does a
// harmful spell at one.
func TestADisguiseIsSeenThroughByAnAttack(t *testing.T) {
	for _, how := range []string{"attack", "spell"} {
		c, _, standing := raceFactionRig(t)
		c.land(t, workbookIllusion(517, "Glamour: Elf", elfMale, true), 0xFFFFFFFF, 0)
		if got := standing(); got != 800 {
			t.Fatalf("%s: as an elf %d, want 800", how, got)
		}
		at := 20 * time.Second
		switch how {
		case "attack":
			if !c.w.EngageAttack(c.sess, c.npcO, Swing{Kind: 0, Delay: 3 * time.Second, Reach: 5}, c.at(at)) {
				t.Fatalf("not engaged: %x", c.sent())
			}
		case "spell":
			nuke := illusionSpell(9001, "Test Bolt", "Target, Damage 10, Instant", "")
			nuke.Def.Harmful = true
			if !c.w.StartCast(c.sess, nuke, c.npcO, c.at(at)) {
				t.Fatalf("the bolt did not begin: %x", c.sent())
			}
		}
		said := 0
		for _, m := range c.sent() {
			if chatText(m) == disguiseSeenThrough {
				said++
			}
		}
		if said != 1 {
			t.Errorf("%s: %q said %d times, want once", how, disguiseSeenThrough, said)
		}
		if got := standing(); got != -200 {
			t.Errorf("%s: after: %d, want its own -200", how, got)
		}
		if c.p.disguised(c.at(at + time.Second)) {
			t.Errorf("%s: still disguised", how)
		}
		if m, _ := channelBody(c.p); m != elfMale || c.p.Mods.Dex != 10 {
			t.Errorf("%s: body %d, DEX +%d; want the elf's body and +10 kept", how, m, c.p.Mods.Dex)
		}
	}
}

// A coachman of a good faction refuses a gnome it hates and serves it in an
// elf's glamour that carries the faction change (Heirloom petition 2794: an
// evil character in an erudite glamour could use no good coach), the face
// too; once the glamour ends it refuses again.
func TestAGoodCoachServesARaceGlamour(t *testing.T) {
	c, court, _ := raceFactionRig(t)
	coach := &NPC{ServerID: 6001, Name: "Coachman Test", Level: 30, X: 104, Y: 10, Z: 100}
	c.w.NPCs[coach.ServerID] = coach
	c.w.SetNPCFactions([]database.NPCFactionRow{{Name: "coachman test", World: -1, Faction: court}})
	c.p.Factions = map[string]int{court: -900} // the gnome's own: KOS
	if !c.w.Refuses(c.sess, coach) {
		t.Fatal("the good coach served a gnome at -900")
	}
	c.land(t, workbookIllusion(517, "Glamour: Elf", elfMale, true), 0xFFFFFFFF, 0)
	if c.w.Refuses(c.sess, coach) {
		t.Errorf("in the elf's glamour (shown %d): refused", -900+800+200)
	}
	c.w.mu.Lock()
	c.w.endIllusionsLocked(c.p, 0)
	c.w.mu.Unlock()
	if !c.w.Refuses(c.sess, coach) {
		t.Error("after the glamour: served")
	}
}
