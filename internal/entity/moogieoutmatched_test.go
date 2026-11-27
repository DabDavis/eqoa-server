package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A swing at Mr. Moogie is refused with CLIENT's own 0x077A ("You are outmatched", word 1196, which
// also stops its auto-attack), a harmful spell at him with the same words, and nothing of a player's
// reaches him, nor a player's pet (the owner, 2026-11-19: "if players accidently attack mr. moogie lets
// use it"), until she attacks again (moogieconsent_test.go, petition 3331); a creature's hate still does (the GM triggers' raids on him). The control: his Metal
// Moogie, challenged, is still fought.
func TestMrMoogieCannotBeFought(t *testing.T) {
	r := newMetalRig(t)
	r.w.mu.Lock()
	r.moogie.NPCType = 2048 // as both shards have him: attackable to CLIENT (no 0x80), a large creature's reach
	r.w.mu.Unlock()
	a, sa := r.player("Anna", 1000, 1026)
	om := r.see(sa, r.moogie.ServerID)
	drain(sa)

	if r.w.EngageAttack(sa, om, Swing{Kind: 4, Delay: 6 * time.Second, Reach: 30}, r.at(time.Second)) {
		t.Error("Anna engaged Mr. Moogie")
	}
	if a.AttackTarget != 0 {
		t.Errorf("Anna's attack target %d", a.AttackTarget)
	}
	outmatched := 0
	for _, m := range drain(sa) {
		if opOfMsg(m) == protocol.OpcodeOutMatched {
			outmatched++
		}
	}
	if outmatched != 1 {
		t.Errorf("0x077A sent %d times, want 1", outmatched)
	}

	nuke := CastSpell{SpellID: 9301, Name: "Test Bolt", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 300")}
	r.w.mu.Lock()
	if _, why, _ := r.w.castRefusalLocked(sa, a, &nuke, om, r.at(2*time.Second), true); why != MoogieOutmatched {
		t.Errorf("Anna's nuke at Mr. Moogie: %q", why)
	}
	hp := r.moogie.HP
	r.w.landOnNPCLocked(&castState{spell: nuke, caster: a.ServerID, targets: []EntityID{r.moogie.ServerID}}, a, sa, r.moogie, r.at(2*time.Second))
	if r.moogie.HP != hp {
		t.Errorf("a nuke landed on Mr. Moogie: %d -> %d", hp, r.moogie.HP)
	}
	// a player's pet (and so a merc, mercMayFight) may not take him on
	pet := &NPC{ServerID: 9001, Name: "Anna's pet", Owner: a.ServerID, World: r.moogie.World, HP: 100, MaxHP: 100}
	if petMayFight(pet, r.moogie) {
		t.Error("Anna's pet may fight Mr. Moogie")
	}
	// creatures still may: the GM triggers' raids set gods on him ("Nagafen Attacks Mr. Moogie")
	god := EntityID(9100)
	r.w.addThreatLocked(r.moogie, god, 500)
	if r.moogie.Threats[god] == 0 {
		t.Error("a creature's hate on Mr. Moogie was refused: the gods' raids on him would not hold")
	}
	r.w.mu.Unlock()

	// the control: his metal twin, Anna's to fight
	_, _, c := r.challenge(sa, 0)
	if c == nil {
		t.Fatal("no Metal Moogie")
	}
	oc := r.see(sa, c.ServerID)
	if !r.w.EngageAttack(sa, oc, Swing{Kind: 4, Delay: 6 * time.Second, Reach: 30}, r.at(10*time.Second)) || a.AttackTarget != c.ServerID {
		t.Error("Anna (the control) did not engage her Metal Moogie")
	}
}
