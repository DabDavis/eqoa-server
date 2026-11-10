package entity

import (
	"testing"
	"time"
)

// puppetStringsLine is the Puppet Strings' use (petition 1426, transfer 97: "Charms
// mobs from 41+ but oor at 53"): a charm of creatures 41 to 52, Allure's CHA share
// and time.
const puppetStringsLine = "Target, Charm from level 41 up to level 52 ; Increase 25% based on CHA, 3min"

func puppetStrings() PotionEffect {
	return NewPotionEffect(713085, "Puppet Strings", puppetStringsLine, "Puppet Strings", 0, "", 0, 0)
}

// The line reads as a charm of levels 41 to 52 lasting 3 minutes (and the CHA
// share): CharmMin from "from level", CharmMax from "up to level".
func TestTheCharmItemsLineIsRead(t *testing.T) {
	d := ParseSpell(puppetStringsLine, "")
	if !d.Charms() || d.CharmMin != 41 || d.CharmMax != 52 || d.Duration != 3*time.Minute {
		t.Fatalf("charms %v, min %d, max %d, %v", d.Charms(), d.CharmMin, d.CharmMax, d.Duration)
	}
	if c := ParseSpell("Target, Charm ; Increase 25% based on CHA, 3min", ""); c.CharmMin != 0 {
		t.Errorf("the control: a charm with no lowest level read %d", c.CharmMin)
	}
}

// An item whose use charms takes a creature as a charm spell does (charm.go): one
// below its lowest level or above its highest is refused, red, and nothing is spent
// (the refusal is the handler's word to keep the charge); one between is taken, its
// master the user; with a charmed creature out another is refused. A player's
// target is never charmed.
func TestACharmItemTakesACreatureInItsLevels(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, fleshID := charmRig(t)
	crab := r.enemy
	pe := puppetStrings()

	// the crab is 34: below 41
	if why, err := r.w.UsePotion(r.sess, pe, crabID, r.at(0)); err != nil || why != charmTooLow {
		t.Fatalf("a level 34 crab: %q (%v), want %q", why, err, charmTooLow)
	}
	if crab.Owner != 0 {
		t.Fatal("a refused charm took the crab")
	}
	crab.Level = 53
	if why, _ := r.w.UsePotion(r.sess, pe, crabID, r.at(time.Second)); why != charmTooHigh {
		t.Fatalf("a level 53 crab: %q, want %q", why, charmTooHigh)
	}
	crab.Level = 45
	if why, err := r.w.UsePotion(r.sess, pe, crabID, r.at(2*time.Second)); err != nil || why != "" {
		t.Fatalf("a level 45 crab: %q (%v), want it taken", why, err)
	}
	if crab.Owner != r.p.ServerID || crab.Charm == nil {
		t.Fatalf("the crab's owner %d, charm %v: not the user's pet", crab.Owner, crab.Charm)
	}
	if crab.Charm.until.IsZero() {
		t.Error("the charm has no end (its 3 minutes)")
	}
	// a second while the crab is out
	flesh.Level = 45
	if why, _ := r.w.UsePotion(r.sess, pe, fleshID, r.at(3*time.Second)); why != charmHavePet {
		t.Fatalf("with the crab out: %q, want %q", why, charmHavePet)
	}
	if flesh.Owner != 0 {
		t.Error("the flesh was taken with a pet out")
	}
}
