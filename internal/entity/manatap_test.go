package entity

import (
	"strings"
	"testing"
	"time"
)

// Petition 1295 (jibrish): a pariah of Xrag's mana tap "should take 1k mana
// as a baseline, but resist down to 200 mana with max AR". A templated
// creature's nuke pinned to a spell that lowers its target's power takes it,
// by its AR: all of it with none, a fifth at the cap (the dice holding), and
// it hits no hit points (the HP the player loses is the creature's melee).
// The control is TestATemplatesSpellIsResisted: a damage spell at the cap
// still takes ResistCapShare.
func TestATemplatesManaTapTakesPowerByAR(t *testing.T) {
	rig := func() *templateRig {
		r := newTemplateRig(t)
		for _, q := range []string{
			`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
				VALUES (760300, 60, 0, 0, 16880, 1, 0, 0, 0, 1, 3, 0, 'Mana Tap', '')`,
			`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (760300, 'Target, Decrease Power by 1000', 0, 'test')`,
			`INSERT INTO classtemplates (name, class, abilities) VALUES ('tapper', -1, '[{"kind": "nuke", "spell_id": 760300, "every": 10}]')`,
		} {
			if _, err := r.db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		r.assign("gnoll pyro", "tapper")
		return r
	}
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { return 0.5 }

	tap := func(ar int) (lost int, lines []string) {
		t.Helper()
		r := rig()
		pyro := r.npc("Gnoll Pyro")
		p, sess := r.player("Tapped", 5001.5, 100)
		p.ArcaneR, p.Power, p.MaxPower = ar, 5000, 5000
		r.fight(pyro, p.ServerID)
		for d := time.Duration(0); d <= 4*time.Second; d += 100 * time.Millisecond {
			r.tick(d)
			lines = append(lines, linesOf(drain(sess))...)
		}
		return 5000 - p.Power, lines
	}
	if lost, lines := tap(0); lost != 1000 || strings.Join(lines, "|") != "Gnoll Pyro's Mana Tap drains 1000 of your power (your resist took 0%)." {
		t.Errorf("AR 0: %d power lost, lines %q; want 1000", lost, lines)
	}
	if lost, lines := tap(ResistCap); lost != 200 ||
		strings.Join(lines, "|") != "Gnoll Pyro's Mana Tap drains 200 of your power (your resist took 80%)." {
		t.Errorf("AR at the cap: %d power lost, lines %q; want 200", lost, lines)
	}
}
