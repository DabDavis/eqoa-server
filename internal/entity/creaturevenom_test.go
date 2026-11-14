package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A wild creature's poison (petitions 5030/5032: "they do not poison you"; tools/creature_venom_20261114.py gives
// Prima's poisoning and infecting kinds a "dot" pinned to a damage over time at a percent of its figure): the dot
// lands on the player it fights at that percent, each tick less the player's resist, and is kept up, never two.
func TestACreaturesPoisonDoTLandsOnAPlayerAtItsPercent(t *testing.T) {
	r := newTemplateRig(t)
	const bolt = 7301
	if _, err := r.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (?, 60, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Toxic Bolt', '')`, bolt); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, 'Target, Decrease HP per tic by 114, 24sec', 99, 'test',
		'{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","effects":[{"id":"F1","type":"Dmg/Heal","kind":"hp_tick","amount":-114,"resist":"pr"}]}')`, bolt); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('roamer lesser poison', 0, ?)`,
		fmt.Sprintf(`[{"kind":"dot","spell_id":%d,"every":24,"first":4,"damage_pct":13}]`, bolt)); err != nil {
		t.Fatal(err)
	}
	if got := r.assign("gnoll guard", "roamer lesser poison"); got != 1 {
		t.Fatalf("%d creatures of the template", got)
	}
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { return 0.5 }
	guard := r.npc("Gnoll Guard")
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 100000, 100000
	r.fight(guard, p.ServerID)
	for d := time.Duration(0); d <= 6*time.Second; d += 500 * time.Millisecond {
		r.tick(d)
	}
	var on []*ActiveEffect
	for _, e := range p.Effects {
		if e.SpellID == bolt {
			on = append(on, e)
		}
	}
	if len(on) != 1 || on[0].HPTick != -15 || on[0].npcFoe != guard.ServerID || on[0].npcResist != StatPR {
		t.Fatalf("its poison on the player: %d effects (%+v), want one of -15 a tick (114 at 13%%) from the creature, PR; lines %q",
			len(on), on, linesOf(drain(sess)))
	}
	lines := strings.Join(linesOf(drain(sess)), "\n")
	if !strings.Contains(lines, "Toxic Bolt hits you for 15") {
		t.Errorf("no tick told: %q", lines)
	}
}
