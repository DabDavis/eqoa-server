package entity

import (
	"testing"
	"time"
)

// A pet's maximum hit points take its buffs (the user, 2026-11-01, of the
// shaman's Avatar: "a one time max hp buff and massive stat buff"): a spell
// raising the maximum by 300 and healing 300, on a pet at full health,
// raises it and heals it to the new maximum, once; the raise lasts the
// effect's time, the balance setting the pet's hit points again keeps it,
// and at the effect's end the maximum and the hit points come back down.
// The control: a heal alone on the pet at full health changes nothing.
// Before the change the raise was never made, so the pet stayed at its own
// maximum, full, and the heal was lost.
func TestAPetsMaximumTakesItsBuffs(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	own := pet.MaxHP
	if pet.HP != own || own <= 0 {
		t.Fatalf("the pet came at %d/%d", pet.HP, own)
	}
	land := func(sp CastSpell, at time.Duration) {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		cs := &castState{spell: sp, caster: r.p.ServerID, targets: []EntityID{pet.ServerID}}
		r.w.landOnPetLocked(cs, r.p, r.sess, pet, r.at(at))
	}
	heal := wbSpell(9201, 20, 1, "Test Mending", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"level":20,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":300}]}`)
	land(heal, 20*time.Second)
	if pet.MaxHP != own || pet.HP != own {
		t.Fatalf("the control: a heal on a full pet left it %d/%d; want %d/%d", pet.HP, pet.MaxHP, own, own)
	}
	giant := wbSpell(9202, 20, 1, "Test Colossus", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":60,"stack":1,"level":20,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":300},
			{"id":"0002","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":300}]}`)
	land(giant, 21*time.Second)
	if pet.MaxHP != own+300 || pet.HP != own+300 {
		t.Fatalf("at its landing: %d/%d; want %d/%d (raised, then healed to it)", pet.HP, pet.MaxHP, own+300, own+300)
	}
	r.tick(40 * time.Second)
	if pet.MaxHP != own+300 || pet.HP != own+300 {
		t.Errorf("19 s on: %d/%d; want %d/%d, no second heal and the raise kept", pet.HP, pet.MaxHP, own+300, own+300)
	}
	r.w.mu.Lock()
	r.w.rebalanceLocked(pet) // a balance table saved under the buff
	r.w.mu.Unlock()
	if pet.MaxHP != own+300 {
		t.Errorf("after the balance set its hit points again: maximum %d; want %d", pet.MaxHP, own+300)
	}
	r.tick(82 * time.Second) // past its minute
	if pet.MaxHP != own || pet.HP != own || len(pet.Effects) != 0 {
		t.Errorf("after its end: %d/%d with %d effects; want %d/%d, none", pet.HP, pet.MaxHP, len(pet.Effects), own, own)
	}
}

// A charmed creature set free keeps what was cast on it until each runs out
// (the user, 2026-11-01: "is charm breaks/release the mob retains all buffs
// until they expire"): a maximum raised by 300 stays raised after its master
// lets it go and comes back down when the buff's minute is over (it was never
// on the creatures' effects, so it never ran out); a respawn brings it back
// without them.
func TestAFreedCharmKeepsItsBuffsUntilTheyEnd(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, _, _ := charmRig(t)
	crab := r.enemy
	if began, _ := castCharm(t, r, captivation(), crabID, 0); !began || crab.Owner == 0 {
		t.Fatal("not charmed")
	}
	own := crab.MaxHP
	giant := wbSpell(9202, 20, 1, "Test Colossus", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":60,"stack":1,"level":20,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":300},
			{"id":"0002","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":300}]}`)
	r.w.mu.Lock()
	cs := &castState{spell: giant, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}
	r.w.landOnPetLocked(cs, r.p, r.sess, crab, r.at(10*time.Second))
	r.w.mu.Unlock()
	if crab.MaxHP != own+300 || crab.HP != own+300 {
		t.Fatalf("on the pet: %d/%d; want %d/%d", crab.HP, crab.MaxHP, own+300, own+300)
	}
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(20*time.Second))
	effects := func(d time.Duration) {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.effectSystem(r.at(d))
	}
	effects(30 * time.Second)
	if crab.Owner != 0 || crab.MaxHP != own+300 || len(crab.Effects) != 1 {
		t.Fatalf("set free, 20 s left of the buff: owner %d, maximum %d, %d effects; want 0, %d, 1", crab.Owner, crab.MaxHP, len(crab.Effects), own+300)
	}
	effects(75 * time.Second)
	if crab.MaxHP != own || crab.HP > own || len(crab.Effects) != 0 {
		t.Errorf("past the buff's minute: %d/%d, %d effects; want at most %d/%d, none", crab.HP, crab.MaxHP, len(crab.Effects), own, own)
	}

	// a death under the buff, then a respawn: its own maximum
	r2, _, _ := charmRig(t)
	crab2 := r2.enemy
	castCharm(t, r2, captivation(), crabID, 0)
	own2 := crab2.MaxHP
	r2.w.mu.Lock()
	cs2 := &castState{spell: giant, caster: r2.p.ServerID, targets: []EntityID{crab2.ServerID}}
	r2.w.landOnPetLocked(cs2, r2.p, r2.sess, crab2, r2.at(10*time.Second))
	r2.w.mu.Unlock()
	r2.w.PetCommand(r2.sess, petCmdDismiss, 0, r2.at(20*time.Second))
	r2.w.mu.Lock()
	r2.w.killNPC(crab2)
	r2.w.respawnNPCLocked(crab2)
	r2.w.mu.Unlock()
	if crab2.MaxHP != own2 || crab2.HP != own2 || len(crab2.Effects) != 0 {
		t.Errorf("back after a death under the buff: %d/%d, %d effects; want %d/%d, none", crab2.HP, crab2.MaxHP, len(crab2.Effects), own2, own2)
	}
}

// A freed charm's kept buffs count in its fights (the user, 2026-11-01:
// "make the released mob's buffs count in its fights too"): STR 100, AC 200
// and AGI 50 cast on the charmed crab, then the crab let go; while the buff
// lasts its roll on its old master has a tank's STR terms on it (its top
// 0.196 a point higher, its floor 0.02 a point), and its master's roll on it
// loses what its AC and AGI take off, as a player's take off a creature's
// (the creature line's AC slope at the attacker's level, a tank's per point
// of AGI); at the buff's end both are as before. The control: a creature
// never charmed, with the same numbers on it from a spell (what players
// cast on a creature counts in no fight), rolls by its level alone. Before
// the change neither roll moved.
func TestAFreedCharmsBuffsCountInItsFights(t *testing.T) {
	defer func(f func(int) int) { charmIntn = f }(charmIntn)
	charmIntn = dice(99)
	r, flesh, _ := charmRig(t)
	crab := r.enemy
	rolls := func(npc *NPC) (meleeRoll, int) {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		_, _, high := playerMeleeRoll(r.p, npc)
		return npcMeleeRollFor(npc, r.p), high
	}
	r0, h0 := rolls(crab)
	if began, _ := castCharm(t, r, captivation(), crabID, 0); !began || crab.Owner == 0 {
		t.Fatal("not charmed")
	}
	buff := wbSpell(9203, 20, 1, "Test Hardening", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":60,"stack":1,"level":20,
		"effects":[{"id":"0001","type":"Statistic Change","kind":"stat","stat":"str","amount":100},
			{"id":"0002","type":"Statistic Change","kind":"stat","stat":"ac","amount":200},
			{"id":"0003","type":"Statistic Change","kind":"stat","stat":"agi","amount":50}]}`)
	r.w.mu.Lock()
	r.w.landOnPetLocked(&castState{spell: buff, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(10*time.Second))
	r.w.mu.Unlock()
	r.w.PetCommand(r.sess, petCmdDismiss, 0, r.at(20*time.Second))
	if crab.Owner != 0 {
		t.Fatal("not let go")
	}
	r1, h1 := rolls(crab)
	if d := r1.Top - r0.Top; d < offHighSTR*100-0.001 || d > offHighSTR*100+0.001 || r1.Floor-r0.Floor != 2 {
		t.Errorf("its roll under the kept buff: top +%.3f, floor +%d; want +%.3f, +2", d, r1.Floor-r0.Floor, offHighSTR*100)
	}
	want := creatureRoll(r.p.Level).ACSlope*200 + agiSlope[archTank]*50
	if d := float64(h0 - h1); d < want-1 || d > want+1 {
		t.Errorf("its master's top on it: %d less; want %.1f less", h0-h1, want)
	}
	r.w.mu.Lock()
	r.w.effectSystem(r.at(75 * time.Second)) // past the buff's minute
	r.w.mu.Unlock()
	if r2, h2 := rolls(crab); r2 != r0 || h2 != h0 {
		t.Errorf("after the buff's end: roll %+v, top on it %d; want %+v, %d", r2, h2, r0, h0)
	}

	f0, fh0 := rolls(flesh)
	flesh.Effects = append(flesh.Effects, &ActiveEffect{SpellID: 9204, Name: "Test Curse", Until: r.at(time.Hour),
		Mods: StatMods{Str: 100, AC: 200, Agi: 50}})
	if f1, fh1 := rolls(flesh); f1 != f0 || fh1 != fh0 {
		t.Errorf("the control, a creature never charmed: roll %+v, top on it %d; want %+v, %d", f1, fh1, f0, fh0)
	}
}
