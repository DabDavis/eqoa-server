package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// toyStringsLine is the Toy Shoppe's Puppet Strings' use (tools/puppet_strings_20261117.py).
const toyStringsLine = "Target, Charm up to light blue, No Recharge, 10min"

func toyStrings(cooldown time.Duration) PotionEffect {
	pe := NewPotionEffect(716914, "Puppet Strings", toyStringsLine, "Puppet Strings", 0, "", 0, 0)
	pe.Cooldown = cooldown
	return pe
}

// The line reads as a charm by colour up to light blue, never recharged, of 10
// minutes; the Cazic Thule strings' line is no charm by colour (the control).
func TestTheCharmByColourLineIsRead(t *testing.T) {
	d := ParseSpell(toyStringsLine, "")
	if !d.Charms() || !d.Harmful || !d.CharmByCon || d.CharmCon != ConLightBlue || !d.NoRecharge || d.Duration != 10*time.Minute ||
		d.CharmMax != 0 || d.CharmMin != 0 || len(d.Unread) != 0 {
		t.Fatalf("%+v", d)
	}
	if c := ParseSpell(puppetStringsLine, ""); c.CharmByCon || c.NoRecharge {
		t.Errorf("the Cazic Thule strings: by colour %v, no recharge %v", c.CharmByCon, c.NoRecharge)
	}
	if g := ParseSpell("Target, Charm up to green, 1min", ""); !g.CharmByCon || g.CharmCon != ConGreen {
		t.Errorf("up to green: %v %d", g.CharmByCon, g.CharmCon)
	}
}

// The owner (2026-11-17): "charms the target if light blue or lower". To an
// enchanter of 49 a creature of 49 is white, 47-48 dark blue (refused, "too
// strong", nothing taken), 46 light blue (taken), 1 green (taken). A taken
// one is never resisted (the dice say resist), lasts 10 minutes, and is the
// user's pet; with it out another is refused.
func TestTheStringsCharmLightBlueAndGreen(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(0) // every roll a resist: the strings roll none
	r, flesh, fleshID := charmRig(t)
	crab := r.enemy
	pe := toyStrings(0)
	for i, lvl := range []int{55, 50, 49, 48, 47} { // red, yellow, white, dark blue x2
		crab.Level = lvl
		if why, err := r.w.UsePotion(r.sess, pe, crabID, r.at(time.Duration(i)*time.Second)); err != nil || why != charmTooStrong {
			t.Fatalf("a level %d crab (con %d): %q (%v), want %q", lvl, ConColor(49, lvl), why, err, charmTooStrong)
		}
		if crab.Owner != 0 || crab.Charm != nil {
			t.Fatalf("a refused level %d crab was taken", lvl)
		}
	}
	crab.Level = 46
	if ConColor(49, 46) != ConLightBlue {
		t.Fatal("the rig: 46 is not light blue to 49")
	}
	if why, err := r.w.UsePotion(r.sess, pe, crabID, r.at(10*time.Second)); err != nil || why != "" {
		t.Fatalf("a light blue crab: %q (%v), want it taken", why, err)
	}
	if crab.Owner != r.p.ServerID || crab.Charm == nil {
		t.Fatalf("the crab's owner %d, charm %v: not the user's pet", crab.Owner, crab.Charm)
	}
	if got := crab.Charm.until.Sub(r.at(10 * time.Second)); got != 10*time.Minute {
		t.Errorf("the charm lasts %v, want 10m (no CHA share)", got)
	}
	if ConColor(49, flesh.Level) != ConGreen {
		t.Fatal("the rig: the flesh is not green")
	}
	if why, _ := r.w.UsePotion(r.sess, pe, fleshID, r.at(11*time.Second)); why != charmHavePet {
		t.Fatalf("with the crab out: %q, want %q", why, charmHavePet)
	}
	if flesh.Owner != 0 {
		t.Fatal("the flesh was taken with a pet out")
	}
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(12*time.Second))
	if crab.Owner != 0 {
		t.Fatal("the dismissed crab is still the pet")
	}
	if why, _ := r.w.UsePotion(r.sess, pe, fleshID, r.at(13*time.Second)); why != "" || flesh.Owner != r.p.ServerID {
		t.Fatalf("a green flesh: %q, owner %d; want it taken", why, flesh.Owner)
	}
}

// What no charm takes is refused, nothing taken: the user (a player), a name
// npccharm makes uncharmable or never charmed (resist 100), a raid boss, a
// quest step's spawn, a trigger's, a training dummy, a rare standing. A summoned
// pet out refuses too (the strings never send one away).
func TestTheStringsRefuseWhatNoCharmTakes(t *testing.T) {
	r, _, _ := charmRig(t)
	crab := r.enemy
	crab.Level = 30 // light blue to 49
	pe := toyStrings(0)
	at := time.Duration(0)
	use := func(target uint32) string {
		t.Helper()
		at += time.Second
		why, err := r.w.UsePotion(r.sess, pe, target, r.at(at))
		if err != nil {
			t.Fatal(err)
		}
		return why
	}
	if why := use(r.sess.PlayerObjectID()); why != castFriendly {
		t.Errorf("at the user: %q, want %q", why, castFriendly)
	}
	key := database.NPCClassKey(crab.Name)
	for _, c := range []struct {
		name string
		set  func()
		undo func()
	}{
		{"uncharmable", func() { r.w.SetCharmRules(map[string]database.CharmRule{key: {Charmable: false}}) },
			func() { r.w.SetCharmRules(nil) }},
		{"resist 100", func() { r.w.SetCharmRules(map[string]database.CharmRule{key: {Charmable: true, ResistPct: 100}}) },
			func() { r.w.SetCharmRules(nil) }},
		{"a raid boss", func() { r.w.SetRaidBossNames([]string{crab.Name}) }, func() { r.w.SetRaidBossNames(nil) }},
		{"a quest spawn", func() { crab.QuestSpawn = true }, func() { crab.QuestSpawn = false }},
		{"a trigger's spawn", func() { crab.EventAggro = true }, func() { crab.EventAggro = false }},
		{"a training dummy", func() { crab.Dummy = TrainingDummy }, func() { crab.Dummy = NotDummy }},
		{"a rare standing", func() {
			rec := &campRec{rec: database.NPCRecord{NPCName: crab.Name}}
			r.w.rares = map[EntityID]*rareRule{crab.ServerID: {own: rec, rare: rec, chance: 10}}
		}, func() { r.w.rares = nil }},
	} {
		c.set()
		if why := use(crabID); why != charmCannot {
			t.Errorf("%s: %q, want %q", c.name, why, charmCannot)
		}
		if crab.Owner != 0 {
			t.Fatalf("%s: taken", c.name)
		}
		c.undo()
	}
	// a summoned pet out
	pet := &NPC{ServerID: 3400, Name: "a summoned servant", Level: 20, HP: 10, MaxHP: 10, World: 2, Owner: r.p.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	if r.w.pets == nil {
		r.w.pets = make(map[EntityID]*petState)
	}
	r.w.pets[r.p.ServerID] = &petState{owner: r.p, npc: pet}
	if why := use(crabID); why != charmHavePet {
		t.Errorf("with a summoned pet out: %q, want %q", why, charmHavePet)
	}
	if r.w.pets[r.p.ServerID].npc != pet || crab.Owner != 0 {
		t.Error("the summoned pet was sent away, or the crab taken")
	}
	delete(r.w.pets, r.p.ServerID)
	if why := use(crabID); why != "" || crab.Owner != r.p.ServerID {
		t.Fatalf("the control, nothing in the way: %q, owner %d", why, crab.Owner)
	}
}

// The cooldown (60 s, kept by potionReady): a refusal begins none; a charm
// does, and a use inside it is refused with its time left.
func TestTheStringsCooldown(t *testing.T) {
	r, flesh, fleshID := charmRig(t)
	crab := r.enemy
	pe := toyStrings(time.Minute)
	crab.Level = 49 // white: refused
	if why, _ := r.w.UsePotion(r.sess, pe, crabID, r.at(0)); why != charmTooStrong {
		t.Fatalf("white: %q", why)
	}
	crab.Level = 40
	if why, _ := r.w.UsePotion(r.sess, pe, crabID, r.at(time.Second)); why != "" || crab.Owner != r.p.ServerID {
		t.Fatalf("a refusal began a cooldown: %q", why)
	}
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(2*time.Second))
	why, _ := r.w.UsePotion(r.sess, pe, fleshID, r.at(31*time.Second))
	if why == "" || flesh.Owner != 0 {
		t.Fatalf("inside the cooldown: %q, owner %d", why, flesh.Owner)
	}
	if why, _ := r.w.UsePotion(r.sess, pe, fleshID, r.at(61*time.Second)); why != "" || flesh.Owner != r.p.ServerID {
		t.Fatalf("after it: %q, owner %d", why, flesh.Owner)
	}
}
