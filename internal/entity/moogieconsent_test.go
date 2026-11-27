package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// moogieConsentRig is the metal rig with Mr. Moogie on a template of his own ("mr moogie": a raid
// boss's hit points, a melee of his), attackable to CLIENT as both shards have him (npc type 2048).
func moogieConsentRig(t *testing.T) *metalRig {
	t.Helper()
	r := newMetalRigWith(t, func(exec func(q string, args ...any)) {
		exec(`INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, proc_chance, proc_damage, proc_name, proc_resist,
			kick_damage, kick_every, kick_first, abilities, note, source)
			VALUES ('mr moogie', 0, 6000, 100, 400, 0, 0, '', '', 0, 0, 0, '[]', 'test', 'test')`)
		exec(`INSERT INTO npctemplate (name, template, note, source) VALUES ('mr. moogie', 'mr moogie', 'test', 'test')`)
	})
	r.w.mu.Lock()
	r.moogie.NPCType = 2048
	r.w.mu.Unlock()
	if r.moogie.tmpl == nil {
		t.Fatal("Mr. Moogie has no template in the rig")
	}
	return r
}

// countOutMatched drains the session and counts its 0x077A.
func countOutMatched(_ *metalRig, sessOf func() [][]byte) int {
	n := 0
	for _, m := range sessOf() {
		if opOfMsg(m) == protocol.OpcodeOutMatched {
			n++
		}
	}
	return n
}

var moogieSwing = Swing{Kind: 4, Delay: 2 * time.Second, Reach: 30}

// A player's first swing at Mr. Moogie is warned off with 0x077A and nothing lands; the same press
// read again within moogieAgainAfter is still warned (no consent); a second press within
// MoogieAgainWindow engages him, and the fight is an ordinary one: her rounds land on him, his hate
// takes her, his template's melee hits back (Heirloom petition 3331; the owner, 2026-11-27: "the
// first warning should say outmatched, but if they attack again it should let them"). Meanwhile
// Mr. Moogie will not help her (MoogieBusy).
func TestMrMoogieSecondSwingFights(t *testing.T) {
	r := moogieConsentRig(t)
	a, sa := r.player("Anna", 1000, 1004)
	om := r.see(sa, r.moogie.ServerID)
	drain(sa)
	hp0 := r.moogie.HP

	if r.w.EngageAttack(sa, om, moogieSwing, r.at(time.Second)) || a.AttackTarget != 0 {
		t.Fatal("Anna's first swing engaged Mr. Moogie")
	}
	if n := countOutMatched(r, func() [][]byte { return drain(sa) }); n != 1 {
		t.Errorf("first swing: 0x077A sent %d times, want 1", n)
	}
	if r.w.EngageAttack(sa, om, moogieSwing, r.at(1500*time.Millisecond)) {
		t.Error("the same press read again (0.5 s on) engaged him")
	}
	drain(sa)
	if r.moogie.HP != hp0 || len(r.moogie.Threats) != 0 {
		t.Fatalf("after the warnings: HP %d of %d, threats %v", r.moogie.HP, hp0, r.moogie.Threats)
	}

	if !r.w.EngageAttack(sa, om, moogieSwing, r.at(5*time.Second)) || a.AttackTarget != r.moogie.ServerID {
		t.Fatal("Anna's second swing (4 s after the warning) did not engage Mr. Moogie")
	}
	if n := countOutMatched(r, func() [][]byte { return drain(sa) }); n != 0 {
		t.Errorf("second swing: 0x077A sent %d times, want none", n)
	}
	a.AutoAttack = true
	aHP := a.HP
	for d := 5 * time.Second; d < 90*time.Second; d += 200 * time.Millisecond {
		r.w.mu.Lock()
		r.w.attackSystem(r.at(d))
		r.w.mu.Unlock()
		r.tick(d)
		drain(sa)
		if r.moogie.HP < hp0 && a.HP < aHP {
			break
		}
	}
	if r.moogie.HP >= hp0 {
		t.Errorf("Anna's rounds took nothing off Mr. Moogie (%d)", r.moogie.HP)
	}
	if r.moogie.Threats[a.ServerID] == 0 || r.moogie.CurrentEnemy != a.ServerID {
		t.Errorf("his hate: %v, enemy %d", r.moogie.Threats, r.moogie.CurrentEnemy)
	}
	if a.HP >= aHP {
		t.Errorf("Mr. Moogie never hit Anna back (HP %d)", a.HP)
	}
	if why := r.w.MoogieBusy(sa, r.moogie.ServerID); why != MoogieLineFoe {
		t.Errorf("his help while she fights him: %q", why)
	}
}

// The consent lasts while the fight does and MoogieConsentAfter after: a press 30 s after his hate let
// her go still engages; one past it is warned again, and so is the next one once a warning's window
// has run out unanswered; a second press after that is the fight again.
func TestMrMoogieConsentLapses(t *testing.T) {
	r := moogieConsentRig(t)
	a, sa := r.player("Anna", 1000, 1004)
	om := r.see(sa, r.moogie.ServerID)
	engage := func(d time.Duration) (bool, int) {
		drain(sa)
		ok := r.w.EngageAttack(sa, om, moogieSwing, r.at(d))
		return ok, countOutMatched(r, func() [][]byte { return drain(sa) })
	}
	leash := func() {
		r.w.mu.Lock()
		r.moogie.Threats, r.moogie.CurrentEnemy = nil, 0
		a.disengage()
		r.w.mu.Unlock()
	}
	if ok, n := engage(time.Second); ok || n != 1 {
		t.Fatalf("first: %v, %d 0x077A", ok, n)
	}
	if ok, n := engage(5 * time.Second); !ok || n != 0 {
		t.Fatalf("second: %v, %d 0x077A", ok, n)
	}
	r.w.mu.Lock()
	r.w.addThreatLocked(r.moogie, a.ServerID, 100) // a fight on
	r.w.mu.Unlock()
	leash()
	if ok, n := engage(35 * time.Second); !ok || n != 0 { // 30 s after its last: within MoogieConsentAfter
		t.Fatalf("30 s after the fight: %v, %d 0x077A", ok, n)
	}
	leash()
	far := 35*time.Second + MoogieConsentAfter + time.Second
	if ok, n := engage(far); ok || n != 1 {
		t.Fatalf("past MoogieConsentAfter: %v, %d 0x077A (want warned again)", ok, n)
	}
	late := far + MoogieAgainWindow + time.Second
	if ok, n := engage(late); ok || n != 1 {
		t.Fatalf("a press past the warning's window: %v, %d 0x077A (want warned again)", ok, n)
	}
	if ok, n := engage(late + 10*time.Second); !ok || n != 0 {
		t.Fatalf("the press after that warning: %v, %d 0x077A", ok, n)
	}
}

// A harmful spell at Mr. Moogie: the first is refused with MoogieOutmatched and nothing lands; the
// second within the window begins and lands. A swing's warning and a spell's are one: a spell after
// a warned swing is the "again".
func TestMrMoogieSecondSpellLands(t *testing.T) {
	r := moogieConsentRig(t)
	a, sa := r.player("Anna", 1000, 1004)
	a.Power, a.MaxPower = 1000, 1000
	om := r.see(sa, r.moogie.ServerID)
	nuke := CastSpell{SpellID: 9301, Name: "Test Bolt", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 300")}
	hp0 := r.moogie.HP
	if r.w.StartCast(sa, nuke, om, r.at(time.Second)) {
		t.Fatal("Anna's first nuke at Mr. Moogie began")
	}
	r.w.mu.Lock()
	if _, why, _ := r.w.castRefusalLocked(sa, a, &nuke, om, r.at(2*time.Second), true); why != MoogieOutmatched {
		t.Errorf("her refusal after the warning, unconsented: %q", why)
	}
	r.w.mu.Unlock()
	if !r.w.StartCast(sa, nuke, om, r.at(5*time.Second)) {
		t.Fatal("Anna's second nuke at Mr. Moogie did not begin")
	}
	for d := 5 * time.Second; d < 15*time.Second; d += 100 * time.Millisecond {
		r.w.mu.Lock()
		r.w.castSystem(r.at(d))
		r.w.mu.Unlock()
		if r.moogie.HP < hp0 {
			break
		}
	}
	if r.moogie.HP >= hp0 || r.moogie.Threats[a.ServerID] == 0 {
		t.Errorf("her second nuke did not land: HP %d of %d, threats %v", r.moogie.HP, hp0, r.moogie.Threats)
	}

	// a warned swing, then a spell: the spell is the "again"
	b, sb := r.player("Bert", 1004, 1004)
	b.Power, b.MaxPower = 1000, 1000
	ob := r.see(sb, r.moogie.ServerID)
	if r.w.EngageAttack(sb, ob, moogieSwing, r.at(20*time.Second)) {
		t.Fatal("Bert's first swing engaged")
	}
	if !r.w.StartCast(sb, nuke, ob, r.at(25*time.Second)) {
		t.Error("Bert's nuke after his warned swing did not begin")
	}
}

// While Anna fights Mr. Moogie (consented), Bert, who has not, stays out: his swing is warned with
// 0x077A, his spell lands nothing, his area does not take Mr. Moogie in, his pet and his merc may not
// fight him, and a pet's hate does not fall to him; Anna's own area, pet and spell (the controls) do.
func TestMrMoogieBystanderStaysOut(t *testing.T) {
	r := moogieConsentRig(t)
	a, sa := r.player("Anna", 1000, 1004)
	b, sb := r.player("Bert", 1004, 1004)
	oa, ob := r.see(sa, r.moogie.ServerID), r.see(sb, r.moogie.ServerID)
	r.w.EngageAttack(sa, oa, moogieSwing, r.at(time.Second))
	if !r.w.EngageAttack(sa, oa, moogieSwing, r.at(5*time.Second)) {
		t.Fatal("Anna's second swing did not engage")
	}
	now := r.at(6 * time.Second)
	r.w.mu.Lock()
	r.w.addThreatLocked(r.moogie, a.ServerID, 100)
	r.w.mu.Unlock()
	drain(sb)

	if r.w.EngageAttack(sb, ob, moogieSwing, now) || b.AttackTarget != 0 {
		t.Error("Bert's first swing engaged him while Anna fights")
	}
	if n := countOutMatched(r, func() [][]byte { return drain(sb) }); n != 1 {
		t.Errorf("Bert: 0x077A sent %d times, want 1", n)
	}

	nuke := CastSpell{SpellID: 9301, Name: "Test Bolt", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 300")}
	storm := CastSpell{SpellID: 9303, Name: "Test Storm", Scope: 1, Range: 100, Def: ParseSpellText("Target, Decrease Hitpoints by 100")}
	storm.Area.Centre = AreaOnTarget
	m := r.moogie
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	hp := m.HP
	r.w.landOnNPCLocked(&castState{spell: nuke, caster: b.ServerID, targets: []EntityID{m.ServerID}}, b, sb, m, now)
	if m.HP != hp || m.Threats[b.ServerID] != 0 {
		t.Errorf("Bert's nuke landed on him: %d -> %d, threats %v", hp, m.HP, m.Threats)
	}
	r.w.landOnNPCLocked(&castState{spell: nuke, caster: a.ServerID, targets: []EntityID{m.ServerID}}, a, sa, m, now)
	if m.HP >= hp {
		t.Errorf("Anna's nuke (the control) took nothing: %d -> %d", hp, m.HP)
	}
	if near := r.w.areaCreaturesLocked(b, &castState{spell: storm, caster: b.ServerID}, m.X, m.Y, m.Z, 10, now); len(near) != 0 {
		t.Errorf("Bert's area takes %v", near)
	}
	if near := r.w.areaCreaturesLocked(a, &castState{spell: storm, caster: a.ServerID}, m.X, m.Y, m.Z, 10, now); len(near) != 1 {
		t.Errorf("Anna's area (the control) takes %v", near)
	}
	petB := &NPC{ServerID: 9001, Name: "Bert's pet", Owner: b.ServerID, World: m.World, HP: 100, MaxHP: 100}
	mercB := &NPC{ServerID: 9003, Name: "Bert's merc", Owner: b.ServerID, World: m.World, HP: 100, MaxHP: 100, merc: &mercState{}}
	petA := &NPC{ServerID: 9002, Name: "Anna's pet", Owner: a.ServerID, World: m.World, HP: 100, MaxHP: 100}
	if petMayFight(petB, m) || petMayFight(mercB, m) || !petMayFight(petA, m) {
		t.Errorf("pets: Bert's %v, Bert's merc %v, Anna's (the control) %v", petMayFight(petB, m), petMayFight(mercB, m), petMayFight(petA, m))
	}
	// a pet's hate falls to its master only for one consented (pethate.go)
	r.w.NPCs[petB.ServerID], r.w.NPCs[petA.ServerID] = petB, petA
	if !r.w.moogieSparesLocked(m, petB.ServerID, now) || r.w.moogieSparesLocked(m, petA.ServerID, now) {
		t.Errorf("by pet: Bert's spared %v, Anna's %v", r.w.moogieSparesLocked(m, petB.ServerID, now), r.w.moogieSparesLocked(m, petA.ServerID, now))
	}
	delete(r.w.NPCs, petB.ServerID)
	delete(r.w.NPCs, petA.ServerID)
}
